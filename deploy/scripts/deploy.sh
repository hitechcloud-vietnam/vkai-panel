#!/usr/bin/env bash
# =============================================================================
# VKAI Panel - release directory deployment
# HiTechCloud (hitechcloud.vn)
#
#   deploy.sh deploy <file.tar.gz>   Deploy a new release (rolls back on failure)
#   deploy.sh rollback               Go back to the previous release
#   deploy.sh list                   List the releases kept on disk
#   deploy.sh status                 Deployment status
#   deploy.sh restart                Restart the services
#
# LAYOUT:
#   /vkai-panel/releases/<version>/   every release in its own directory
#   /vkai-panel/current -> releases/... symlink to the running release
#   /vkai-panel/etc, logs, www, ssl    SHARED data, never inside a release
#
#   The systemd units point at /vkai-panel/current/..., so "deploying" is moving
#   a symlink and restarting. Rolling back is moving it the other way.
#
# NO DOCKER here: the API is a Go binary and the UI is a Next.js standalone
# build run by node, both managed by systemd.
#
# A release package must contain:
#   core/bin/vkai-api                     the API binary
#   core/migrations/*.sql                 migrations
#   panel/.next/standalone/server.js      the UI build
#   panel/.next/standalone/.next/static   static assets (MANDATORY)
#   agent/bin/vkai-agent                  (optional)
# =============================================================================

set -Eeuo pipefail

readonly PANEL_ROOT="/vkai-panel"
readonly ETC_DIR="${PANEL_ROOT}/etc"
readonly LOG_DIR="${PANEL_ROOT}/logs"
readonly RELEASES_DIR="${PANEL_ROOT}/releases"
readonly CURRENT_LINK="${PANEL_ROOT}/current"
readonly BACKUP_DIR="${PANEL_ROOT}/www/backup"
readonly ENV_FILE="${ETC_DIR}/.env"
readonly MIGRATIONS_STATE="${ETC_DIR}/migrations.applied"

readonly SVC_API="vkai-api"
readonly SVC_UI="vkai-ui"
readonly SVC_AGENT="vkai-agent"

readonly VKAI_USER="vkai"
readonly VKAI_GROUP="vkai"

# How many OLD releases to keep (the running one does not count).
readonly MAX_OLD_RELEASES=5

readonly DEFAULT_API_PORT="30110"
readonly DEFAULT_UI_PORT="3000"
# The panel vhost the installer renders. Only reconcile_nginx touches it.
readonly PANEL_VHOST="/etc/nginx/conf.d/vkai-panel.conf"
readonly HEALTH_RETRIES=15
readonly HEALTH_SLEEP=2

if [[ -t 1 ]]; then
    C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'; C_YELLOW=$'\033[1;33m'; C_OFF=$'\033[0m'
else
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_OFF=""
fi

log()  { printf '%s[%s]%s %s\n' "$C_GREEN"  "$(date '+%F %T')" "$C_OFF" "$*"; }
warn() { printf '%s[%s] WARN:%s %s\n' "$C_YELLOW" "$(date '+%F %T')" "$C_OFF" "$*" >&2; }
die()  { printf '%s[%s] ERROR:%s %s\n' "$C_RED" "$(date '+%F %T')" "$C_OFF" "$*" >&2; exit 1; }

on_error() { die "Failed at line $2 (exit code $1): $3"; }
trap 'on_error $? $LINENO "$BASH_COMMAND"' ERR

check_root() {
    [[ "${EUID}" -eq 0 ]] || die "Root privileges are required: sudo $0 $*"
}

env_get() {
    local key="$1" line
    [[ -f "$ENV_FILE" ]] || return 1
    line="$(grep -E "^${key}=" "$ENV_FILE" | tail -n1 || true)"
    [[ -n "$line" ]] || return 1
    printf '%s' "${line#*=}"
}

api_port() { env_get VKAI_SERVER_PORT || printf '%s' "$DEFAULT_API_PORT"; }
ui_port()  { env_get PORT             || printf '%s' "$DEFAULT_UI_PORT"; }

# Directory size, never fatal when the directory does not exist yet.
dir_size() {
    local out
    out="$(du -sh "$1" 2>/dev/null | awk '{print $1}' || true)"
    printf '%s' "${out:-?}"
}

create_directories() {
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$RELEASES_DIR" "$LOG_DIR" "$BACKUP_DIR"
}

# The REAL path of the running release (empty when there is none).
current_target() {
    [[ -L "$CURRENT_LINK" ]] || return 0
    readlink -f "$CURRENT_LINK" 2>/dev/null || true
}

# One release per line, sorted ascending by name (which is chronological).
list_releases() {
    [[ -d "$RELEASES_DIR" ]] || return 0
    find "$RELEASES_DIR" -mindepth 1 -maxdepth 1 -type d | sort
}

# The release immediately BEFORE the running one (empty when there is none).
previous_release() {
    local current prev="" d found="false"
    current="$(current_target)"

    while IFS= read -r d; do
        [[ -n "$d" ]] || continue
        if [[ "$d" == "$current" ]]; then
            found="true"
            break
        fi
        prev="$d"
    done < <(list_releases)

    if [[ "$found" == "true" ]]; then
        if [[ -n "$prev" ]]; then
            printf '%s' "$prev"
            return 0
        fi
        # Running the first release in releases/: fall back to the in-place
        # build produced by deploy/install.sh, if it is still complete.
        if [[ "$current" != "$PANEL_ROOT" && -x "${PANEL_ROOT}/core/bin/vkai-api" ]]; then
            printf '%s' "$PANEL_ROOT"
        fi
        return 0
    fi

    # current is not inside releases/ (it is the in-place build): take the newest.
    list_releases | tail -n1
}

# =============================================================================
# Release package validation
# =============================================================================
# validate_package rejects anything the CI deploy user should not be able to hand
# to a root-owned script. The deploy user may write into /tmp, so the argument is
# attacker-controlled from this script's point of view even though CI produced it.
validate_package() {
    local pkg="$1"

    [[ "$pkg" =~ ^/tmp/vkai-panel-[0-9a-f]{7,40}\.tar\.gz$ ]] \
        || die "Refusing package ${pkg}: expected /tmp/vkai-panel-<sha>.tar.gz"
    [[ -f "$pkg" ]] || die "Package not found: ${pkg}"
    [[ -L "$pkg" ]] && die "Refusing package ${pkg}: it is a symlink"

    # An archive member starting with / or containing .. escapes the release
    # directory when extracted as root. tar would happily follow it.
    local bad
    bad="$(tar -tzf "$pkg" | grep -E '^/|(^|/)\.\.(/|$)' | head -n5 || true)"
    [[ -z "$bad" ]] || die "Refusing package ${pkg}: unsafe member paths:"$'\n'"${bad}"
}

validate_release() {
    local dir="$1"
    [[ -d "$dir" ]] || die "Release directory not found: ${dir}"

    [[ -x "${dir}/core/bin/vkai-api" ]] ||
        die "Release ${dir} has no executable core/bin/vkai-api."

    [[ -f "${dir}/panel/.next/standalone/server.js" ]] ||
        die "Release ${dir} has no panel/.next/standalone/server.js - the UI cannot start."

    # Next.js output 'standalone' does NOT copy .next/static and public/ itself.
    # Without them the page returns HTML while every /_next/static/*.js returns
    # 404 -> "Application error: a client-side exception has occurred".
    [[ -d "${dir}/panel/.next/standalone/.next/static" ]] ||
        die "Release ${dir} has no panel/.next/standalone/.next/static - the UI would show 'Application error: a client-side exception has occurred'."

    log "Release $(basename "$dir") is valid."
}

link_shared_state() {
    local dir="$1"
    # The shared configuration lives outside the release, and Next.js only reads
    # .env from the project root, so it has to be linked into the release.
    [[ -f "$ENV_FILE" ]] || die "${ENV_FILE} not found. Run deploy/install.sh first."
    ln -sfn "$ENV_FILE" "${dir}/panel/.env" 2>/dev/null ||
        install -o "$VKAI_USER" -g "$VKAI_GROUP" -m 600 "$ENV_FILE" "${dir}/panel/.env"
    ln -sfn "$ENV_FILE" "${dir}/core/.env" 2>/dev/null || true
    [[ -d "${dir}/agent" ]] && { ln -sfn "$ENV_FILE" "${dir}/agent/.env" 2>/dev/null || true; }
    return 0
}

# =============================================================================
# Database backup and migrations
# =============================================================================
backup_database() {
    local db_name db_user db_pass file
    db_name="$(env_get VKAI_DB_NAME || echo vkai_panel)"
    db_user="$(env_get VKAI_DB_USER || echo vkai)"
    db_pass="$(env_get VKAI_DB_PASSWORD || true)"
    file="${BACKUP_DIR}/predeploy_$(date +%Y%m%d_%H%M%S).sql.gz"

    if ! command -v pg_dump >/dev/null 2>&1; then
        warn "pg_dump is not available, the database backup was skipped."
        return 0
    fi

    log "Dumping database '${db_name}' -> ${file}"
    if [[ -n "$db_pass" ]]; then
        PGPASSWORD="$db_pass" pg_dump -h 127.0.0.1 -U "$db_user" "$db_name" | gzip >"$file"
    else
        sudo -u postgres pg_dump "$db_name" | gzip >"$file"
    fi
    chown "$VKAI_USER:$VKAI_GROUP" "$file"
    chmod 600 "$file"
}

# Migrations come FROM THE NEW RELEASE and run BEFORE the symlink moves.
run_migrations() {
    local dir="${1}/core/migrations"
    [[ -d "$dir" ]] || { warn "${dir} not found, no migrations applied."; return 0; }
    command -v psql >/dev/null 2>&1 || { warn "psql is not available, migrations skipped."; return 0; }

    local db_name db_user db_pass
    db_name="$(env_get VKAI_DB_NAME || echo vkai_panel)"
    db_user="$(env_get VKAI_DB_USER || echo vkai)"
    db_pass="$(env_get VKAI_DB_PASSWORD || true)"

    touch "$MIGRATIONS_STATE"; chmod 600 "$MIGRATIONS_STATE"

    local f name applied=0
    while IFS= read -r f; do
        name="$(basename "$f")"
        grep -qxF "$name" "$MIGRATIONS_STATE" && continue
        log "  migration -> ${name}"
        PGPASSWORD="$db_pass" psql -h 127.0.0.1 -U "$db_user" -d "$db_name" \
            -v ON_ERROR_STOP=1 --quiet -f "$f" >/dev/null ||
            die "Migration '${name}' failed. The symlink has not moved, so the old release is still serving."
        echo "$name" >>"$MIGRATIONS_STATE"
        applied=$((applied + 1))
    done < <(find "$dir" -maxdepth 1 -name '*.sql' -type f | sort)

    log "Applied ${applied} migration(s)."
}

# =============================================================================
# Switching releases and health checking
# =============================================================================
restart_services() {
    if ! systemctl list-unit-files "${SVC_API}.service" --no-legend | grep -q .; then
        die "${SVC_API}.service does not exist. This host has never been installed;
run deploy/install.sh once before deploying releases to it."
    fi
    log "Restarting the services..."
    systemctl daemon-reload
    systemctl restart "$SVC_API"
    systemctl restart "$SVC_UI"
    # The agent is optional: only restart it when the operator enabled it.
    if systemctl is-enabled "$SVC_AGENT" >/dev/null 2>&1; then
        systemctl restart "$SVC_AGENT" || warn "Could not restart ${SVC_AGENT}."
    fi
    systemctl reload nginx 2>/dev/null || true
}

# =============================================================================
# nginx: keep the panel port pointed at the access gate
# =============================================================================
# A release is a symlink move; the nginx vhost is NOT part of it, so a host
# installed before the panel gained its single front door keeps a configuration
# in which "location /" goes straight to the Next.js service. That configuration
# serves the whole interface - the login form included - to anyone who finds the
# panel port, because the security entrance is enforced in the API and nothing
# else. Deploying the fixed code onto such a host would fix nothing.
#
# This repairs exactly that one thing: the upstream each location proxies to.
# It runs only when the release being activated no longer knows about a UI
# upstream at all AND the installed vhost still proxies to one, so it is a
# no-op on every host that is already correct, and it never touches TLS, the
# allow list, ports, log paths or anything else the operator may have tuned.
#
# The now-unused "upstream vkai_ui" block is deliberately left in place: nginx
# accepts an upstream nothing references, and removing a multi-line block from
# a file an operator may have edited is a bigger risk than leaving it.
reconcile_nginx() {
    local dir="$1"
    local template="${dir}/deploy/nginx/vkai-panel.conf"

    command -v nginx >/dev/null 2>&1 || return 0
    [[ -f "$PANEL_VHOST" ]] || return 0
    [[ -f "$template" ]]    || return 0

    # A release whose own template still has a UI upstream predates the single
    # front door. Leave the vhost alone: it matches that release.
    grep -q 'vkai_ui' "$template" && return 0

    # Nothing to repair when no location proxies to the UI. Commented-out lines
    # cannot match: the pattern is anchored at the start of the directive.
    grep -qE '^[[:space:]]*proxy_pass[[:space:]]+https?://vkai_ui' "$PANEL_VHOST" || return 0

    warn "${PANEL_VHOST} still sends requests straight to the UI, so the security entrance protects only /api."
    log  "Pointing every location at the API, which is what enforces the entrance..."

    local backup="${PANEL_VHOST}.pre-frontdoor.bak"
    cp -a "$PANEL_VHOST" "$backup"

    sed -i -E 's#^([[:space:]]*proxy_pass[[:space:]]+https?://)vkai_ui#\1vkai_api#' "$PANEL_VHOST"

    if ! nginx -t >/dev/null 2>&1; then
        mv -f "$backup" "$PANEL_VHOST"
        warn "nginx -t failed after the change; ${PANEL_VHOST} was restored unchanged."
        warn "THE PANEL INTERFACE IS STILL REACHABLE WITHOUT THE SECURITY ENTRANCE."
        warn "Re-run deploy/install.sh on this host to render a correct vhost."
        return 0
    fi

    systemctl reload nginx 2>/dev/null || true
    log "The panel port now has one front door: nginx -> ${SVC_API} -> UI. Previous vhost kept at ${backup}."

    # Prove it from outside: without the entrance cookie the panel port must not
    # serve the interface at all. One request, one scheme - curl prints
    # "%{http_code}" even when it fails, so a fallback attempt would concatenate
    # two codes into one string.
    local port scheme code
    port="$(env_get VKAI_PANEL_PUBLIC_PORT || echo 8888)"
    scheme="$(env_get VKAI_PANEL_PUBLIC_SCHEME || echo http)"
    code="$(curl -s -o /dev/null -k --max-time 5 -w '%{http_code}' "${scheme}://127.0.0.1:${port}/" 2>/dev/null || true)"
    case "$code" in
        404)      log "Verified: GET / on the panel port answers 404 without the entrance." ;;
        ""|000)   warn "Could not reach ${scheme}://127.0.0.1:${port}/ to verify; check it by hand." ;;
        *)        warn "GET / on the panel port answered ${code}, expected 404. Check ${PANEL_VHOST} by hand." ;;
    esac
}

switch_to() {
    local dir="$1"
    log "Pointing ${CURRENT_LINK} at $(basename "$dir")"
    ln -sfn "$dir" "$CURRENT_LINK"
    restart_services
}

# Returns 0 when both the API and the UI answer.
health_check() {
    local api ui i ok_api="false" ok_ui="false"
    api="$(api_port)"; ui="$(ui_port)"

    log "Health check (API 127.0.0.1:${api}, UI 127.0.0.1:${ui})..."
    for ((i = 1; i <= HEALTH_RETRIES; i++)); do
        if [[ "$ok_api" != "true" ]] &&
           curl -fsS --max-time 3 "http://127.0.0.1:${api}/health" >/dev/null 2>&1; then
            ok_api="true"
        fi
        if [[ "$ok_ui" != "true" ]] &&
           curl -fsS --max-time 5 -o /dev/null "http://127.0.0.1:${ui}/" 2>/dev/null; then
            ok_ui="true"
        fi
        if [[ "$ok_api" == "true" && "$ok_ui" == "true" ]]; then
            log "API and UI are both healthy."
            return 0
        fi
        warn "Not ready yet (${i}/${HEALTH_RETRIES}): API=${ok_api} UI=${ok_ui}"
        sleep "$HEALTH_SLEEP"
    done

    warn "Health check failed: API=${ok_api} UI=${ok_ui}"
    return 1
}

# =============================================================================
# Deploy
# =============================================================================
deploy_release() {
    local file="${1:-}"
    [[ -n "$file" ]] || die "No release package given. Usage: $0 deploy <file.tar.gz>"

    # Only enforced for the unattended CI path, where the caller is the deploy
    # user and the argument reaches a root-owned script through sudo. An operator
    # running this by hand is already root and can pass any path they like.
    if [[ "${VKAI_DEPLOY_STRICT:-auto}" != "off" && "$file" == /tmp/* ]]; then
        validate_package "$file"
    fi
    [[ -f "$file" ]] || die "Release package not found: ${file}"

    create_directories

    local previous id dir
    previous="$(current_target)"
    id="$(date +%Y%m%d_%H%M%S)"
    dir="${RELEASES_DIR}/${id}"
    [[ -e "$dir" ]] && die "Release '${id}' already exists. Wait one second and try again."

    log "Unpacking ${file} -> ${dir}"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$dir"
    tar -xzf "$file" -C "$dir" --no-same-owner --no-same-permissions

    validate_release "$dir"
    link_shared_state "$dir"
    chown -R "$VKAI_USER:$VKAI_GROUP" "$dir"

    backup_database
    run_migrations "$dir"

    switch_to "$dir"

    if health_check; then
        log "Release ${id} is live."
        # After the release is proven healthy, never before: a vhost repair must
        # not be the thing that makes a bad deploy harder to roll back.
        reconcile_nginx "$dir"
        cleanup_old_releases
        return 0
    fi

    warn "Release ${id} is unhealthy. Rolling back..."
    if [[ -z "$previous" || ! -d "$previous" ]]; then
        die "There is no previous release to roll back to. ${id} is still the current one.
Inspect: journalctl -u ${SVC_API} -n 80 ; journalctl -u ${SVC_UI} -n 80"
    fi

    switch_to "$previous"
    if health_check; then
        die "Release ${id} failed -> rolled back to $(basename "$previous").
The broken release is still at ${dir}. Database migrations that already ran are NOT rolled back.
Inspect: journalctl -u ${SVC_API} -n 80 ; journalctl -u ${SVC_UI} -n 80"
    fi
    die "Release ${id} failed AND the previous release $(basename "$previous") is unhealthy too. Manual intervention needed now.
Inspect: journalctl -u ${SVC_API} -n 80 ; journalctl -u ${SVC_UI} -n 80"
}

rollback() {
    local previous
    previous="$(previous_release)"
    [[ -n "$previous" && -d "$previous" ]] || die "No previous release found in ${RELEASES_DIR}."
    [[ "$previous" != "$(current_target)" ]] || die "The previous release is the running one; there is nothing to roll back to."

    log "Rolling back to $(basename "$previous")..."
    validate_release "$previous"
    link_shared_state "$previous"
    switch_to "$previous"

    health_check || die "The rollback finished but the services are unhealthy. Inspect: journalctl -u ${SVC_API} -n 80"
    log "Rolled back. NOTE: database migrations are NOT rolled back automatically."

    # The vhost is not part of a release, so it does not travel back with one.
    if [[ -f "$PANEL_VHOST" ]] && ! grep -qE '^[[:space:]]*proxy_pass[[:space:]]+https?://vkai_ui' "$PANEL_VHOST" &&
       [[ -f "${previous}/deploy/nginx/vkai-panel.conf" ]] &&
       grep -q 'vkai_ui' "${previous}/deploy/nginx/vkai-panel.conf"; then
        warn "This release predates the panel's single front door, but ${PANEL_VHOST} sends everything to ${SVC_API}."
        warn "That release cannot serve the interface that way: the panel port will answer 404 for every page."
        warn "Either deploy a release that has the front door, or re-run deploy/install.sh to render a matching vhost."
    fi
}

# Keep the running release plus the MAX_OLD_RELEASES newest, delete the rest.
cleanup_old_releases() {
    local current kept=0 d
    current="$(current_target)"

    while IFS= read -r d; do
        [[ -n "$d" ]] || continue
        [[ "$d" == "$current" ]] && continue
        if (( kept < MAX_OLD_RELEASES )); then
            kept=$((kept + 1))
            continue
        fi
        log "Removing old release $(basename "$d")"
        rm -rf "$d"
    done < <(list_releases | sort -r)
}

# =============================================================================
list_cmd() {
    local current d mark
    current="$(current_target)"
    printf '=== Releases in %s ===\n' "$RELEASES_DIR"
    while IFS= read -r d; do
        [[ -n "$d" ]] || continue
        mark="  "
        [[ "$d" == "$current" ]] && mark="->"
        printf '%s %-20s %s\n' "$mark" "$(basename "$d")" "$(dir_size "$d")"
    done < <(list_releases | sort -r)
    [[ -n "$current" ]] || printf '(nothing is linked through %s yet)\n' "$CURRENT_LINK"
}

status() {
    local current
    current="$(current_target)"

    printf '=== VKAI Panel - deployment status ===\n\n'
    if [[ -n "$current" ]]; then
        printf 'Current release : %s\n' "$(basename "$current")"
        printf 'Path            : %s\n' "$current"
    else
        printf 'Current release : no %s symlink\n' "$CURRENT_LINK"
    fi
    printf 'Install root    : %s\n' "$PANEL_ROOT"
    printf 'API (core)      : %s\n' "${CURRENT_LINK}/core/bin/vkai-api"
    printf 'UI (panel)      : %s\n' "${CURRENT_LINK}/panel/.next/standalone/server.js"
    printf 'Configuration   : %s\n\n' "$ENV_FILE"

    local svc
    for svc in "$SVC_API" "$SVC_UI"; do
        printf -- '--- %s ---\n' "$svc"
        systemctl status "$svc" --no-pager 2>/dev/null | head -5 || true
        printf '\n'
    done

    printf 'Disk usage: %s\n' "$(dir_size "$PANEL_ROOT")"
    printf '\n--- Latest API log lines ---\n'
    journalctl -u "$SVC_API" -n 5 --no-pager 2>/dev/null || true
}

usage() {
    cat <<USAGE
VKAI Panel - release deployment (release directory layout, no Docker)

Usage: $0 {deploy|rollback|list|status|restart} [file]

  deploy <file.tar.gz>  Unpack into ${RELEASES_DIR}/<version>, back up the
                        database, run migrations, move ${CURRENT_LINK}, restart
                        the services and health check; ROLLS BACK automatically
                        when the new release is unhealthy.
  rollback              Go back to the previous release.
  list                  List the releases kept (at most ${MAX_OLD_RELEASES} old ones).
  status                Deployment status.
  restart               Restart vkai-api and vkai-ui.
USAGE
}

main() {
    case "${1:-}" in
        deploy)
            check_root "$@"
            deploy_release "${2:-}"
            log "Deployment complete."
            ;;
        rollback)
            check_root "$@"
            rollback
            ;;
        list)    list_cmd ;;
        status)  status ;;
        restart)
            check_root "$@"
            restart_services
            health_check || die "The services are unhealthy after the restart."
            log "Restarted."
            ;;
        ""|-h|--help) usage ;;
        *)
            usage >&2
            die "Unknown command: $1"
            ;;
    esac
}

main "$@"
