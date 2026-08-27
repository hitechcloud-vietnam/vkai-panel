#!/usr/bin/env bash
# =============================================================================
# VKAI Panel - trien khai ban phat hanh (release) da dong goi san
# HiTech Cloud (hitechcloud.vn)
#
#   deploy.sh deploy <file.tar.gz>   Trien khai ban moi
#   deploy.sh rollback               Quay ve ban truoc
#   deploy.sh status                 Trang thai trien khai
#   deploy.sh restart                Khoi dong lai dich vu
#
# Goi release phai co cau truc:
#   core/bin/vkai-api                binary API
#   core/migrations/*.sql            migration
#   panel/.next/standalone/server.js ban build UI (kem .next/static va public)
#   agent/bin/vkai-agent             (tuy chon)
# =============================================================================

set -Eeuo pipefail

readonly PANEL_ROOT="/vkai-panel"
readonly CORE_DIR="${PANEL_ROOT}/core"
readonly UI_DIR="${PANEL_ROOT}/panel"
readonly AGENT_DIR="${PANEL_ROOT}/agent"
readonly ETC_DIR="${PANEL_ROOT}/etc"
readonly LOG_DIR="${PANEL_ROOT}/logs"
readonly RELEASES_DIR="${PANEL_ROOT}/releases"
readonly CURRENT_LINK="${PANEL_ROOT}/current"
readonly BACKUP_DIR="${PANEL_ROOT}/www/backup"
readonly ENV_FILE="${ETC_DIR}/.env"

readonly SVC_API="vkai-api"
readonly SVC_UI="vkai-ui"

readonly VKAI_USER="vkai"
readonly VKAI_GROUP="vkai"
readonly MAX_RELEASES=5
readonly API_HEALTH_PORT=30110

if [[ -t 1 ]]; then
    C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'; C_YELLOW=$'\033[1;33m'; C_OFF=$'\033[0m'
else
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_OFF=""
fi

log()  { printf '%s[%s]%s %s\n' "$C_GREEN"  "$(date '+%F %T')" "$C_OFF" "$*"; }
warn() { printf '%s[%s] CANH:%s %s\n' "$C_YELLOW" "$(date '+%F %T')" "$C_OFF" "$*" >&2; }
die()  { printf '%s[%s] LOI:%s %s\n' "$C_RED" "$(date '+%F %T')" "$C_OFF" "$*" >&2; exit 1; }

on_error() { die "That bai o dong $2 (ma loi $1): $3"; }
trap 'on_error $? $LINENO "$BASH_COMMAND"' ERR

check_root() {
    [[ "${EUID}" -eq 0 ]] || die "Can quyen root: sudo $0 $*"
}

env_get() {
    local key="$1" line
    [[ -f "$ENV_FILE" ]] || return 1
    line="$(grep -E "^${key}=" "$ENV_FILE" | tail -n1 || true)"
    [[ -n "$line" ]] || return 1
    printf '%s' "${line#*=}"
}

create_directories() {
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$RELEASES_DIR" "$LOG_DIR"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$BACKUP_DIR"
}

backup_database() {
    local db_name db_user db_pass file
    db_name="$(env_get VKAI_DB_NAME || echo vkai_panel)"
    db_user="$(env_get VKAI_DB_USER || echo vkai)"
    db_pass="$(env_get VKAI_DB_PASSWORD || true)"
    file="${BACKUP_DIR}/predeploy_$(date +%Y%m%d_%H%M%S).sql.gz"

    if ! command -v pg_dump >/dev/null 2>&1; then
        warn "Khong co pg_dump, bo qua sao luu CSDL."
        return 0
    fi

    log "Sao luu CSDL '${db_name}' -> ${file}"
    if [[ -n "$db_pass" ]]; then
        PGPASSWORD="$db_pass" pg_dump -h 127.0.0.1 -U "$db_user" "$db_name" | gzip >"$file"
    else
        sudo -u postgres pg_dump "$db_name" | gzip >"$file"
    fi
    chown "$VKAI_USER:$VKAI_GROUP" "$file"
    chmod 600 "$file"
}

# Kiem tra goi release co du thu can thiet truoc khi dung dich vu.
validate_release() {
    local dir="$1"
    [[ -x "${dir}/core/bin/vkai-api" ]] ||
        die "Goi release thieu core/bin/vkai-api (hoac khong co quyen thuc thi)."
    [[ -f "${dir}/panel/.next/standalone/server.js" ]] ||
        die "Goi release thieu panel/.next/standalone/server.js."
    # Next.js output 'standalone' KHONG tu copy .next/static va public/. Thieu
    # chung thi trang tra ve HTML nhung moi /_next/static/*.js deu 404 ->
    # "Application error: a client-side exception has occurred".
    [[ -d "${dir}/panel/.next/standalone/.next/static" ]] ||
        die "Goi release thieu panel/.next/standalone/.next/static - UI se loi client-side exception."
    log "Goi release hop le."
}

deploy_release() {
    local file="${1:-}"
    [[ -n "$file" ]] || die "Thieu duong dan goi release. Dung: $0 deploy <file.tar.gz>"
    [[ -f "$file" ]] || die "Khong thay goi release: ${file}"

    local id dir
    id="$(date +%Y%m%d_%H%M%S)"
    dir="${RELEASES_DIR}/${id}"

    log "Giai nen ban ${id}..."
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$dir"
    tar -xzf "$file" -C "$dir"

    validate_release "$dir"

    chown -R "$VKAI_USER:$VKAI_GROUP" "$dir"
    ln -sfn "$dir" "$CURRENT_LINK"

    log "Cai dat ban ${id} vao ${PANEL_ROOT}..."
    # core/ va panel/ la duong dan ma systemd unit tro toi, nen phai thay noi dung
    # tai cho thay vi doi symlink cua tung dich vu.
    rsync -a --delete --exclude 'migrations' "${dir}/core/"  "${CORE_DIR}/"
    rsync -a           "${dir}/core/migrations/" "${CORE_DIR}/migrations/" 2>/dev/null || true
    rsync -a --delete "${dir}/panel/" "${UI_DIR}/"
    if [[ -d "${dir}/agent" ]]; then
        rsync -a --delete "${dir}/agent/" "${AGENT_DIR}/"
    fi

    # Next.js chi doc .env tai goc du an.
    ln -sfn "$ENV_FILE" "${UI_DIR}/.env" 2>/dev/null ||
        install -o "$VKAI_USER" -g "$VKAI_GROUP" -m 600 "$ENV_FILE" "${UI_DIR}/.env"

    chown -R "$VKAI_USER:$VKAI_GROUP" "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"
    log "Da trien khai ban ${id}."
}

run_migrations() {
    local dir="${CORE_DIR}/migrations"
    [[ -d "$dir" ]] || { warn "Khong thay ${dir}, bo qua migration."; return 0; }

    local db_name db_user db_pass state
    db_name="$(env_get VKAI_DB_NAME || echo vkai_panel)"
    db_user="$(env_get VKAI_DB_USER || echo vkai)"
    db_pass="$(env_get VKAI_DB_PASSWORD || true)"
    state="${ETC_DIR}/migrations.applied"
    touch "$state"; chmod 600 "$state"

    local f name applied=0
    while IFS= read -r f; do
        name="$(basename "$f")"
        grep -qxF "$name" "$state" && continue
        log "  migration -> ${name}"
        PGPASSWORD="$db_pass" psql -h 127.0.0.1 -U "$db_user" -d "$db_name" \
            -v ON_ERROR_STOP=1 --quiet -f "$f" >/dev/null ||
            die "Migration '${name}' that bai."
        echo "$name" >>"$state"
        applied=$((applied + 1))
    done < <(find "$dir" -maxdepth 1 -name '*.sql' -type f | sort)

    log "Da ap dung ${applied} migration."
}

restart_services() {
    log "Khoi dong lai dich vu..."
    systemctl daemon-reload
    systemctl restart "$SVC_API"
    systemctl restart "$SVC_UI"
    systemctl reload nginx 2>/dev/null || true
}

health_check() {
    local retries=15 i
    log "Kiem tra suc khoe API..."
    for ((i = 1; i <= retries; i++)); do
        if curl -fsS --max-time 3 "http://127.0.0.1:${API_HEALTH_PORT}/health" >/dev/null 2>&1; then
            log "API khoe."
            return 0
        fi
        warn "Chua phan hoi (${i}/${retries})..."
        sleep 2
    done
    die "API khong phan hoi sau ${retries} lan thu. Xem: journalctl -u ${SVC_API} -n 80"
}

cleanup_old_releases() {
    local count
    count="$(find "$RELEASES_DIR" -mindepth 1 -maxdepth 1 -type d | wc -l)"
    if (( count > MAX_RELEASES )); then
        local remove=$(( count - MAX_RELEASES ))
        log "Xoa ${remove} ban cu..."
        find "$RELEASES_DIR" -mindepth 1 -maxdepth 1 -type d | sort | head -n "$remove" |
            xargs -r rm -rf
    fi
}

rollback() {
    local current previous
    [[ -L "$CURRENT_LINK" ]] || die "Chua co ban nao duoc trien khai."
    current="$(readlink -f "$CURRENT_LINK")"
    previous="$(find "$RELEASES_DIR" -mindepth 1 -maxdepth 1 -type d | sort |
                grep -B1 -x "$current" | head -n1 || true)"

    [[ -n "$previous" && "$previous" != "$current" ]] || die "Khong tim thay ban truoc do."

    log "Quay ve ban $(basename "$previous")..."
    validate_release "$previous"
    rsync -a --delete "${previous}/core/"  "${CORE_DIR}/"
    rsync -a --delete "${previous}/panel/" "${UI_DIR}/"
    [[ -d "${previous}/agent" ]] && rsync -a --delete "${previous}/agent/" "${AGENT_DIR}/"
    ln -sfn "$ENV_FILE" "${UI_DIR}/.env" 2>/dev/null || true
    ln -sfn "$previous" "$CURRENT_LINK"
    chown -R "$VKAI_USER:$VKAI_GROUP" "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"

    restart_services
    health_check
    log "Da quay lui xong. Luu y: migration CSDL KHONG tu dong quay lui."
}

status() {
    printf '=== VKAI Panel - trang thai trien khai ===\n\n'
    if [[ -L "$CURRENT_LINK" ]]; then
        printf 'Ban hien tai : %s\n' "$(basename "$(readlink -f "$CURRENT_LINK")")"
    else
        printf 'Ban hien tai : chua trien khai qua deploy.sh\n'
    fi
    printf 'Goc cai dat  : %s\n' "$PANEL_ROOT"
    printf 'API (core)   : %s\n' "${CORE_DIR}/bin/vkai-api"
    printf 'UI (panel)   : %s\n\n' "${UI_DIR}/.next/standalone/server.js"

    printf -- '--- %s ---\n' "$SVC_API"
    systemctl status "$SVC_API" --no-pager 2>/dev/null | head -5 || true
    printf -- '\n--- %s ---\n' "$SVC_UI"
    systemctl status "$SVC_UI" --no-pager 2>/dev/null | head -5 || true

    printf '\nDung o dia: %s\n' "$(du -sh "$PANEL_ROOT" 2>/dev/null | awk '{print $1}')"
    printf '\n--- Nhat ky API gan nhat ---\n'
    journalctl -u "$SVC_API" -n 5 --no-pager 2>/dev/null || true
}

usage() {
    cat <<USAGE
VKAI Panel - trien khai ban phat hanh

Dung: $0 {deploy|rollback|status|restart} [file]

  deploy <file.tar.gz>  Trien khai ban moi (sao luu CSDL, migration, health check)
  rollback              Quay ve ban truoc do
  status                Trang thai trien khai
  restart               Khoi dong lai vkai-api va vkai-ui
USAGE
}

main() {
    case "${1:-}" in
        deploy)
            check_root "$@"
            create_directories
            backup_database
            deploy_release "${2:-}"
            run_migrations
            restart_services
            health_check
            cleanup_old_releases
            log "Trien khai hoan tat."
            ;;
        rollback)
            check_root "$@"
            rollback
            ;;
        status)
            status
            ;;
        restart)
            check_root "$@"
            restart_services
            health_check
            log "Da khoi dong lai."
            ;;
        ""|-h|--help)
            usage
            ;;
        *)
            usage >&2
            die "Lenh khong hop le: $1"
            ;;
    esac
}

main "$@"
