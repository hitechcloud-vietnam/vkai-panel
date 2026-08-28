#!/usr/bin/env bash
# =============================================================================
# VKAI Panel - one command, fully automatic installer (multi OS)
# HiTechCloud (hitechcloud.vn)
#
#   curl -fsSL https://get.vkai.vn/install.sh | bash     # unattended, defaults
#   bash deploy/install.sh                               # unattended, defaults
#   bash deploy/install.sh --port 9001 --domain panel.example.com
#   bash deploy/install.sh --tls-mode letsencrypt --admin-email me@example.com
#   bash deploy/install.sh --uninstall                   # keeps customer data
#   bash deploy/install.sh --uninstall --purge           # removes everything
#
# The installer NEVER asks a question. Every decision is a flag with a sane
# default, so the same command works from a terminal, from cloud-init and from
# a pipe with no tty at all.
#
# Ports 80 and 443 stay reserved for the customer websites. The panel always
# listens on its own port (8888 by default) behind a secret entrance such as
# /vkai_a1b2c3d4. Port 80 is opened and an ACME webroot is prepared because an
# HTTP-01 challenge for an IP identifier can only be answered there.
# =============================================================================

set -Eeuo pipefail

# -----------------------------------------------------------------------------
# Constants
# -----------------------------------------------------------------------------
# The product version. It is NOT written here: the file VERSION at the repository
# root is the single source of truth that the Makefile, the release workflow, the
# UI build and the Go binaries all read. A literal in this script is how an
# installed panel came to report 1.0.0 while VERSION said 0.5.0.
#
# It is resolved as soon as the source tree is known, which for a "curl | bash"
# run is only after bootstrap_sources has downloaded one - hence the placeholder
# and the second call in main().
VKAI_VERSION="unknown"
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
# HTTP-01 for an IP identifier has no DNS-01 alternative: the challenge file has
# to be readable over plain HTTP on port 80, from this exact directory.
readonly ACME_WEBROOT="${WWW_DEFAULT_DIR}"
readonly ACME_CHALLENGE_DIR="${WWW_DEFAULT_DIR}/.well-known/acme-challenge"
readonly BIN_DIR="${PANEL_ROOT}/bin"
readonly ETC_DIR="${PANEL_ROOT}/etc"
readonly LOG_DIR="${PANEL_ROOT}/logs"
readonly LOG_SITES_DIR="${LOG_DIR}/sites"
readonly SSL_DIR="${PANEL_ROOT}/ssl"
readonly TMP_DIR="${PANEL_ROOT}/tmp"
# Release layout: the systemd units point at ${CURRENT_LINK}/... This installer
# builds in place (${CORE_DIR}, ${UI_DIR}) and points current at ${PANEL_ROOT};
# deploy/scripts/deploy.sh moves current to ${RELEASES_DIR}/<version>.
readonly RELEASES_DIR="${PANEL_ROOT}/releases"
readonly CURRENT_LINK="${PANEL_ROOT}/current"

readonly ENV_FILE="${ETC_DIR}/.env"
readonly PANEL_STATE_FILE="${ETC_DIR}/panel_access.json"
readonly SUMMARY_FILE="${ETC_DIR}/install-summary.txt"
# What is installed, on which channel, since when. "vkai upgrade" reads this to
# know what it is upgrading FROM, which matters most on a machine where somebody
# replaced a binary by hand and the running code no longer matches any release.
readonly VERSION_FILE="${ETC_DIR}/version.json"
# The machine the panel is installed on is the first managed node. These two
# files are what makes that true across reinstalls: agent.env is the agent's own
# configuration (kept apart from .env, which this script rewrites wholesale), and
# node.json records which servers row is this machine. Both are written by
# "vkai node register" and both live in ${ETC_DIR}, which an uninstall without
# --purge keeps.
readonly AGENT_ENV_FILE="${ETC_DIR}/agent.env"
readonly NODE_RECORD_FILE="${ETC_DIR}/node.json"   # written by internal/localnode
readonly DEFAULT_CHANNEL="stable"
readonly MIGRATIONS_STATE="${ETC_DIR}/migrations.applied"
readonly INSTALL_LOG="${LOG_DIR}/install.log"

readonly PANEL_CERT="${SSL_DIR}/panel.crt"
readonly PANEL_KEY="${SSL_DIR}/panel.key"
readonly ACME_ACCOUNT_KEY="${SSL_DIR}/acme-account.key"

# Let's Encrypt production and staging directories. A certificate for an IP
# address is only issued through the "shortlived" profile (about six days), so
# the profile is chosen from the identifier, not from taste.
readonly ACME_DIRECTORY_PROD="https://acme-v02.api.letsencrypt.org/directory"
readonly ACME_DIRECTORY_STAGING="https://acme-staging-v02.api.letsencrypt.org/directory"
readonly ACME_PROFILE_IP="shortlived"
readonly ACME_PROFILE_DOMAIN="tlsserver"

readonly VKAI_USER="vkai"
readonly VKAI_GROUP="vkai"

# Runtimes downloaded when the machine has nothing usable.
readonly GO_VERSION="1.22.5"
readonly GO_MIN_MINOR="22"
readonly NODE_VERSION="20.18.1"
readonly NODE_MIN_MAJOR="20"

# Internal ports: loopback only, nginx is the only thing that talks to them.
readonly API_BIND_PORT="30110"
readonly UI_BIND_PORT="3000"
readonly AGENT_PORT="30111"

readonly DEFAULT_PANEL_PORT="8888"

# Minimum machine.
readonly MIN_RAM_MB="900"
readonly MIN_DISK_MB="5120"

# Where "curl | bash" fetches the sources from when the script runs on its own.
readonly SOURCE_REPO="hitechcloud-vietnam/vkai-panel"
DEFAULT_SOURCE_URL="https://codeload.github.com/${SOURCE_REPO}/tar.gz/refs/heads/main"
readonly DEFAULT_SOURCE_URL

# Source tree (the parent directory of deploy/). Empty when the script was piped
# into bash, in which case bootstrap_sources() downloads a tree.
SRC_DIR=""
SELF_CMD="bash deploy/install.sh"
if [[ -n "${BASH_SOURCE[0]:-}" && -f "${BASH_SOURCE[0]}" ]]; then
    SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
    SELF_CMD="bash ${BASH_SOURCE[0]}"
fi
readonly SELF_CMD

# read_product_version fills VKAI_VERSION from ${SRC_DIR}/VERSION. It returns
# non-zero when no source tree is available yet, so the caller can decide
# whether that is fatal.
read_product_version() {
    local file="${SRC_DIR}/VERSION" value
    [[ -n "$SRC_DIR" && -f "$file" ]] || return 1
    value="$(tr -d '[:space:]' <"$file")"
    [[ -n "$value" ]] || return 1
    VKAI_VERSION="$value"
}

# Best effort before the banner is printed; main() insists on it once the source
# tree is certain.
read_product_version || true

# -----------------------------------------------------------------------------
# Command line options (defaults chosen so that "no flags" is a valid install)
# -----------------------------------------------------------------------------
OPT_PORT=""
OPT_ENTRANCE=""
OPT_DOMAIN=""
OPT_ADMIN_EMAIL=""
OPT_TLS_MODE="self-signed"      # self-signed | letsencrypt | none
OPT_CHANNEL="${DEFAULT_CHANNEL}" # release channel recorded in version.json
OPT_ACME_STAGING="false"
OPT_ALLOW_IPS=""                # comma separated, empty = any source IP
OPT_RANDOM_PORT="false"
OPT_NO_FIREWALL="false"
OPT_SKIP_DEPS="false"
OPT_ASSUME_YES="false"          # proceed past soft refusals (low RAM, other panel)
OPT_QUIET="false"
OPT_UNINSTALL="false"
OPT_PURGE="false"
OPT_FORCE_OS="false"
OPT_CI_USER=""
OPT_CI_PUBKEY=""
OPT_SKIP_CHECKSUM="false"
OPT_ADMIN_USER="admin"
OPT_API_URL=""
OPT_SOURCE_URL="${VKAI_SOURCE_URL:-}"

# -----------------------------------------------------------------------------
# State filled in while running
# -----------------------------------------------------------------------------
OS_ID=""; OS_VERSION_ID=""; OS_MAJOR=""; OS_LIKE=""; OS_PRETTY=""
OS_FAMILY=""          # debian | rhel | suse
PKG=""                # apt-get | dnf | yum | zypper
ARCH=""; NODE_ARCH=""; GO_ARCH=""
INSTALL_MODE="fresh"  # fresh | upgrade
PANEL_PORT=""
PANEL_ENTRANCE=""
PANEL_DOMAIN=""
PANEL_SCHEME="https"
TLS_MODE=""
ACME_IDENTIFIER=""
ACME_PROFILE=""
ACME_DIRECTORY=""
ACME_STATUS="not requested"
CERT_SOURCE="none"
CERT_FINGERPRINT="(none)"
CERT_EXPIRY="(none)"
DB_NAME="vkai_panel"
DB_USER="vkai"
DB_PASS=""
JWT_SECRET=""
SECRET_KEY=""
AGENT_TOKEN=""
ADMIN_USER=""
ADMIN_PASS=""
ADMIN_EMAIL=""
ADMIN_PASS_CHANGED="false"
GO_BIN=""
NODE_BIN=""
NPM_BIN=""
REDIS_SERVICE=""
PG_SERVICE=""
SERVER_IP=""
NODE_STATUS="not attempted"
NODE_ID="(none)"
NODE_HOSTNAME=""
FIREWALL_TOOL="none"
FIREWALL_PORTS=""
LOGGING_STARTED="false"
MIRROR_CONSOLE="false"
BOOTSTRAPPED_SRC=""

# -----------------------------------------------------------------------------
# Colours and logging
#
# Everything the installer prints also lands in ${INSTALL_LOG}. With --quiet the
# console only receives warnings, errors and the final access table; the full
# transcript still goes to the log file.
# -----------------------------------------------------------------------------
if [[ -t 1 ]]; then
    C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'; C_YELLOW=$'\033[1;33m'
    C_BLUE=$'\033[0;34m'; C_CYAN=$'\033[0;36m'; C_BOLD=$'\033[1m'; C_OFF=$'\033[0m'
else
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_CYAN=""; C_BOLD=""; C_OFF=""
fi

# fd 3 stays attached to the original stdout even after the log redirection.
exec 3>&1

ts() { date '+%Y-%m-%d %H:%M:%S'; }

# console <text> - mirror one line to the real terminal while --quiet has
# swallowed stdout into the log file.
console() {
    [[ "$MIRROR_CONSOLE" == "true" ]] || return 0
    printf '%s\n' "$*" >&3
}

log_info()  { printf '%s[INFO ]%s %s %s\n' "$C_GREEN"  "$C_OFF" "$(ts)" "$*"; }
log_step()  { printf '%s[STEP ]%s %s %s%s%s\n' "$C_BLUE" "$C_OFF" "$(ts)" "$C_BOLD" "$*" "$C_OFF"; }
log_warn()  {
    printf '%s[WARN ]%s %s %s\n' "$C_YELLOW" "$C_OFF" "$(ts)" "$*" >&2
    console "${C_YELLOW}[WARN ]${C_OFF} $*"
}
log_error() {
    printf '%s[ERROR]%s %s %s\n' "$C_RED" "$C_OFF" "$(ts)" "$*" >&2
    console "${C_RED}[ERROR]${C_OFF} $*"
}

die() {
    log_error "$*"
    exit 1
}

# refuse <reason...> - a preflight refusal: polite, explicit, and it names the
# way out instead of leaving the operator guessing.
refuse() {
    printf '\n'
    log_error "Installation refused."
    local line
    for line in "$@"; do
        printf '        %s\n' "$line" >&2
        console "        ${line}"
    done
    printf '\n'
    exit 1
}

on_error() {
    local exit_code=$1 line=$2 cmd=$3
    printf '\n'
    log_error "Installation failed at line ${line} (exit code ${exit_code})."
    log_error "Command: ${cmd}"
    if [[ "$LOGGING_STARTED" == "true" ]]; then
        log_error "Full transcript: ${INSTALL_LOG}"
    fi
    log_error "OS: ${OS_PRETTY:-unknown} | architecture: ${ARCH:-unknown}"
    exit "$exit_code"
}
trap 'on_error $? $LINENO "$BASH_COMMAND"' ERR

start_logging() {
    mkdir -p "$LOG_DIR"
    touch "$INSTALL_LOG"
    chmod 640 "$INSTALL_LOG"
    if [[ "$OPT_QUIET" == "true" ]]; then
        exec >>"$INSTALL_LOG" 2>&1
        MIRROR_CONSOLE="true"
    else
        exec > >(tee -a "$INSTALL_LOG") 2>&1
    fi
    LOGGING_STARTED="true"
    printf '\n===== %s: installing %s v%s =====\n' "$(ts)" "$BRAND_NAME" "$VKAI_VERSION"
}

has() { command -v "$1" >/dev/null 2>&1; }

rand_hex()   { openssl rand -hex "$1"; }
rand_alnum() {
    # openssl writes a fixed amount and exits, so nothing here can be killed by
    # SIGPIPE the way "tr </dev/urandom | head" can be under pipefail.
    openssl rand -base64 96 | LC_ALL=C tr -dc 'A-Za-z0-9' | cut -c1-"$1"
}

banner() {
    printf '%s\n' "$C_CYAN"
    cat <<'ASCII'
 __     __ _  __    _     ___
 \ \   / /| |/ /   / \   |_ _|
  \ \ / / | ' /   / _ \   | |
   \ V /  | . \  / ___ \  | |
    \_/   |_|\_\/_/   \_\|___|
ASCII
    printf '%s' "$C_OFF"
    printf '  %s v%s - %s\n' "$BRAND_NAME" "$VKAI_VERSION" "$BRAND_ORG"
    printf '  Installing into %s | dedicated panel port, 80/443 left to the sites\n\n' "$PANEL_ROOT"
}

usage() {
    cat <<USAGE
${BRAND_NAME} v${VKAI_VERSION} - fully automatic multi OS installer
${BRAND_ORG}

Usage:
  sudo bash deploy/install.sh [options]
  curl -fsSL <url>/install.sh | sudo bash

The installer never prompts. Running it with no options performs a complete,
unattended installation with generated secrets.

Options:
  --port <number>        Public panel port (default ${DEFAULT_PANEL_PORT}). 80 and 443 are refused.
  --entrance <path>      Security entrance, e.g. /vkai_a1b2c3d4. Empty = generated.
  --domain <name>        Domain the panel answers on. Empty = reached by IP.
  --admin-email <email>  Administrator e-mail, also used as the ACME contact.
  --tls-mode <mode>      self-signed (default) | letsencrypt | none.
  --channel <name>       Release channel to follow: stable (default) or beta.
  --acme-staging         Use the Let's Encrypt staging directory (rehearsal, untrusted certs).
  --allow-ip <list>      Restrict panel access to these IPs/CIDRs. Repeatable, comma separated.
  --no-firewall          Do not touch ufw/firewalld.
  --skip-deps            Do not install system packages (they are assumed present).
  --uninstall            Remove services, binaries and configuration.
  --purge                With --uninstall: also drop the database and ${WWW_DIR}.
  -y, --yes              Proceed past soft refusals (low RAM, another panel present).
  -q, --quiet            Console shows warnings, errors and the final table only.

Additional options:
  --random-port          Pick a free random port between 10000 and 60000.
  --admin-user <name>    First administrator account name (default admin).
  --api-url <url>        Hard-code NEXT_PUBLIC_API_URL. Empty = same origin.
  --source-url <url>     Tarball to install from when the script runs on its own.
  --skip-checksum        Skip Go/Node download checksum verification.
  --force-os             Install on an OS release outside the tested matrix.
  --ci-deploy-user <n>   Create an unprivileged account for CI deployments. It may run
                         only ${BIN_DIR}/vkai-deploy, nothing else.
  --ci-deploy-key <key>  Public SSH key authorised for that account.
  -h, --help             Show this help.
  -v, --version          Show the version.

Supported operating systems:
  Ubuntu 20.04 / 22.04 / 24.04       Debian 11 / 12
  CentOS Stream 8 / 9                RHEL 8 / 9
  Rocky Linux 8 / 9                  AlmaLinux 8 / 9
  Fedora 38+                         openSUSE Leap 15.x
  Amazon Linux 2023
  Architectures: x86_64 (amd64) and aarch64 (arm64)
USAGE
}

# add_allow_ip <list> - accumulates --allow-ip values, comma separated.
add_allow_ip() {
    local value="${1//[[:space:]]/}"
    [[ -n "$value" ]] || return 0
    if [[ -z "$OPT_ALLOW_IPS" ]]; then
        OPT_ALLOW_IPS="$value"
    else
        OPT_ALLOW_IPS="${OPT_ALLOW_IPS},${value}"
    fi
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --port)          [[ $# -ge 2 ]] || die "--port needs a value"; OPT_PORT="$2"; shift 2 ;;
            --port=*)        OPT_PORT="${1#*=}"; shift ;;
            --random-port)   OPT_RANDOM_PORT="true"; shift ;;
            --entrance)      [[ $# -ge 2 ]] || die "--entrance needs a value"; OPT_ENTRANCE="$2"; shift 2 ;;
            --entrance=*)    OPT_ENTRANCE="${1#*=}"; shift ;;
            --domain)        [[ $# -ge 2 ]] || die "--domain needs a value"; OPT_DOMAIN="$2"; shift 2 ;;
            --domain=*)      OPT_DOMAIN="${1#*=}"; shift ;;
            --admin-email)   [[ $# -ge 2 ]] || die "--admin-email needs a value"; OPT_ADMIN_EMAIL="$2"; shift 2 ;;
            --admin-email=*) OPT_ADMIN_EMAIL="${1#*=}"; shift ;;
            --tls-mode)      [[ $# -ge 2 ]] || die "--tls-mode needs a value"; OPT_TLS_MODE="$2"; shift 2 ;;
            --tls-mode=*)    OPT_TLS_MODE="${1#*=}"; shift ;;
            --channel)       [[ $# -ge 2 ]] || die "--channel needs a value"; OPT_CHANNEL="$2"; shift 2 ;;
            --channel=*)     OPT_CHANNEL="${1#*=}"; shift ;;
            --acme-staging)  OPT_ACME_STAGING="true"; shift ;;
            --allow-ip)      [[ $# -ge 2 ]] || die "--allow-ip needs a value"; add_allow_ip "$2"; shift 2 ;;
            --allow-ip=*)    add_allow_ip "${1#*=}"; shift ;;
            --admin-user)    [[ $# -ge 2 ]] || die "--admin-user needs a value"; OPT_ADMIN_USER="$2"; shift 2 ;;
            --admin-user=*)  OPT_ADMIN_USER="${1#*=}"; shift ;;
            --api-url)       [[ $# -ge 2 ]] || die "--api-url needs a value"; OPT_API_URL="$2"; shift 2 ;;
            --api-url=*)     OPT_API_URL="${1#*=}"; shift ;;
            --source-url)    [[ $# -ge 2 ]] || die "--source-url needs a value"; OPT_SOURCE_URL="$2"; shift 2 ;;
            --source-url=*)  OPT_SOURCE_URL="${1#*=}"; shift ;;
            --no-firewall)   OPT_NO_FIREWALL="true"; shift ;;
            --skip-deps)     OPT_SKIP_DEPS="true"; shift ;;
            --skip-checksum) OPT_SKIP_CHECKSUM="true"; shift ;;
            --force-os)      OPT_FORCE_OS="true"; shift ;;
            --ci-deploy-user)   [[ $# -ge 2 ]] || die "--ci-deploy-user needs a value"; OPT_CI_USER="$2"; shift 2 ;;
            --ci-deploy-user=*) OPT_CI_USER="${1#*=}"; shift ;;
            --ci-deploy-key)    [[ $# -ge 2 ]] || die "--ci-deploy-key needs a value"; OPT_CI_PUBKEY="$2"; shift 2 ;;
            --ci-deploy-key=*)  OPT_CI_PUBKEY="${1#*=}"; shift ;;
            -y|--yes)        OPT_ASSUME_YES="true"; shift ;;
            -q|--quiet)      OPT_QUIET="true"; shift ;;
            --uninstall)     OPT_UNINSTALL="true"; shift ;;
            --purge)         OPT_PURGE="true"; shift ;;
            -h|--help)       usage; exit 0 ;;
            -v|--version)    printf '%s v%s\n' "$BRAND_NAME" "$VKAI_VERSION"; exit 0 ;;
            *)               usage >&2; die "Unknown option: $1" ;;
        esac
    done

    if [[ -n "$OPT_PORT" ]]; then
        [[ "$OPT_PORT" =~ ^[0-9]+$ ]] || die "--port must be a number: '$OPT_PORT'"
        (( OPT_PORT >= 1 && OPT_PORT <= 65535 )) || die "--port out of range 1-65535: $OPT_PORT"
        if [[ "$OPT_PORT" == "80" || "$OPT_PORT" == "443" ]]; then
            die "Ports 80/443 belong to the customer websites. Pick another port, for example ${DEFAULT_PANEL_PORT}."
        fi
    fi
    if [[ -n "$OPT_ENTRANCE" ]]; then
        [[ "$OPT_ENTRANCE" == /* ]] || OPT_ENTRANCE="/${OPT_ENTRANCE}"
        [[ "$OPT_ENTRANCE" =~ ^/[A-Za-z0-9_-]{4,64}$ ]] ||
            die "--entrance accepts letters, digits, '-' and '_', 4-64 characters. Example: /vkai_a1b2c3d4"
    fi
    if [[ -n "$OPT_DOMAIN" ]]; then
        OPT_DOMAIN="${OPT_DOMAIN,,}"
        [[ "$OPT_DOMAIN" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$ ]] ||
            die "--domain must be a bare host name without scheme or port: '${OPT_DOMAIN}'"
    fi
    if [[ -n "$OPT_ADMIN_EMAIL" ]]; then
        [[ "$OPT_ADMIN_EMAIL" =~ ^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$ ]] ||
            die "--admin-email is not a valid address: '${OPT_ADMIN_EMAIL}'"
    fi
    case "${OPT_CHANNEL,,}" in
        stable|beta) OPT_CHANNEL="${OPT_CHANNEL,,}" ;;
        *) die "--channel accepts stable or beta: '${OPT_CHANNEL}'" ;;
    esac
    case "$OPT_TLS_MODE" in
        self-signed|selfsigned|self) OPT_TLS_MODE="self-signed" ;;
        letsencrypt|le|acme)         OPT_TLS_MODE="letsencrypt" ;;
        none|off|disabled)           OPT_TLS_MODE="none" ;;
        *) die "--tls-mode accepts self-signed, letsencrypt or none: '${OPT_TLS_MODE}'" ;;
    esac
    if [[ -n "$OPT_ALLOW_IPS" ]]; then
        local entry
        IFS=',' read -r -a __allow <<<"$OPT_ALLOW_IPS"
        for entry in "${__allow[@]}"; do
            [[ -n "$entry" ]] || continue
            [[ "$entry" =~ ^[0-9a-fA-F.:]+(/[0-9]{1,3})?$ ]] ||
                die "--allow-ip accepts IP addresses or CIDR ranges: '${entry}'"
        done
        unset __allow
    fi
    [[ "$OPT_ADMIN_USER" =~ ^[A-Za-z0-9_.-]{3,32}$ ]] ||
        die "--admin-user accepts letters, digits, '.', '-', '_', 3-32 characters."
    if [[ "$OPT_PURGE" == "true" && "$OPT_UNINSTALL" != "true" ]]; then
        die "--purge only means something together with --uninstall."
    fi
    if [[ "$OPT_ACME_STAGING" == "true" && "$OPT_TLS_MODE" != "letsencrypt" ]]; then
        log_warn "--acme-staging has no effect with --tls-mode ${OPT_TLS_MODE}."
    fi
}

# =============================================================================
# 0. Sources
#
# "curl ... | bash" leaves the script with no source tree next to it. Download
# one instead of failing, so the documented one-liner really is one command.
# =============================================================================
sources_present() {
    local dir="$1"
    [[ -n "$dir" && -d "${dir}/core" && -d "${dir}/panel" && -d "${dir}/agent" ]]
}

bootstrap_sources() {
    if sources_present "$SRC_DIR"; then
        log_info "Source tree: ${SRC_DIR}"
        return 0
    fi

    local url="${OPT_SOURCE_URL:-$DEFAULT_SOURCE_URL}"
    log_step "Downloading the source tree (the script is running on its own)"
    has curl || die "'curl' is required to download the sources. Install it and run again."
    has tar  || die "'tar' is required to unpack the sources. Install it and run again."

    local work
    work="$(mktemp -d /tmp/vkai-src.XXXXXX)"
    BOOTSTRAPPED_SRC="$work"
    curl -fsSL --retry 3 --connect-timeout 20 -o "${work}/source.tar.gz" "$url" ||
        die "Could not download the sources from ${url}
Pass --source-url <url> or run the installer from an unpacked source tree."
    tar -C "$work" -xzf "${work}/source.tar.gz" ||
        die "Could not unpack ${work}/source.tar.gz"
    rm -f "${work}/source.tar.gz"

    local candidate
    for candidate in "$work" "$work"/*/; do
        candidate="${candidate%/}"
        if sources_present "$candidate"; then
            SRC_DIR="$candidate"
            log_info "Source tree: ${SRC_DIR} (downloaded)"
            return 0
        fi
    done
    die "The downloaded archive does not contain core/, panel/ and agent/. Wrong --source-url?"
}

cleanup_bootstrap() {
    [[ -n "$BOOTSTRAPPED_SRC" && -d "$BOOTSTRAPPED_SRC" ]] || return 0
    rm -rf "$BOOTSTRAPPED_SRC"
}

# =============================================================================
# 1. Operating system and architecture
# =============================================================================
detect_arch() {
    local machine
    machine="$(uname -m)"
    case "$machine" in
        x86_64|amd64)   ARCH="amd64"; GO_ARCH="amd64"; NODE_ARCH="x64" ;;
        aarch64|arm64)  ARCH="arm64"; GO_ARCH="arm64"; NODE_ARCH="arm64" ;;
        *)
            refuse "Architecture '${machine}' is not supported." \
                   "${BRAND_NAME} runs on x86_64 (amd64) and aarch64 (arm64) only."
            ;;
    esac
}

detect_os() {
    if [[ -r /etc/os-release ]]; then
        # shellcheck disable=SC1091
        . /etc/os-release
        OS_ID="${ID:-}"
        OS_VERSION_ID="${VERSION_ID:-}"
        OS_LIKE="${ID_LIKE:-}"
        OS_PRETTY="${PRETTY_NAME:-${NAME:-} ${VERSION_ID:-}}"
    elif [[ -r /etc/redhat-release ]]; then
        # Only very old RHEL derivatives lack /etc/os-release.
        local rel
        rel="$(cat /etc/redhat-release)"
        OS_PRETTY="$rel"
        OS_VERSION_ID="$(grep -oE '[0-9]+(\.[0-9]+)?' <<<"$rel" | head -n1)"
        case "${rel,,}" in
            *centos*)     OS_ID="centos" ;;
            *rocky*)      OS_ID="rocky" ;;
            *alma*)       OS_ID="almalinux" ;;
            *fedora*)     OS_ID="fedora" ;;
            *red\ hat*)   OS_ID="rhel" ;;
            *)            OS_ID="" ;;
        esac
        OS_LIKE="rhel fedora"
    else
        refuse "Neither /etc/os-release nor /etc/redhat-release is readable." \
               "The installer refuses to guess which distribution this is." \
               "Install manually following deploy/README.md."
    fi

    [[ -n "$OS_ID" ]] || die "No operating system ID found in /etc/os-release."
    OS_MAJOR="${OS_VERSION_ID%%.*}"
    [[ -n "$OS_MAJOR" ]] || OS_MAJOR="0"
}

# Resolve the OS family and check it against the tested matrix.
resolve_os_family() {
    local supported="false"

    case "$OS_ID" in
        ubuntu)
            OS_FAMILY="debian"
            case "$OS_VERSION_ID" in 20.04|22.04|24.04) supported="true" ;; esac
            ;;
        debian)
            OS_FAMILY="debian"
            case "$OS_MAJOR" in 11|12) supported="true" ;; esac
            ;;
        linuxmint|pop|elementary|raspbian)
            OS_FAMILY="debian"
            ;;
        centos)
            OS_FAMILY="rhel"
            case "$OS_MAJOR" in 8|9) supported="true" ;; esac
            ;;
        rhel|redhat)
            OS_FAMILY="rhel"
            case "$OS_MAJOR" in 8|9) supported="true" ;; esac
            ;;
        rocky|almalinux|ol|oracle)
            OS_FAMILY="rhel"
            case "$OS_MAJOR" in 8|9) supported="true" ;; esac
            ;;
        fedora)
            OS_FAMILY="rhel"
            if [[ "$OS_MAJOR" =~ ^[0-9]+$ ]] && (( OS_MAJOR >= 38 )); then supported="true"; fi
            ;;
        amzn)
            OS_FAMILY="rhel"
            [[ "$OS_VERSION_ID" == "2023" ]] && supported="true"
            if [[ "$OS_VERSION_ID" == "2" ]]; then
                refuse "Amazon Linux 2 is not supported (glibc and systemd are too old)." \
                       "Use Amazon Linux 2023."
            fi
            ;;
        opensuse-leap|opensuse|sles|sled)
            OS_FAMILY="suse"
            case "$OS_MAJOR" in 15) supported="true" ;; esac
            ;;
        *)
            # Derive from ID_LIKE, but never silently.
            case " ${OS_LIKE} " in
                *debian*|*ubuntu*)          OS_FAMILY="debian" ;;
                *rhel*|*fedora*|*centos*)   OS_FAMILY="rhel" ;;
                *suse*)                     OS_FAMILY="suse" ;;
                *) OS_FAMILY="" ;;
            esac
            ;;
    esac

    if [[ -z "$OS_FAMILY" ]]; then
        refuse "'${OS_PRETTY}' (ID=${OS_ID}) is not a supported distribution," \
               "and no family could be derived from ID_LIKE." \
               "" \
               "Supported: Ubuntu 20.04/22.04/24.04, Debian 11/12, CentOS Stream 8/9," \
               "RHEL 8/9, Rocky 8/9, AlmaLinux 8/9, Fedora 38+, openSUSE Leap 15.x," \
               "Amazon Linux 2023."
    fi

    if [[ "$supported" != "true" ]]; then
        log_warn "'${OS_PRETTY}' is outside the tested matrix (family '${OS_FAMILY}' assumed)."
        if [[ "$OPT_FORCE_OS" != "true" ]]; then
            refuse "'${OS_PRETTY}' has not been tested with ${BRAND_NAME}." \
                   "Run again with --force-os to install anyway, without any guarantee."
        fi
        log_warn "--force-os given: continuing without any guarantee."
    fi

    log_info "Operating system: ${OS_PRETTY} (ID=${OS_ID}, family=${OS_FAMILY}, arch=${ARCH})"
}

# =============================================================================
# 2. Package manager abstraction
# =============================================================================
detect_pkg_manager() {
    case "$OS_FAMILY" in
        debian) has apt-get || die "apt-get not found on a Debian family system."; PKG="apt-get" ;;
        rhel)
            if has dnf; then PKG="dnf"
            elif has yum; then PKG="yum"
            else die "Neither dnf nor yum found on a RHEL family system."
            fi
            ;;
        suse)  has zypper || die "zypper not found on a SUSE family system."; PKG="zypper" ;;
        *)     die "Unknown operating system family: '${OS_FAMILY}'" ;;
    esac
    log_info "Package manager: ${PKG}"
}

# apt-get is regularly blocked by unattended-upgrades holding the dpkg lock.
# DPkg::Lock::Timeout waits instead of failing immediately.
apt_run() {
    DEBIAN_FRONTEND=noninteractive apt-get \
        -o DPkg::Lock::Timeout=600 \
        -o Dpkg::Options::=--force-confdef \
        -o Dpkg::Options::=--force-confold \
        "$@"
}

pkg_update() {
    case "$PKG" in
        apt-get) apt_run update -qq ;;
        dnf)     dnf -y makecache ;;
        yum)     yum -y makecache ;;
        zypper)  zypper --non-interactive --gpg-auto-import-keys refresh ;;
    esac
}

# pkg_install <package...> - install, fail loudly.
pkg_install() {
    [[ $# -gt 0 ]] || return 0
    case "$PKG" in
        apt-get) apt_run install -y "$@" ;;
        dnf)     dnf install -y "$@" ;;
        yum)     yum install -y "$@" ;;
        zypper)  zypper --non-interactive install --auto-agree-with-licenses "$@" ;;
    esac
}

# pkg_install_optional <package...> - install one by one, skip what is missing.
pkg_install_optional() {
    local p
    for p in "$@"; do
        [[ -n "$p" ]] || continue
        if ! pkg_install "$p" >/dev/null 2>&1; then
            log_warn "Package '${p}' is not in the repositories, skipped."
        fi
    done
}

pkg_installed() {
    case "$PKG" in
        apt-get) dpkg-query -W -f='${Status}' "$1" 2>/dev/null | grep -q "ok installed" ;;
        dnf|yum) rpm -q "$1" >/dev/null 2>&1 ;;
        zypper)  rpm -q "$1" >/dev/null 2>&1 ;;
    esac
}

# pkg_enable_service <name> - enable and start, tolerating the different unit
# names between families (redis-server vs redis vs redis6).
pkg_enable_service() {
    local svc="$1" i

    # A unit file the package manager wrote seconds ago is not visible to
    # systemctl until systemd reloads. Checking without reloading makes the
    # installer announce that a service does not exist on the very machine where
    # it has just been installed successfully - which is what happened with
    # postgresql.service on Ubuntu 24.04.
    systemctl daemon-reload 2>/dev/null || true

    for i in 1 2 3 4 5; do
        if systemctl list-unit-files "${svc}.service" --no-legend 2>/dev/null | grep -q .; then
            break
        fi
        if (( i == 5 )); then
            log_warn "Unit '${svc}.service' not found, skipped."
            return 1
        fi
        sleep 1
        systemctl daemon-reload 2>/dev/null || true
    done

    systemctl enable "$svc" >/dev/null 2>&1 || log_warn "Could not enable ${svc}"

    # Restarting a service whose dependencies are still settling can lose a race
    # that a second attempt wins. Only the second failure is a real failure.
    if ! systemctl restart "$svc" 2>/dev/null; then
        sleep 3
        if ! systemctl restart "$svc"; then
            log_error "Could not start ${svc}. Inspect: journalctl -u ${svc} -n 50"
            return 1
        fi
    fi
    return 0
}

# Package names differ between apt / dnf / zypper. Return the package list
# (space separated) for one generic name.
pkg_names_for() {
    local generic="$1"
    case "$generic" in
        base)
            case "$OS_FAMILY" in
                debian) echo "curl wget git tar unzip xz-utils openssl ca-certificates gnupg lsb-release jq rsync lsof procps" ;;
                rhel)   echo "curl wget git tar unzip xz openssl ca-certificates gnupg2 jq rsync lsof procps-ng" ;;
                suse)   echo "curl wget git-core tar unzip xz openssl ca-certificates gpg2 jq rsync lsof procps" ;;
            esac
            ;;
        buildtools)
            case "$OS_FAMILY" in
                debian) echo "build-essential pkg-config" ;;
                rhel)   echo "gcc gcc-c++ make pkgconf-pkg-config" ;;
                suse)   echo "gcc gcc-c++ make pkg-config" ;;
            esac
            ;;
        postgresql)
            case "$OS_FAMILY" in
                debian) echo "postgresql postgresql-contrib postgresql-client" ;;
                rhel)
                    if [[ "$OS_ID" == "amzn" ]]; then
                        echo "postgresql15 postgresql15-server postgresql15-contrib"
                    else
                        echo "postgresql postgresql-server postgresql-contrib"
                    fi
                    ;;
                suse)   echo "postgresql postgresql-server postgresql-contrib" ;;
            esac
            ;;
        redis)
            case "$OS_FAMILY" in
                debian) echo "redis-server" ;;
                rhel)   [[ "$OS_ID" == "amzn" ]] && echo "redis6" || echo "redis" ;;
                suse)   echo "redis" ;;
            esac
            ;;
        nginx)      echo "nginx" ;;
        cron)
            case "$OS_FAMILY" in
                debian) echo "cron logrotate" ;;
                rhel)   echo "cronie logrotate" ;;
                suse)   echo "cron logrotate" ;;
            esac
            ;;
        selinux)
            case "$OS_FAMILY" in
                rhel) echo "policycoreutils-python-utils" ;;
                *)    echo "" ;;
            esac
            ;;
    esac
}

# =============================================================================
# 3. Preflight
#
# Every check either passes or refuses with a sentence the operator can act on.
# Nothing here waits for a keystroke: --yes is the only way past a soft refusal.
# =============================================================================
preflight_root() {
    if [[ "${EUID}" -ne 0 ]]; then
        refuse "${BRAND_NAME} has to be installed as root." \
               "It creates a system user, writes /etc/systemd/system and configures nginx." \
               "Run again with: sudo ${SELF_CMD}"
    fi
}

preflight_systemd() {
    has systemctl || refuse "systemd was not found." \
                            "${BRAND_NAME} ships systemd units and cannot run without it."
    [[ -d /run/systemd/system ]] || log_warn "systemd is not PID 1 (container?). systemctl calls may fail."
}

preflight_resources() {
    local ram_mb disk_mb target="/"
    ram_mb="$(awk '/MemTotal/ {printf "%d", $2/1024}' /proc/meminfo)"
    [[ -d "$PANEL_ROOT" ]] && target="$PANEL_ROOT"
    disk_mb="$(df -Pm "$target" | awk 'NR==2 {print $4}')"

    log_info "RAM: ${ram_mb} MB | free disk (${target}): ${disk_mb} MB"

    if (( ram_mb < MIN_RAM_MB )); then
        if [[ "$OPT_ASSUME_YES" != "true" ]]; then
            refuse "This machine has ${ram_mb} MB of RAM, below the ${MIN_RAM_MB} MB minimum." \
                   "The Next.js build step is the part that runs out of memory first." \
                   "Add memory or swap, or run again with --yes to try anyway."
        fi
        log_warn "RAM ${ram_mb} MB is below the ${MIN_RAM_MB} MB minimum; --yes given, continuing."
    fi
    if (( disk_mb < MIN_DISK_MB )); then
        refuse "Only ${disk_mb} MB of free disk space on ${target}." \
               "${BRAND_NAME} needs at least ${MIN_DISK_MB} MB for the toolchains and the build." \
               "Free some space and run again."
    fi
}

preflight_other_panels() {
    local found=()
    [[ -d /usr/local/cpanel ]]      && found+=("cPanel (/usr/local/cpanel)")
    [[ -d /www/server/panel ]]      && found+=("aaPanel / BT Panel (/www/server/panel)")
    [[ -d /usr/local/psa ]]         && found+=("Plesk (/usr/local/psa)")
    [[ -d /usr/local/directadmin ]] && found+=("DirectAdmin (/usr/local/directadmin)")
    [[ -d /usr/local/vesta ]]       && found+=("VestaCP (/usr/local/vesta)")
    [[ -d /usr/local/hestia ]]      && found+=("HestiaCP (/usr/local/hestia)")

    (( ${#found[@]} > 0 )) || return 0

    if [[ "$OPT_ASSUME_YES" != "true" ]]; then
        local lines=("Another control panel is already installed on this machine:")
        local p
        for p in "${found[@]}"; do lines+=("  - ${p}"); done
        lines+=("" \
                "Two panels on one machine fight over nginx, PHP, ports and certificates." \
                "Use a clean machine, or run again with --yes to install side by side.")
        refuse "${lines[@]}"
    fi

    log_warn "Another control panel is present; --yes given, continuing:"
    local p
    for p in "${found[@]}"; do log_warn "  - ${p}"; done
}

port_in_use() {
    local port="$1"
    if has ss; then
        ss -H -ltn "sport = :${port}" 2>/dev/null | grep -q . && return 0
    elif has netstat; then
        netstat -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${port}\$" && return 0
    elif has lsof; then
        lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1 && return 0
    fi
    return 1
}

port_owner() {
    local port="$1"
    if has ss; then
        ss -H -ltnp "sport = :${port}" 2>/dev/null | grep -oP 'users:\(\("\K[^"]+' | head -n1
    fi
}

preflight_ports() {
    local p owner
    for p in 80 443; do
        if port_in_use "$p"; then
            owner="$(port_owner "$p" || true)"
            log_info "Port ${p} is held by '${owner:-an unknown process}'. That port belongs to the customer websites; the panel never binds it."
        fi
    done

    if port_in_use "$PANEL_PORT"; then
        owner="$(port_owner "$PANEL_PORT" || true)"
        if [[ "$owner" == "nginx" && -f /etc/nginx/conf.d/vkai-panel.conf ]]; then
            log_info "Port ${PANEL_PORT} is held by the ${BRAND_NAME} nginx server (reinstall)."
        else
            refuse "Panel port ${PANEL_PORT} is already taken by '${owner:-an unknown process}'." \
                   "Choose a free port: ${SELF_CMD} --port <other_port>" \
                   "Or let the installer pick one: ${SELF_CMD} --random-port"
        fi
    fi

    for p in "$API_BIND_PORT" "$UI_BIND_PORT"; do
        if port_in_use "$p" && ! systemctl is-active --quiet vkai-api vkai-ui 2>/dev/null; then
            log_warn "Internal port ${p} is in use. If it is not ${BRAND_NAME}, the service will fail to start."
        fi
    done

    # The agent on this machine listens on ${AGENT_PORT} for the mutual-TLS
    # control channel. On a reinstall that port is held by the agent this
    # installer is about to restart, which is not a conflict - hence the check
    # for the running unit.
    if port_in_use "$AGENT_PORT" && ! systemctl is-active --quiet vkai-agent 2>/dev/null; then
        log_warn "Port ${AGENT_PORT} is in use by '$(port_owner "$AGENT_PORT" || echo 'an unknown process')'."
        log_warn "That is the agent control channel. This machine will register as a node but its agent"
        log_warn "will not start until the port is free, or VKAI_AGENT_PORT names another one."
    fi
}

detect_install_mode() {
    if [[ -f "$ENV_FILE" || -x "${CORE_DIR}/bin/vkai-api" ]]; then
        INSTALL_MODE="upgrade"
        log_info "Existing installation detected: upgrading in place (database, certificates and entrance are preserved)."
    else
        INSTALL_MODE="fresh"
        log_info "No existing installation found: performing a fresh install."
    fi
}

# =============================================================================
# 4. System dependencies
# =============================================================================
enable_extra_repos() {
    case "$OS_FAMILY" in
        rhel)
            # EPEL does not apply to Fedora (already included) or Amazon Linux 2023.
            case "$OS_ID" in
                fedora|amzn) : ;;
                *)
                    if ! rpm -q epel-release >/dev/null 2>&1; then
                        log_info "Enabling the EPEL repository for RHEL ${OS_MAJOR}..."
                        pkg_install epel-release >/dev/null 2>&1 ||
                            pkg_install "https://dl.fedoraproject.org/pub/epel/epel-release-latest-${OS_MAJOR}.noarch.rpm" >/dev/null 2>&1 ||
                            log_warn "EPEL could not be enabled. A few optional packages (jq, redis) may be missing."
                    fi
                    # CodeReady Builder / PowerTools carry the -devel packages EPEL needs.
                    if has dnf; then
                        case "$OS_MAJOR" in
                            8) dnf config-manager --set-enabled powertools >/dev/null 2>&1 ||
                               dnf config-manager --set-enabled PowerTools >/dev/null 2>&1 || true ;;
                            9|10) dnf config-manager --set-enabled crb >/dev/null 2>&1 || true ;;
                        esac
                    fi
                    ;;
            esac
            ;;
        suse)
            zypper --non-interactive refresh >/dev/null 2>&1 || true
            ;;
    esac
}

install_dependencies() {
    if [[ "$OPT_SKIP_DEPS" == "true" ]]; then
        log_warn "--skip-deps: not installing any system package."
        return 0
    fi

    log_step "Refreshing the package index"
    pkg_update

    enable_extra_repos

    log_step "Installing system dependencies"
    local group
    for group in base buildtools postgresql redis nginx cron; do
        local raw=() names=() p
        read -r -a raw <<<"$(pkg_names_for "$group")"
        for p in "${raw[@]}"; do
            # RHEL 9 and Amazon Linux 2023 ship curl-minimal, and "dnf install
            # curl" there is a package conflict rather than an upgrade.
            if [[ "$p" == "curl" ]] && has curl; then
                continue
            fi
            names+=("$p")
        done
        (( ${#names[@]} > 0 )) || continue
        log_info "Group '${group}': ${names[*]}"
        if ! pkg_install "${names[@]}"; then
            log_warn "Installing group '${group}' at once failed, retrying package by package..."
            pkg_install_optional "${names[@]}"
        fi
    done

    # SELinux tooling only means something on the RHEL family.
    local selinux_pkgs
    selinux_pkgs="$(pkg_names_for selinux)"
    if [[ -n "$selinux_pkgs" ]]; then
        # shellcheck disable=SC2086  # deliberate word splitting: several package names
        pkg_install_optional $selinux_pkgs
    fi

    has openssl || die "'openssl' is missing after installing dependencies - secrets cannot be generated."
    has curl    || die "'curl' is missing after installing dependencies."
    log_info "System dependencies installed."
}

# =============================================================================
# 5. Go and Node.js, per architecture, with checksum verification
# =============================================================================
verify_sha256() {
    local file="$1" expected="$2" actual
    if [[ "$OPT_SKIP_CHECKSUM" == "true" ]]; then
        log_warn "--skip-checksum: not verifying $(basename "$file")."
        return 0
    fi
    [[ -n "$expected" ]] || return 1
    actual="$(sha256sum "$file" | awk '{print $1}')"
    if [[ "$actual" != "$expected" ]]; then
        rm -f "$file"
        die "Checksum mismatch for $(basename "$file").
  Expected: ${expected}
  Actual  : ${actual}
The download is corrupt or was tampered with. The file has been deleted."
    fi
    log_info "Checksum verified: $(basename "$file")"
}

go_needs_install() {
    local v major minor
    if ! has go; then return 0; fi
    v="$(go version 2>/dev/null | awk '{print $3}')"   # e.g. go1.22.5
    v="${v#go}"
    major="${v%%.*}"; minor="${v#*.}"; minor="${minor%%.*}"
    [[ "$major" =~ ^[0-9]+$ ]] || return 0
    [[ "$minor" =~ ^[0-9]+$ ]] || return 0
    (( major > 1 )) && return 1
    (( major == 1 && minor >= GO_MIN_MINOR )) && return 1
    return 0
}

install_golang() {
    GO_BIN="$(command -v go || true)"
    if ! go_needs_install; then
        log_info "Go is already recent enough: $(go version)"
        GO_BIN="$(command -v go)"
        return 0
    fi

    local tarball url sha_url expected
    tarball="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
    url="https://go.dev/dl/${tarball}"
    sha_url="https://dl.google.com/go/${tarball}.sha256"

    log_step "Installing Go ${GO_VERSION} (${GO_ARCH})"
    mkdir -p "$TMP_DIR"
    curl -fsSL --retry 3 --connect-timeout 20 -o "${TMP_DIR}/${tarball}" "$url" ||
        die "Could not download Go from ${url}"

    expected="$(curl -fsSL --retry 3 --connect-timeout 20 "$sha_url" 2>/dev/null | tr -d ' \n' || true)"
    if [[ -z "$expected" ]]; then
        # Fallback: the hashes are also in the go.dev JSON index. That JSON may
        # or may not contain whitespace, so strip all of it before splitting.
        expected="$(curl -fsSL --retry 3 "https://go.dev/dl/?mode=json&include=all" 2>/dev/null |
            tr -d ' \t' | sed 's/[,{}]/\n/g' |
            grep -A8 "\"filename\":\"${tarball}\"" |
            grep -m1 '"sha256":' |
            sed 's/.*"sha256":"\([a-f0-9]*\)".*/\1/' || true)"
    fi
    if [[ -z "$expected" && "$OPT_SKIP_CHECKSUM" != "true" ]]; then
        die "Could not fetch the official checksum for ${tarball}.
Check network access, or run again with --skip-checksum if you accept the risk."
    fi
    verify_sha256 "${TMP_DIR}/${tarball}" "$expected"

    rm -rf /usr/local/go
    tar -C /usr/local -xzf "${TMP_DIR}/${tarball}"
    rm -f "${TMP_DIR}/${tarball}"

    cat >/etc/profile.d/vkai-golang.sh <<'PROFILE'
# VKAI Panel: Go toolchain
export PATH="$PATH:/usr/local/go/bin"
PROFILE
    chmod 644 /etc/profile.d/vkai-golang.sh
    export PATH="$PATH:/usr/local/go/bin"
    GO_BIN="/usr/local/go/bin/go"
    ln -sf /usr/local/go/bin/go /usr/local/bin/go
    ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

    log_info "Installed: $("$GO_BIN" version)"
}

node_needs_install() {
    local v major
    if ! has node; then return 0; fi
    v="$(node --version 2>/dev/null)"    # e.g. v20.18.1
    v="${v#v}"; major="${v%%.*}"
    [[ "$major" =~ ^[0-9]+$ ]] || return 0
    (( major >= NODE_MIN_MAJOR )) && return 1
    return 0
}

install_nodejs() {
    if ! node_needs_install; then
        log_info "Node.js is already recent enough: $(node --version)"
    else
        local tarball url expected
        tarball="node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz"
        url="https://nodejs.org/dist/v${NODE_VERSION}/${tarball}"

        log_step "Installing Node.js ${NODE_VERSION} (${NODE_ARCH})"
        mkdir -p "$TMP_DIR"
        curl -fsSL --retry 3 --connect-timeout 20 -o "${TMP_DIR}/${tarball}" "$url" ||
            die "Could not download Node.js from ${url}"

        expected="$(curl -fsSL --retry 3 --connect-timeout 20 \
            "https://nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt" 2>/dev/null |
            awk -v f="$tarball" '$2 == f {print $1}' || true)"
        if [[ -z "$expected" && "$OPT_SKIP_CHECKSUM" != "true" ]]; then
            die "Could not fetch the Node.js SHASUMS256.txt. Run again with --skip-checksum if you accept the risk."
        fi
        verify_sha256 "${TMP_DIR}/${tarball}" "$expected"

        rm -rf "/usr/local/lib/nodejs-v${NODE_VERSION}"
        mkdir -p /usr/local/lib
        tar -C /usr/local/lib -xJf "${TMP_DIR}/${tarball}"
        mv "/usr/local/lib/node-v${NODE_VERSION}-linux-${NODE_ARCH}" "/usr/local/lib/nodejs-v${NODE_VERSION}"
        rm -f "${TMP_DIR}/${tarball}"

        ln -sfn "/usr/local/lib/nodejs-v${NODE_VERSION}" /usr/local/lib/nodejs
        local b
        for b in node npm npx; do
            ln -sf "/usr/local/lib/nodejs/bin/${b}" "/usr/local/bin/${b}"
        done
        export PATH="/usr/local/lib/nodejs/bin:$PATH"
        log_info "Installed: $(node --version)"
    fi

    NODE_BIN="$(command -v node)"
    NPM_BIN="$(command -v npm)"
    [[ -n "$NODE_BIN" ]] || die "'node' not found after installation."
    [[ -n "$NPM_BIN"  ]] || die "'npm' not found after installation."

    # vkai-ui.service executes /usr/bin/node - make sure that path exists.
    if [[ ! -x /usr/bin/node ]]; then
        ln -sf "$NODE_BIN" /usr/bin/node
        log_info "Linked /usr/bin/node -> ${NODE_BIN}"
    fi
}

# =============================================================================
# 6. Service account and directory tree
# =============================================================================
setup_user() {
    if getent group "$VKAI_GROUP" >/dev/null; then
        log_info "Group '${VKAI_GROUP}' already exists."
    else
        groupadd --system "$VKAI_GROUP"
        log_info "Created system group '${VKAI_GROUP}'."
    fi

    if id "$VKAI_USER" >/dev/null 2>&1; then
        log_info "User '${VKAI_USER}' already exists."
    else
        useradd --system --gid "$VKAI_GROUP" --home-dir "$PANEL_ROOT" \
                --shell /usr/sbin/nologin --comment "VKAI Panel service account" "$VKAI_USER" 2>/dev/null ||
        useradd --system --gid "$VKAI_GROUP" --home-dir "$PANEL_ROOT" \
                --shell /sbin/nologin --comment "VKAI Panel service account" "$VKAI_USER"
        log_info "Created system user '${VKAI_USER}'."
    fi
}

setup_directories() {
    log_step "Creating the ${PANEL_ROOT} directory tree"

    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$PANEL_ROOT"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"

    # www/ has to be 755: nginx runs as its own user and must traverse into the
    # site directories.
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 755 "$WWW_DIR" "$WWW_DOMAINS_DIR" "$WWW_DEFAULT_DIR"
    # Backups contain database dumps - not world readable.
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$WWW_BACKUP_DIR"

    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$LOG_DIR" "$LOG_SITES_DIR"
    # etc/ holds .env and the secrets -> 700.
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 700 "$ETC_DIR"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 700 "$SSL_DIR"
    # The agent's private key and certificate. Owned by root, not by the panel
    # account: the agent runs as root and the panel has no business reading the
    # key of the thing it authenticates. It has to exist before the unit starts
    # because vkai-agent.service lists it under ReadWritePaths.
    install -d -o root -g root -m 700 "${SSL_DIR}/agent"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$TMP_DIR"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$RELEASES_DIR"

    # The panel root must be traversable by nginx so it can read www/.
    chmod 751 "$PANEL_ROOT"

    [[ -f "$INSTALL_LOG" ]] && chown "$VKAI_USER:$VKAI_GROUP" "$INSTALL_LOG"

    setup_acme_webroot

    if [[ ! -f "${WWW_DEFAULT_DIR}/index.html" ]]; then
        cat >"${WWW_DEFAULT_DIR}/index.html" <<HTML
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>${BRAND_NAME}</title>
<style>
 body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;margin:0;
      display:grid;place-items:center;min-height:100vh;background:#0B398C;color:#fff}
 .card{text-align:center;padding:2rem}
 h1{margin:0 0 .5rem;font-size:1.6rem}
 p{margin:.25rem 0;color:#cfe4f2}
 a{color:#1791C8}
</style></head>
<body><div class="card">
  <h1>${BRAND_NAME}</h1>
  <p>The server is ready. No website points at this name yet.</p>
  <p>${BRAND_ORG}</p>
</div></body></html>
HTML
        chown "$VKAI_USER:$VKAI_GROUP" "${WWW_DEFAULT_DIR}/index.html"
        chmod 644 "${WWW_DEFAULT_DIR}/index.html"
    fi

    log_info "Directory tree ready."
}

# The ACME webroot exists on every install, not only with --tls-mode letsencrypt:
# a certificate ordered later from the panel UI answers its HTTP-01 challenge
# from exactly this directory, and an IP identifier has no DNS-01 alternative.
setup_acme_webroot() {
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 755 "${WWW_DEFAULT_DIR}/.well-known"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 755 "$ACME_CHALLENGE_DIR"
    # nginx serves the token file, the panel writes it: readable by everyone,
    # writable by the panel account only.
    log_info "ACME HTTP-01 webroot: ${ACME_CHALLENGE_DIR} (owner ${VKAI_USER}:${VKAI_GROUP}, mode 755)"
}

# =============================================================================
# 7. Copy the sources into /vkai-panel
# =============================================================================
sync_sources() {
    log_step "Copying the sources into ${PANEL_ROOT}"

    [[ -d "${SRC_DIR}/core"  ]] || die "${SRC_DIR}/core not found. Run the installer from a complete source tree."
    [[ -d "${SRC_DIR}/panel" ]] || die "${SRC_DIR}/panel not found."
    [[ -d "${SRC_DIR}/agent" ]] || die "${SRC_DIR}/agent not found."

    if [[ "$(cd "$SRC_DIR" && pwd -P)" == "$PANEL_ROOT" ]]; then
        log_info "The sources already live in ${PANEL_ROOT}, nothing to copy."
    else
        local rsync_opts=(-a --delete
            --exclude '.git' --exclude 'node_modules' --exclude '.next'
            --exclude '*.log' --exclude 'tmp/')
        if has rsync; then
            rsync "${rsync_opts[@]}" "${SRC_DIR}/core/"  "${CORE_DIR}/"
            rsync "${rsync_opts[@]}" "${SRC_DIR}/panel/" "${UI_DIR}/"
            rsync "${rsync_opts[@]}" "${SRC_DIR}/agent/" "${AGENT_DIR}/"
        else
            log_warn "rsync is not available, falling back to cp (stale files are kept)."
            cp -a "${SRC_DIR}/core/."  "${CORE_DIR}/"
            cp -a "${SRC_DIR}/panel/." "${UI_DIR}/"
            cp -a "${SRC_DIR}/agent/." "${AGENT_DIR}/"
        fi
    fi

    # The UI build reads the version from the repository root, one level above
    # panel/. On a developer machine that file is simply there; in an installed
    # tree it only exists if it is copied, and without it "npm run build" stops
    # at its prebuild step with ENOENT on /vkai-panel/VERSION.
    if [[ -f "${SRC_DIR}/VERSION" ]]; then
        install -m 644 "${SRC_DIR}/VERSION" "${PANEL_ROOT}/VERSION"
        log_info "Version $(tr -d '[:space:]' <"${PANEL_ROOT}/VERSION") recorded at ${PANEL_ROOT}/VERSION."
    else
        die "${SRC_DIR}/VERSION not found. It is the single source of truth for the product version and the UI cannot be built without it."
    fi

    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "${CORE_DIR}/bin" "${AGENT_DIR}/bin"
    chown -R "$VKAI_USER:$VKAI_GROUP" "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"
    log_info "core/, panel/ and agent/ are in place."
}

# =============================================================================
# 8. Configuration - written BEFORE the UI is built
#
# NEXT_PUBLIC_API_URL is inlined into the Next.js bundle at build time. Writing
# the configuration after the build silently produces a UI that talks to the
# wrong origin, so the order in main() is not negotiable.
# =============================================================================
env_get() {
    local key="$1"
    [[ -f "$ENV_FILE" ]] || return 1
    local line
    line="$(grep -E "^${key}=" "$ENV_FILE" | tail -n1 || true)"
    [[ -n "$line" ]] || return 1
    printf '%s' "${line#*=}"
}

# Keep the previous value when there is one (idempotent), otherwise generate.
reuse_or_generate() {
    local key="$1" generator="$2" existing
    existing="$(env_get "$key" || true)"
    if [[ -n "$existing" ]]; then
        printf '%s' "$existing"
    else
        eval "$generator"
    fi
}

detect_server_ip() {
    # The address packets actually leave from beats the first entry of
    # "hostname -I", which on a multi homed box is often a private bridge.
    SERVER_IP="$(ip route get 1.1.1.1 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}' || true)"
    [[ -n "$SERVER_IP" ]] || SERVER_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
    [[ -n "$SERVER_IP" ]] || SERVER_IP="127.0.0.1"
}

# The ACME identifier is what the certificate is issued for: the pinned domain
# when there is one, otherwise this machine's address.
resolve_acme_identifier() {
    if [[ -n "$PANEL_DOMAIN" ]]; then
        ACME_IDENTIFIER="$PANEL_DOMAIN"
        ACME_PROFILE="$ACME_PROFILE_DOMAIN"
        return 0
    fi

    ACME_IDENTIFIER="$SERVER_IP"
    # Let's Encrypt only issues for an IP identifier through the "shortlived"
    # profile, and those certificates last about six days. That is a fact of the
    # CA, not a preference: renewal has to run daily.
    ACME_PROFILE="$ACME_PROFILE_IP"

    [[ "$TLS_MODE" == "letsencrypt" ]] || return 0

    # Only when an order is coming: Let's Encrypt validates the address it can
    # reach from the internet, which behind NAT is not the one on the interface.
    local public_ip
    public_ip="$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)"
    if [[ "$public_ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ && "$public_ip" != "$SERVER_IP" ]]; then
        log_warn "This machine's egress address is ${public_ip} but its interface holds ${SERVER_IP} (NAT)."
        log_warn "The certificate will be requested for ${public_ip}; port 80 must reach this host from the internet."
        ACME_IDENTIFIER="$public_ip"
    fi
}

resolve_panel_access() {
    # Computed once: preflight_ports and setup_config share the result.
    [[ -z "$PANEL_PORT" ]] || return 0

    detect_server_ip

    # Panel port: command line > previously installed value > random > 8888.
    if [[ -n "$OPT_PORT" ]]; then
        PANEL_PORT="$OPT_PORT"
    else
        PANEL_PORT="$(env_get VKAI_PANEL_PUBLIC_PORT || true)"
        [[ -n "$PANEL_PORT" ]] || PANEL_PORT="$(env_get VKAI_PANEL_PORT || true)"
        # An older layout stored the loopback API port here; never inherit that.
        [[ "$PANEL_PORT" == "$API_BIND_PORT" ]] && PANEL_PORT=""
        if [[ -z "$PANEL_PORT" ]]; then
            if [[ "$OPT_RANDOM_PORT" == "true" ]]; then
                PANEL_PORT="$(( (RANDOM % 50000) + 10000 ))"
                while port_in_use "$PANEL_PORT"; do
                    PANEL_PORT="$(( (RANDOM % 50000) + 10000 ))"
                done
            else
                PANEL_PORT="$DEFAULT_PANEL_PORT"
            fi
        fi
    fi
    [[ "$PANEL_PORT" != "80" && "$PANEL_PORT" != "443" ]] ||
        die "The panel port cannot be ${PANEL_PORT}: 80/443 belong to the customer websites."

    # Security entrance: an existing one is never regenerated, because the
    # operator has it bookmarked.
    if [[ -n "$OPT_ENTRANCE" ]]; then
        PANEL_ENTRANCE="$OPT_ENTRANCE"
    else
        PANEL_ENTRANCE="$(env_get VKAI_PANEL_ENTRANCE || true)"
        [[ -n "$PANEL_ENTRANCE" ]] || PANEL_ENTRANCE="/vkai_$(rand_hex 4)"
    fi

    # Pinned domain.
    if [[ -n "$OPT_DOMAIN" ]]; then
        PANEL_DOMAIN="$OPT_DOMAIN"
    else
        PANEL_DOMAIN="$(env_get VKAI_PANEL_DOMAIN || true)"
    fi

    TLS_MODE="$OPT_TLS_MODE"
    if [[ "$TLS_MODE" == "none" ]]; then
        PANEL_SCHEME="http"
    else
        PANEL_SCHEME="https"
    fi

    if [[ "$OPT_ACME_STAGING" == "true" ]]; then
        ACME_DIRECTORY="$ACME_DIRECTORY_STAGING"
    else
        ACME_DIRECTORY="$ACME_DIRECTORY_PROD"
    fi
    resolve_acme_identifier
}

setup_config() {
    log_step "Writing ${ENV_FILE} (before the UI build)"

    resolve_panel_access

    DB_NAME="$(reuse_or_generate VKAI_DB_NAME 'printf vkai_panel')"
    DB_USER="$(reuse_or_generate VKAI_DB_USER 'printf vkai')"
    # Hex only: the panel configuration rejects any secret containing a colour
    # keyword, and hex can never contain one.
    DB_PASS="$(reuse_or_generate VKAI_DB_PASSWORD 'rand_hex 18')"
    JWT_SECRET="$(reuse_or_generate VKAI_JWT_SECRET 'rand_hex 32')"   # 64 characters
    SECRET_KEY="$(reuse_or_generate VKAI_SECRET_KEY 'rand_hex 32')"
    AGENT_TOKEN="$(reuse_or_generate VKAI_AGENT_TOKEN 'rand_hex 24')"

    (( ${#JWT_SECRET} >= 64 )) || die "The generated JWT secret is too short (${#JWT_SECRET} characters)."

    if [[ -n "$OPT_ADMIN_EMAIL" ]]; then
        ADMIN_EMAIL="$OPT_ADMIN_EMAIL"
    else
        ADMIN_EMAIL="$(env_get VKAI_ADMIN_EMAIL || true)"
        [[ -n "$ADMIN_EMAIL" ]] || ADMIN_EMAIL="${OPT_ADMIN_USER}@$(hostname -f 2>/dev/null || echo 'localhost')"
    fi

    local allowed_ips="$OPT_ALLOW_IPS"
    [[ -n "$allowed_ips" ]] || allowed_ips="$(env_get VKAI_PANEL_ALLOWED_IPS || true)"

    # An allow list that leaves out loopback locks this machine out of its own
    # panel, and the agent that runs here is the first thing that stops working.
    # Loopback is not a hole: only a process already on this host can present it
    # as a source, and 127.0.0.1 is also the only address trusted to set
    # X-Forwarded-For. An empty list still means "any source address".
    if [[ -n "$allowed_ips" ]]; then
        case ",${allowed_ips}," in
            *,127.0.0.1,*|*,127.0.0.0/8,*) : ;;
            *) allowed_ips="127.0.0.1,${allowed_ips}" ;;
        esac
        case ",${allowed_ips}," in
            *,::1,*|*,::1/128,*) : ;;
            *) allowed_ips="${allowed_ips},::1" ;;
        esac
        log_info "Panel allow list: ${allowed_ips} (loopback added so this machine can manage itself)."
    fi

    local panel_host="$SERVER_IP"
    [[ -n "$PANEL_DOMAIN" ]] && panel_host="$PANEL_DOMAIN"

    # Empty means same origin: the browser calls /api/... on the panel port, so
    # changing the IP or the domain later never requires a UI rebuild.
    local api_url="$OPT_API_URL"

    local acme_enabled="false"
    [[ "$TLS_MODE" == "letsencrypt" ]] && acme_enabled="true"

    local tmp_env
    tmp_env="$(mktemp "${ETC_DIR}/.env.XXXXXX")"
    cat >"$tmp_env" <<ENVEOF
# =============================================================================
# ${BRAND_NAME} - server configuration
# Generated: $(ts)
# Do NOT share this file. Mode 0600, owned by ${VKAI_USER}.
# =============================================================================

# --- Panel access ------------------------------------------------------------
# The panel NEVER listens on 80/443: those belong to the customer websites.
VKAI_PANEL_ENABLED=true
# The API binds loopback only; nginx owns the public port ${PANEL_PORT} and
# forwards to it. VKAI_PANEL_PORT is what the process binds, VKAI_PANEL_PUBLIC_PORT
# is what an operator types.
VKAI_PANEL_BIND=127.0.0.1
VKAI_PANEL_PORT=${API_BIND_PORT}
VKAI_PANEL_PUBLIC_PORT=${PANEL_PORT}
VKAI_PANEL_PUBLIC_SCHEME=${PANEL_SCHEME}
VKAI_PANEL_ENTRANCE=${PANEL_ENTRANCE}
VKAI_PANEL_ENTRANCE_ENABLED=true
VKAI_PANEL_SESSION_TTL=12h
# IPs/CIDRs allowed to reach the panel. Empty = any source address.
VKAI_PANEL_ALLOWED_IPS=${allowed_ips}
# Only trust X-Forwarded-For from the nginx instance on this machine.
VKAI_PANEL_TRUSTED_PROXIES=127.0.0.1,::1
VKAI_PANEL_DOMAIN=${PANEL_DOMAIN}
VKAI_PANEL_CONFIG_FILE=${PANEL_STATE_FILE}

# --- Panel TLS ---------------------------------------------------------------
# TLS is terminated by nginx on port ${PANEL_PORT}, which is also the process
# that serves the UI, so the API itself speaks plain HTTP on loopback and
# VKAI_PANEL_TLS_ENABLED stays false. The certificate paths are still recorded:
# whoever renews the certificate writes these two files and reloads nginx.
VKAI_PANEL_TLS_MODE=${TLS_MODE}
VKAI_PANEL_TLS_CERT=${PANEL_CERT}
VKAI_PANEL_TLS_KEY=${PANEL_KEY}
VKAI_PANEL_TLS_SELF_SIGNED=false
VKAI_PANEL_TLS_ENABLED=false

# --- ACME (Let's Encrypt) for the panel certificate --------------------------
# An IP identifier can only be validated with HTTP-01 on port 80 or TLS-ALPN-01
# on port 443. There is no DNS-01 for an IP. Port 443 belongs to the customer
# sites, so HTTP-01 from the webroot below is the only route, and Let's Encrypt
# only issues IP certificates through the "shortlived" profile (~6 days), which
# means renewal has to run daily.
VKAI_PANEL_ACME_ENABLED=${acme_enabled}
VKAI_PANEL_ACME_DIRECTORY=${ACME_DIRECTORY}
VKAI_PANEL_ACME_PROFILE=${ACME_PROFILE}
VKAI_PANEL_ACME_IDENTIFIER=${ACME_IDENTIFIER}
VKAI_PANEL_ACME_EMAIL=${ADMIN_EMAIL}
VKAI_PANEL_ACME_WEBROOT=${ACME_WEBROOT}
VKAI_PANEL_ACME_CHALLENGE_DIR=${ACME_CHALLENGE_DIR}
VKAI_PANEL_ACME_ACCOUNT_KEY=${ACME_ACCOUNT_KEY}
VKAI_PANEL_ACME_TERMS_AGREED=true

# --- API server --------------------------------------------------------------
VKAI_SERVER_HOST=127.0.0.1
VKAI_SERVER_PORT=${API_BIND_PORT}
VKAI_SERVER_MODE=release

# Where the API forwards requests for the user interface. The panel has ONE
# front door: nginx sends the whole panel port to the API, the API checks the
# security entrance, and only then does anything reach Next.js. nginx never
# talks to Next.js, which is why the entrance protects the login form and not
# just /api. Loopback only.
VKAI_UI_UPSTREAM=http://127.0.0.1:${UI_BIND_PORT}

# --- Database ----------------------------------------------------------------
VKAI_DB_HOST=127.0.0.1
VKAI_DB_PORT=5432
VKAI_DB_NAME=${DB_NAME}
VKAI_DB_USER=${DB_USER}
VKAI_DB_PASSWORD=${DB_PASS}
# The database is on this machine, so 'disable' is acceptable here.
VKAI_DB_SSLMODE=disable

# --- Redis -------------------------------------------------------------------
VKAI_REDIS_HOST=127.0.0.1
VKAI_REDIS_PORT=6379
VKAI_REDIS_PASSWORD=
VKAI_REDIS_DB=0

# --- Secrets -----------------------------------------------------------------
VKAI_JWT_SECRET=${JWT_SECRET}
VKAI_JWT_ACCESS_EXPIRY=15
VKAI_JWT_REFRESH_EXPIRY=10080
VKAI_JWT_ISSUER=vkai-panel
VKAI_SECRET_KEY=${SECRET_KEY}
VKAI_AGENT_TOKEN=${AGENT_TOKEN}
VKAI_AGENT_PORT=${AGENT_PORT}

# --- Administrator -----------------------------------------------------------
VKAI_ADMIN_USER=${OPT_ADMIN_USER}
VKAI_ADMIN_EMAIL=${ADMIN_EMAIL}

# --- Data paths --------------------------------------------------------------
VKAI_FILEMANAGER_ROOT=${WWW_DOMAINS_DIR}
VKAI_BACKUP_ROOT=${WWW_BACKUP_DIR}
VKAI_WWW_ROOT=${WWW_DOMAINS_DIR}
VKAI_SITE_LOG_ROOT=${LOG_SITES_DIR}
VKAI_SSL_ROOT=${SSL_DIR}
VKAI_TMP_ROOT=${TMP_DIR}

# --- Logging -----------------------------------------------------------------
VKAI_LOG_LEVEL=info
VKAI_LOG_DIR=${LOG_DIR}

# --- CORS / RBAC / cron ------------------------------------------------------
VKAI_CORS_ALLOWED_ORIGINS=${PANEL_SCHEME}://${panel_host}:${PANEL_PORT}
VKAI_RBAC_ENFORCE=true
VKAI_CRON_USER=${VKAI_USER}

# --- Next.js UI --------------------------------------------------------------
# This value is INLINED into the bundle by "npm run build". Changing it here
# requires a rebuild ("vkai update") before it takes effect.
# Empty = same origin: the browser calls /api/... on the panel port.
NEXT_PUBLIC_API_URL=${api_url}
NODE_ENV=production
PORT=${UI_BIND_PORT}
HOSTNAME=127.0.0.1
ENVEOF

    mv "$tmp_env" "$ENV_FILE"
    chown "$VKAI_USER:$VKAI_GROUP" "$ENV_FILE"
    chmod 600 "$ENV_FILE"

    # Next.js only reads .env from the project root -> link it into panel/.
    # When linking is impossible (different filesystem) copy and tighten.
    if ln -sfn "$ENV_FILE" "${UI_DIR}/.env" 2>/dev/null; then
        log_info "Linked ${UI_DIR}/.env -> ${ENV_FILE}"
    else
        install -o "$VKAI_USER" -g "$VKAI_GROUP" -m 600 "$ENV_FILE" "${UI_DIR}/.env"
        log_warn "Symlink failed, COPIED .env to ${UI_DIR}/.env instead."
    fi
    ln -sfn "$ENV_FILE" "${CORE_DIR}/.env" 2>/dev/null || true

    # /etc/vkai is the default path baked into the Go code; point it at
    # ${ETC_DIR} so there is exactly one source of truth for configuration.
    if [[ ! -e /etc/vkai ]]; then
        ln -sfn "$ETC_DIR" /etc/vkai
        log_info "Linked /etc/vkai -> ${ETC_DIR}"
    fi

    log_info "Configuration: port ${PANEL_PORT}, entrance ${PANEL_ENTRANCE}, scheme ${PANEL_SCHEME}, TLS mode ${TLS_MODE}"
}

# =============================================================================
# 9. Panel certificate
#
# nginx cannot start with "listen <port> ssl" unless the certificate files
# already exist, so a self-signed pair is always created first. With
# --tls-mode letsencrypt it is the bootstrap certificate that keeps the panel
# reachable until the real one is issued; an existing certificate is never
# overwritten, which is what makes a re-run safe.
# =============================================================================
cert_fingerprint() {
    local file="$1"
    [[ -f "$file" ]] || { printf '(none)'; return 0; }
    openssl x509 -in "$file" -noout -fingerprint -sha256 2>/dev/null |
        sed 's/^.*Fingerprint=//' | tr -d '\n' || printf '(unreadable)'
}

cert_expiry() {
    local file="$1"
    [[ -f "$file" ]] || { printf '(none)'; return 0; }
    openssl x509 -in "$file" -noout -enddate 2>/dev/null |
        sed 's/^notAfter=//' | tr -d '\n' || printf '(unreadable)'
}

cert_is_self_signed() {
    local file="$1"
    [[ -f "$file" ]] || return 1
    local subject issuer
    subject="$(openssl x509 -in "$file" -noout -subject 2>/dev/null || true)"
    issuer="$(openssl x509 -in "$file" -noout -issuer 2>/dev/null || true)"
    [[ "${subject#subject=}" == "${issuer#issuer=}" ]]
}

generate_self_signed_cert() {
    local hosts="DNS:localhost,IP:127.0.0.1,IP:::1"
    local hostname_fqdn
    hostname_fqdn="$(hostname -f 2>/dev/null || hostname 2>/dev/null || true)"
    [[ -n "$hostname_fqdn" && "$hostname_fqdn" != "localhost" ]] && hosts="${hosts},DNS:${hostname_fqdn}"
    [[ -n "$PANEL_DOMAIN" ]] && hosts="${hosts},DNS:${PANEL_DOMAIN}"
    if [[ "$SERVER_IP" =~ ^[0-9.]+$ && "$SERVER_IP" != "127.0.0.1" ]]; then
        hosts="${hosts},IP:${SERVER_IP}"
    fi
    if [[ -n "$ACME_IDENTIFIER" && "$ACME_IDENTIFIER" =~ ^[0-9.]+$ && "$ACME_IDENTIFIER" != "$SERVER_IP" ]]; then
        hosts="${hosts},IP:${ACME_IDENTIFIER}"
    fi

    local cn="${PANEL_DOMAIN:-$SERVER_IP}"
    log_info "Generating a self-signed certificate for ${cn} (SAN: ${hosts})"

    # -addext needs OpenSSL 1.1.1, present on every supported release.
    openssl req -x509 -newkey rsa:2048 -sha256 -days 825 -nodes \
        -keyout "$PANEL_KEY" -out "$PANEL_CERT" \
        -subj "/O=${BRAND_NAME}/CN=${cn}" \
        -addext "subjectAltName=${hosts}" \
        -addext "keyUsage=critical,digitalSignature,keyEncipherment" \
        -addext "extendedKeyUsage=serverAuth" >/dev/null 2>&1 ||
        die "Could not generate the self-signed certificate with openssl."

    chown "$VKAI_USER:$VKAI_GROUP" "$PANEL_CERT" "$PANEL_KEY"
    chmod 644 "$PANEL_CERT"
    chmod 640 "$PANEL_KEY"
}

setup_certificate() {
    if [[ "$TLS_MODE" == "none" ]]; then
        log_step "Panel TLS disabled (--tls-mode none)"
        log_warn "The panel will be served over plain HTTP. Anyone on the path can read the session cookie."
        CERT_SOURCE="none"
        return 0
    fi

    log_step "Preparing the panel certificate"

    if [[ -s "$PANEL_CERT" && -s "$PANEL_KEY" ]]; then
        log_info "Keeping the existing certificate ${PANEL_CERT} (a re-run never replaces it)."
    else
        generate_self_signed_cert
    fi

    if cert_is_self_signed "$PANEL_CERT"; then
        CERT_SOURCE="self-signed"
    else
        CERT_SOURCE="CA issued"
    fi
    CERT_FINGERPRINT="$(cert_fingerprint "$PANEL_CERT")"
    CERT_EXPIRY="$(cert_expiry "$PANEL_CERT")"
    log_info "Certificate: ${CERT_SOURCE}, expires ${CERT_EXPIRY}"
    log_info "SHA-256 fingerprint: ${CERT_FINGERPRINT}"
}

# =============================================================================
# 10. PostgreSQL and Redis
# =============================================================================
detect_pg_service() {
    local svc
    for svc in postgresql postgresql.service postgresql-16 postgresql-15 postgresql-14 postgresql-13; do
        if systemctl list-unit-files 2>/dev/null | grep -q "^${svc%.service}\.service"; then
            PG_SERVICE="${svc%.service}"
            return 0
        fi
    done
    PG_SERVICE="postgresql"
}

pg_initdb_if_needed() {
    # Debian and Ubuntu initialise the cluster when the package is installed.
    # RHEL and SUSE do not.
    case "$OS_FAMILY" in
        rhel)
            local setup_bin
            setup_bin="$(command -v postgresql-setup || true)"
            if [[ -n "$setup_bin" && ! -f /var/lib/pgsql/data/PG_VERSION ]]; then
                log_info "Initialising the PostgreSQL cluster..."
                "$setup_bin" --initdb >/dev/null 2>&1 || "$setup_bin" initdb >/dev/null 2>&1 || true
            fi
            ;;
        suse)
            # openSUSE runs initdb on the first start.
            :
            ;;
    esac
}

pg_hba_allow_local_tcp() {
    local hba
    hba="$(sudo -u postgres psql -tAc 'SHOW hba_file' 2>/dev/null || true)"
    [[ -n "$hba" && -f "$hba" ]] || { log_warn "pg_hba.conf could not be located, skipped."; return 0; }

    if grep -qE "^host\s+${DB_NAME}\s+${DB_USER}\s" "$hba"; then
        log_info "pg_hba.conf already has a rule for ${DB_USER}."
        return 0
    fi

    # The authentication method has to match how the server hashes passwords:
    # PostgreSQL 13 and older default to md5, 14 and newer to scram-sha-256.
    # Writing the wrong one rejects the connection even with the right password.
    local method
    method="$(sudo -u postgres psql -tAc 'SHOW password_encryption' 2>/dev/null | tr -d '[:space:]' || true)"
    case "$method" in
        scram-sha-256|md5) : ;;
        *) method="scram-sha-256" ;;
    esac

    log_info "Adding a pg_hba.conf rule for ${DB_USER}@127.0.0.1 (${method})."
    cp -a "$hba" "${hba}.vkai.bak.$(date +%Y%m%d%H%M%S)"
    # Prepended: pg_hba is evaluated top down and the first matching line wins.
    local tmp
    tmp="$(mktemp)"
    {
        echo "# --- ${BRAND_NAME}: panel database access over loopback ---"
        echo "host    ${DB_NAME}    ${DB_USER}    127.0.0.1/32    ${method}"
        echo "host    ${DB_NAME}    ${DB_USER}    ::1/128         ${method}"
        cat "$hba"
    } >"$tmp"
    cat "$tmp" >"$hba"
    rm -f "$tmp"
    systemctl reload "$PG_SERVICE" 2>/dev/null || systemctl restart "$PG_SERVICE"
}

setup_database() {
    log_step "Preparing PostgreSQL"

    detect_pg_service
    pg_initdb_if_needed
    # pg_isready is the authority on whether the database is usable. A failed
    # enable on a host where PostgreSQL is already serving is not a reason to
    # abort an installation.
    if ! pkg_enable_service "$PG_SERVICE"; then
        if command -v pg_isready >/dev/null 2>&1 && pg_isready -q 2>/dev/null; then
            log_warn "Could not manage ${PG_SERVICE} through systemd, but PostgreSQL is answering; continuing."
        else
            die "PostgreSQL did not start. Inspect: journalctl -u ${PG_SERVICE} -n 50"
        fi
    fi

    # Wait for the server to accept connections.
    local i
    for i in $(seq 1 30); do
        sudo -u postgres psql -tAc 'SELECT 1' >/dev/null 2>&1 && break
        sleep 1
        (( i == 30 )) && die "PostgreSQL did not answer within 30 seconds."
    done

    local esc_pass="${DB_PASS//\'/\'\'}"
    if sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1; then
        sudo -u postgres psql -qc "ALTER ROLE \"${DB_USER}\" WITH LOGIN PASSWORD '${esc_pass}';"
        log_info "Updated the password of role '${DB_USER}'."
    else
        sudo -u postgres psql -qc "CREATE ROLE \"${DB_USER}\" WITH LOGIN PASSWORD '${esc_pass}';"
        log_info "Created database role '${DB_USER}'."
    fi

    # An existing database is kept as it is: re-running the installer must never
    # destroy data.
    if sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
        log_info "Database '${DB_NAME}' already exists and is left untouched."
    else
        sudo -u postgres createdb -O "$DB_USER" "$DB_NAME"
        log_info "Created database '${DB_NAME}' (owner ${DB_USER})."
    fi

    # uuid-ossp has to be installed by a superuser (migration 001 needs
    # uuid_generate_v4).
    sudo -u postgres psql -q -d "$DB_NAME" -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp";' ||
        die "Could not create the uuid-ossp extension. Is postgresql-contrib missing?"

    pg_hba_allow_local_tcp
}

psql_vkai() {
    PGPASSWORD="$DB_PASS" psql -h 127.0.0.1 -p 5432 -U "$DB_USER" -d "$DB_NAME" \
        -v ON_ERROR_STOP=1 --quiet "$@"
}

run_migrations() {
    log_step "Applying database migrations"

    local dir="${CORE_DIR}/migrations"
    [[ -d "$dir" ]] || { log_warn "${dir} not found, no migrations applied."; return 0; }

    touch "$MIGRATIONS_STATE"
    chmod 600 "$MIGRATIONS_STATE"

    local applied=0
    apply_migration_files "$applied" fatal "" < <(find "$dir" -maxdepth 1 -name '*.sql' -type f | sort)
    applied="$MIGRATIONS_APPLIED"

    # migrations/pending holds schema that is written but not yet numbered into
    # the contiguous 001-023 sequence, which is verified against a real
    # PostgreSQL and must not be renumbered. It is applied here, AFTER the
    # sequence, because the alternative is shipping features whose tables never
    # exist on any installed panel: the panel host cannot be a managed node
    # without server_local_node, and it would say so at every start.
    #
    # Every file there creates tables with IF NOT EXISTS and adds nothing to an
    # existing one, so applying it twice is a no-op and applying it before it is
    # numbered costs nothing when it later is. The state file records it under
    # its "pending/" name so that folding it into the sequence and re-running it
    # is still harmless.
    local pending="${dir}/pending"
    if [[ -d "$pending" ]]; then
        apply_migration_files "$applied" tolerant pending < <(find "$pending" -maxdepth 1 -name '*.sql' -type f | sort)
        applied="$MIGRATIONS_APPLIED"
    fi

    log_info "Applied ${applied} new migration(s)."
}

# apply_migration_files <already-applied-count> <fatal|tolerant> <prefix>
#
# Reads file paths on stdin. The running total comes back in MIGRATIONS_APPLIED,
# because the loop runs in this shell and a subshell could not report it.
#
# The two modes are the difference between the two directories. The numbered
# sequence is contiguous and verified against a real PostgreSQL: a failure there
# leaves a half migrated database and the install must stop. migrations/pending
# is a staging area for schema that has not been folded into the sequence yet;
# every file in it creates its tables additively, and a panel runs without them
# by degrading the feature that wanted them. A syntax error in a file somebody is
# still writing must therefore cost that one feature, not the whole install.
MIGRATIONS_APPLIED=0
apply_migration_files() {
    local applied="$1" mode="$2" prefix="${3:-}"
    local f name recorded
    while IFS= read -r f; do
        name="$(basename "$f")"
        recorded="$name"
        [[ -z "$prefix" ]] || recorded="${prefix}/${name}"
        if grep -qxF "$recorded" "$MIGRATIONS_STATE"; then
            continue
        fi
        log_info "  -> ${recorded}"
        if psql_vkai -f "$f" >/dev/null; then
            echo "$recorded" >>"$MIGRATIONS_STATE"
            applied=$((applied + 1))
        elif [[ "$mode" == "fatal" ]]; then
            die "Migration '${recorded}' failed. The database is half migrated - see ${INSTALL_LOG}."
        else
            log_warn "Staged migration '${recorded}' did not apply. It is not recorded, so it will be tried"
            log_warn "again on the next install or upgrade. Whatever needs its tables is unavailable until"
            log_warn "then; see ${INSTALL_LOG} for the error PostgreSQL gave."
        fi
    done
    MIGRATIONS_APPLIED="$applied"
}

setup_redis() {
    log_step "Preparing Redis"
    case "$OS_FAMILY" in
        debian) REDIS_SERVICE="redis-server" ;;
        *)      REDIS_SERVICE="redis" ;;
    esac
    if ! systemctl list-unit-files 2>/dev/null | grep -q "^${REDIS_SERVICE}\.service"; then
        # Amazon Linux 2023 names the unit redis6.
        local alt
        for alt in redis6 redis redis-server; do
            if systemctl list-unit-files 2>/dev/null | grep -q "^${alt}\.service"; then
                REDIS_SERVICE="$alt"
                break
            fi
        done
    fi
    pkg_enable_service "$REDIS_SERVICE" || log_warn "Redis is not running. The panel works but loses caching and queues."
    log_info "Redis service: ${REDIS_SERVICE}"
}

# =============================================================================
# 11. First administrator account
# =============================================================================
bcrypt_hash() {
    local plain="$1" out="" gendir="${CORE_DIR}/tools/vkai-hashgen"

    # Go first: golang.org/x/crypto/bcrypt is already a dependency of core/.
    mkdir -p "$gendir"
    cat >"${gendir}/main.go" <<'GOEOF'
package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(os.Args[1]), 12)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(string(h))
}
GOEOF
    out="$( ( cd "$CORE_DIR" && "$GO_BIN" run ./tools/vkai-hashgen "$plain" ) 2>/dev/null || true )"
    rm -rf "${CORE_DIR}/tools/vkai-hashgen"
    rmdir "${CORE_DIR}/tools" 2>/dev/null || true

    if [[ -z "$out" ]] && has python3; then
        out="$(python3 - "$plain" <<'PYEOF' 2>/dev/null || true
import sys
try:
    import bcrypt
except ImportError:
    sys.exit(1)
print(bcrypt.hashpw(sys.argv[1].encode(), bcrypt.gensalt(12)).decode(), end="")
PYEOF
)"
    fi

    printf '%s' "$out"
}

setup_admin_account() {
    log_step "Creating the first administrator"

    ADMIN_USER="$OPT_ADMIN_USER"

    # Re-install: never reset the password of an administrator already in use.
    local existing
    existing="$(psql_vkai -tAc "SELECT 1 FROM users WHERE username = '${ADMIN_USER//\'/\'\'}'" 2>/dev/null | tr -d '[:space:]' || true)"
    if [[ "$existing" == "1" && -f "$SUMMARY_FILE" ]]; then
        log_info "Account '${ADMIN_USER}' already exists - its password is left unchanged."
        ADMIN_PASS="(unchanged - the password you already have)"
        ADMIN_PASS_CHANGED="true"
        return 0
    fi

    ADMIN_PASS="$(rand_alnum 20)"
    local hash
    hash="$(bcrypt_hash "$ADMIN_PASS")"

    if [[ -z "$hash" || "$hash" != \$2* ]]; then
        log_warn "Could not compute a bcrypt hash (no Go toolchain and no python3-bcrypt)."
        log_warn "The default admin/admin123 account from the migration is still in force."
        log_warn "CHANGE THE PASSWORD IMMEDIATELY after the first login."
        ADMIN_USER="admin"
        ADMIN_PASS="admin123"
        ADMIN_PASS_CHANGED="false"
        return 0
    fi

    psql_vkai <<SQLEOF >/dev/null
UPDATE users
   SET username      = '${ADMIN_USER//\'/\'\'}',
       email         = '${ADMIN_EMAIL//\'/\'\'}',
       password_hash = '${hash//\'/\'\'}',
       status        = 'active',
       updated_at    = NOW()
 WHERE id = 'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11';
SQLEOF

    ADMIN_PASS_CHANGED="true"
    log_info "Administrator: ${ADMIN_USER} (20 character generated password)."
}

# =============================================================================
# 12. Building
# =============================================================================
# panel_ldflags echoes the linker flags every Go build must use, straight out of
# the Makefile.
#
# The flags are what put the VERSION file, the commit and the build date INTO the
# binary: core/internal/version declares three empty strings for the linker to
# overwrite, and without -X every one of them falls back to a compiled-in
# default. This script used to build with "-ldflags '-s -w'" only, so the whole
# versioning mechanism was inert in the shipped product and the in-panel upgrade
# check compared releases against a version that was never stamped.
#
# The definition is READ from the Makefile rather than repeated here. A second
# copy is a copy that drifts, and the two would drift silently: a wrongly stamped
# binary still builds, still starts and still serves.
panel_ldflags() {
    [[ -f "${SRC_DIR}/Makefile" ]] ||
        die "${SRC_DIR}/Makefile not found. It holds the single definition of the build stamp (LDFLAGS)."
    has make ||
        die "make is not installed, so the build stamp cannot be read from ${SRC_DIR}/Makefile. Install make and re-run."

    local flags
    # --no-print-directory: -C makes some versions of make announce the
    # directory change on stdout, which would end up inside the linker flags.
    flags="$(make -s --no-print-directory -C "$SRC_DIR" GO="$GO_BIN" print-ldflags)" ||
        die "'make print-ldflags' failed in ${SRC_DIR}."
    [[ -n "$flags" ]] || die "'make print-ldflags' produced nothing in ${SRC_DIR}."
    printf '%s' "$flags"
}

build_core() {
    log_step "Building the API (core/)"
    cd "$CORE_DIR"
    export HOME="${TMP_DIR}"
    export GOCACHE="${TMP_DIR}/go-build" GOMODCACHE="${TMP_DIR}/go-mod" GOFLAGS="-buildvcs=false"
    mkdir -p "$GOCACHE" "$GOMODCACHE"

    local ldflags
    ldflags="$(panel_ldflags)"
    log_info "Build stamp: ${ldflags}"

    "$GO_BIN" mod download
    "$GO_BIN" build -trimpath -ldflags "$ldflags" -o "${CORE_DIR}/bin/vkai-api"       ./cmd/api
    "$GO_BIN" build -trimpath -ldflags "$ldflags" -o "${CORE_DIR}/bin/vkai-panelctl"  ./cmd/panelctl
    if [[ -f "${CORE_DIR}/cmd/cli/main.go" ]]; then
        "$GO_BIN" build -trimpath -ldflags "$ldflags" -o "${CORE_DIR}/bin/vkai-cli" ./cmd/cli
    fi

    [[ -x "${CORE_DIR}/bin/vkai-api" ]] || die "${CORE_DIR}/bin/vkai-api was not produced"

    install -m 0755 "${CORE_DIR}/bin/vkai-panelctl" /usr/local/bin/vkai-panelctl
    [[ -x "${CORE_DIR}/bin/vkai-cli" ]] && install -m 0755 "${CORE_DIR}/bin/vkai-cli" /usr/local/bin/vkai-cli

    chown -R "$VKAI_USER:$VKAI_GROUP" "${CORE_DIR}/bin"
    log_info "API built: ${CORE_DIR}/bin/vkai-api"
}

build_agent() {
    log_step "Building the agent (agent/)"
    cd "$AGENT_DIR"
    export HOME="${TMP_DIR}"
    export GOCACHE="${TMP_DIR}/go-build" GOMODCACHE="${TMP_DIR}/go-mod" GOFLAGS="-buildvcs=false"

    # The same stamp the Makefile's build-agent target uses. The agent is a
    # separate Go module and cannot import core/internal/version, so the -X
    # flags have no target there and the linker ignores them; passing the one
    # definition anyway is what keeps this script and the Makefile identical.
    local ldflags
    ldflags="$(panel_ldflags)"

    "$GO_BIN" mod download
    if [[ -f "${AGENT_DIR}/cmd/main.go" ]]; then
        "$GO_BIN" build -trimpath -ldflags "$ldflags" -o "${AGENT_DIR}/bin/vkai-agent" ./cmd
    else
        "$GO_BIN" build -trimpath -ldflags "$ldflags" -o "${AGENT_DIR}/bin/vkai-agent" .
    fi

    [[ -x "${AGENT_DIR}/bin/vkai-agent" ]] || die "${AGENT_DIR}/bin/vkai-agent was not produced"
    chown -R "$VKAI_USER:$VKAI_GROUP" "${AGENT_DIR}/bin"
    log_info "Agent built: ${AGENT_DIR}/bin/vkai-agent"
}

build_ui() {
    log_step "Building the UI (panel/)"
    [[ -f "$ENV_FILE" ]] || die "${ENV_FILE} is missing: the configuration must be written BEFORE the UI is built."

    cd "$UI_DIR"
    export HOME="${TMP_DIR}"
    export npm_config_cache="${TMP_DIR}/npm"
    mkdir -p "$npm_config_cache"

    if [[ -f package-lock.json ]]; then
        "$NPM_BIN" ci --no-audit --no-fund
    else
        "$NPM_BIN" install --no-audit --no-fund
    fi

    # NEXT_PUBLIC_* is inlined into the bundle right here.
    set -a
    # shellcheck disable=SC1090
    . "$ENV_FILE"
    set +a
    NODE_ENV=production "$NPM_BIN" run build

    # output: 'standalone' DELIBERATELY does not copy .next/static and public/
    # into .next/standalone. Without that copy every /_next/static/*.js request
    # returns 404 and the browser shows
    # "Application error: a client-side exception has occurred".
    # package.json already has postbuild:standalone; this is the safety net for
    # the day that script is removed or the build runs through another tool.
    local sa="${UI_DIR}/.next/standalone"
    [[ -d "$sa" ]] || die "${sa} not found. next.config.js must set output: 'standalone'."

    if [[ -d "${UI_DIR}/.next/static" && ! -d "${sa}/.next/static" ]]; then
        log_warn ".next/standalone/.next/static is missing - copying it manually."
        mkdir -p "${sa}/.next"
        cp -a "${UI_DIR}/.next/static" "${sa}/.next/static"
    fi
    if [[ -d "${UI_DIR}/public" && ! -d "${sa}/public" ]]; then
        log_warn ".next/standalone/public is missing - copying it manually."
        cp -a "${UI_DIR}/public" "${sa}/public"
    fi

    verify_ui_build "$UI_DIR"

    chown -R "$VKAI_USER:$VKAI_GROUP" "$UI_DIR"
    log_info "UI built: ${sa}/server.js"
}

# The single gate that decides whether the UI build is usable. It exists because
# of a failure this project shipped once: the page returned HTML while every
# /_next/static/*.js returned 404, and the browser showed
# "Application error: a client-side exception has occurred" on every page.
# Called after EVERY UI build and before vkai-ui is started.
verify_ui_build() {
    local ui_dir="$1"
    local sa="${ui_dir}/.next/standalone"

    [[ -d "$sa" ]] ||
        die "UI BUILD VERIFICATION FAILED: ${sa} does not exist.
next.config.js must set output: 'standalone'. Fix: cd ${ui_dir} && npm run build"

    [[ -f "${sa}/server.js" ]] ||
        die "UI BUILD VERIFICATION FAILED: ${sa}/server.js is missing.
vkai-ui.service starts with 'node .../server.js', so without this file nothing can run.
Fix: cd ${ui_dir} && npm run build"

    [[ -s "${sa}/server.js" ]] ||
        die "UI BUILD VERIFICATION FAILED: ${sa}/server.js is empty. Rebuild the UI."

    [[ -d "${sa}/.next/static" ]] ||
        die "UI BUILD VERIFICATION FAILED: ${sa}/.next/static is missing.
Next.js with output 'standalone' does NOT copy .next/static itself. Without it the page
returns HTML while every /_next/static/*.js returns 404, and the browser shows
'Application error: a client-side exception has occurred'.
Fix: cd ${ui_dir} && npm run build   (the postbuild:standalone script performs the copy)
or copy it by hand: cp -a ${ui_dir}/.next/static ${sa}/.next/static"

    [[ -n "$(find "${sa}/.next/static" -mindepth 1 -maxdepth 2 -print -quit 2>/dev/null)" ]] ||
        die "UI BUILD VERIFICATION FAILED: ${sa}/.next/static is empty.
Same symptom: 'Application error: a client-side exception has occurred'. Rebuild the UI."

    if [[ -d "${ui_dir}/public" && ! -d "${sa}/public" ]]; then
        log_warn "${sa}/public is missing: static files under public/ will 404 (not fatal)."
    fi

    log_info "UI build verified: server.js and .next/static are both present."
}

# /vkai-panel/current -> the running build. The systemd units go through this
# symlink instead of pointing at ${CORE_DIR}/${UI_DIR} directly, so deploy.sh can
# switch releases with one 'ln -sfn' plus a restart.
setup_current_link() {
    log_step "Pointing ${CURRENT_LINK} at the build just produced"

    if [[ -L "$CURRENT_LINK" ]]; then
        local old
        old="$(readlink -f "$CURRENT_LINK" 2>/dev/null || true)"
        if [[ -n "$old" && "$old" != "$PANEL_ROOT" ]]; then
            log_warn "current points at ${old} (a deploy.sh release)."
            log_warn "This installer built in place, so current is moved back to ${PANEL_ROOT}."
        fi
    elif [[ -e "$CURRENT_LINK" ]]; then
        die "${CURRENT_LINK} exists and is NOT a symlink. Move it aside and run again."
    fi

    ln -sfn "$PANEL_ROOT" "$CURRENT_LINK"
    chown -h "$VKAI_USER:$VKAI_GROUP" "$CURRENT_LINK" 2>/dev/null || true

    [[ -x "${CURRENT_LINK}/core/bin/vkai-api" ]] ||
        die "${CURRENT_LINK}/core/bin/vkai-api is not executable through the symlink."
    [[ -f "${CURRENT_LINK}/panel/.next/standalone/server.js" ]] ||
        die "${CURRENT_LINK}/panel/.next/standalone/server.js does not exist through the symlink."

    log_info "${CURRENT_LINK} -> ${PANEL_ROOT}"
}

# =============================================================================
# 12b. Version record
# =============================================================================
# ${VERSION_FILE} is the answer to "what is installed here?". It is written on
# every install and rewritten by every successful upgrade, and it lives outside
# any release directory so a rollback cannot take it back to a lie.
#
# It is deliberately NOT derived from the binaries: those can be replaced by
# hand, and an upgrade that guessed the current version from a binary it did not
# build would guess wrong exactly on the machine where the answer matters.
#
# Fields:
#   version       what this installer put on disk
#   channel       which release stream this panel follows (stable | beta)
#   pin           when set, upgrades are held at this version (see docs/UPGRADE.md)
#   installed_at  first installation, preserved across upgrades
#   updated_at    when the running code last changed
#   release       the release directory current points at, or the in-place build
#
# Mode 0644 root:${VKAI_GROUP}: it holds no secret and the API, which runs as
# ${VKAI_USER}, displays it.
record_version() {
    log_step "Recording the installed version"

    local installed_at
    installed_at="$(json_field_from_file "$VERSION_FILE" installed_at)"
    [[ -n "$installed_at" ]] || installed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

    # An in-place install always serves ${PANEL_ROOT} through the symlink.
    local release="in-place"

    local pin
    pin="$(json_field_from_file "$VERSION_FILE" pin)"

    local tmp
    tmp="$(mktemp "${ETC_DIR}/.version.XXXXXX")"
    cat >"$tmp" <<JSON
{
  "version": "${VKAI_VERSION}",
  "channel": "${OPT_CHANNEL}",
  "pin": "${pin}",
  "installed_at": "${installed_at}",
  "updated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "release": "${release}",
  "install_mode": "${INSTALL_MODE}",
  "os": "${OS_PRETTY}",
  "arch": "${ARCH}"
}
JSON
    chmod 0644 "$tmp"
    chown "root:${VKAI_GROUP}" "$tmp" 2>/dev/null || true
    mv -f "$tmp" "$VERSION_FILE"

    log_info "${VERSION_FILE}: version ${VKAI_VERSION}, channel ${OPT_CHANNEL}, installed ${installed_at}"
}

# json_field_from_file <file> <key> - one top-level string field, empty when the
# file or the field is absent. jq is not a dependency of the installer, and the
# only JSON it reads is the one it wrote itself.
json_field_from_file() {
    local file="$1" key="$2"
    [[ -f "$file" ]] || return 0
    # shellcheck disable=SC2020  # three single characters, each mapped to a newline
    tr ',{}' '\n\n\n' <"$file" |
        sed -n -E "s/^[[:space:]]*\"${key}\"[[:space:]]*:[[:space:]]*\"([^\"]*)\".*/\\1/p" | sed -n '1p'
}

# =============================================================================
# 13. systemd units
# =============================================================================
install_systemd_units() {
    log_step "Installing the systemd units"

    local src="${SRC_DIR}/deploy/systemd"
    [[ -d "$src" ]] || die "${src} not found"

    local unit
    for unit in vkai-api.service vkai-ui.service vkai-agent.service; do
        [[ -f "${src}/${unit}" ]] || die "${src}/${unit} is missing"
        install -m 0644 "${src}/${unit}" "/etc/systemd/system/${unit}"
        log_info "  -> /etc/systemd/system/${unit}"
    done

    # The certificate renewal pair is installed always and enabled only when a
    # CA issued certificate is in play. An IP certificate lasts about six days,
    # so a daily timer is the difference between a panel that stays reachable
    # and one that stops trusting itself next week.
    for unit in vkai-cert-renew.service vkai-cert-renew.timer; do
        if [[ -f "${src}/${unit}" ]]; then
            install -m 0644 "${src}/${unit}" "/etc/systemd/system/${unit}"
            log_info "  -> /etc/systemd/system/${unit}"
        fi
    done

    # The daily update CHECK. It is installed and enabled on every machine
    # because a panel that cannot tell its operator a security release exists is
    # worse than one that asks. It only ever checks - see the unit file for why
    # an unattended install is never scheduled.
    for unit in vkai-upgrade-check.service vkai-upgrade-check.timer; do
        if [[ -f "${src}/${unit}" ]]; then
            install -m 0644 "${src}/${unit}" "/etc/systemd/system/${unit}"
            log_info "  -> /etc/systemd/system/${unit}"
        fi
    done

    # Remove unit names from older installations.
    local old
    for old in vkai-frontend.service vkai-panel-api.service vkai-panel-frontend.service; do
        if [[ -f "/etc/systemd/system/${old}" ]]; then
            systemctl disable --now "${old%.service}" >/dev/null 2>&1 || true
            rm -f "/etc/systemd/system/${old}"
            log_info "  Removed obsolete unit: ${old}"
        fi
    done

    systemctl daemon-reload
    systemctl enable vkai-api vkai-ui >/dev/null 2>&1
    # The agent is NOT enabled here. It is enabled and started by
    # register_local_node, after the API is up, because it can only be useful
    # once it has an enrolment token and something to enrol with - and a unit
    # enabled before that would restart-loop until it got one.
    if [[ "$TLS_MODE" == "letsencrypt" && -f /etc/systemd/system/vkai-cert-renew.timer ]]; then
        systemctl enable --now vkai-cert-renew.timer >/dev/null 2>&1 ||
            log_warn "Could not enable vkai-cert-renew.timer; renew manually with 'vkai cert renew'."
        log_info "Daily certificate renewal timer enabled (vkai-cert-renew.timer)."
    fi
    if [[ -f /etc/systemd/system/vkai-upgrade-check.timer ]]; then
        systemctl enable --now vkai-upgrade-check.timer >/dev/null 2>&1 ||
            log_warn "Could not enable vkai-upgrade-check.timer; check manually with 'vkai upgrade --check'."
        log_info "Daily update CHECK timer enabled (vkai-upgrade-check.timer). It never installs anything."
    fi
    log_info "vkai-api and vkai-ui enabled at boot."
}

# =============================================================================
# 14. nginx
# =============================================================================
# render_nginx_template <template> <output> - expands the __VKAI_*__ tokens and
# the block markers. Done in bash rather than sed so that certificate paths and
# multi line blocks need no escaping.
render_nginx_template() {
    local template="$1" output="$2"
    local listen_opts="" line
    [[ "$TLS_MODE" != "none" ]] && listen_opts=" ssl"

    local server_name="_"
    [[ -n "$PANEL_DOMAIN" ]] && server_name="$PANEL_DOMAIN"

    : >"$output"
    while IFS= read -r line || [[ -n "$line" ]]; do
        # A comment is documentation, not a token. The header of the template
        # shows how to render it by hand and therefore mentions __VKAI_TLS_BLOCK__
        # literally; expanding that line emitted the TLS block a second time at
        # http level, where nginx.conf already sets ssl_prefer_server_ciphers, and
        # nginx refused the whole configuration.
        if [[ "$line" =~ ^[[:space:]]*# ]]; then
            printf '%s\n' "$line" >>"$output"
            continue
        fi
        case "$line" in
            *__VKAI_TLS_BLOCK__*)
                if [[ "$TLS_MODE" != "none" ]]; then
                    {
                        echo "    # TLS for the panel only. Completely independent of the customer certificates."
                        echo "    ssl_certificate     ${PANEL_CERT};"
                        echo "    ssl_certificate_key ${PANEL_KEY};"
                        echo "    ssl_protocols       TLSv1.2 TLSv1.3;"
                        echo "    ssl_prefer_server_ciphers off;"
                        echo "    ssl_session_cache   shared:VKAIPanelTLS:4m;"
                        echo "    ssl_session_timeout 1h;"
                        echo "    add_header Strict-Transport-Security \"max-age=31536000\" always;"
                    } >>"$output"
                else
                    echo "    # TLS disabled at install time (--tls-mode none)." >>"$output"
                fi
                continue
                ;;
            *__VKAI_ALLOW_BLOCK__*)
                if [[ -n "$OPT_ALLOW_IPS" ]]; then
                    local entry
                    echo "    # Restricted by --allow-ip. The panel enforces the same list again." >>"$output"
                    IFS=',' read -r -a __entries <<<"$OPT_ALLOW_IPS"
                    for entry in "${__entries[@]}"; do
                        [[ -n "$entry" ]] || continue
                        echo "    allow ${entry};" >>"$output"
                    done
                    unset __entries
                    echo "    deny  all;" >>"$output"
                else
                    echo "    # No --allow-ip given: any source address may reach the entrance." >>"$output"
                fi
                continue
                ;;
        esac
        line="${line//__VKAI_PANEL_PORT__/$PANEL_PORT}"
        line="${line//__VKAI_LISTEN_OPTS__/$listen_opts}"
        line="${line//__VKAI_API_PORT__/$API_BIND_PORT}"
        line="${line//__VKAI_SERVER_NAME__/$server_name}"
        line="${line//__VKAI_LOG_DIR__/$LOG_DIR}"
        line="${line//__VKAI_ACME_WEBROOT__/$ACME_WEBROOT}"
        printf '%s\n' "$line" >>"$output"
    done <"$template"
}

setup_nginx() {
    log_step "Configuring nginx for the panel (port ${PANEL_PORT})"

    has nginx || { log_warn "nginx is not installed, the reverse proxy is skipped."; return 0; }

    local conf_dir="/etc/nginx/conf.d"
    [[ -d "$conf_dir" ]] || mkdir -p "$conf_dir"

    local template="${SRC_DIR}/deploy/nginx/vkai-panel.conf"
    [[ -f "$template" ]] || die "${template} not found"

    setup_nginx_acme

    local target="${conf_dir}/vkai-panel.conf"
    local backup=""
    if [[ -f "$target" ]]; then
        backup="${target}.vkai.bak"
        cp -a "$target" "$backup"
    fi

    render_nginx_template "$template" "$target"
    chmod 644 "$target"

    # Every directive that may appear only once per context must appear exactly
    # once. nginx -t catches a duplicate, but only as "directive is duplicate in
    # <file>:<line>", which says nothing about why the renderer produced two.
    local d n
    for d in ssl_certificate_key ssl_prefer_server_ciphers ssl_session_cache; do
        n=$(grep -cE "^[[:space:]]*${d}[[:space:]]" "$target" || true)
        if [[ "$n" -gt 1 ]]; then
            rm -f "$target"
            [[ -n "$backup" ]] && mv "$backup" "$target"
            die "The rendered nginx configuration contains ${d} ${n} times. A token was expanded more than once - check render_nginx_template against ${template}."
        fi
    done

    # Belt and braces: the upstreams MUST be loopback. The panel no longer runs
    # in a container, so any compose style service name is a leftover mistake.
    if ! grep -qE '^[[:space:]]*server[[:space:]]+127\.0\.0\.1:[0-9]+;' "$target"; then
        rm -f "$target"
        die "The rendered nginx configuration has no 127.0.0.1 upstream. Was ${template} modified?"
    fi

    # The single front door, asserted rather than assumed. Every request on the
    # panel port must reach the API, because the API is what enforces the
    # security entrance; a proxy_pass to the Next.js service from here would put
    # the interface - the login form included - back outside the gate, which is
    # exactly the defect this arrangement fixes.
    if grep -qE 'proxy_pass[[:space:]]+https?://vkai_ui' "$target"; then
        rm -f "$target"
        [[ -n "$backup" ]] && mv "$backup" "$target"
        die "${template} proxies to the UI directly. Everything on the panel port must go to the API, which is what enforces the security entrance."
    fi

    if ! nginx -t; then
        rm -f "$target"
        if [[ -n "$backup" ]]; then
            mv "$backup" "$target"
            log_warn "Restored the previous ${target}."
        fi
        die "nginx -t failed. The generated panel configuration was removed so the running nginx stays intact."
    fi
    rm -f "$backup"

    pkg_enable_service nginx || log_warn "nginx did not start."
    systemctl reload nginx 2>/dev/null || true
    log_info "nginx serves the panel on port ${PANEL_PORT} over ${PANEL_SCHEME}."
}

# Port 80 stays the customers' port, but the ACME HTTP-01 challenge for an IP
# identifier can only be answered there. Two artefacts are produced:
#   * an include fragment every customer vhost can pull in, and
#   * a port 80 server that answers the challenge when nothing else claims that
#     port as its default server.
setup_nginx_acme() {
    local conf_dir="/etc/nginx/conf.d"
    local fragment="${conf_dir}/vkai-acme-challenge.inc"

    # ".inc" is deliberately not ".conf": nginx includes conf.d/*.conf, so this
    # fragment is only active where a vhost includes it explicitly.
    cat >"$fragment" <<ACMEINC
# ${BRAND_NAME}: HTTP-01 challenge webroot.
# Include this from any vhost on port 80 that must be able to answer an ACME
# challenge:   include ${fragment};
location ^~ /.well-known/acme-challenge/ {
    root ${ACME_WEBROOT};
    default_type "text/plain";
    allow all;
    access_log off;
}
ACMEINC
    chmod 644 "$fragment"

    local server_conf="${conf_dir}/vkai-acme-challenge.conf"
    local default_server=""
    # A second "default_server" on port 80 is a fatal nginx error, so only claim
    # the role when nothing else has taken it.
    if ! nginx -T 2>/dev/null | grep -qE '^\s*listen\s+([0-9.]+:)?80(\s|;).*default_server'; then
        default_server=" default_server"
    fi

    # Name the hosts this block actually answers for instead of relying on "_".
    # The stock Debian and Ubuntu site already uses server_name _ on port 80, and
    # two blocks with the same name make nginx keep the first and log
    # "conflicting server name _, ignored" - a warning, not an error, so the
    # installation appeared to succeed while every ACME challenge quietly went to
    # /var/www/html and every certificate request failed. An explicit name is more
    # specific than the default server, so it wins the match without a conflict.
    local acme_names=""
    [[ -n "$PANEL_DOMAIN" ]] && acme_names="$PANEL_DOMAIN"
    [[ -n "${SERVER_IP:-}" ]] && acme_names="${acme_names:+$acme_names }${SERVER_IP}"
    [[ -n "$acme_names" ]] || acme_names="_"

    cat >"$server_conf" <<ACMESRV
# ${BRAND_NAME}: answers ACME HTTP-01 challenges on port 80.
#
# An identifier that is an IP ADDRESS can only be validated over HTTP-01 (port
# 80) or TLS-ALPN-01 (port 443); there is no DNS-01 for an IP. Port 443 belongs
# to the customer sites, so this small port 80 server is the only route to a
# publicly trusted certificate for the panel.
#
# It serves nothing but the challenge directory; everything else returns 404 so
# this block can never shadow a customer site.
server {
    listen 80${default_server};
    listen [::]:80${default_server};
    server_name ${acme_names};

    server_tokens off;
    access_log ${LOG_DIR}/acme-challenge.access.log;
    error_log  ${LOG_DIR}/acme-challenge.error.log warn;

    include ${fragment};

    location / {
        return 404;
    }
}
ACMESRV
    chmod 644 "$server_conf"

    if ! nginx -t >/dev/null 2>&1; then
        rm -f "$server_conf"
        log_warn "The port 80 ACME server conflicts with the existing nginx configuration; it was removed."
        log_warn "Add 'include ${fragment};' to whichever vhost owns port 80 instead."
        return 0
    fi
    log_info "ACME HTTP-01 on port 80 is served from ${ACME_CHALLENGE_DIR}."
}

setup_logrotate() {
    [[ -d /etc/logrotate.d ]] || return 0
    cat >/etc/logrotate.d/vkai-panel <<ROTATE
${LOG_DIR}/*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 0640 ${VKAI_USER} ${VKAI_GROUP}
    sharedscripts
    postrotate
        systemctl reload vkai-api >/dev/null 2>&1 || true
    endscript
}

${LOG_SITES_DIR}/*/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    create 0640 ${VKAI_USER} ${VKAI_GROUP}
    sharedscripts
    postrotate
        systemctl reload nginx >/dev/null 2>&1 || true
    endscript
}
ROTATE
    chmod 644 /etc/logrotate.d/vkai-panel
    log_info "Log rotation configured."
}

# =============================================================================
# 15. SELinux
# =============================================================================
setup_selinux() {
    has getenforce || return 0
    local mode
    mode="$(getenforce 2>/dev/null || echo Disabled)"
    if [[ "$mode" == "Disabled" ]]; then
        log_info "SELinux is disabled, no contexts to fix."
        return 0
    fi

    log_step "Adjusting SELinux (mode ${mode})"

    if has semanage; then
        # nginx can only bind the panel port once it is labelled http_port_t.
        if ! semanage port -l 2>/dev/null | grep -E '^http_port_t' | grep -qw "$PANEL_PORT"; then
            semanage port -a -t http_port_t -p tcp "$PANEL_PORT" 2>/dev/null ||
            semanage port -m -t http_port_t -p tcp "$PANEL_PORT" 2>/dev/null ||
            log_warn "Could not label port ${PANEL_PORT} as http_port_t."
        fi
        # Website directories: nginx reads and writes (uploads, cache).
        semanage fcontext -a -t httpd_sys_rw_content_t "${WWW_DIR}(/.*)?" 2>/dev/null ||
        semanage fcontext -m -t httpd_sys_rw_content_t "${WWW_DIR}(/.*)?" 2>/dev/null || true
        semanage fcontext -a -t httpd_log_t "${LOG_SITES_DIR}(/.*)?" 2>/dev/null ||
        semanage fcontext -m -t httpd_log_t "${LOG_SITES_DIR}(/.*)?" 2>/dev/null || true
    else
        log_warn "'semanage' is missing (package policycoreutils-python-utils). Port and context labelling skipped."
    fi

    if has restorecon; then
        restorecon -R "$WWW_DIR" >/dev/null 2>&1 || true
        restorecon -R "$LOG_SITES_DIR" >/dev/null 2>&1 || true
    fi

    if has setsebool; then
        # nginx has to reach the loopback upstreams (API and UI).
        setsebool -P httpd_can_network_connect 1 2>/dev/null ||
            log_warn "Could not set httpd_can_network_connect. nginx may fail to proxy."
    fi
    log_info "SELinux contexts adjusted."
}

# =============================================================================
# 16. Firewall
#
# Three ports matter: the panel port, 80 and 443. Port 80 is opened because an
# HTTP-01 challenge for an IP identifier is answered there and because customer
# sites need it; 443 is opened for the customer sites only - the panel never
# binds it.
# =============================================================================
setup_firewall() {
    if [[ "$OPT_NO_FIREWALL" == "true" ]]; then
        log_warn "--no-firewall: the firewall is left alone. Open ${PANEL_PORT}/tcp and 80/tcp yourself."
        FIREWALL_TOOL="skipped (--no-firewall)"
        FIREWALL_PORTS="none"
        return 0
    fi

    log_step "Opening the panel port, 80 and 443 on the firewall"
    FIREWALL_PORTS="${PANEL_PORT}/tcp (panel), 80/tcp (ACME HTTP-01 + customer sites), 443/tcp (customer sites)"

    if has ufw && ufw status 2>/dev/null | grep -qi '^Status: active'; then
        FIREWALL_TOOL="ufw"
        ufw allow "${PANEL_PORT}/tcp" >/dev/null 2>&1 || log_warn "ufw could not open port ${PANEL_PORT}."
        ufw allow 80/tcp  >/dev/null 2>&1 || log_warn "ufw could not open port 80."
        ufw allow 443/tcp >/dev/null 2>&1 || true
        log_info "ufw: opened ${FIREWALL_PORTS}."
    elif has firewall-cmd && systemctl is-active --quiet firewalld; then
        FIREWALL_TOOL="firewalld"
        firewall-cmd --permanent --add-port="${PANEL_PORT}/tcp" >/dev/null 2>&1 || log_warn "firewalld could not open port ${PANEL_PORT}."
        firewall-cmd --permanent --add-service=http  >/dev/null 2>&1 || log_warn "firewalld could not open port 80."
        firewall-cmd --permanent --add-service=https >/dev/null 2>&1 || true
        firewall-cmd --reload >/dev/null 2>&1 || true
        log_info "firewalld: opened ${FIREWALL_PORTS}."
    else
        FIREWALL_TOOL="none"
        FIREWALL_PORTS="not managed"
        log_warn "No active ufw and no running firewalld was found."
        log_warn "If this machine has another firewall (iptables/nftables, or a provider security group),"
        log_warn "open ${PANEL_PORT}/tcp and 80/tcp yourself: without 80 no ACME challenge can be answered,"
        log_warn "and without ${PANEL_PORT} you cannot reach the panel at all."
    fi

    if [[ -z "$OPT_ALLOW_IPS" ]]; then
        log_warn "Recommended: restrict port ${PANEL_PORT} to your administration IPs (--allow-ip)."
    fi
}

# =============================================================================
# 17. Let's Encrypt certificate for the panel
#
# The panel deliberately does not depend on the distribution's certbot: Ubuntu
# 24.04 still ships certbot 2.9.0, which knows nothing about ACME profiles, and
# a certificate for an IP address is only issued through the "shortlived"
# profile. certbot stays in charge of the CUSTOMER sites; the panel's own
# certificate is ordered by the panel itself.
# =============================================================================
acme_selftest() {
    local token file
    token="vkai-selftest-$(rand_hex 6)"
    file="${ACME_CHALLENGE_DIR}/${token}"
    printf 'vkai-acme-selftest\n' >"$file"
    chown "$VKAI_USER:$VKAI_GROUP" "$file"
    chmod 644 "$file"

    local ok="false"
    if curl -fsS --max-time 5 "http://127.0.0.1/.well-known/acme-challenge/${token}" 2>/dev/null | grep -q 'vkai-acme-selftest'; then
        ok="true"
        log_info "ACME self test: the challenge directory is served on port 80 of this machine."
    else
        log_warn "ACME self test: http://127.0.0.1/.well-known/acme-challenge/ is not served."
        log_warn "Whatever owns port 80 has to serve that path. Add this line to it:"
        log_warn "  include /etc/nginx/conf.d/vkai-acme-challenge.inc;"
    fi

    if [[ -n "$ACME_IDENTIFIER" ]]; then
        if curl -fsS --max-time 8 "http://${ACME_IDENTIFIER}/.well-known/acme-challenge/${token}" 2>/dev/null | grep -q 'vkai-acme-selftest'; then
            log_info "ACME self test: ${ACME_IDENTIFIER} answers the challenge from outside the loopback."
        else
            log_warn "ACME self test: http://${ACME_IDENTIFIER}/.well-known/acme-challenge/ did not answer."
            log_warn "Let's Encrypt validates from the internet, so port 80 must reach this host."
        fi
    fi

    rm -f "$file"
    [[ "$ok" == "true" ]]
}

request_acme_certificate() {
    [[ "$TLS_MODE" == "letsencrypt" ]] || return 0

    log_step "Requesting a Let's Encrypt certificate for ${ACME_IDENTIFIER}"
    ACME_STATUS="requested"

    if [[ "$OPT_ACME_STAGING" == "true" ]]; then
        log_warn "--acme-staging: the certificate comes from the staging CA and browsers will NOT trust it."
    fi
    if [[ "$ACME_PROFILE" == "$ACME_PROFILE_IP" ]]; then
        log_info "Identifier ${ACME_IDENTIFIER} is an IP address: profile '${ACME_PROFILE_IP}', roughly six day lifetime, daily renewal."
    fi

    acme_selftest || log_warn "Continuing anyway; the order is likely to fail validation."

    local ctl="/usr/local/bin/vkai-panelctl"
    if [[ ! -x "$ctl" ]] || ! "$ctl" panel cert --help >/dev/null 2>&1; then
        ACME_STATUS="not available in this build - self-signed certificate kept"
        log_warn "This build of vkai-panelctl has no 'panel cert' command."
        log_warn "The panel keeps its self-signed certificate. Everything an order needs is in place:"
        log_warn "  webroot   ${ACME_CHALLENGE_DIR}"
        log_warn "  directory ${ACME_DIRECTORY}"
        log_warn "  profile   ${ACME_PROFILE}"
        log_warn "Order it from the panel UI once the feature is available."
        return 0
    fi

    if "$ctl" panel cert issue \
            --identifier "$ACME_IDENTIFIER" \
            --email "$ADMIN_EMAIL" \
            --webroot "$ACME_WEBROOT" \
            --directory "$ACME_DIRECTORY" \
            --profile "$ACME_PROFILE" \
            --cert "$PANEL_CERT" \
            --key "$PANEL_KEY" \
            --agree-tos; then
        chown "$VKAI_USER:$VKAI_GROUP" "$PANEL_CERT" "$PANEL_KEY" 2>/dev/null || true
        chmod 644 "$PANEL_CERT"; chmod 640 "$PANEL_KEY"
        if [[ "$OPT_ACME_STAGING" == "true" ]]; then
            CERT_SOURCE="Let's Encrypt (staging)"
        else
            CERT_SOURCE="Let's Encrypt"
        fi
        CERT_FINGERPRINT="$(cert_fingerprint "$PANEL_CERT")"
        CERT_EXPIRY="$(cert_expiry "$PANEL_CERT")"
        ACME_STATUS="issued for ${ACME_IDENTIFIER} (profile ${ACME_PROFILE})"
        log_info "Certificate issued, expires ${CERT_EXPIRY}."
        if nginx -t >/dev/null 2>&1; then
            systemctl reload nginx >/dev/null 2>&1 || true
        fi
    else
        ACME_STATUS="order failed - self-signed certificate kept"
        log_warn "The certificate order failed. The panel keeps its self-signed certificate and stays reachable."
        log_warn "Retry later with: vkai cert issue"
    fi
}

# =============================================================================
# 18. Start and health check
# =============================================================================
start_services() {
    log_step "Starting the services"

    systemctl restart vkai-api || die "vkai-api did not start. Inspect: journalctl -u vkai-api -n 80"
    systemctl restart vkai-ui  || die "vkai-ui did not start. Inspect: journalctl -u vkai-ui -n 80"

    local i ok="false"
    for i in $(seq 1 30); do
        if curl -fsS --max-time 3 "http://127.0.0.1:${API_BIND_PORT}/health" >/dev/null 2>&1; then
            ok="true"; break
        fi
        sleep 1
    done
    if [[ "$ok" == "true" ]]; then
        log_info "The API answers /health."
    else
        log_warn "The API did not answer /health within 30 seconds. Inspect: journalctl -u vkai-api -n 80"
    fi

    ok="false"
    for i in $(seq 1 30); do
        if curl -fsS --max-time 3 -o /dev/null "http://127.0.0.1:${UI_BIND_PORT}/" 2>/dev/null; then
            ok="true"; break
        fi
        sleep 1
    done
    if [[ "$ok" == "true" ]]; then
        log_info "The UI answers on port ${UI_BIND_PORT}."
    else
        log_warn "The UI did not answer. Inspect: journalctl -u vkai-ui -n 80"
    fi
}

# =============================================================================
# 18c. The panel host becomes the first managed node
#
# Until this step the panel is a control plane with nothing under it: the
# services run, an administrator can sign in, and there is not one machine to
# create a website on - not even this one. aaPanel, which this product is
# measured against, installs onto a VPS and manages THAT VPS. This step is what
# makes that true here.
#
# It is deliberately NON FATAL. Everything above has already produced a working
# panel; refusing to finish the install because the node could not be registered
# would trade a panel that works and says what is missing for one that does not
# exist. Every failure path below records why in ${NODE_STATUS}, which the final
# table prints along with the single command that resumes the work.
#
# It runs AFTER start_services on purpose. The registration mints a single-use
# enrolment token through the running API, because the API process owns the
# certificate authority's state: it holds it in memory and rewrites the file on
# every change, so a token written underneath it would be invisible to it and
# then overwritten. The panel has to be up for the token to exist at all.
# =============================================================================
register_local_node() {
    log_step "Registering this machine as the first managed node"

    # The same value the node row holds: internal/localnode reads os.Hostname(),
    # which is what "hostname" prints and not what "hostname -f" resolves to.
    NODE_HOSTNAME="$(hostname 2>/dev/null || echo unknown)"

    local bin="" candidate
    for candidate in /usr/local/bin/vkai-cli "${CORE_DIR}/bin/vkai-cli"; do
        if [[ -x "$candidate" ]]; then bin="$candidate"; break; fi
    done
    if [[ -z "$bin" ]]; then
        NODE_STATUS="NOT REGISTERED - vkai-cli was not built on this machine"
        log_warn "vkai-cli is missing, so this machine was not registered."
        log_warn "Register it later with: sudo vkai node register"
        return 0
    fi

    local out="" rc=0
    out="$("$bin" node register --timeout 120s 2>&1)" || rc=$?

    # The command's own output is the record of what happened; it goes into the
    # install log line by line so a failure a week later can be read there.
    local line
    while IFS= read -r line; do
        if [[ -n "$line" ]]; then log_info "  ${line}"; fi
    done <<<"$out"

    NODE_ID="$(json_field_from_file "$NODE_RECORD_FILE" node_id)"
    [[ -n "$NODE_ID" ]] || NODE_ID="(none)"

    case "$rc" in
        0)
            NODE_STATUS="registered and its agent is enrolled"
            log_info "This machine is under management (server ${NODE_ID})."
            ;;
        3)
            # The row exists, the agent does not hold a certificate yet. That is
            # the one half worth separating: the row is what an operator cannot
            # safely reproduce by hand, the enrolment is a retry.
            NODE_STATUS="registered, agent NOT enrolled - run: sudo vkai node register"
            log_warn "This machine is registered but its agent has not enrolled."
            log_warn "Retry with: sudo vkai node register"
            ;;
        *)
            NODE_STATUS="NOT REGISTERED - run: sudo vkai node register"
            log_warn "This machine could not be registered (exit ${rc})."
            log_warn "The panel is installed and usable; register the node with: sudo vkai node register"
            ;;
    esac
    return 0
}

install_vkai_cli() {
    local src="${SRC_DIR}/deploy/vkai.sh"
    [[ -f "$src" ]] || die "${src} not found"
    install -m 0755 "$src" /usr/local/bin/vkai
    log_info "Administration command: vkai (/usr/local/bin/vkai)"
}

# =============================================================================
# 18b. Deployment entry point for CI
# =============================================================================
# The CI deploy user must be able to publish a release without holding general
# root. Granting "sudo bash <script from the uploaded package>" would be exactly
# general root wearing a narrower label, because that user controls the file.
#
# Instead the release script is installed here, owned by root in a directory the
# deploy user cannot write, and sudo is granted for that one path only.
install_deploy_entrypoint() {
    local src="${SRC_DIR}/deploy/scripts/deploy.sh"
    [[ -f "$src" ]] || { log_warn "${src} not found; skipping the CI deploy entry point."; return 0; }

    install -d -o root -g root -m 0755 "$BIN_DIR"
    install -o root -g root -m 0755 "$src" "${BIN_DIR}/vkai-deploy"
    log_info "Deployment entry point: ${BIN_DIR}/vkai-deploy (root owned)"
}

# setup_ci_deploy_user creates the unprivileged account GitHub Actions logs in as.
# Only runs when --ci-deploy-user was given.
setup_ci_deploy_user() {
    [[ -n "${OPT_CI_USER:-}" ]] || return 0
    local user="$OPT_CI_USER"

    id "$user" >/dev/null 2>&1 || useradd --create-home --shell /bin/bash "$user"
    usermod -aG "$VKAI_GROUP" "$user"

    if [[ -n "${OPT_CI_PUBKEY:-}" ]]; then
        local ssh_dir="/home/${user}/.ssh"
        install -d -o "$user" -g "$user" -m 0700 "$ssh_dir"
        touch "${ssh_dir}/authorized_keys"
        grep -qxF "$OPT_CI_PUBKEY" "${ssh_dir}/authorized_keys" 2>/dev/null \
            || printf '%s\n' "$OPT_CI_PUBKEY" >>"${ssh_dir}/authorized_keys"
        chown "$user:$user" "${ssh_dir}/authorized_keys"
        chmod 0600 "${ssh_dir}/authorized_keys"
    fi

    # One command, one fixed path, one argument shape. The glob cannot match a
    # slash, so it cannot be walked out of /tmp.
    cat >"/etc/sudoers.d/${user}" <<SUDOERS
# ${BRAND_NAME}: CI deployment. This account may publish a release and nothing else.
${user} ALL=(root) NOPASSWD: ${BIN_DIR}/vkai-deploy deploy /tmp/vkai-panel-*.tar.gz
${user} ALL=(root) NOPASSWD: ${BIN_DIR}/vkai-deploy rollback
${user} ALL=(root) NOPASSWD: ${BIN_DIR}/vkai-deploy status
SUDOERS
    chmod 0440 "/etc/sudoers.d/${user}"
    visudo -c -f "/etc/sudoers.d/${user}" >/dev/null || die "Generated sudoers file for ${user} is invalid"

    log_info "CI deploy user: ${user} (may run ${BIN_DIR}/vkai-deploy only)"
}

# =============================================================================
# 19. Final access table
# =============================================================================
print_summary() {
    local url="${PANEL_SCHEME}://${PANEL_DOMAIN:-$SERVER_IP}:${PANEL_PORT}${PANEL_ENTRANCE}/"
    local pass_note=""
    [[ "$ADMIN_PASS_CHANGED" == "true" ]] || pass_note="  (!) This is the DEFAULT password - change it immediately after logging in."
    local tls_note="self-signed: your browser will warn. Compare the fingerprint below before accepting."
    case "$CERT_SOURCE" in
        "Let's Encrypt"*) tls_note="publicly trusted; renewed automatically by vkai-cert-renew.timer (twice a day)." ;;
        none)             tls_note="TLS is switched off (--tls-mode none): traffic is plain HTTP." ;;
    esac

    local summary
    summary="$(cat <<SUM
=============================================================================
 ${BRAND_NAME} v${VKAI_VERSION} - INSTALLATION COMPLETE (${INSTALL_MODE})
 ${BRAND_ORG}
 Finished: $(ts)
 System  : ${OS_PRETTY} (${ARCH})
=============================================================================

PANEL ACCESS
  Full URL   : ${url}
  Port       : ${PANEL_PORT}   (80/443 stay reserved for the customer websites)
  Entrance   : ${PANEL_ENTRANCE}
  Domain     : ${PANEL_DOMAIN:-(none - reached by IP ${SERVER_IP})}
  Allowed IPs: ${OPT_ALLOW_IPS:-any source address}
  Any other path returns a neutral 404. That is deliberate.

ADMINISTRATOR
  Username : ${ADMIN_USER}
  Password : ${ADMIN_PASS}
  E-mail   : ${ADMIN_EMAIL}
${pass_note}

CERTIFICATE
  Mode        : ${TLS_MODE}
  Source      : ${CERT_SOURCE}
  Expires     : ${CERT_EXPIRY}
  SHA-256     : ${CERT_FINGERPRINT}
  ACME status : ${ACME_STATUS}
  ACME webroot: ${ACME_CHALLENGE_DIR}
  Note        : ${tls_note}

DATA PATHS
  Install root      : ${PANEL_ROOT}
  Running build     : ${CURRENT_LINK} -> ${PANEL_ROOT}
  Releases          : ${RELEASES_DIR}        (deploy/scripts/deploy.sh)
  API binaries      : ${CORE_DIR}            (bin/vkai-api)
  UI build          : ${UI_DIR}              (.next/standalone/server.js)
  Agent             : ${AGENT_DIR}           (bin/vkai-agent)
  Website documents : ${WWW_DOMAINS_DIR}/<domain>
  Backups           : ${WWW_BACKUP_DIR}
  Default site      : ${WWW_DEFAULT_DIR}
  Configuration     : ${ENV_FILE}
  Agent settings    : ${AGENT_ENV_FILE}     (this node's agent: panel URL, state dir)
  Node identity     : ${NODE_RECORD_FILE}     (which servers row is this machine)
  Agent identity    : ${SSL_DIR}/agent      (this node's key and certificate)
  Version record    : ${VERSION_FILE}      (version ${VKAI_VERSION}, channel ${OPT_CHANNEL})
  Panel logs        : ${LOG_DIR}
  Per site logs     : ${LOG_SITES_DIR}/<domain>
  Certificates      : ${SSL_DIR}
  Temporary         : ${TMP_DIR}
  Install log       : ${INSTALL_LOG}

THIS MACHINE IS THE FIRST MANAGED NODE
  Node       : ${NODE_HOSTNAME} (${SERVER_IP})
  Server ID  : ${NODE_ID}
  State      : ${NODE_STATUS}
  Role       : panel       (the machine this panel runs on)
  Create a website, a database or a certificate here now - no second machine
  is needed. Inspect or repair the node with:
      vkai node list
      sudo vkai node register
  Add ANOTHER machine later (optional, this is not a precondition for anything):
      1. In the panel: Servers -> Add agent, which mints a single-use token.
      2. On that machine: install the agent, put the token in
         VKAI_AGENT_ENROLMENT_TOKEN and start vkai-agent.
      See docs/INSTALL.md and docs/AGENT_CHANNEL.md.

SERVICES
  vkai-api   -> ${CURRENT_LINK}/core/bin/vkai-api   (loopback:${API_BIND_PORT})
  vkai-ui    -> ${CURRENT_LINK}/panel/.next/standalone/server.js (loopback:${UI_BIND_PORT})
  vkai-agent -> ${CURRENT_LINK}/agent/bin/vkai-agent (this node's agent, port ${AGENT_PORT})
  nginx      -> reverse proxy on port ${PANEL_PORT}, ACME challenge on port 80
  Database   : ${DB_NAME} / role ${DB_USER}
  Redis      : ${REDIS_SERVICE:-unknown}
  Firewall   : ${FIREWALL_TOOL} - ${FIREWALL_PORTS}

UPDATES
  Version    : ${VKAI_VERSION} on the "${OPT_CHANNEL}" channel
  Daily check: vkai-upgrade-check.timer - it CHECKS ONLY and never installs.
               The result lands in ${ETC_DIR}/upgrade-check.json and in the panel.
  Upgrading  : sudo vkai upgrade    (see docs/UPGRADE.md)

THE "vkai" COMMAND
  vkai status              Service status
  vkai start|stop|restart  Control the services
  vkai logs api|ui|agent   Follow the logs
  vkai info                Print the access details again
  vkai port 9001           Change the panel port
  vkai entrance random     Generate a new security entrance
  vkai cert renew          Renew the panel certificate
  vkai version             Installed version, channel and running release
  vkai upgrade --check     Is a newer release available? (changes nothing)
  vkai upgrade             Upgrade the panel, after showing what it will do
  vkai node list           The machines this panel manages
  vkai node register       Register/repair THIS machine as a managed node
  vkai backup              Back up the database and the configuration
  vkai update              Rebuild and restart
  vkai uninstall           Remove the panel

REMEMBER
  * Write down the URL and the password above. The password is not recoverable.
  * A copy of this table: ${SUMMARY_FILE} (mode 600)
  * Restrict port ${PANEL_PORT} to your administration IPs.
  * Change the administrator password and enable 2FA at first login.
=============================================================================
SUM
)"

    printf '\n%s%s%s\n' "$C_GREEN" "$summary" "$C_OFF"
    console ""
    console "$summary"

    umask 077
    printf '%s\n' "$summary" >"$SUMMARY_FILE"
    chown "$VKAI_USER:$VKAI_GROUP" "$SUMMARY_FILE"
    chmod 600 "$SUMMARY_FILE"
}

# =============================================================================
# 20. Uninstall
#
# Services, binaries and configuration go. Customer data under ${WWW_DIR} and
# the database survive unless --purge is given as well.
# =============================================================================
do_uninstall() {
    banner
    log_step "Uninstalling ${BRAND_NAME}"
    if [[ "$OPT_PURGE" == "true" ]]; then
        log_warn "--purge: the database, ${WWW_DIR} and the ${VKAI_USER} account WILL be removed."
    else
        log_info "Customer data under ${WWW_DIR}, the database and the certificates are KEPT."
        log_info "Add --purge to remove them as well."
    fi

    log_step "Stopping the services"
    local svc
    for svc in vkai-ui vkai-api vkai-agent vkai-cert-renew.timer vkai-cert-renew \
               vkai-upgrade-check.timer vkai-upgrade-check; do
        systemctl disable --now "$svc" >/dev/null 2>&1 || true
    done
    for svc in vkai-ui vkai-api vkai-agent vkai-cert-renew.service vkai-cert-renew.timer \
               vkai-upgrade-check.service vkai-upgrade-check.timer; do
        rm -f "/etc/systemd/system/${svc}" "/etc/systemd/system/${svc}.service"
    done
    for svc in vkai-frontend vkai-panel-api vkai-panel-frontend; do
        systemctl disable --now "$svc" >/dev/null 2>&1 || true
        rm -f "/etc/systemd/system/${svc}.service"
    done
    systemctl daemon-reload

    log_step "Removing the nginx configuration"
    rm -f /etc/nginx/conf.d/vkai-panel.conf
    rm -f /etc/nginx/conf.d/vkai-acme-challenge.conf
    rm -f /etc/nginx/conf.d/vkai-acme-challenge.inc
    rm -f /etc/nginx/sites-available/vkai-panel /etc/nginx/sites-enabled/vkai-panel
    if has nginx && nginx -t >/dev/null 2>&1; then
        systemctl reload nginx >/dev/null 2>&1 || true
    fi

    rm -f /etc/logrotate.d/vkai-panel /etc/profile.d/vkai-golang.sh
    rm -f /usr/local/bin/vkai /usr/local/bin/vkai-panelctl /usr/local/bin/vkai-cli
    [[ -L /etc/vkai ]] && rm -f /etc/vkai

    if [[ "$OPT_PURGE" == "true" ]]; then
        log_step "Purging the database and all data"
        local name user
        name="$(env_get VKAI_DB_NAME || echo "$DB_NAME")"
        user="$(env_get VKAI_DB_USER || echo "$DB_USER")"
        sudo -u postgres psql -qc "DROP DATABASE IF EXISTS \"${name}\";" 2>/dev/null || true
        sudo -u postgres psql -qc "DROP ROLE IF EXISTS \"${user}\";" 2>/dev/null || true
        rm -rf "$PANEL_ROOT"
        userdel "$VKAI_USER" 2>/dev/null || true
        groupdel "$VKAI_GROUP" 2>/dev/null || true
        printf '\n%s%s has been removed, including the database, %s and the %s account.%s\n\n' \
            "$C_GREEN" "$BRAND_NAME" "$WWW_DIR" "$VKAI_USER" "$C_OFF"
    else
        log_step "Removing the program files only"
        rm -rf "$CORE_DIR" "$UI_DIR" "$AGENT_DIR" "$RELEASES_DIR"
        rm -f "$CURRENT_LINK"
        printf '\n%s%s has been removed.%s\n' "$C_GREEN" "$BRAND_NAME" "$C_OFF"
        printf 'KEPT (no customer data was touched):\n'
        printf '  %s   website documents and backups\n' "$WWW_DIR"
        printf '  %s   configuration and secrets\n' "$ETC_DIR"
        printf '  %s   logs\n' "$LOG_DIR"
        printf '  %s   certificates\n' "$SSL_DIR"
        printf '  database "%s" and role "%s"\n' "$DB_NAME" "$DB_USER"
        printf 'Run again with --purge to remove those as well.\n\n'
    fi
}

# =============================================================================
# Main
# =============================================================================
main() {
    parse_args "$@"
    preflight_root
    detect_arch
    detect_os
    resolve_os_family
    detect_pkg_manager

    if [[ "$OPT_UNINSTALL" == "true" ]]; then
        do_uninstall
        exit 0
    fi

    banner
    preflight_systemd
    preflight_resources
    preflight_other_panels

    # The log directory can only be created once the install is going ahead.
    mkdir -p "$LOG_DIR"
    start_logging
    detect_install_mode
    log_info "Options: port='${OPT_PORT:-default}' entrance='${OPT_ENTRANCE:-generated}' domain='${OPT_DOMAIN:-none}' channel=${OPT_CHANNEL} tls-mode=${OPT_TLS_MODE} acme-staging=${OPT_ACME_STAGING} allow-ip='${OPT_ALLOW_IPS:-any}' skip-deps=${OPT_SKIP_DEPS} no-firewall=${OPT_NO_FIREWALL} quiet=${OPT_QUIET}"

    bootstrap_sources
    read_product_version ||
        die "${SRC_DIR}/VERSION not found or empty. It is the single source of truth for the product version."
    log_info "Installing ${BRAND_NAME} v${VKAI_VERSION} (from ${SRC_DIR}/VERSION)."
    install_dependencies
    install_golang
    install_nodejs
    setup_user
    setup_directories

    # The panel port is fixed before anything checks whether it is free.
    resolve_panel_access
    preflight_ports

    sync_sources

    # >>> MANDATORY ORDER: configuration FIRST, UI build AFTER. <<<
    # NEXT_PUBLIC_API_URL is inlined into the bundle by "npm run build".
    setup_config
    setup_certificate

    setup_database
    run_migrations
    setup_redis

    build_core
    build_agent
    build_ui           # reads NEXT_PUBLIC_API_URL from the .env written above
    setup_current_link # the systemd units run through /vkai-panel/current
    record_version     # /vkai-panel/etc/version.json: what "vkai upgrade" reads

    setup_admin_account
    install_vkai_cli     # the renewal unit calls /usr/local/bin/vkai
    install_deploy_entrypoint
    setup_ci_deploy_user
    install_systemd_units
    setup_nginx
    setup_logrotate
    setup_selinux
    setup_firewall
    request_acme_certificate
    start_services

    # The panel is up. Now make the machine it runs on a node it manages.
    register_local_node

    print_summary
    cleanup_bootstrap
}

main "$@"
