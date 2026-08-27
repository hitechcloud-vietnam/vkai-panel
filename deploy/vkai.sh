#!/usr/bin/env bash
# =============================================================================
# vkai - lenh quan tri VKAI Panel
# HiTechCloud (hitechcloud.vn)
#
# Cai dat: install -m 0755 deploy/vkai.sh /usr/local/bin/vkai
# =============================================================================

set -Eeuo pipefail

readonly BRAND_NAME="VKAI Panel"
readonly BRAND_ORG="HiTechCloud (hitechcloud.vn)"

readonly PANEL_ROOT="/vkai-panel"
readonly CORE_DIR="${PANEL_ROOT}/core"
readonly UI_DIR="${PANEL_ROOT}/panel"
readonly AGENT_DIR="${PANEL_ROOT}/agent"
readonly WWW_DIR="${PANEL_ROOT}/www"
readonly WWW_DOMAINS_DIR="${WWW_DIR}/domains"
readonly WWW_BACKUP_DIR="${WWW_DIR}/backup"
readonly ETC_DIR="${PANEL_ROOT}/etc"
readonly LOG_DIR="${PANEL_ROOT}/logs"
readonly ENV_FILE="${ETC_DIR}/.env"
readonly SUMMARY_FILE="${ETC_DIR}/install-summary.txt"

readonly SVC_API="vkai-api"
readonly SVC_UI="vkai-ui"
readonly SVC_AGENT="vkai-agent"

readonly VKAI_USER="vkai"
readonly VKAI_GROUP="vkai"

if [[ -t 1 ]]; then
    C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'; C_YELLOW=$'\033[1;33m'
    C_BLUE=$'\033[0;34m'; C_BOLD=$'\033[1m'; C_OFF=$'\033[0m'
else
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_BOLD=""; C_OFF=""
fi

info()  { printf '%s[INFO]%s %s\n' "$C_GREEN"  "$C_OFF" "$*"; }
warn()  { printf '%s[CANH]%s %s\n' "$C_YELLOW" "$C_OFF" "$*" >&2; }
err()   { printf '%s[LOI ]%s %s\n' "$C_RED"    "$C_OFF" "$*" >&2; }
die()   { err "$*"; exit 1; }

has()   { command -v "$1" >/dev/null 2>&1; }

need_root() {
    [[ "${EUID}" -eq 0 ]] || die "Lenh nay can quyen root: sudo vkai $*"
}

env_get() {
    local key="$1" line
    [[ -f "$ENV_FILE" ]] || return 1
    line="$(grep -E "^${key}=" "$ENV_FILE" | tail -n1 || true)"
    [[ -n "$line" ]] || return 1
    printf '%s' "${line#*=}"
}

usage() {
    cat <<USAGE
${C_BOLD}${BRAND_NAME}${C_OFF} - lenh quan tri "vkai"
${BRAND_ORG}

Cach dung: vkai <lenh> [tham so]

Dich vu
  start                 Khoi dong ${SVC_API}, ${SVC_UI}, nginx
  stop                  Dung ${SVC_UI}, ${SVC_API}
  restart               Khoi dong lai toan bo
  status                Trang thai dich vu + tai nguyen may
  logs <api|ui|agent|nginx|install> [-n N]
                        Xem nhat ky (mac dinh theo doi realtime)

Truy cap panel
  info                  In URL panel, cong, loi vao va duong dan du lieu
  port [<so>|random]    Xem hoac doi cong panel (khong chap nhan 80/443)
  entrance [<duong>|random]
                        Xem hoac doi loi vao an toan

Van hanh
  backup [ten]          Sao luu CSDL + cau hinh vao ${WWW_BACKUP_DIR}
  update [duong-dan-ma-nguon]
                        Keo cau hinh cu, build lai core/ va panel/, khoi dong lai
  uninstall             Go cai dat (uy quyen cho deploy/install.sh --uninstall)

Nghiep vu (uy quyen cho vkai-cli)
  site|db|ssl|firewall|server|backup-cli ...
                        Vi du: vkai site create example.com

Ghi chu
  Cong 80/443 danh RIENG cho website cua khach. Panel luon nghe tren cong rieng.
USAGE
}

# -----------------------------------------------------------------------------
# Dich vu
# -----------------------------------------------------------------------------
svc_state() {
    local svc="$1"
    if ! systemctl list-unit-files 2>/dev/null | grep -q "^${svc}\.service"; then
        printf '%s- chua cai%s' "$C_YELLOW" "$C_OFF"
    elif systemctl is-active --quiet "$svc"; then
        printf '%s* dang chay%s' "$C_GREEN" "$C_OFF"
    else
        printf '%s* da dung%s' "$C_RED" "$C_OFF"
    fi
}

redis_service() {
    local svc
    for svc in redis-server redis redis6; do
        if systemctl list-unit-files 2>/dev/null | grep -q "^${svc}\.service"; then
            printf '%s' "$svc"; return 0
        fi
    done
    printf 'redis'
}

cmd_start() {
    need_root start
    info "Khoi dong ${BRAND_NAME}..."
    systemctl start "$SVC_API"
    systemctl start "$SVC_UI"
    systemctl reload nginx 2>/dev/null || systemctl start nginx 2>/dev/null || warn "nginx khong khoi dong duoc."
    cmd_status
}

cmd_stop() {
    need_root stop
    info "Dung ${BRAND_NAME}..."
    systemctl stop "$SVC_UI"  || true
    systemctl stop "$SVC_API" || true
    info "Da dung. nginx van chay de phuc vu website cua khach."
}

cmd_restart() {
    need_root restart
    info "Khoi dong lai ${BRAND_NAME}..."
    systemctl restart "$SVC_API"
    systemctl restart "$SVC_UI"
    systemctl reload nginx 2>/dev/null || true
    cmd_status
}

cmd_status() {
    local redis_svc
    redis_svc="$(redis_service)"

    printf '\n%sTrang thai dich vu%s\n' "$C_BLUE" "$C_OFF"
    printf '  %-14s %s\n' "$SVC_API"    "$(svc_state "$SVC_API")"
    printf '  %-14s %s\n' "$SVC_UI"     "$(svc_state "$SVC_UI")"
    printf '  %-14s %s\n' "$SVC_AGENT"  "$(svc_state "$SVC_AGENT")"
    printf '  %-14s %s\n' "nginx"       "$(svc_state nginx)"
    printf '  %-14s %s\n' "postgresql"  "$(svc_state postgresql)"
    printf '  %-14s %s\n' "$redis_svc"  "$(svc_state "$redis_svc")"

    printf '\n%sTai nguyen may%s\n' "$C_BLUE" "$C_OFF"
    printf '  RAM:  %s\n' "$(free -h 2>/dev/null | awk '/^Mem:/ {print $3 "/" $2}')"
    printf '  Dia:  %s\n' "$(df -h "$PANEL_ROOT" 2>/dev/null | awk 'NR==2 {print $3 "/" $2 " (" $5 " da dung)"}')"
    printf '  Tai:  %s\n' "$(awk '{print $1 ", " $2 ", " $3}' /proc/loadavg)"

    local port entrance
    port="$(env_get VKAI_PANEL_PUBLIC_PORT || env_get VKAI_PANEL_PORT || true)"
    entrance="$(env_get VKAI_PANEL_ENTRANCE || true)"
    if [[ -n "$port" ]]; then
        printf '\n%sPanel%s cong %s, loi vao %s   (xem day du: vkai info)\n' \
            "$C_BLUE" "$C_OFF" "$port" "${entrance:-(chua dat)}"
    fi
    printf '\n'
}

cmd_logs() {
    local target="${1:-}"; shift || true
    local lines="${1:-}"
    case "$target" in
        api)     journalctl -u "$SVC_API"   -f --no-pager ;;
        ui)      journalctl -u "$SVC_UI"    -f --no-pager ;;
        agent)   journalctl -u "$SVC_AGENT" -f --no-pager ;;
        nginx)   journalctl -u nginx        -f --no-pager ;;
        install)
            [[ -f "${LOG_DIR}/install.log" ]] || die "Khong thay ${LOG_DIR}/install.log"
            tail -n "${lines:-200}" -f "${LOG_DIR}/install.log"
            ;;
        ""|-h|--help)
            die "Dung: vkai logs <api|ui|agent|nginx|install>"
            ;;
        *)
            die "Khong biet nhat ky '${target}'. Chon: api, ui, agent, nginx, install."
            ;;
    esac
}

# -----------------------------------------------------------------------------
# Truy cap panel
# -----------------------------------------------------------------------------
panelctl_bin() {
    if has vkai-panelctl; then
        printf 'vkai-panelctl'
    elif [[ -x "${CORE_DIR}/bin/vkai-panelctl" ]]; then
        printf '%s' "${CORE_DIR}/bin/vkai-panelctl"
    else
        return 1
    fi
}

cmd_info() {
    local bin ip port entrance scheme
    if bin="$(panelctl_bin)"; then
        "$bin" panel info || true
        printf '\n'
    fi

    ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
    port="$(env_get VKAI_PANEL_PUBLIC_PORT || env_get VKAI_PANEL_PORT || echo 8888)"
    entrance="$(env_get VKAI_PANEL_ENTRANCE || true)"
    scheme="$(env_get VKAI_PANEL_PUBLIC_SCHEME || echo http)"

    printf '%s%s - thong tin truy cap%s\n' "$C_BOLD" "$BRAND_NAME" "$C_OFF"
    printf '  URL       : %s://%s:%s%s/\n' "$scheme" "${ip:-<IP-may-chu>}" "$port" "$entrance"
    printf '  Cong      : %s   (80/443 danh RIENG cho website cua khach)\n' "$port"
    printf '  Loi vao   : %s\n' "${entrance:-(tat)}"
    printf '\n%sDuong dan%s\n' "$C_BLUE" "$C_OFF"
    printf '  Goc cai dat      : %s\n' "$PANEL_ROOT"
    printf '  API (core)       : %s\n' "$CORE_DIR"
    printf '  Giao dien (panel): %s\n' "$UI_DIR"
    printf '  Agent            : %s\n' "$AGENT_DIR"
    printf '  Website khach    : %s/<domain>\n' "$WWW_DOMAINS_DIR"
    printf '  Sao luu          : %s\n' "$WWW_BACKUP_DIR"
    printf '  Cau hinh         : %s\n' "$ENV_FILE"
    printf '  Nhat ky          : %s\n' "$LOG_DIR"
    if [[ -f "$SUMMARY_FILE" ]]; then
        printf '\n  Bang tong ket cai dat: %s\n' "$SUMMARY_FILE"
    fi
    printf '\n'
}

# Ghi lai mot khoa trong .env (giu nguyen quyen 600).
env_set() {
    local key="$1" value="$2"
    [[ -f "$ENV_FILE" ]] || die "Khong thay ${ENV_FILE}"
    local tmp
    tmp="$(mktemp "${ETC_DIR}/.env.XXXXXX")"
    chmod 600 "$tmp"
    if grep -qE "^${key}=" "$ENV_FILE"; then
        sed -E "s|^${key}=.*|${key}=${value}|" "$ENV_FILE" >"$tmp"
    else
        cat "$ENV_FILE" >"$tmp"
        printf '%s=%s\n' "$key" "$value" >>"$tmp"
    fi
    chown "$VKAI_USER:$VKAI_GROUP" "$tmp"
    mv "$tmp" "$ENV_FILE"
}

cmd_port() {
    local value="${1:-}"
    if [[ -z "$value" ]]; then
        printf 'Cong panel hien tai: %s\n' "$(env_get VKAI_PANEL_PUBLIC_PORT || env_get VKAI_PANEL_PORT || echo '(chua dat)')"
        return 0
    fi
    need_root "port $value"

    if [[ "$value" == "random" ]]; then
        value="$(( (RANDOM % 50000) + 10000 ))"
    fi
    [[ "$value" =~ ^[0-9]+$ ]] || die "Cong phai la so: '$value'"
    (( value >= 1 && value <= 65535 )) || die "Cong ngoai khoang 1-65535: $value"
    [[ "$value" != "80" && "$value" != "443" ]] ||
        die "Cong 80/443 danh rieng cho website cua khach. Chon cong khac."

    local old
    old="$(env_get VKAI_PANEL_PUBLIC_PORT || env_get VKAI_PANEL_PORT || echo 8888)"

    env_set VKAI_PANEL_PORT "$value"
    env_set VKAI_PANEL_PUBLIC_PORT "$value"

    local conf="/etc/nginx/conf.d/vkai-panel.conf"
    if [[ -f "$conf" ]]; then
        sed -i -E "s/^(\s*listen\s+(\[::\]:)?)${old}(\s|;)/\1${value}\3/" "$conf"
        if nginx -t >/dev/null 2>&1; then
            systemctl reload nginx
        else
            warn "nginx -t that bai sau khi doi cong. Kiem tra ${conf}."
        fi
    fi

    # Mo cong moi tren tuong lua, dong cong cu.
    if has ufw && ufw status 2>/dev/null | grep -qi '^Status: active'; then
        ufw allow "${value}/tcp" >/dev/null 2>&1 || true
        ufw delete allow "${old}/tcp" >/dev/null 2>&1 || true
    elif has firewall-cmd && systemctl is-active --quiet firewalld; then
        firewall-cmd --permanent --add-port="${value}/tcp" >/dev/null 2>&1 || true
        firewall-cmd --permanent --remove-port="${old}/tcp" >/dev/null 2>&1 || true
        firewall-cmd --reload >/dev/null 2>&1 || true
    else
        warn "Khong tu mo duoc cong. Hay mo ${value}/tcp thu cong."
    fi

    if has semanage; then
        semanage port -a -t http_port_t -p tcp "$value" 2>/dev/null ||
        semanage port -m -t http_port_t -p tcp "$value" 2>/dev/null || true
    fi

    systemctl restart "$SVC_API"
    info "Cong panel: ${old} -> ${value}"
    cmd_info
}

cmd_entrance() {
    local value="${1:-}"
    if [[ -z "$value" ]]; then
        printf 'Loi vao an toan hien tai: %s\n' "$(env_get VKAI_PANEL_ENTRANCE || echo '(tat)')"
        return 0
    fi
    need_root "entrance $value"

    if [[ "$value" == "random" ]]; then
        value="/vkai_$(openssl rand -hex 4)"
    fi
    [[ "$value" == /* ]] || value="/${value}"
    [[ "$value" =~ ^/[A-Za-z0-9_-]{4,64}$ ]] ||
        die "Loi vao chi cho phep chu, so, '-' va '_', dai 4-64 ky tu. Vi du: /vkai_a1b2c3d4"

    env_set VKAI_PANEL_ENTRANCE "$value"
    env_set VKAI_PANEL_ENTRANCE_ENABLED true
    systemctl restart "$SVC_API"
    info "Loi vao an toan moi: ${value}"
    cmd_info
}

# -----------------------------------------------------------------------------
# Sao luu
# -----------------------------------------------------------------------------
cmd_backup() {
    need_root backup
    local label="${1:-manual}"
    local stamp dest
    stamp="$(date +%Y%m%d_%H%M%S)"
    dest="${WWW_BACKUP_DIR}/${label}_${stamp}"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$dest"

    local db_name db_user db_pass
    db_name="$(env_get VKAI_DB_NAME || echo vkai_panel)"
    db_user="$(env_get VKAI_DB_USER || echo vkai)"
    db_pass="$(env_get VKAI_DB_PASSWORD || true)"

    if has pg_dump; then
        info "Sao luu CSDL '${db_name}'..."
        if [[ -n "$db_pass" ]]; then
            PGPASSWORD="$db_pass" pg_dump -h 127.0.0.1 -U "$db_user" "$db_name" | gzip >"${dest}/${db_name}.sql.gz"
        else
            sudo -u postgres pg_dump "$db_name" | gzip >"${dest}/${db_name}.sql.gz"
        fi
    else
        warn "Khong co pg_dump, bo qua CSDL."
    fi

    info "Sao luu cau hinh..."
    tar -czf "${dest}/etc.tar.gz" -C "$PANEL_ROOT" etc 2>/dev/null || warn "Khong nen duoc ${ETC_DIR}"

    if [[ -d /etc/nginx ]]; then
        tar -czf "${dest}/nginx.tar.gz" -C /etc nginx 2>/dev/null || true
    fi

    chown -R "$VKAI_USER:$VKAI_GROUP" "$dest"
    chmod -R go-rwx "$dest"
    info "Ban sao luu: ${dest}"
    du -sh "$dest" 2>/dev/null || true
    warn "Ma nguon website (${WWW_DOMAINS_DIR}) KHONG nam trong ban sao luu nay - dung 'vkai site backup' hoac tar rieng."
}

# -----------------------------------------------------------------------------
# Cap nhat
# -----------------------------------------------------------------------------
cmd_update() {
    need_root update
    local src="${1:-}"

    if [[ -z "$src" ]]; then
        src="$PANEL_ROOT"
        info "Khong chi dinh ma nguon: build lai tai cho tu ${PANEL_ROOT}."
    fi
    [[ -d "${src}/core" && -d "${src}/panel" ]] ||
        die "'${src}' khong phai thu muc ma nguon hop le (thieu core/ hoac panel/)."

    cmd_backup pre-update

    local go_bin npm_bin
    go_bin="$(command -v go || echo /usr/local/go/bin/go)"
    npm_bin="$(command -v npm || true)"
    [[ -x "$go_bin" ]] || die "Khong tim thay 'go'."
    [[ -n "$npm_bin" ]] || die "Khong tim thay 'npm'."

    if [[ "$(cd "$src" && pwd -P)" != "$(cd "$PANEL_ROOT" && pwd -P)" ]]; then
        info "Dong bo ma nguon moi vao ${PANEL_ROOT}..."
        has rsync || die "Can rsync de dong bo ma nguon."
        rsync -a --delete --exclude '.git' --exclude 'node_modules' --exclude '.next' \
            "${src}/core/"  "${CORE_DIR}/"
        rsync -a --delete --exclude '.git' --exclude 'node_modules' --exclude '.next' \
            "${src}/panel/" "${UI_DIR}/"
        if [[ -d "${src}/agent" ]]; then
            rsync -a --delete --exclude '.git' "${src}/agent/" "${AGENT_DIR}/"
        fi
        chown -R "$VKAI_USER:$VKAI_GROUP" "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"
    fi

    # .env phai co MAT o goc du an Next.js truoc khi build: NEXT_PUBLIC_* duoc
    # nhung vao bundle luc build, khong doc luc chay.
    ln -sfn "$ENV_FILE" "${UI_DIR}/.env" 2>/dev/null ||
        install -o "$VKAI_USER" -g "$VKAI_GROUP" -m 600 "$ENV_FILE" "${UI_DIR}/.env"

    systemctl stop "$SVC_UI" "$SVC_API" || true

    export HOME="${PANEL_ROOT}/tmp"
    export GOCACHE="${HOME}/go-build" GOMODCACHE="${HOME}/go-mod" GOFLAGS="-buildvcs=false"
    export npm_config_cache="${HOME}/npm"
    mkdir -p "$GOCACHE" "$GOMODCACHE" "$npm_config_cache"

    info "Build lai API..."
    ( cd "$CORE_DIR" && "$go_bin" mod download &&
      "$go_bin" build -trimpath -ldflags "-s -w" -o "${CORE_DIR}/bin/vkai-api" ./cmd/api &&
      "$go_bin" build -trimpath -ldflags "-s -w" -o "${CORE_DIR}/bin/vkai-panelctl" ./cmd/panelctl )
    install -m 0755 "${CORE_DIR}/bin/vkai-panelctl" /usr/local/bin/vkai-panelctl
    if [[ -d "${CORE_DIR}/cmd/cli" ]]; then
        ( cd "$CORE_DIR" && "$go_bin" build -trimpath -ldflags "-s -w" -o "${CORE_DIR}/bin/vkai-cli" ./cmd/cli )
        install -m 0755 "${CORE_DIR}/bin/vkai-cli" /usr/local/bin/vkai-cli
    fi

    if [[ -d "$AGENT_DIR" ]]; then
        info "Build lai agent..."
        ( cd "$AGENT_DIR" && "$go_bin" mod download &&
          "$go_bin" build -trimpath -ldflags "-s -w" -o "${AGENT_DIR}/bin/vkai-agent" ./cmd ) ||
            warn "Build agent that bai, bo qua."
    fi

    info "Build lai giao dien..."
    (
        cd "$UI_DIR"
        set -a
        # shellcheck disable=SC1090
        . "$ENV_FILE"
        set +a
        if [[ -f package-lock.json ]]; then "$npm_bin" ci --no-audit --no-fund
        else "$npm_bin" install --no-audit --no-fund; fi
        NODE_ENV=production "$npm_bin" run build
    )

    # Bat buoc: standalone khong tu co .next/static va public/.
    local sa="${UI_DIR}/.next/standalone"
    [[ -d "$sa" ]] || die "Khong thay ${sa} sau khi build."
    [[ -d "${sa}/.next/static" ]] || { mkdir -p "${sa}/.next"; cp -a "${UI_DIR}/.next/static" "${sa}/.next/static"; }
    [[ -d "${sa}/public" || ! -d "${UI_DIR}/public" ]] || cp -a "${UI_DIR}/public" "${sa}/public"
    [[ -f "${sa}/server.js" ]] || die "Khong thay ${sa}/server.js - giao dien se khong chay duoc."

    chown -R "$VKAI_USER:$VKAI_GROUP" "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"

    systemctl daemon-reload
    systemctl start "$SVC_API" "$SVC_UI"
    info "Cap nhat hoan tat."
    cmd_status
}

cmd_uninstall() {
    need_root uninstall
    local installer=""
    for installer in "${PANEL_ROOT}/deploy/install.sh" "$(dirname "$(readlink -f "$0")")/install.sh"; do
        if [[ -f "$installer" ]]; then
            exec bash "$installer" --uninstall
        fi
    done
    die "Khong tim thay install.sh de go cai dat. Chay: bash <ma-nguon>/deploy/install.sh --uninstall"
}

# -----------------------------------------------------------------------------
# Uy quyen cac lenh nghiep vu cho vkai-cli (binary Go)
# -----------------------------------------------------------------------------
delegate_cli() {
    local bin=""
    if has vkai-cli; then
        bin="vkai-cli"
    elif [[ -x "${CORE_DIR}/bin/vkai-cli" ]]; then
        bin="${CORE_DIR}/bin/vkai-cli"
    else
        die "Khong tim thay 'vkai-cli'. Build lai: cd ${CORE_DIR} && go build -o bin/vkai-cli ./cmd/cli"
    fi
    exec "$bin" "$@"
}

# -----------------------------------------------------------------------------
main() {
    local cmd="${1:-}"
    if (( $# > 0 )); then shift; fi

    case "$cmd" in
        start)      cmd_start ;;
        stop)       cmd_stop ;;
        restart)    cmd_restart ;;
        status)     cmd_status ;;
        logs)       cmd_logs "$@" ;;
        info)       cmd_info ;;
        port)       cmd_port "${1:-}" ;;
        entrance)   cmd_entrance "${1:-}" ;;
        backup)     cmd_backup "${1:-}" ;;
        update)     cmd_update "${1:-}" ;;
        uninstall)  cmd_uninstall ;;
        panel)
            # Tuong thich nguoc: "vkai panel port 8888" -> "vkai port 8888".
            local sub="${1:-info}"
            if (( $# > 0 )); then shift; fi
            case "$sub" in
                info)     cmd_info ;;
                port)     cmd_port "${1:-}" ;;
                entrance) cmd_entrance "${1:-}" ;;
                *)        panelctl_bin >/dev/null && exec "$(panelctl_bin)" panel "$sub" "$@" ;;
            esac
            ;;
        backup-cli)
            # "vkai backup" la sao luu cua panel; "vkai backup-cli" mo lenh
            # backup cua vkai-cli.
            delegate_cli backup "$@"
            ;;
        site|db|ssl|firewall|server|service|version)
            delegate_cli "$cmd" "$@"
            ;;
        ""|-h|--help|help)
            usage
            ;;
        *)
            usage >&2
            die "Lenh khong hop le: ${cmd}"
            ;;
    esac
}

main "$@"
