#!/usr/bin/env bash
# =============================================================================
# vkai - the VKAI Panel administration command
# HiTechCloud (hitechcloud.vn)
#
# Installed by: install -m 0755 deploy/vkai.sh /usr/local/bin/vkai
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
readonly WWW_DEFAULT_DIR="${WWW_DIR}/default"
readonly ACME_CHALLENGE_DIR="${WWW_DEFAULT_DIR}/.well-known/acme-challenge"
# The systemd units run through this symlink (see deploy/systemd/*.service).
readonly RELEASES_DIR="${PANEL_ROOT}/releases"
readonly CURRENT_LINK="${PANEL_ROOT}/current"
readonly ETC_DIR="${PANEL_ROOT}/etc"
# The only privileged entry point that moves ${CURRENT_LINK}; root owned.
readonly BIN_DEPLOY="${PANEL_ROOT}/bin/vkai-deploy"
readonly LOG_DIR="${PANEL_ROOT}/logs"
readonly SSL_DIR="${PANEL_ROOT}/ssl"
readonly ENV_FILE="${ETC_DIR}/.env"
readonly SUMMARY_FILE="${ETC_DIR}/install-summary.txt"
# What is installed, on which channel, since when. Written by deploy/install.sh
# and rewritten by every successful upgrade, so it stays true even when the
# binaries were replaced by hand.
readonly VERSION_FILE="${ETC_DIR}/version.json"
# Result of the last update check, written by "vkai upgrade --check" and read by
# the panel to show the "an update is available" banner.
readonly UPGRADE_STATE_FILE="${ETC_DIR}/upgrade-check.json"
readonly DEFAULT_CHANNEL="stable"
readonly RELEASE_NOTES_URL="https://github.com/hitechcloud-vietnam/vkai-panel/releases"
readonly PANEL_CERT="${SSL_DIR}/panel.crt"
readonly PANEL_KEY="${SSL_DIR}/panel.key"

readonly SVC_API="vkai-api"
readonly SVC_UI="vkai-ui"
readonly SVC_AGENT="vkai-agent"
readonly SVC_CERT_TIMER="vkai-cert-renew.timer"

readonly VKAI_USER="vkai"
readonly VKAI_GROUP="vkai"

if [[ -t 1 ]]; then
    C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'; C_YELLOW=$'\033[1;33m'
    C_BLUE=$'\033[0;34m'; C_BOLD=$'\033[1m'; C_OFF=$'\033[0m'
else
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_BOLD=""; C_OFF=""
fi

QUIET="false"

info()  { [[ "$QUIET" == "true" ]] || printf '%s[INFO]%s %s\n' "$C_GREEN"  "$C_OFF" "$*"; }
warn()  { printf '%s[WARN]%s %s\n' "$C_YELLOW" "$C_OFF" "$*" >&2; }
err()   { printf '%s[ERR ]%s %s\n' "$C_RED"    "$C_OFF" "$*" >&2; }
die()   { err "$*"; exit 1; }

has()   { command -v "$1" >/dev/null 2>&1; }

need_root() {
    [[ "${EUID}" -eq 0 ]] || die "This command needs root: sudo vkai $*"
}

env_get() {
    local key="$1" line
    [[ -f "$ENV_FILE" ]] || return 1
    line="$(grep -E "^${key}=" "$ENV_FILE" | tail -n1 || true)"
    [[ -n "$line" ]] || return 1
    printf '%s' "${line#*=}"
}

# Read one top-level string field out of a small JSON file. The panel writes
# these files itself and jq is not a dependency of the panel, so a field regex
# is enough - and a missing field must never be fatal here.
json_get() {
    local file="$1" key="$2" value
    [[ -f "$file" ]] || return 1
    value="$(sed -n -E "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"([^\"]*)\".*/\\1/p" "$file" | head -n1)"
    [[ -n "$value" ]] || return 1
    printf '%s' "$value"
}

# Write one key back into .env, keeping mode 600.
env_set() {
    local key="$1" value="$2"
    [[ -f "$ENV_FILE" ]] || die "${ENV_FILE} not found"
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

# Which release is being served. "in place" = built inside /vkai-panel by the
# installer; anything else is a directory under /vkai-panel/releases.
current_release() {
    local target
    if [[ ! -L "$CURRENT_LINK" ]]; then
        printf '(no %s yet)' "$CURRENT_LINK"
        return 0
    fi
    target="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"
    if [[ -z "$target" ]]; then
        printf '(broken symlink)'
    elif [[ "$target" == "$PANEL_ROOT" ]]; then
        printf 'in place (%s)' "$PANEL_ROOT"
    else
        printf '%s' "$(basename "$target")"
    fi
}

# A UI build needs BOTH server.js AND .next/static. Without .next/static the
# page returns HTML while every /_next/static/*.js returns 404 and the browser
# shows "Application error: a client-side exception has occurred".
verify_ui_build() {
    local sa="${1}/.next/standalone"
    [[ -d "$sa" ]] ||
        die "UI BUILD VERIFICATION FAILED: ${sa} is missing (next.config.js must set output: 'standalone')."
    [[ -s "${sa}/server.js" ]] ||
        die "UI BUILD VERIFICATION FAILED: ${sa}/server.js is missing or empty - vkai-ui cannot start."
    [[ -d "${sa}/.next/static" ]] ||
        die "UI BUILD VERIFICATION FAILED: ${sa}/.next/static is missing.
The panel would show 'Application error: a client-side exception has occurred'.
Fix: cd ${1} && npm run build"
    [[ -n "$(find "${sa}/.next/static" -mindepth 1 -maxdepth 2 -print -quit 2>/dev/null)" ]] ||
        die "UI BUILD VERIFICATION FAILED: ${sa}/.next/static is empty. Rebuild the UI."
    info "UI build verified."
}

usage() {
    cat <<USAGE
${C_BOLD}${BRAND_NAME}${C_OFF} - the "vkai" administration command
${BRAND_ORG}

Usage: vkai <command> [arguments]

Services
  start                 Start ${SVC_API}, ${SVC_UI} and nginx
  stop                  Stop ${SVC_UI} and ${SVC_API}
  restart               Restart everything
  status                Service status and machine resources
  logs <api|ui|agent|nginx|install> [-n N]
                        Follow a log (live by default)

Panel access
  info                  Print the panel URL, port, entrance and data paths
  port [<number>|random]
                        Show or change the public panel port (never 80/443)
  entrance [<path>|random]
                        Show or change the security entrance

Certificate
  cert info             Show the panel certificate, its fingerprint and expiry
  cert renew            Renew it if it is close to expiry (used by the timer)
  cert issue            Order a new certificate now

Version and upgrade
  version               Show the installed version, channel and running release
  upgrade --check       Report whether a newer release exists; changes nothing.
                        Exits 10 when an update is available, so it can drive
                        monitoring.
  upgrade [--to <version>] [--yes]
                        Upgrade the panel. Without --yes it prints the plan and
                        asks first. Rolls back by itself when the new release
                        does not come up.

Operations
  backup [label]        Back up the database and configuration into ${WWW_BACKUP_DIR}
  update [source-path]  Keep the configuration, rebuild core/ and panel/, restart
  uninstall [--purge]   Remove the panel (delegates to deploy/install.sh)

Domain commands (delegated to vkai-cli)
  site|db|ssl|firewall|server ...
                        Example: vkai site create example.com

Note
  Ports 80 and 443 belong to the customer websites. The panel always listens on
  its own port. Port 80 also answers the ACME HTTP-01 challenge from
  ${ACME_CHALLENGE_DIR}.
USAGE
}

# -----------------------------------------------------------------------------
# Services
# -----------------------------------------------------------------------------
svc_state() {
    local svc="$1"
    if ! systemctl list-unit-files 2>/dev/null | grep -q "^${svc}\.service"; then
        printf '%s- not installed%s' "$C_YELLOW" "$C_OFF"
    elif systemctl is-active --quiet "$svc"; then
        printf '%s* running%s' "$C_GREEN" "$C_OFF"
    else
        printf '%s* stopped%s' "$C_RED" "$C_OFF"
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
    info "Starting ${BRAND_NAME}..."
    systemctl start "$SVC_API"
    systemctl start "$SVC_UI"
    systemctl reload nginx 2>/dev/null || systemctl start nginx 2>/dev/null || warn "nginx did not start."
    cmd_status
}

cmd_stop() {
    need_root stop
    info "Stopping ${BRAND_NAME}..."
    systemctl stop "$SVC_UI"  || true
    systemctl stop "$SVC_API" || true
    info "Stopped. nginx keeps running so the customer websites stay up."
}

cmd_restart() {
    need_root restart
    info "Restarting ${BRAND_NAME}..."
    systemctl restart "$SVC_API"
    systemctl restart "$SVC_UI"
    systemctl reload nginx 2>/dev/null || true
    cmd_status
}

cmd_status() {
    local redis_svc
    redis_svc="$(redis_service)"

    printf '\n%sServices%s\n' "$C_BLUE" "$C_OFF"
    printf '  %-14s %s\n' "$SVC_API"    "$(svc_state "$SVC_API")"
    printf '  %-14s %s\n' "$SVC_UI"     "$(svc_state "$SVC_UI")"
    printf '  %-14s %s\n' "$SVC_AGENT"  "$(svc_state "$SVC_AGENT")"
    printf '  %-14s %s\n' "nginx"       "$(svc_state nginx)"
    printf '  %-14s %s\n' "postgresql"  "$(svc_state postgresql)"
    printf '  %-14s %s\n' "$redis_svc"  "$(svc_state "$redis_svc")"
    if systemctl list-unit-files 2>/dev/null | grep -q "^${SVC_CERT_TIMER}"; then
        if systemctl is-active --quiet "$SVC_CERT_TIMER"; then
            printf '  %-14s %s* armed%s\n' "cert renew" "$C_GREEN" "$C_OFF"
        else
            printf '  %-14s %s* off%s\n' "cert renew" "$C_YELLOW" "$C_OFF"
        fi
    fi

    printf '\n%sRunning build%s\n' "$C_BLUE" "$C_OFF"
    printf '  %-14s %s\n' "current" "$(current_release)"

    printf '\n%sMachine%s\n' "$C_BLUE" "$C_OFF"
    printf '  RAM:  %s\n' "$(free -h 2>/dev/null | awk '/^Mem:/ {print $3 "/" $2}' || true)"
    printf '  Disk: %s\n' "$(df -h "$PANEL_ROOT" 2>/dev/null | awk 'NR==2 {print $3 "/" $2 " (" $5 " used)"}' || true)"
    printf '  Load: %s\n' "$(awk '{print $1 ", " $2 ", " $3}' /proc/loadavg)"

    local port entrance
    port="$(env_get VKAI_PANEL_PUBLIC_PORT || true)"
    entrance="$(env_get VKAI_PANEL_ENTRANCE || true)"
    if [[ -n "$port" ]]; then
        printf '\n%sPanel%s port %s, entrance %s   (full details: vkai info)\n' \
            "$C_BLUE" "$C_OFF" "$port" "${entrance:-(not set)}"
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
            [[ -f "${LOG_DIR}/install.log" ]] || die "${LOG_DIR}/install.log not found"
            tail -n "${lines:-200}" -f "${LOG_DIR}/install.log"
            ;;
        ""|-h|--help)
            die "Usage: vkai logs <api|ui|agent|nginx|install>"
            ;;
        *)
            die "Unknown log '${target}'. Choose: api, ui, agent, nginx, install."
            ;;
    esac
}

# -----------------------------------------------------------------------------
# Panel access
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
    local bin host port entrance scheme
    if bin="$(panelctl_bin)"; then
        "$bin" panel info || true
        printf '\n'
    fi

    host="$(env_get VKAI_PANEL_DOMAIN || true)"
    [[ -n "$host" ]] || host="$(hostname -I 2>/dev/null | awk '{print $1}')"
    port="$(env_get VKAI_PANEL_PUBLIC_PORT || echo 8888)"
    entrance="$(env_get VKAI_PANEL_ENTRANCE || true)"
    scheme="$(env_get VKAI_PANEL_PUBLIC_SCHEME || echo https)"

    printf '%s%s - access details%s\n' "$C_BOLD" "$BRAND_NAME" "$C_OFF"
    printf '  URL      : %s://%s:%s%s/\n' "$scheme" "${host:-<server-ip>}" "$port" "$entrance"
    printf '  Port     : %s   (80/443 stay reserved for the customer websites)\n' "$port"
    printf '  Entrance : %s\n' "${entrance:-(disabled)}"
    printf '\n%sPaths%s\n' "$C_BLUE" "$C_OFF"
    printf '  Install root     : %s\n' "$PANEL_ROOT"
    printf '  Running build    : %s -> %s\n' "$CURRENT_LINK" "$(current_release)"
    printf '  Releases         : %s\n' "$RELEASES_DIR"
    printf '  API (core)       : %s\n' "$CORE_DIR"
    printf '  UI (panel)       : %s\n' "$UI_DIR"
    printf '  Agent            : %s\n' "$AGENT_DIR"
    printf '  Customer sites   : %s/<domain>\n' "$WWW_DOMAINS_DIR"
    printf '  Backups          : %s\n' "$WWW_BACKUP_DIR"
    printf '  Configuration    : %s\n' "$ENV_FILE"
    printf '  Certificates     : %s\n' "$SSL_DIR"
    printf '  ACME webroot     : %s\n' "$ACME_CHALLENGE_DIR"
    printf '  Logs             : %s\n' "$LOG_DIR"
    if [[ -f "$SUMMARY_FILE" ]]; then
        printf '\n  Installation summary: %s\n' "$SUMMARY_FILE"
    fi
    printf '\n'
}

cmd_port() {
    local value="${1:-}"
    if [[ -z "$value" ]]; then
        printf 'Current panel port: %s\n' "$(env_get VKAI_PANEL_PUBLIC_PORT || echo '(not set)')"
        return 0
    fi
    need_root "port $value"

    if [[ "$value" == "random" ]]; then
        value="$(( (RANDOM % 50000) + 10000 ))"
    fi
    [[ "$value" =~ ^[0-9]+$ ]] || die "The port must be a number: '$value'"
    (( value >= 1 && value <= 65535 )) || die "Port out of range 1-65535: $value"
    [[ "$value" != "80" && "$value" != "443" ]] ||
        die "Ports 80/443 belong to the customer websites. Choose another port."

    local old
    old="$(env_get VKAI_PANEL_PUBLIC_PORT || echo 8888)"

    # Only the PUBLIC port changes. VKAI_PANEL_PORT is the loopback port the API
    # binds behind nginx and must stay where it is.
    env_set VKAI_PANEL_PUBLIC_PORT "$value"

    local conf="/etc/nginx/conf.d/vkai-panel.conf"
    if [[ -f "$conf" ]]; then
        cp -a "$conf" "${conf}.bak"
        sed -i -E "s/^(\s*listen\s+(\[::\]:)?)${old}(\s|;)/\1${value}\3/" "$conf"
        if nginx -t >/dev/null 2>&1; then
            systemctl reload nginx
            rm -f "${conf}.bak"
        else
            mv "${conf}.bak" "$conf"
            die "nginx -t failed after the port change; the previous ${conf} was restored."
        fi
    fi

    # Open the new port on the firewall and close the old one.
    if has ufw && ufw status 2>/dev/null | grep -qi '^Status: active'; then
        ufw allow "${value}/tcp" >/dev/null 2>&1 || true
        ufw delete allow "${old}/tcp" >/dev/null 2>&1 || true
    elif has firewall-cmd && systemctl is-active --quiet firewalld; then
        firewall-cmd --permanent --add-port="${value}/tcp" >/dev/null 2>&1 || true
        firewall-cmd --permanent --remove-port="${old}/tcp" >/dev/null 2>&1 || true
        firewall-cmd --reload >/dev/null 2>&1 || true
    else
        warn "No managed firewall was found. Open ${value}/tcp yourself."
    fi

    if has semanage; then
        semanage port -a -t http_port_t -p tcp "$value" 2>/dev/null ||
        semanage port -m -t http_port_t -p tcp "$value" 2>/dev/null || true
    fi

    systemctl restart "$SVC_API"
    info "Panel port: ${old} -> ${value}"
    cmd_info
}

cmd_entrance() {
    local value="${1:-}"
    if [[ -z "$value" ]]; then
        printf 'Current security entrance: %s\n' "$(env_get VKAI_PANEL_ENTRANCE || echo '(disabled)')"
        return 0
    fi
    need_root "entrance $value"

    if [[ "$value" == "random" ]]; then
        value="/vkai_$(openssl rand -hex 4)"
    fi
    [[ "$value" == /* ]] || value="/${value}"
    [[ "$value" =~ ^/[A-Za-z0-9_-]{4,64}$ ]] ||
        die "The entrance accepts letters, digits, '-' and '_', 4-64 characters. Example: /vkai_a1b2c3d4"

    env_set VKAI_PANEL_ENTRANCE "$value"
    env_set VKAI_PANEL_ENTRANCE_ENABLED true
    systemctl restart "$SVC_API"
    info "New security entrance: ${value}"
    cmd_info
}

# -----------------------------------------------------------------------------
# Certificate
#
# The panel's own certificate, not the customer certificates - those stay with
# certbot and "vkai ssl". A certificate issued for an IP ADDRESS comes from the
# Let's Encrypt "shortlived" profile and lives about six days, which is why the
# renewal timer runs twice a day.
# -----------------------------------------------------------------------------
cert_field() {
    local what="$1"
    [[ -f "$PANEL_CERT" ]] || { printf '(no certificate)'; return 0; }
    case "$what" in
        fingerprint) openssl x509 -in "$PANEL_CERT" -noout -fingerprint -sha256 2>/dev/null | sed 's/^.*Fingerprint=//' ;;
        expiry)      openssl x509 -in "$PANEL_CERT" -noout -enddate 2>/dev/null | sed 's/^notAfter=//' ;;
        issuer)      openssl x509 -in "$PANEL_CERT" -noout -issuer 2>/dev/null | sed 's/^issuer=//' ;;
        subject)     openssl x509 -in "$PANEL_CERT" -noout -subject 2>/dev/null | sed 's/^subject=//' ;;
        san)         openssl x509 -in "$PANEL_CERT" -noout -ext subjectAltName 2>/dev/null | tail -n +2 | tr -d ' ' ;;
    esac
}

cert_days_left() {
    [[ -f "$PANEL_CERT" ]] || { printf '0'; return 0; }
    local end_epoch now_epoch
    end_epoch="$(date -d "$(openssl x509 -in "$PANEL_CERT" -noout -enddate 2>/dev/null | sed 's/^notAfter=//')" +%s 2>/dev/null || echo 0)"
    now_epoch="$(date +%s)"
    if (( end_epoch <= now_epoch )); then printf '0'; else printf '%d' $(( (end_epoch - now_epoch) / 86400 )); fi
}

cmd_cert_info() {
    local mode identifier
    mode="$(env_get VKAI_PANEL_TLS_MODE || echo 'unknown')"
    identifier="$(env_get VKAI_PANEL_ACME_IDENTIFIER || echo '(none)')"

    printf '\n%s%s - panel certificate%s\n' "$C_BOLD" "$BRAND_NAME" "$C_OFF"
    printf '  Mode        : %s\n' "$mode"
    printf '  Identifier  : %s\n' "$identifier"
    printf '  Certificate : %s\n' "$PANEL_CERT"
    printf '  Key         : %s\n' "$PANEL_KEY"
    printf '  Subject     : %s\n' "$(cert_field subject)"
    printf '  Issuer      : %s\n' "$(cert_field issuer)"
    printf '  SAN         : %s\n' "$(cert_field san)"
    printf '  Expires     : %s (%s days left)\n' "$(cert_field expiry)" "$(cert_days_left)"
    printf '  SHA-256     : %s\n' "$(cert_field fingerprint)"
    printf '  ACME webroot: %s\n' "$ACME_CHALLENGE_DIR"
    printf '\n'
}

# cert_call <subcommand> [args...] - hand the work to vkai-panelctl, which owns
# the ACME client. Returns 2 when this build has no such command.
cert_call() {
    local bin
    bin="$(panelctl_bin)" || return 2
    "$bin" panel cert --help >/dev/null 2>&1 || return 2
    "$bin" panel cert "$@"
}

cmd_cert_renew() {
    need_root "cert renew"
    local mode days
    mode="$(env_get VKAI_PANEL_TLS_MODE || echo 'self-signed')"
    days="$(cert_days_left)"

    if [[ "$mode" != "letsencrypt" ]]; then
        info "TLS mode is '${mode}': there is nothing to renew (the certificate is local)."
        return 0
    fi

    info "Panel certificate has ${days} day(s) left; asking the ACME client to renew if needed."
    local rc=0
    cert_call renew \
        --identifier "$(env_get VKAI_PANEL_ACME_IDENTIFIER || true)" \
        --webroot    "$(env_get VKAI_PANEL_ACME_WEBROOT || echo "$WWW_DEFAULT_DIR")" \
        --directory  "$(env_get VKAI_PANEL_ACME_DIRECTORY || true)" \
        --profile    "$(env_get VKAI_PANEL_ACME_PROFILE || true)" \
        --cert "$PANEL_CERT" --key "$PANEL_KEY" || rc=$?

    if (( rc == 2 )); then
        warn "This build of vkai-panelctl has no 'panel cert' command; renewal was skipped."
        return 0
    fi
    if (( rc != 0 )); then
        err "Renewal failed (exit ${rc}). The existing certificate is still in place."
        return "$rc"
    fi

    chown "$VKAI_USER:$VKAI_GROUP" "$PANEL_CERT" "$PANEL_KEY" 2>/dev/null || true
    chmod 644 "$PANEL_CERT" 2>/dev/null || true
    chmod 640 "$PANEL_KEY" 2>/dev/null || true
    if nginx -t >/dev/null 2>&1; then
        systemctl reload nginx
        info "Certificate renewed and nginx reloaded. New expiry: $(cert_field expiry)"
    else
        warn "The certificate was renewed but nginx -t failed; nginx was NOT reloaded."
    fi
}

cmd_cert_issue() {
    need_root "cert issue"
    local rc=0
    cert_call issue \
        --identifier "$(env_get VKAI_PANEL_ACME_IDENTIFIER || true)" \
        --email      "$(env_get VKAI_PANEL_ACME_EMAIL || true)" \
        --webroot    "$(env_get VKAI_PANEL_ACME_WEBROOT || echo "$WWW_DEFAULT_DIR")" \
        --directory  "$(env_get VKAI_PANEL_ACME_DIRECTORY || true)" \
        --profile    "$(env_get VKAI_PANEL_ACME_PROFILE || true)" \
        --cert "$PANEL_CERT" --key "$PANEL_KEY" --agree-tos || rc=$?

    if (( rc == 2 )); then
        die "This build of vkai-panelctl has no 'panel cert' command, so no order can be placed.
Everything an order needs is already prepared: webroot ${ACME_CHALLENGE_DIR}, port 80 open."
    fi
    (( rc == 0 )) || die "The certificate order failed (exit ${rc})."

    env_set VKAI_PANEL_TLS_MODE letsencrypt
    chown "$VKAI_USER:$VKAI_GROUP" "$PANEL_CERT" "$PANEL_KEY" 2>/dev/null || true
    chmod 644 "$PANEL_CERT"; chmod 640 "$PANEL_KEY"
    if nginx -t >/dev/null 2>&1; then
        systemctl reload nginx
    fi
    systemctl enable --now "$SVC_CERT_TIMER" >/dev/null 2>&1 || true
    info "Certificate issued. Expiry: $(cert_field expiry)"
    cmd_cert_info
}

cmd_cert() {
    local sub="${1:-info}"
    if (( $# > 0 )); then shift; fi
    # The renewal timer passes --quiet.
    local arg
    for arg in "$@"; do
        if [[ "$arg" == "--quiet" || "$arg" == "-q" ]]; then
            QUIET="true"
        fi
    done
    case "$sub" in
        info)   cmd_cert_info ;;
        renew)  cmd_cert_renew ;;
        issue)  cmd_cert_issue ;;
        *)      die "Unknown certificate command '${sub}'. Choose: info, renew, issue." ;;
    esac
}

# -----------------------------------------------------------------------------
# Backup
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
        info "Dumping database '${db_name}'..."
        if [[ -n "$db_pass" ]]; then
            PGPASSWORD="$db_pass" pg_dump -h 127.0.0.1 -U "$db_user" "$db_name" | gzip >"${dest}/${db_name}.sql.gz"
        else
            sudo -u postgres pg_dump "$db_name" | gzip >"${dest}/${db_name}.sql.gz"
        fi
    else
        warn "pg_dump is not available, the database was skipped."
    fi

    info "Archiving the configuration..."
    tar -czf "${dest}/etc.tar.gz" -C "$PANEL_ROOT" etc 2>/dev/null || warn "Could not archive ${ETC_DIR}"

    if [[ -d /etc/nginx ]]; then
        tar -czf "${dest}/nginx.tar.gz" -C /etc nginx 2>/dev/null || true
    fi

    chown -R "$VKAI_USER:$VKAI_GROUP" "$dest"
    chmod -R go-rwx "$dest"
    info "Backup written to ${dest}"
    du -sh "$dest" 2>/dev/null || true
    warn "Website documents (${WWW_DOMAINS_DIR}) are NOT in this backup - use 'vkai site backup' or tar them separately."
}

# -----------------------------------------------------------------------------
# Version and upgrade
#
# The upgrade engine itself lives in the Go binary (vkai-cli upgrade). This
# wrapper is the operator-facing surface: it resolves the binary, keeps the
# machine-readable check record up to date and degrades to printed instructions
# on a build whose vkai-cli is older than the upgrade command.
# -----------------------------------------------------------------------------

# Read one top-level string field out of a JSON document held in a variable.
# Commas and braces are turned into line breaks first, so the same expression
# works on pretty-printed and single-line JSON, and the anchor at the start of
# the line stops "version" from matching inside "installed_version".
# The panel writes these documents itself and never puts a comma inside one of
# these values, which is why this is enough and jq is not a dependency.
json_field() {
    local json="$1" key="$2" value
    # shellcheck disable=SC2020  # three single characters, each mapped to a newline
    value="$(printf '%s' "$json" | tr ',{}' '\n\n\n' |
        sed -n -E "s/^[[:space:]]*\"${key}\"[[:space:]]*:[[:space:]]*\"([^\"]*)\".*/\1/p" | sed -n '1p')"
    [[ -n "$value" ]] || return 1
    printf '%s' "$value"
}

installed_version() { json_get "$VERSION_FILE" version || printf 'unknown'; }
installed_channel() { json_get "$VERSION_FILE" channel || printf '%s' "$DEFAULT_CHANNEL"; }
version_pin()       { json_get "$VERSION_FILE" pin     || printf ''; }
now_utc()           { date -u +%Y-%m-%dT%H:%M:%SZ; }

# Replace a file in ${ETC_DIR} in one step. A half-written version.json is a
# panel that no longer knows what it is running, so it is never written in place.
atomic_write_etc() {
    local dest="$1" mode="$2" payload="$3" tmp
    tmp="$(mktemp "${ETC_DIR}/.$(basename "$dest").XXXXXX" 2>/dev/null)" || return 1
    printf '%s\n' "$payload" >"$tmp" || { rm -f "$tmp"; return 1; }
    chmod "$mode" "$tmp"
    # Owned by root, readable by the panel: vkai-api only reads these files.
    chown "root:${VKAI_GROUP}" "$tmp" 2>/dev/null || true
    mv -f "$tmp" "$dest"
}

# Record the result of an update check where the panel can display it.
write_upgrade_state() {
    local payload="$1"
    if ! atomic_write_etc "$UPGRADE_STATE_FILE" 0644 "$payload"; then
        warn "Could not write ${UPGRADE_STATE_FILE} (run as root to record the result)."
        return 0
    fi
}

# Build a check record locally, for the cases where vkai-cli could not produce
# one. Keep every value free of '"' and ',' - see json_field.
upgrade_state_json() {
    local status="$1" latest="$2" detail="$3"
    cat <<JSON
{
  "checked_at": "$(now_utc)",
  "status": "${status}",
  "installed_version": "$(installed_version)",
  "latest_version": "${latest}",
  "channel": "$(installed_channel)",
  "pinned": "$(version_pin)",
  "release_notes_url": "${RELEASE_NOTES_URL}",
  "detail": "${detail}"
}
JSON
}

# Rewrite version.json after the running code changed, keeping the fields this
# caller has no business changing (channel, pin, install date). The version is
# taken from the argument when the caller knows it; otherwise the recorded one
# is kept, because the upgrade engine writes this file too and its value is
# newer than anything this wrapper could guess.
stamp_version_file() {
    local version channel pin installed
    version="${1:-$(installed_version)}"
    channel="$(installed_channel)"
    pin="$(version_pin)"
    installed="$(json_get "$VERSION_FILE" installed_at || now_utc)"
    atomic_write_etc "$VERSION_FILE" 0644 "$(cat <<JSON
{
  "version": "${version}",
  "channel": "${channel}",
  "pin": "${pin}",
  "installed_at": "${installed}",
  "updated_at": "$(now_utc)",
  "release": "$(current_release)"
}
JSON
)" || warn "Could not update ${VERSION_FILE}."
}

# The Go CLI, which owns the upgrade engine and the domain commands.
cli_bin() {
    if has vkai-cli; then
        printf 'vkai-cli'
    elif [[ -x "${CORE_DIR}/bin/vkai-cli" ]]; then
        printf '%s' "${CORE_DIR}/bin/vkai-cli"
    elif [[ -x "${CURRENT_LINK}/core/bin/vkai-cli" ]]; then
        printf '%s' "${CURRENT_LINK}/core/bin/vkai-cli"
    else
        return 1
    fi
}

# upgrade_cli <args...> - hand the work to vkai-cli. Returns 2 when this build
# has no "upgrade" command, which is exactly the build an operator is upgrading
# FROM, so it must produce instructions rather than a stack of shell errors.
upgrade_cli() {
    local bin
    bin="$(cli_bin)" || return 2
    "$bin" upgrade --help >/dev/null 2>&1 || return 2
    "$bin" upgrade "$@"
}

manual_upgrade_hint() {
    cat >&2 <<HINT
Upgrade from a release package instead:
  1. Download vkai-panel-<version>.tar.gz onto this server.
  2. sudo ${BIN_DEPLOY} deploy /tmp/vkai-panel-<version>.tar.gz
It unpacks into ${RELEASES_DIR}/<id>, migrates, moves ${CURRENT_LINK}, restarts
and rolls back by itself if the new release does not answer. Details: docs/UPGRADE.md
HINT
}

cmd_version() {
    local last_status last_latest last_checked
    printf '\n%s%s %s%s\n' "$C_BOLD" "$BRAND_NAME" "$(installed_version)" "$C_OFF"
    printf '  Channel        : %s\n' "$(installed_channel)"
    local pin; pin="$(version_pin)"
    [[ -z "$pin" ]] || printf '  Pinned to      : %s   (upgrades are held at this version)\n' "$pin"
    printf '  Installed      : %s\n' "$(json_get "$VERSION_FILE" installed_at || printf 'unknown')"
    printf '  Last changed   : %s\n' "$(json_get "$VERSION_FILE" updated_at || printf 'unknown')"
    printf '  Running release: %s\n' "$(current_release)"
    printf '  Record         : %s\n' "$VERSION_FILE"

    if [[ -f "$UPGRADE_STATE_FILE" ]]; then
        last_status="$(json_get "$UPGRADE_STATE_FILE" status || printf 'unknown')"
        last_latest="$(json_get "$UPGRADE_STATE_FILE" latest_version || printf '')"
        last_checked="$(json_get "$UPGRADE_STATE_FILE" checked_at || printf 'unknown')"
        printf '\n%sLast update check%s\n' "$C_BLUE" "$C_OFF"
        printf '  When           : %s\n' "$last_checked"
        printf '  Result         : %s%s\n' "$last_status" \
            "$( [[ -n "$last_latest" ]] && printf ' (latest %s)' "$last_latest" || printf '' )"
        [[ "$last_status" != "update-available" ]] ||
            printf '  Install it     : sudo vkai upgrade\n'
    else
        printf '\n  No update check has run yet: vkai upgrade --check\n'
    fi

    local bin
    if bin="$(cli_bin)"; then
        printf '\n%sBinaries%s\n' "$C_BLUE" "$C_OFF"
        printf '  %s\n' "$("$bin" version 2>/dev/null | head -n1 || printf 'vkai-cli (no version output)')"
    fi
    printf '\n'
}

# Report only. Never changes the installed code - the only file it touches is
# the check record the panel reads.
# Exit codes: 0 up to date or pinned, 10 an update is available,
#             2 this build cannot check, 1 the check failed.
upgrade_check() {
    local as_json="$1" out="" rc=0 payload=""
    out="$(upgrade_cli --check --json 2>/dev/null)" || rc=$?

    if [[ "$out" == \{* ]]; then
        payload="$out"
    elif (( rc == 2 )); then
        payload="$(upgrade_state_json unsupported "" "the installed vkai-cli has no upgrade command")"
    else
        payload="$(upgrade_state_json error "" "the update check produced no usable result (exit ${rc})")"
        rc=1
    fi

    write_upgrade_state "$payload"

    if [[ "$as_json" == "true" ]]; then
        printf '%s\n' "$payload"
        exit "$rc"
    fi
    print_upgrade_summary "$payload"
    exit "$rc"
}

print_upgrade_summary() {
    local payload="$1" status installed latest channel pinned detail
    status="$(json_field "$payload" status || printf 'unknown')"
    installed="$(json_field "$payload" installed_version || printf 'unknown')"
    latest="$(json_field "$payload" latest_version || printf 'unknown')"
    channel="$(json_field "$payload" channel || printf 'unknown')"
    pinned="$(json_field "$payload" pinned || printf '')"
    detail="$(json_field "$payload" detail || printf '')"

    # The daily timer runs with --quiet: one line in the journal, no banner.
    if [[ "$QUIET" == "true" ]]; then
        printf 'update check: %s (installed %s, latest %s, channel %s)\n' \
            "$status" "$installed" "$latest" "$channel"
        case "$status" in
            unsupported|error) warn "${detail:-the update check did not succeed}" ;;
        esac
        return 0
    fi

    printf '\n%s%s - update check%s\n' "$C_BOLD" "$BRAND_NAME" "$C_OFF"
    printf '  Installed : %s\n' "$installed"
    printf '  Latest    : %s\n' "$latest"
    printf '  Channel   : %s\n' "$channel"
    [[ -z "$pinned" ]] || printf '  Pinned to : %s\n' "$pinned"
    printf '  Checked   : %s\n' "$(json_field "$payload" checked_at || printf 'unknown')"
    printf '\n'

    case "$status" in
        update-available)
            printf 'An update is available: %s -> %s\n' "$installed" "$latest"
            printf 'Install it with : sudo vkai upgrade\n'
            printf 'Release notes   : %s\n\n' "$RELEASE_NOTES_URL"
            ;;
        pinned)
            printf 'A newer release exists but this panel is pinned to %s.\n' "$pinned"
            printf 'Clear "pin" in %s to allow upgrades again.\n\n' "$VERSION_FILE"
            ;;
        up-to-date)
            printf 'This panel is up to date.\n\n'
            ;;
        unsupported)
            err "This build cannot check for updates: ${detail:-no upgrade engine}"
            manual_upgrade_hint
            ;;
        *)
            err "The update check failed: ${detail:-unknown reason}"
            ;;
    esac
}

# Re-run the check after the code changed, so the panel stops advertising an
# update it has just installed. Best effort: never fails the upgrade.
refresh_upgrade_state() {
    local out
    out="$(upgrade_cli --check --json 2>/dev/null || true)"
    [[ "$out" == \{* ]] || return 0
    write_upgrade_state "$out"
}

upgrade_apply() {
    local target="$1" assume_yes="$2" rc=0
    local args=()
    [[ -z "$target" ]] || args+=(--to "$target")
    [[ "$assume_yes" != "true" ]] || args+=(--yes)

    upgrade_cli ${args[@]+"${args[@]}"} || rc=$?

    if (( rc == 2 )); then
        err "This build has no upgrade engine (vkai-cli has no 'upgrade' command)."
        manual_upgrade_hint
        exit 2
    fi
    if (( rc != 0 )); then
        err "The upgrade did not complete (exit ${rc})."
        err "Check what is running now: vkai version && vkai status"
        err "If neither the new release nor the rollback came up, follow the manual"
        err "recovery in docs/UPGRADE.md."
        exit "$rc"
    fi

    stamp_version_file "$target"
    refresh_upgrade_state
    info "Upgrade finished."
    cmd_status
}

cmd_upgrade() {
    local check="false" target="" assume_yes="false" as_json="false"
    while (( $# > 0 )); do
        case "$1" in
            --check)    check="true"; shift ;;
            --to)       [[ $# -ge 2 ]] || die "--to needs a version, for example: vkai upgrade --to 1.4.2"
                        target="$2"; shift 2 ;;
            --to=*)     target="${1#*=}"; shift ;;
            -y|--yes)   assume_yes="true"; shift ;;
            --json)     as_json="true"; shift ;;
            -q|--quiet) QUIET="true"; shift ;;
            -h|--help)  usage; return 0 ;;
            *)          die "Unknown option for 'vkai upgrade': $1
Usage: vkai upgrade [--check] [--to <version>] [--yes]" ;;
        esac
    done

    if [[ "$check" == "true" ]]; then
        # Deliberately allowed without root: reporting must be cheap. Only the
        # record in ${ETC_DIR} needs privileges, and failing to write it is a
        # warning, not an error.
        upgrade_check "$as_json"
    fi

    [[ "$as_json" != "true" ]] || die "--json only makes sense together with --check."
    need_root upgrade
    upgrade_apply "$target" "$assume_yes"
}

# -----------------------------------------------------------------------------
# Update
# -----------------------------------------------------------------------------
cmd_update() {
    need_root update
    local src="${1:-}"

    if [[ -z "$src" ]]; then
        src="$PANEL_ROOT"
        info "No source given: rebuilding in place from ${PANEL_ROOT}."
    fi
    [[ -d "${src}/core" && -d "${src}/panel" ]] ||
        die "'${src}' is not a valid source tree (core/ or panel/ is missing)."

    cmd_backup pre-update

    local go_bin npm_bin
    go_bin="$(command -v go || echo /usr/local/go/bin/go)"
    npm_bin="$(command -v npm || true)"
    [[ -x "$go_bin" ]] || die "'go' not found."
    [[ -n "$npm_bin" ]] || die "'npm' not found."

    if [[ "$(cd "$src" && pwd -P)" != "$(cd "$PANEL_ROOT" && pwd -P)" ]]; then
        info "Copying the new sources into ${PANEL_ROOT}..."
        has rsync || die "rsync is required to copy the sources."
        rsync -a --delete --exclude '.git' --exclude 'node_modules' --exclude '.next' \
            "${src}/core/"  "${CORE_DIR}/"
        rsync -a --delete --exclude '.git' --exclude 'node_modules' --exclude '.next' \
            "${src}/panel/" "${UI_DIR}/"
        if [[ -d "${src}/agent" ]]; then
            rsync -a --delete --exclude '.git' "${src}/agent/" "${AGENT_DIR}/"
        fi
        chown -R "$VKAI_USER:$VKAI_GROUP" "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"
    fi

    # .env must be present at the Next.js project root BEFORE the build:
    # NEXT_PUBLIC_* is inlined at build time, never read at runtime.
    ln -sfn "$ENV_FILE" "${UI_DIR}/.env" 2>/dev/null ||
        install -o "$VKAI_USER" -g "$VKAI_GROUP" -m 600 "$ENV_FILE" "${UI_DIR}/.env"

    systemctl stop "$SVC_UI" "$SVC_API" || true

    export HOME="${PANEL_ROOT}/tmp"
    export GOCACHE="${HOME}/go-build" GOMODCACHE="${HOME}/go-mod" GOFLAGS="-buildvcs=false"
    export npm_config_cache="${HOME}/npm"
    mkdir -p "$GOCACHE" "$GOMODCACHE" "$npm_config_cache"

    info "Rebuilding the API..."
    ( cd "$CORE_DIR" && "$go_bin" mod download &&
      "$go_bin" build -trimpath -ldflags "-s -w" -o "${CORE_DIR}/bin/vkai-api" ./cmd/api &&
      "$go_bin" build -trimpath -ldflags "-s -w" -o "${CORE_DIR}/bin/vkai-panelctl" ./cmd/panelctl )
    install -m 0755 "${CORE_DIR}/bin/vkai-panelctl" /usr/local/bin/vkai-panelctl
    if [[ -d "${CORE_DIR}/cmd/cli" ]]; then
        ( cd "$CORE_DIR" && "$go_bin" build -trimpath -ldflags "-s -w" -o "${CORE_DIR}/bin/vkai-cli" ./cmd/cli )
        install -m 0755 "${CORE_DIR}/bin/vkai-cli" /usr/local/bin/vkai-cli
    fi

    if [[ -d "$AGENT_DIR" ]]; then
        info "Rebuilding the agent..."
        ( cd "$AGENT_DIR" && "$go_bin" mod download &&
          "$go_bin" build -trimpath -ldflags "-s -w" -o "${AGENT_DIR}/bin/vkai-agent" ./cmd ) ||
            warn "The agent build failed, skipped."
    fi

    info "Rebuilding the UI..."
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

    # Mandatory: standalone does not include .next/static and public/ by itself.
    local sa="${UI_DIR}/.next/standalone"
    [[ -d "$sa" ]] || die "${sa} does not exist after the build."
    [[ -d "${sa}/.next/static" ]] || { mkdir -p "${sa}/.next"; cp -a "${UI_DIR}/.next/static" "${sa}/.next/static"; }
    [[ -d "${sa}/public" || ! -d "${UI_DIR}/public" ]] || cp -a "${UI_DIR}/public" "${sa}/public"
    verify_ui_build "$UI_DIR"

    chown -R "$VKAI_USER:$VKAI_GROUP" "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"

    # This was an in-place rebuild, so the running build has to be /vkai-panel
    # again rather than a directory under releases/ left by deploy.sh.
    if [[ -e "$CURRENT_LINK" && ! -L "$CURRENT_LINK" ]]; then
        die "${CURRENT_LINK} exists but is not a symlink. Move it aside and run again."
    fi
    ln -sfn "$PANEL_ROOT" "$CURRENT_LINK"
    chown -h "$VKAI_USER:$VKAI_GROUP" "$CURRENT_LINK" 2>/dev/null || true
    info "Running build: ${CURRENT_LINK} -> ${PANEL_ROOT}"

    systemctl daemon-reload
    systemctl start "$SVC_API" "$SVC_UI"
    # The rebuild replaced the running code, so the version record has to say
    # so. "vkai update" does not know a release number, hence only the
    # timestamp and the running release are refreshed.
    stamp_version_file
    info "Update complete."
    cmd_status
}

cmd_uninstall() {
    need_root uninstall
    local installer=""
    for installer in "${PANEL_ROOT}/deploy/install.sh" "$(dirname "$(readlink -f "$0")")/install.sh"; do
        if [[ -f "$installer" ]]; then
            exec bash "$installer" --uninstall "$@"
        fi
    done
    die "install.sh not found. Run: bash <source>/deploy/install.sh --uninstall"
}

# -----------------------------------------------------------------------------
# Domain commands are delegated to vkai-cli (the Go binary)
# -----------------------------------------------------------------------------
delegate_cli() {
    local bin=""
    bin="$(cli_bin)" ||
        die "'vkai-cli' not found. Rebuild it: cd ${CORE_DIR} && go build -o bin/vkai-cli ./cmd/cli"
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
        cert)       cmd_cert "$@" ;;
        version)    cmd_version ;;
        upgrade)    cmd_upgrade "$@" ;;
        backup)     cmd_backup "${1:-}" ;;
        update)     cmd_update "${1:-}" ;;
        uninstall)  cmd_uninstall "$@" ;;
        panel)
            # Backwards compatible: "vkai panel port 8888" -> "vkai port 8888".
            local sub="${1:-info}"
            if (( $# > 0 )); then shift; fi
            case "$sub" in
                info)     cmd_info ;;
                port)     cmd_port "${1:-}" ;;
                entrance) cmd_entrance "${1:-}" ;;
                cert)     cmd_cert "$@" ;;
                *)        panelctl_bin >/dev/null && exec "$(panelctl_bin)" panel "$sub" "$@" ;;
            esac
            ;;
        backup-cli)
            # "vkai backup" backs up the panel; "vkai backup-cli" opens the
            # backup commands of vkai-cli.
            delegate_cli backup "$@"
            ;;
        site|db|ssl|firewall|server|service)
            delegate_cli "$cmd" "$@"
            ;;
        ""|-h|--help|help)
            usage
            ;;
        *)
            usage >&2
            die "Unknown command: ${cmd}"
            ;;
    esac
}

main "$@"
