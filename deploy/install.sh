#!/usr/bin/env bash
# =============================================================================
# VKAI Panel - bo cai dat tu dong, ho tro nhieu he dieu hanh
# HiTech Cloud (hitechcloud.vn)
#
#   bash deploy/install.sh                      # cai mac dinh (cong 8888)
#   bash deploy/install.sh --port 9001 --yes    # khong hoi, cong 9001
#   bash deploy/install.sh --uninstall          # go cai dat
#
# Cong 80/443 danh RIENG cho website cua khach. Panel luon nghe tren cong rieng
# (mac dinh 8888) kem mot loi vao an toan dang /vkai_a1b2c3d4.
# =============================================================================

set -Eeuo pipefail

# -----------------------------------------------------------------------------
# Hang so
# -----------------------------------------------------------------------------
readonly VKAI_VERSION="1.0.0"
readonly BRAND_NAME="VKAI Panel"
readonly BRAND_ORG="HiTech Cloud (hitechcloud.vn)"

readonly PANEL_ROOT="/vkai-panel"
readonly CORE_DIR="${PANEL_ROOT}/core"
readonly UI_DIR="${PANEL_ROOT}/panel"
readonly AGENT_DIR="${PANEL_ROOT}/agent"
readonly WWW_DIR="${PANEL_ROOT}/www"
readonly WWW_DOMAINS_DIR="${WWW_DIR}/domains"
readonly WWW_BACKUP_DIR="${WWW_DIR}/backup"
readonly WWW_DEFAULT_DIR="${WWW_DIR}/default"
readonly ETC_DIR="${PANEL_ROOT}/etc"
readonly LOG_DIR="${PANEL_ROOT}/logs"
readonly LOG_SITES_DIR="${LOG_DIR}/sites"
readonly SSL_DIR="${PANEL_ROOT}/ssl"
readonly TMP_DIR="${PANEL_ROOT}/tmp"

readonly ENV_FILE="${ETC_DIR}/.env"
readonly PANEL_STATE_FILE="${ETC_DIR}/panel_access.json"
readonly SUMMARY_FILE="${ETC_DIR}/install-summary.txt"
readonly MIGRATIONS_STATE="${ETC_DIR}/migrations.applied"
readonly INSTALL_LOG="${LOG_DIR}/install.log"

readonly VKAI_USER="vkai"
readonly VKAI_GROUP="vkai"

# Phien ban runtime tai ve khi may chua co ban dung.
readonly GO_VERSION="1.22.5"
readonly GO_MIN_MINOR="22"
readonly NODE_VERSION="20.18.1"
readonly NODE_MIN_MAJOR="20"

# Cong noi bo: chi nghe tren loopback, nginx dung lam upstream.
readonly API_BIND_PORT="30110"
readonly UI_BIND_PORT="3000"
readonly AGENT_PORT="30111"

readonly DEFAULT_PANEL_PORT="8888"

# Yeu cau toi thieu.
readonly MIN_RAM_MB="900"
readonly MIN_DISK_MB="5120"

# Thu muc ma nguon (thu muc cha cua deploy/).
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly SRC_DIR

# -----------------------------------------------------------------------------
# Tuy chon dong lenh (gia tri mac dinh)
# -----------------------------------------------------------------------------
OPT_PORT=""
OPT_ENTRANCE=""
OPT_RANDOM_PORT="false"
OPT_NO_FIREWALL="false"
OPT_SKIP_DEPS="false"
OPT_ASSUME_YES="false"
OPT_UNINSTALL="false"
OPT_FORCE_OS="false"
OPT_SKIP_CHECKSUM="false"
OPT_ADMIN_USER="admin"
OPT_API_URL=""

# -----------------------------------------------------------------------------
# Trang thai duoc dien trong qua trinh chay
# -----------------------------------------------------------------------------
OS_ID=""; OS_VERSION_ID=""; OS_MAJOR=""; OS_LIKE=""; OS_PRETTY=""
OS_FAMILY=""          # debian | rhel | suse
PKG=""                # apt-get | dnf | yum | zypper
ARCH=""; NODE_ARCH=""; GO_ARCH=""
PANEL_PORT=""
PANEL_ENTRANCE=""
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
FIREWALL_TOOL="none"
LOGGING_STARTED="false"

# -----------------------------------------------------------------------------
# Mau sac + ghi log
# -----------------------------------------------------------------------------
if [[ -t 1 ]]; then
    C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'; C_YELLOW=$'\033[1;33m'
    C_BLUE=$'\033[0;34m'; C_CYAN=$'\033[0;36m'; C_BOLD=$'\033[1m'; C_OFF=$'\033[0m'
else
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_CYAN=""; C_BOLD=""; C_OFF=""
fi

ts() { date '+%Y-%m-%d %H:%M:%S'; }

log_info()  { printf '%s[INFO ]%s %s %s\n' "$C_GREEN"  "$C_OFF" "$(ts)" "$*"; }
log_step()  { printf '%s[BUOC ]%s %s %s%s%s\n' "$C_BLUE" "$C_OFF" "$(ts)" "$C_BOLD" "$*" "$C_OFF"; }
log_warn()  { printf '%s[CANH ]%s %s %s\n' "$C_YELLOW" "$C_OFF" "$(ts)" "$*" >&2; }
log_error() { printf '%s[LOI  ]%s %s %s\n' "$C_RED"    "$C_OFF" "$(ts)" "$*" >&2; }

die() {
    log_error "$*"
    exit 1
}

on_error() {
    local exit_code=$1 line=$2 cmd=$3
    printf '\n'
    log_error "Cai dat that bai o dong ${line} (ma loi ${exit_code})."
    log_error "Lenh: ${cmd}"
    if [[ "$LOGGING_STARTED" == "true" ]]; then
        log_error "Nhat ky day du: ${INSTALL_LOG}"
    fi
    log_error "He dieu hanh: ${OS_PRETTY:-khong xac dinh} | kien truc: ${ARCH:-khong xac dinh}"
    exit "$exit_code"
}
trap 'on_error $? $LINENO "$BASH_COMMAND"' ERR

start_logging() {
    mkdir -p "$LOG_DIR"
    touch "$INSTALL_LOG"
    chmod 640 "$INSTALL_LOG"
    exec > >(tee -a "$INSTALL_LOG") 2>&1
    LOGGING_STARTED="true"
    printf '\n===== %s: bat dau cai dat %s v%s =====\n' "$(ts)" "$BRAND_NAME" "$VKAI_VERSION"
}

has() { command -v "$1" >/dev/null 2>&1; }

confirm() {
    local prompt="$1"
    [[ "$OPT_ASSUME_YES" == "true" ]] && return 0
    if [[ ! -t 0 ]]; then
        die "Khong co terminal de xac nhan. Chay lai voi --yes."
    fi
    local answer
    read -r -p "${prompt} [y/N]: " answer
    [[ "$answer" =~ ^([yY]|[yY][eE][sS])$ ]]
}

rand_hex()   { openssl rand -hex "$1"; }
rand_alnum() { LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c "$1"; }

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
    printf '  Cai dat tai %s | cong panel rieng, khong dung 80/443\n\n' "$PANEL_ROOT"
}

usage() {
    cat <<USAGE
${BRAND_NAME} v${VKAI_VERSION} - bo cai dat da he dieu hanh
${BRAND_ORG}

Cach dung:
  sudo bash deploy/install.sh [tuy chon]

Tuy chon:
  --port <so>          Cong truy cap panel (mac dinh ${DEFAULT_PANEL_PORT}). Khong chap nhan 80/443.
  --random-port        Sinh cong ngau nhien trong khoang 10000-60000.
  --entrance <duong>   Loi vao an toan, vi du /vkai_a1b2c3d4. Bo trong = sinh ngau nhien.
  --admin-user <ten>   Ten tai khoan quan tri dau tien (mac dinh admin).
  --api-url <url>      Gan cung NEXT_PUBLIC_API_URL. Bo trong = same-origin qua nginx.
  --no-firewall        Khong dong toi tuong lua (ufw/firewalld).
  --skip-deps          Bo qua buoc cai goi he thong (dung khi da chuan bi san).
  --skip-checksum      Bo qua kiem tra checksum khi tai Go/Node (chi dung khi buoc phai).
  --force-os           Van cai tren phien ban OS ngoai ma tran ho tro.
  -y, --yes            Khong hoi lai, tra loi "co" cho moi xac nhan.
  --uninstall          Go cai dat panel (hoi truoc khi xoa du lieu).
  -h, --help           In tro giup nay.
  -v, --version        In phien ban.

He dieu hanh ho tro:
  Ubuntu 20.04 / 22.04 / 24.04       Debian 11 / 12
  CentOS Stream 8 / 9                RHEL 8 / 9
  Rocky Linux 8 / 9                  AlmaLinux 8 / 9
  Fedora 38+                         openSUSE Leap 15.x
  Amazon Linux 2023
  Kien truc: x86_64 (amd64) va aarch64 (arm64)
USAGE
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --port)         [[ $# -ge 2 ]] || die "--port thieu gia tri"; OPT_PORT="$2"; shift 2 ;;
            --port=*)       OPT_PORT="${1#*=}"; shift ;;
            --random-port)  OPT_RANDOM_PORT="true"; shift ;;
            --entrance)     [[ $# -ge 2 ]] || die "--entrance thieu gia tri"; OPT_ENTRANCE="$2"; shift 2 ;;
            --entrance=*)   OPT_ENTRANCE="${1#*=}"; shift ;;
            --admin-user)   [[ $# -ge 2 ]] || die "--admin-user thieu gia tri"; OPT_ADMIN_USER="$2"; shift 2 ;;
            --admin-user=*) OPT_ADMIN_USER="${1#*=}"; shift ;;
            --api-url)      [[ $# -ge 2 ]] || die "--api-url thieu gia tri"; OPT_API_URL="$2"; shift 2 ;;
            --api-url=*)    OPT_API_URL="${1#*=}"; shift ;;
            --no-firewall)  OPT_NO_FIREWALL="true"; shift ;;
            --skip-deps)    OPT_SKIP_DEPS="true"; shift ;;
            --skip-checksum) OPT_SKIP_CHECKSUM="true"; shift ;;
            --force-os)     OPT_FORCE_OS="true"; shift ;;
            -y|--yes)       OPT_ASSUME_YES="true"; shift ;;
            --uninstall)    OPT_UNINSTALL="true"; shift ;;
            -h|--help)      usage; exit 0 ;;
            -v|--version)   printf '%s v%s\n' "$BRAND_NAME" "$VKAI_VERSION"; exit 0 ;;
            *)              usage >&2; die "Tuy chon khong hop le: $1" ;;
        esac
    done

    if [[ -n "$OPT_PORT" ]]; then
        [[ "$OPT_PORT" =~ ^[0-9]+$ ]] || die "--port phai la so: '$OPT_PORT'"
        (( OPT_PORT >= 1 && OPT_PORT <= 65535 )) || die "--port ngoai khoang 1-65535: $OPT_PORT"
        if [[ "$OPT_PORT" == "80" || "$OPT_PORT" == "443" ]]; then
            die "Cong 80/443 danh rieng cho website cua khach. Chon cong khac, vi du ${DEFAULT_PANEL_PORT}."
        fi
    fi
    if [[ -n "$OPT_ENTRANCE" ]]; then
        [[ "$OPT_ENTRANCE" == /* ]] || OPT_ENTRANCE="/${OPT_ENTRANCE}"
        [[ "$OPT_ENTRANCE" =~ ^/[A-Za-z0-9_-]{4,64}$ ]] ||
            die "--entrance chi cho phep chu, so, '-' va '_', dai 4-64 ky tu. Vi du: /vkai_a1b2c3d4"
    fi
    [[ "$OPT_ADMIN_USER" =~ ^[A-Za-z0-9_.-]{3,32}$ ]] ||
        die "--admin-user chi cho phep chu, so, '.', '-', '_', dai 3-32 ky tu."
}

# =============================================================================
# 1. Nhan dien he dieu hanh va kien truc
# =============================================================================
detect_arch() {
    local machine
    machine="$(uname -m)"
    case "$machine" in
        x86_64|amd64)   ARCH="amd64"; GO_ARCH="amd64"; NODE_ARCH="x64" ;;
        aarch64|arm64)  ARCH="arm64"; GO_ARCH="arm64"; NODE_ARCH="arm64" ;;
        *)
            die "Kien truc '${machine}' khong duoc ho tro. ${BRAND_NAME} chi chay tren x86_64 va aarch64."
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
        # Chi con cac ban RHEL rat cu moi thieu /etc/os-release.
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
        die "Khong doc duoc /etc/os-release lan /etc/redhat-release nen khong biet day la he dieu hanh nao.
Bo cai KHONG doan bua. Hay cai thu cong theo huong dan deploy/README.md."
    fi

    [[ -n "$OS_ID" ]] || die "Khong xac dinh duoc ID he dieu hanh trong /etc/os-release."
    OS_MAJOR="${OS_VERSION_ID%%.*}"
    [[ -n "$OS_MAJOR" ]] || OS_MAJOR="0"
}

# Xac dinh ho OS + kiem tra ma tran ho tro.
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
                die "Amazon Linux 2 khong duoc ho tro (glibc/systemd qua cu). Dung Amazon Linux 2023."
            fi
            ;;
        opensuse-leap|opensuse|sles|sled)
            OS_FAMILY="suse"
            case "$OS_MAJOR" in 15) supported="true" ;; esac
            ;;
        *)
            # Suy ra tu ID_LIKE, nhung khong bao gio im lang.
            case " ${OS_LIKE} " in
                *debian*|*ubuntu*)          OS_FAMILY="debian" ;;
                *rhel*|*fedora*|*centos*)   OS_FAMILY="rhel" ;;
                *suse*)                     OS_FAMILY="suse" ;;
                *) OS_FAMILY="" ;;
            esac
            ;;
    esac

    if [[ -z "$OS_FAMILY" ]]; then
        die "He dieu hanh '${OS_PRETTY}' (ID=${OS_ID}) khong nam trong danh sach ho tro va cung khong suy ra duoc ho OS tu ID_LIKE.
Bo cai dung lai thay vi doan bua.

He dieu hanh duoc ho tro:
  Ubuntu 20.04/22.04/24.04, Debian 11/12,
  CentOS Stream 8/9, RHEL 8/9, Rocky 8/9, AlmaLinux 8/9,
  Fedora 38+, openSUSE Leap 15.x, Amazon Linux 2023"
    fi

    if [[ "$supported" != "true" ]]; then
        log_warn "'${OS_PRETTY}' khong nam trong ma tran da kiem thu (suy ra ho '${OS_FAMILY}')."
        if [[ "$OPT_FORCE_OS" != "true" ]]; then
            die "Dung lai de tranh cai hong. Neu chac chan, chay lai voi --force-os."
        fi
        log_warn "--force-os duoc bat: tiep tuc, nhung khong co bao dam."
    fi

    log_info "He dieu hanh: ${OS_PRETTY} (ID=${OS_ID}, ho=${OS_FAMILY}, kien truc=${ARCH})"
}

# =============================================================================
# 2. Lop truu tuong trinh quan ly goi
# =============================================================================
detect_pkg_manager() {
    case "$OS_FAMILY" in
        debian) has apt-get || die "Khong tim thay apt-get tren he dieu hanh ho Debian."; PKG="apt-get" ;;
        rhel)
            if has dnf; then PKG="dnf"
            elif has yum; then PKG="yum"
            else die "Khong tim thay dnf lan yum tren he dieu hanh ho RHEL."
            fi
            ;;
        suse)  has zypper || die "Khong tim thay zypper tren he dieu hanh ho SUSE."; PKG="zypper" ;;
        *)     die "Ho he dieu hanh khong xac dinh: '${OS_FAMILY}'" ;;
    esac
    log_info "Trinh quan ly goi: ${PKG}"
}

# apt-get hay bi ket boi unattended-upgrades dang giu lock. DPkg::Lock::Timeout
# bao apt cho lock thay vi bao loi ngay.
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

# pkg_install <goi...> - cai va bao loi neu that bai.
pkg_install() {
    [[ $# -gt 0 ]] || return 0
    case "$PKG" in
        apt-get) apt_run install -y "$@" ;;
        dnf)     dnf install -y "$@" ;;
        yum)     yum install -y "$@" ;;
        zypper)  zypper --non-interactive install --auto-agree-with-licenses "$@" ;;
    esac
}

# pkg_install_optional <goi...> - cai tung goi mot, bo qua goi khong ton tai.
pkg_install_optional() {
    local p
    for p in "$@"; do
        [[ -n "$p" ]] || continue
        if ! pkg_install "$p" >/dev/null 2>&1; then
            log_warn "Kho goi khong co '${p}', bo qua."
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

# pkg_enable_service <ten-service> - enable + start, chiu duoc ten khac nhau
# giua cac ho OS (vi du redis-server vs redis).
pkg_enable_service() {
    local svc="$1"
    if ! systemctl list-unit-files 2>/dev/null | grep -q "^${svc}\.service"; then
        log_warn "Khong tim thay unit '${svc}.service', bo qua."
        return 1
    fi
    systemctl enable "$svc" >/dev/null 2>&1 || log_warn "Khong enable duoc ${svc}"
    systemctl restart "$svc" || {
        log_error "Khong khoi dong duoc ${svc}. Xem: journalctl -u ${svc} -n 50"
        return 1
    }
    return 0
}

# Ten goi khac nhau giua apt / dnf / zypper. Tra ve danh sach goi (cach nhau
# bang khoang trang) cho mot "ten chung".
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
# 3. Kiem tra truoc khi cai
# =============================================================================
preflight_root() {
    if [[ "${EUID}" -ne 0 ]]; then
        die "Phai chay bang quyen root: sudo bash $0 $*"
    fi
}

preflight_systemd() {
    has systemctl || die "Khong tim thay systemd. ${BRAND_NAME} chay bang systemd unit nen bat buoc phai co."
    [[ -d /run/systemd/system ]] || log_warn "systemd khong o vai tro init (container?). Cac lenh systemctl co the that bai."
}

preflight_resources() {
    local ram_mb disk_mb target="/"
    ram_mb="$(awk '/MemTotal/ {printf "%d", $2/1024}' /proc/meminfo)"
    [[ -d "$PANEL_ROOT" ]] && target="$PANEL_ROOT"
    disk_mb="$(df -Pm "$target" | awk 'NR==2 {print $4}')"

    log_info "RAM: ${ram_mb} MB | trong o dia (${target}): ${disk_mb} MB"

    if (( ram_mb < MIN_RAM_MB )); then
        log_warn "RAM ${ram_mb} MB thap hon muc khuyen nghi ${MIN_RAM_MB} MB. Buoc build UI (Next.js) co the bi OOM."
        confirm "Van tiep tuc?" || die "Da huy theo yeu cau nguoi dung."
    fi
    if (( disk_mb < MIN_DISK_MB )); then
        die "Chi con ${disk_mb} MB trong. Can it nhat ${MIN_DISK_MB} MB de cai ${BRAND_NAME}."
    fi
}

preflight_other_panels() {
    local found=()
    [[ -d /usr/local/cpanel ]]      && found+=("cPanel (/usr/local/cpanel)")
    [[ -d /www/server/panel ]]      && found+=("aaPanel/BT Panel (/www/server/panel)")
    [[ -d /usr/local/psa ]]         && found+=("Plesk (/usr/local/psa)")
    [[ -d /usr/local/directadmin ]] && found+=("DirectAdmin (/usr/local/directadmin)")
    [[ -d /usr/local/vesta ]]       && found+=("VestaCP (/usr/local/vesta)")
    [[ -d /usr/local/hestia ]]      && found+=("HestiaCP (/usr/local/hestia)")

    if (( ${#found[@]} > 0 )); then
        log_warn "Phat hien panel khac dang co tren may:"
        local p
        for p in "${found[@]}"; do log_warn "  - ${p}"; done
        log_warn "Cai chong len nhau se tranh chap nginx, PHP, cong va chung chi."
        confirm "Van tiep tuc cai ${BRAND_NAME}?" || die "Da huy theo yeu cau nguoi dung."
    fi
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
            log_warn "Cong ${p} dang duoc '${owner:-tien trinh khong ro}' su dung. Day la cong cua WEBSITE KHACH, panel khong dung toi."
        fi
    done

    if port_in_use "$PANEL_PORT"; then
        owner="$(port_owner "$PANEL_PORT" || true)"
        if [[ "$owner" == "nginx" ]] && [[ -f /etc/nginx/conf.d/vkai-panel.conf ]]; then
            log_info "Cong ${PANEL_PORT} dang do nginx cua ${BRAND_NAME} giu (cai dat lai)."
        else
            die "Cong panel ${PANEL_PORT} da bi '${owner:-tien trinh khong ro}' chiem.
Chon cong khac: bash $0 --port <cong_khac>   (hoac --random-port)"
        fi
    fi

    for p in "$API_BIND_PORT" "$UI_BIND_PORT"; do
        if port_in_use "$p" && ! systemctl is-active --quiet vkai-api vkai-ui 2>/dev/null; then
            log_warn "Cong noi bo ${p} dang bi chiem. Neu khong phai cua ${BRAND_NAME}, dich vu se khong khoi dong duoc."
        fi
    done
}

# =============================================================================
# 4. Cai phu thuoc he thong
# =============================================================================
enable_extra_repos() {
    case "$OS_FAMILY" in
        rhel)
            # EPEL: khong ap dung cho Fedora (da co san) va Amazon Linux 2023.
            case "$OS_ID" in
                fedora|amzn) : ;;
                *)
                    if ! rpm -q epel-release >/dev/null 2>&1; then
                        log_info "Bat kho EPEL cho ho RHEL ${OS_MAJOR}..."
                        pkg_install epel-release >/dev/null 2>&1 ||
                            pkg_install "https://dl.fedoraproject.org/pub/epel/epel-release-latest-${OS_MAJOR}.noarch.rpm" >/dev/null 2>&1 ||
                            log_warn "Khong bat duoc EPEL. Mot vai goi phu (jq, redis) co the thieu."
                    fi
                    # CodeReady Builder / PowerTools chua cac goi -devel ma EPEL can.
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
            # Leap can kho "backports"/"update" mac dinh; chi canh bao neu thieu nginx.
            zypper --non-interactive refresh >/dev/null 2>&1 || true
            ;;
    esac
}

install_dependencies() {
    if [[ "$OPT_SKIP_DEPS" == "true" ]]; then
        log_warn "--skip-deps: bo qua cai goi he thong."
        return 0
    fi

    log_step "Cap nhat danh sach goi"
    pkg_update

    enable_extra_repos

    log_step "Cai phu thuoc he thong"
    local group
    for group in base buildtools postgresql redis nginx cron; do
        local raw=() names=() p
        read -r -a raw <<<"$(pkg_names_for "$group")"
        for p in "${raw[@]}"; do
            # RHEL 9 / Amazon Linux 2023 cai san curl-minimal, va "dnf install
            # curl" o do la mot xung dot goi chu khong phai mot ban nang cap.
            if [[ "$p" == "curl" ]] && has curl; then
                continue
            fi
            names+=("$p")
        done
        (( ${#names[@]} > 0 )) || continue
        log_info "Nhom '${group}': ${names[*]}"
        if ! pkg_install "${names[@]}"; then
            log_warn "Cai ca nhom '${group}' that bai, thu cai tung goi..."
            pkg_install_optional "${names[@]}"
        fi
    done

    # Cong cu SELinux chi co y nghia tren ho RHEL.
    local selinux_pkgs
    selinux_pkgs="$(pkg_names_for selinux)"
    if [[ -n "$selinux_pkgs" ]]; then
        # shellcheck disable=SC2086  # co y tach tu: bien chua nhieu ten goi
        pkg_install_optional $selinux_pkgs
    fi

    has openssl || die "Thieu 'openssl' sau khi cai phu thuoc - khong the sinh bi mat."
    has curl    || die "Thieu 'curl' sau khi cai phu thuoc."
    log_info "Da cai xong phu thuoc he thong."
}

# =============================================================================
# 5. Go va Node.js theo kien truc, co kiem tra checksum
# =============================================================================
verify_sha256() {
    local file="$1" expected="$2" actual
    if [[ "$OPT_SKIP_CHECKSUM" == "true" ]]; then
        log_warn "--skip-checksum: bo qua kiem tra checksum cho $(basename "$file")."
        return 0
    fi
    [[ -n "$expected" ]] || return 1
    actual="$(sha256sum "$file" | awk '{print $1}')"
    if [[ "$actual" != "$expected" ]]; then
        rm -f "$file"
        die "Checksum khong khop cho $(basename "$file").
  Mong doi: ${expected}
  Thuc te : ${actual}
File tai ve co the bi hong hoac bi can thiep. Da xoa file."
    fi
    log_info "Checksum hop le: $(basename "$file")"
}

go_needs_install() {
    local v major minor
    if ! has go; then return 0; fi
    v="$(go version 2>/dev/null | awk '{print $3}')"   # vd: go1.22.5
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
        log_info "Go da dung phien ban: $(go version)"
        GO_BIN="$(command -v go)"
        return 0
    fi

    local tarball url sha_url expected
    tarball="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
    url="https://go.dev/dl/${tarball}"
    sha_url="https://dl.google.com/go/${tarball}.sha256"

    log_step "Cai Go ${GO_VERSION} (${GO_ARCH})"
    mkdir -p "$TMP_DIR"
    curl -fsSL --retry 3 --connect-timeout 20 -o "${TMP_DIR}/${tarball}" "$url" ||
        die "Khong tai duoc Go tu ${url}"

    expected="$(curl -fsSL --retry 3 --connect-timeout 20 "$sha_url" 2>/dev/null | tr -d ' \n' || true)"
    if [[ -z "$expected" ]]; then
        # Du phong: bang bam nam trong API JSON cua go.dev.
        # go.dev tra ve JSON co the co hoac khong co khoang trang -> bo het
        # khoang trang truoc khi tach.
        expected="$(curl -fsSL --retry 3 "https://go.dev/dl/?mode=json&include=all" 2>/dev/null |
            tr -d ' \t' | sed 's/[,{}]/\n/g' |
            grep -A8 "\"filename\":\"${tarball}\"" |
            grep -m1 '"sha256":' |
            sed 's/.*"sha256":"\([a-f0-9]*\)".*/\1/' || true)"
    fi
    if [[ -z "$expected" && "$OPT_SKIP_CHECKSUM" != "true" ]]; then
        die "Khong lay duoc checksum chinh thuc cho ${tarball}.
Kiem tra ket noi mang, hoac chay lai voi --skip-checksum neu ban chap nhan rui ro."
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

    log_info "Da cai: $("$GO_BIN" version)"
}

node_needs_install() {
    local v major
    if ! has node; then return 0; fi
    v="$(node --version 2>/dev/null)"    # vd: v20.18.1
    v="${v#v}"; major="${v%%.*}"
    [[ "$major" =~ ^[0-9]+$ ]] || return 0
    (( major >= NODE_MIN_MAJOR )) && return 1
    return 0
}

install_nodejs() {
    if ! node_needs_install; then
        log_info "Node.js da dung phien ban: $(node --version)"
    else
        local tarball url expected
        tarball="node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz"
        url="https://nodejs.org/dist/v${NODE_VERSION}/${tarball}"

        log_step "Cai Node.js ${NODE_VERSION} (${NODE_ARCH})"
        mkdir -p "$TMP_DIR"
        curl -fsSL --retry 3 --connect-timeout 20 -o "${TMP_DIR}/${tarball}" "$url" ||
            die "Khong tai duoc Node.js tu ${url}"

        expected="$(curl -fsSL --retry 3 --connect-timeout 20 \
            "https://nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt" 2>/dev/null |
            awk -v f="$tarball" '$2 == f {print $1}' || true)"
        if [[ -z "$expected" && "$OPT_SKIP_CHECKSUM" != "true" ]]; then
            die "Khong lay duoc SHASUMS256.txt cua Node.js. Chay lai voi --skip-checksum neu ban chap nhan rui ro."
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
        log_info "Da cai: $(node --version)"
    fi

    NODE_BIN="$(command -v node)"
    NPM_BIN="$(command -v npm)"
    [[ -n "$NODE_BIN" ]] || die "Khong tim thay 'node' sau khi cai."
    [[ -n "$NPM_BIN"  ]] || die "Khong tim thay 'npm' sau khi cai."

    # Unit vkai-ui.service goi /usr/bin/node - bao dam duong dan do ton tai.
    if [[ ! -x /usr/bin/node ]]; then
        ln -sf "$NODE_BIN" /usr/bin/node
        log_info "Da tao lien ket /usr/bin/node -> ${NODE_BIN}"
    fi
}

# =============================================================================
# 6. Nguoi dung + cay thu muc chuan
# =============================================================================
setup_user() {
    if getent group "$VKAI_GROUP" >/dev/null; then
        log_info "Nhom '${VKAI_GROUP}' da ton tai."
    else
        groupadd --system "$VKAI_GROUP"
        log_info "Da tao nhom he thong '${VKAI_GROUP}'."
    fi

    if id "$VKAI_USER" >/dev/null 2>&1; then
        log_info "Nguoi dung '${VKAI_USER}' da ton tai."
    else
        useradd --system --gid "$VKAI_GROUP" --home-dir "$PANEL_ROOT" \
                --shell /usr/sbin/nologin --comment "VKAI Panel service account" "$VKAI_USER" 2>/dev/null ||
        useradd --system --gid "$VKAI_GROUP" --home-dir "$PANEL_ROOT" \
                --shell /sbin/nologin --comment "VKAI Panel service account" "$VKAI_USER"
        log_info "Da tao nguoi dung he thong '${VKAI_USER}'."
    fi
}

setup_directories() {
    log_step "Tao cay thu muc ${PANEL_ROOT}"

    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$PANEL_ROOT"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"

    # www/ phai 755: nginx (chay bang user rieng) can duyet vao thu muc website.
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 755 "$WWW_DIR" "$WWW_DOMAINS_DIR" "$WWW_DEFAULT_DIR"
    # Ban sao luu chua dump CSDL - khong de nguoi khac doc.
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$WWW_BACKUP_DIR"

    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$LOG_DIR" "$LOG_SITES_DIR"
    # etc/ chua .env va bi mat -> 700.
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 700 "$ETC_DIR"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 700 "$SSL_DIR"
    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "$TMP_DIR"

    # Panel root phai duyet duoc tu nginx de doc www/.
    chmod 751 "$PANEL_ROOT"

    [[ -f "$INSTALL_LOG" ]] && chown "$VKAI_USER:$VKAI_GROUP" "$INSTALL_LOG"

    if [[ ! -f "${WWW_DEFAULT_DIR}/index.html" ]]; then
        cat >"${WWW_DEFAULT_DIR}/index.html" <<HTML
<!doctype html>
<html lang="vi">
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
  <p>May chu da san sang. Chua co website nao tro ve ten mien nay.</p>
  <p>${BRAND_ORG}</p>
</div></body></html>
HTML
        chown "$VKAI_USER:$VKAI_GROUP" "${WWW_DEFAULT_DIR}/index.html"
        chmod 644 "${WWW_DEFAULT_DIR}/index.html"
    fi

    log_info "Cay thu muc san sang."
}

# =============================================================================
# 7. Dong bo ma nguon vao /vkai-panel
# =============================================================================
sync_sources() {
    log_step "Dong bo ma nguon vao ${PANEL_ROOT}"

    [[ -d "${SRC_DIR}/core"  ]] || die "Khong thay ${SRC_DIR}/core. Chay script tu trong ma nguon da giai nen."
    [[ -d "${SRC_DIR}/panel" ]] || die "Khong thay ${SRC_DIR}/panel."
    [[ -d "${SRC_DIR}/agent" ]] || die "Khong thay ${SRC_DIR}/agent."

    if [[ "$(cd "$SRC_DIR" && pwd -P)" == "$PANEL_ROOT" ]]; then
        log_info "Ma nguon da nam ngay tai ${PANEL_ROOT}, bo qua buoc copy."
    else
        local rsync_opts=(-a --delete
            --exclude '.git' --exclude 'node_modules' --exclude '.next'
            --exclude '*.log' --exclude 'tmp/')
        if has rsync; then
            rsync "${rsync_opts[@]}" "${SRC_DIR}/core/"  "${CORE_DIR}/"
            rsync "${rsync_opts[@]}" "${SRC_DIR}/panel/" "${UI_DIR}/"
            rsync "${rsync_opts[@]}" "${SRC_DIR}/agent/" "${AGENT_DIR}/"
        else
            log_warn "Khong co rsync, dung cp (khong xoa file thua)."
            cp -a "${SRC_DIR}/core/."  "${CORE_DIR}/"
            cp -a "${SRC_DIR}/panel/." "${UI_DIR}/"
            cp -a "${SRC_DIR}/agent/." "${AGENT_DIR}/"
        fi
    fi

    install -d -o "$VKAI_USER" -g "$VKAI_GROUP" -m 750 "${CORE_DIR}/bin" "${AGENT_DIR}/bin"
    chown -R "$VKAI_USER:$VKAI_GROUP" "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"
    log_info "Da dong bo core/, panel/, agent/."
}

# =============================================================================
# 8. Sinh cau hinh - PHAI chay TRUOC khi build UI
#    NEXT_PUBLIC_API_URL duoc Next.js nhung thang vao bundle luc build, doi
#    .env sau khi build se khong co tac dung.
# =============================================================================
env_get() {
    local key="$1"
    [[ -f "$ENV_FILE" ]] || return 1
    local line
    line="$(grep -E "^${key}=" "$ENV_FILE" | tail -n1 || true)"
    [[ -n "$line" ]] || return 1
    printf '%s' "${line#*=}"
}

# Giu lai gia tri cu neu da cai truoc do (idempotent), neu khong thi sinh moi.
reuse_or_generate() {
    local key="$1" generator="$2" existing
    existing="$(env_get "$key" || true)"
    if [[ -n "$existing" ]]; then
        printf '%s' "$existing"
    else
        eval "$generator"
    fi
}

resolve_panel_access() {
    # Chi tinh mot lan: preflight_ports va setup_config dung chung ket qua.
    [[ -z "$PANEL_PORT" ]] || return 0

    # Cong panel: cu the tren dong lenh > gia tri da cai > ngau nhien > 8888.
    if [[ -n "$OPT_PORT" ]]; then
        PANEL_PORT="$OPT_PORT"
    else
        PANEL_PORT="$(env_get VKAI_PANEL_PORT || true)"
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
        die "Cong panel khong duoc la ${PANEL_PORT}: 80/443 danh cho website khach."

    # Loi vao an toan.
    if [[ -n "$OPT_ENTRANCE" ]]; then
        PANEL_ENTRANCE="$OPT_ENTRANCE"
    else
        PANEL_ENTRANCE="$(env_get VKAI_PANEL_ENTRANCE || true)"
        [[ -n "$PANEL_ENTRANCE" ]] || PANEL_ENTRANCE="/vkai_$(rand_hex 4)"
    fi
}

setup_config() {
    log_step "Sinh cau hinh ${ENV_FILE} (truoc khi build UI)"

    resolve_panel_access

    DB_NAME="$(reuse_or_generate VKAI_DB_NAME 'printf vkai_panel')"
    DB_USER="$(reuse_or_generate VKAI_DB_USER 'printf vkai')"
    # Chi dung hex: cau hinh cua panel tu choi moi bi mat co chua tu khoa mau.
    DB_PASS="$(reuse_or_generate VKAI_DB_PASSWORD 'rand_hex 18')"
    JWT_SECRET="$(reuse_or_generate VKAI_JWT_SECRET 'rand_hex 32')"   # 64 ky tu
    SECRET_KEY="$(reuse_or_generate VKAI_SECRET_KEY 'rand_hex 32')"
    AGENT_TOKEN="$(reuse_or_generate VKAI_AGENT_TOKEN 'rand_hex 24')"

    (( ${#JWT_SECRET} >= 64 )) || die "JWT secret sinh ra qua ngan (${#JWT_SECRET} ky tu)."

    SERVER_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
    [[ -n "$SERVER_IP" ]] || SERVER_IP="127.0.0.1"

    # Bo trong = same-origin: trinh duyet goi /api/... ngay tren cong panel,
    # nen doi IP hay ten mien khong phai build lai UI.
    local api_url="$OPT_API_URL"

    local tmp_env
    tmp_env="$(mktemp "${ETC_DIR}/.env.XXXXXX")"
    cat >"$tmp_env" <<ENVEOF
# =============================================================================
# ${BRAND_NAME} - cau hinh may chu
# Sinh tu dong: $(ts)
# KHONG chia se file nay. Quyen 0600, chu so huu ${VKAI_USER}.
# =============================================================================

# --- Cong truy cap panel -----------------------------------------------------
# Panel KHONG BAO GIO nghe tren 80/443: hai cong do danh cho website cua khach.
VKAI_PANEL_ENABLED=true
# API chi nghe loopback; nginx giu cong cong khai ${PANEL_PORT} va chuyen tiep vao.
VKAI_PANEL_BIND=127.0.0.1
VKAI_PANEL_PORT=${PANEL_PORT}
VKAI_PANEL_PUBLIC_PORT=${PANEL_PORT}
VKAI_PANEL_PUBLIC_SCHEME=http
VKAI_PANEL_ENTRANCE=${PANEL_ENTRANCE}
VKAI_PANEL_ENTRANCE_ENABLED=true
VKAI_PANEL_SESSION_TTL=12h
# Danh sach IP/CIDR duoc vao panel. De trong = cho phep tat ca.
VKAI_PANEL_ALLOWED_IPS=
# Chi tin X-Forwarded-For tu nginx tren chinh may nay.
VKAI_PANEL_TRUSTED_PROXIES=127.0.0.1,::1
VKAI_PANEL_DOMAIN=
VKAI_PANEL_TLS_CERT=
VKAI_PANEL_TLS_KEY=
VKAI_PANEL_TLS_SELF_SIGNED=false
VKAI_PANEL_CONFIG_FILE=${PANEL_STATE_FILE}

# --- May chu API -------------------------------------------------------------
VKAI_SERVER_HOST=127.0.0.1
VKAI_SERVER_PORT=${API_BIND_PORT}
VKAI_SERVER_MODE=release

# --- CSDL --------------------------------------------------------------------
VKAI_DB_HOST=127.0.0.1
VKAI_DB_PORT=5432
VKAI_DB_NAME=${DB_NAME}
VKAI_DB_USER=${DB_USER}
VKAI_DB_PASSWORD=${DB_PASS}
# CSDL nam ngay tren may nay nen 'disable' duoc chap nhan.
VKAI_DB_SSLMODE=disable

# --- Redis -------------------------------------------------------------------
VKAI_REDIS_HOST=127.0.0.1
VKAI_REDIS_PORT=6379
VKAI_REDIS_PASSWORD=
VKAI_REDIS_DB=0

# --- Bi mat ------------------------------------------------------------------
VKAI_JWT_SECRET=${JWT_SECRET}
VKAI_JWT_ACCESS_EXPIRY=15
VKAI_JWT_REFRESH_EXPIRY=10080
VKAI_JWT_ISSUER=vkai-panel
VKAI_SECRET_KEY=${SECRET_KEY}
VKAI_AGENT_TOKEN=${AGENT_TOKEN}
VKAI_AGENT_PORT=${AGENT_PORT}

# --- Duong dan du lieu -------------------------------------------------------
VKAI_FILEMANAGER_ROOT=${WWW_DOMAINS_DIR}
VKAI_BACKUP_ROOT=${WWW_BACKUP_DIR}
VKAI_WWW_ROOT=${WWW_DOMAINS_DIR}
VKAI_SITE_LOG_ROOT=${LOG_SITES_DIR}
VKAI_SSL_ROOT=${SSL_DIR}
VKAI_TMP_ROOT=${TMP_DIR}

# --- Nhat ky -----------------------------------------------------------------
VKAI_LOG_LEVEL=info
VKAI_LOG_DIR=${LOG_DIR}

# --- CORS / RBAC / cron ------------------------------------------------------
VKAI_CORS_ALLOWED_ORIGINS=http://${SERVER_IP}:${PANEL_PORT}
VKAI_RBAC_ENFORCE=true
VKAI_CRON_USER=${VKAI_USER}

# --- Giao dien Next.js -------------------------------------------------------
# Bien nay duoc NHUNG vao bundle luc "npm run build". Doi gia tri o day thi
# PHAI build lai UI (vkai update) moi co tac dung.
# De trong = same-origin: trinh duyet goi /api/... ngay tren cong panel.
NEXT_PUBLIC_API_URL=${api_url}
NODE_ENV=production
PORT=${UI_BIND_PORT}
HOSTNAME=127.0.0.1
ENVEOF

    mv "$tmp_env" "$ENV_FILE"
    chown "$VKAI_USER:$VKAI_GROUP" "$ENV_FILE"
    chmod 600 "$ENV_FILE"

    # Next.js chi doc .env tai goc du an -> lien ket vao thu muc panel/.
    # Neu khong lien ket duoc (khac filesystem), copy roi siet quyen.
    if ln -sfn "$ENV_FILE" "${UI_DIR}/.env" 2>/dev/null; then
        log_info "Da lien ket ${UI_DIR}/.env -> ${ENV_FILE}"
    else
        install -o "$VKAI_USER" -g "$VKAI_GROUP" -m 600 "$ENV_FILE" "${UI_DIR}/.env"
        log_warn "Khong tao duoc symlink, da COPY .env vao ${UI_DIR}/.env"
    fi
    ln -sfn "$ENV_FILE" "${CORE_DIR}/.env" 2>/dev/null || true

    # /etc/vkai la duong dan mac dinh trong ma Go; tro no ve ${ETC_DIR} de chi
    # con MOT nguon su that ve cau hinh.
    if [[ ! -e /etc/vkai ]]; then
        ln -sfn "$ETC_DIR" /etc/vkai
        log_info "Da lien ket /etc/vkai -> ${ETC_DIR}"
    fi

    log_info "Cau hinh: cong ${PANEL_PORT}, loi vao ${PANEL_ENTRANCE}"
}

# =============================================================================
# 9. PostgreSQL + Redis
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
    # Debian/Ubuntu tu khoi tao cum khi cai goi. RHEL va SUSE thi khong.
    case "$OS_FAMILY" in
        rhel)
            local setup_bin
            setup_bin="$(command -v postgresql-setup || true)"
            if [[ -n "$setup_bin" ]]; then
                if [[ ! -f /var/lib/pgsql/data/PG_VERSION ]]; then
                    log_info "Khoi tao cum PostgreSQL..."
                    "$setup_bin" --initdb >/dev/null 2>&1 || "$setup_bin" initdb >/dev/null 2>&1 || true
                fi
            fi
            ;;
        suse)
            # openSUSE initdb ngay lan start dau tien.
            :
            ;;
    esac
}

pg_hba_allow_local_tcp() {
    local hba
    hba="$(sudo -u postgres psql -tAc 'SHOW hba_file' 2>/dev/null || true)"
    [[ -n "$hba" && -f "$hba" ]] || { log_warn "Khong xac dinh duoc pg_hba.conf, bo qua."; return 0; }

    if grep -qE "^host\s+${DB_NAME}\s+${DB_USER}\s" "$hba"; then
        log_info "pg_hba.conf da co quy tac cho ${DB_USER}."
        return 0
    fi

    # Phuong thuc xac thuc PHAI khop cach may chu bam mat khau: PostgreSQL 13
    # tro xuong mac dinh md5, tu 14 tro len la scram-sha-256. Ghi nham thi
    # ket noi bi tu choi du mat khau dung.
    local method
    method="$(sudo -u postgres psql -tAc 'SHOW password_encryption' 2>/dev/null | tr -d '[:space:]' || true)"
    case "$method" in
        scram-sha-256|md5) : ;;
        *) method="scram-sha-256" ;;
    esac

    log_info "Them quy tac pg_hba.conf cho ${DB_USER}@127.0.0.1 (${method})."
    cp -a "$hba" "${hba}.vkai.bak.$(date +%Y%m%d%H%M%S)"
    # Chen len TRUOC cac quy tac khac: pg_hba xet tu tren xuong, dong dau tien khop se thang.
    local tmp
    tmp="$(mktemp)"
    {
        echo "# --- ${BRAND_NAME}: truy cap CSDL cua panel qua loopback ---"
        echo "host    ${DB_NAME}    ${DB_USER}    127.0.0.1/32    ${method}"
        echo "host    ${DB_NAME}    ${DB_USER}    ::1/128         ${method}"
        cat "$hba"
    } >"$tmp"
    cat "$tmp" >"$hba"
    rm -f "$tmp"
    systemctl reload "$PG_SERVICE" 2>/dev/null || systemctl restart "$PG_SERVICE"
}

setup_database() {
    log_step "Chuan bi PostgreSQL"

    detect_pg_service
    pg_initdb_if_needed
    pkg_enable_service "$PG_SERVICE" || die "PostgreSQL khong khoi dong duoc. Xem: journalctl -u ${PG_SERVICE} -n 50"

    # Cho postgres san sang.
    local i
    for i in $(seq 1 30); do
        sudo -u postgres psql -tAc 'SELECT 1' >/dev/null 2>&1 && break
        sleep 1
        (( i == 30 )) && die "PostgreSQL khong phan hoi sau 30 giay."
    done

    local esc_pass="${DB_PASS//\'/\'\'}"
    if sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1; then
        sudo -u postgres psql -qc "ALTER ROLE \"${DB_USER}\" WITH LOGIN PASSWORD '${esc_pass}';"
        log_info "Da cap nhat mat khau cho vai tro '${DB_USER}'."
    else
        sudo -u postgres psql -qc "CREATE ROLE \"${DB_USER}\" WITH LOGIN PASSWORD '${esc_pass}';"
        log_info "Da tao vai tro CSDL '${DB_USER}'."
    fi

    if sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
        log_info "CSDL '${DB_NAME}' da ton tai."
    else
        sudo -u postgres createdb -O "$DB_USER" "$DB_NAME"
        log_info "Da tao CSDL '${DB_NAME}' (chu so huu ${DB_USER})."
    fi

    # uuid-ossp phai do superuser cai (migration 001 can uuid_generate_v4).
    sudo -u postgres psql -q -d "$DB_NAME" -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp";' ||
        die "Khong cai duoc extension uuid-ossp. Thieu goi postgresql-contrib?"

    pg_hba_allow_local_tcp
}

psql_vkai() {
    PGPASSWORD="$DB_PASS" psql -h 127.0.0.1 -p 5432 -U "$DB_USER" -d "$DB_NAME" \
        -v ON_ERROR_STOP=1 --quiet "$@"
}

run_migrations() {
    log_step "Ap dung migration CSDL"

    local dir="${CORE_DIR}/migrations"
    [[ -d "$dir" ]] || { log_warn "Khong thay ${dir}, bo qua migration."; return 0; }

    touch "$MIGRATIONS_STATE"
    chmod 600 "$MIGRATIONS_STATE"

    local f name applied=0
    while IFS= read -r f; do
        name="$(basename "$f")"
        if grep -qxF "$name" "$MIGRATIONS_STATE"; then
            continue
        fi
        log_info "  -> ${name}"
        if psql_vkai -f "$f" >/dev/null; then
            echo "$name" >>"$MIGRATIONS_STATE"
            applied=$((applied + 1))
        else
            die "Migration '${name}' that bai. CSDL dang o trang thai do dang - xem ${INSTALL_LOG}."
        fi
    done < <(find "$dir" -maxdepth 1 -name '*.sql' -type f | sort)

    log_info "Da ap dung ${applied} migration moi."
}

setup_redis() {
    log_step "Chuan bi Redis"
    case "$OS_FAMILY" in
        debian) REDIS_SERVICE="redis-server" ;;
        *)      REDIS_SERVICE="redis" ;;
    esac
    if ! systemctl list-unit-files 2>/dev/null | grep -q "^${REDIS_SERVICE}\.service"; then
        # Amazon Linux 2023 dat ten unit la redis6.
        local alt
        for alt in redis6 redis redis-server; do
            if systemctl list-unit-files 2>/dev/null | grep -q "^${alt}\.service"; then
                REDIS_SERVICE="$alt"
                break
            fi
        done
    fi
    pkg_enable_service "$REDIS_SERVICE" || log_warn "Redis chua chay. Panel van hoat dong nhung mat cache/hang doi."
    log_info "Dich vu Redis: ${REDIS_SERVICE}"
}

# =============================================================================
# 10. Tai khoan quan tri dau tien
# =============================================================================
bcrypt_hash() {
    local plain="$1" out="" gendir="${CORE_DIR}/tools/vkai-hashgen"

    # Uu tien Go: golang.org/x/crypto/bcrypt la phu thuoc san co cua core/.
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
    log_step "Tao tai khoan quan tri dau tien"

    ADMIN_USER="$OPT_ADMIN_USER"
    ADMIN_EMAIL="${ADMIN_USER}@$(hostname -f 2>/dev/null || echo 'localhost')"

    # Cai lai: khong dat lai mat khau cua quan tri vien dang dung.
    local existing
    existing="$(psql_vkai -tAc "SELECT 1 FROM users WHERE username = '${ADMIN_USER//\'/\'\'}'" 2>/dev/null | tr -d '[:space:]' || true)"
    if [[ "$existing" == "1" && -f "$SUMMARY_FILE" ]]; then
        log_info "Tai khoan '${ADMIN_USER}' da ton tai - giu nguyen mat khau hien tai."
        ADMIN_PASS="(khong doi - dung mat khau ban da dat)"
        return 0
    fi

    ADMIN_PASS="$(rand_alnum 20)"
    local hash
    hash="$(bcrypt_hash "$ADMIN_PASS")"

    if [[ -z "$hash" || "$hash" != \$2* ]]; then
        log_warn "Khong sinh duoc bam bcrypt (thieu Go va python3-bcrypt)."
        log_warn "Tai khoan mac dinh admin/admin123 tu migration van con hieu luc."
        log_warn "DOI MAT KHAU NGAY sau lan dang nhap dau tien."
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
    log_info "Tai khoan quan tri: ${ADMIN_USER} (mat khau ngau nhien 20 ky tu)."
}

# =============================================================================
# 11. Bien dich
# =============================================================================
build_core() {
    log_step "Bien dich API (core/)"
    cd "$CORE_DIR"
    export HOME="${TMP_DIR}"
    export GOCACHE="${TMP_DIR}/go-build" GOMODCACHE="${TMP_DIR}/go-mod" GOFLAGS="-buildvcs=false"
    mkdir -p "$GOCACHE" "$GOMODCACHE"

    "$GO_BIN" mod download
    "$GO_BIN" build -trimpath -ldflags "-s -w" -o "${CORE_DIR}/bin/vkai-api"       ./cmd/api
    "$GO_BIN" build -trimpath -ldflags "-s -w" -o "${CORE_DIR}/bin/vkai-panelctl"  ./cmd/panelctl
    if [[ -f "${CORE_DIR}/cmd/cli/main.go" ]]; then
        "$GO_BIN" build -trimpath -ldflags "-s -w" -o "${CORE_DIR}/bin/vkai-cli" ./cmd/cli
    fi

    [[ -x "${CORE_DIR}/bin/vkai-api" ]] || die "Khong tao duoc ${CORE_DIR}/bin/vkai-api"

    install -m 0755 "${CORE_DIR}/bin/vkai-panelctl" /usr/local/bin/vkai-panelctl
    [[ -x "${CORE_DIR}/bin/vkai-cli" ]] && install -m 0755 "${CORE_DIR}/bin/vkai-cli" /usr/local/bin/vkai-cli

    chown -R "$VKAI_USER:$VKAI_GROUP" "${CORE_DIR}/bin"
    log_info "API da bien dich: ${CORE_DIR}/bin/vkai-api"
}

build_agent() {
    log_step "Bien dich agent (agent/)"
    cd "$AGENT_DIR"
    export HOME="${TMP_DIR}"
    export GOCACHE="${TMP_DIR}/go-build" GOMODCACHE="${TMP_DIR}/go-mod" GOFLAGS="-buildvcs=false"

    "$GO_BIN" mod download
    if [[ -f "${AGENT_DIR}/cmd/main.go" ]]; then
        "$GO_BIN" build -trimpath -ldflags "-s -w" -o "${AGENT_DIR}/bin/vkai-agent" ./cmd
    else
        "$GO_BIN" build -trimpath -ldflags "-s -w" -o "${AGENT_DIR}/bin/vkai-agent" .
    fi

    [[ -x "${AGENT_DIR}/bin/vkai-agent" ]] || die "Khong tao duoc ${AGENT_DIR}/bin/vkai-agent"
    chown -R "$VKAI_USER:$VKAI_GROUP" "${AGENT_DIR}/bin"
    log_info "Agent da bien dich: ${AGENT_DIR}/bin/vkai-agent"
}

build_ui() {
    log_step "Bien dich giao dien (panel/)"
    [[ -f "$ENV_FILE" ]] || die "Khong co ${ENV_FILE}: phai sinh cau hinh TRUOC khi build UI."

    cd "$UI_DIR"
    export HOME="${TMP_DIR}"
    export npm_config_cache="${TMP_DIR}/npm"
    mkdir -p "$npm_config_cache"

    if [[ -f package-lock.json ]]; then
        "$NPM_BIN" ci --no-audit --no-fund
    else
        "$NPM_BIN" install --no-audit --no-fund
    fi

    # NEXT_PUBLIC_* duoc nhung vao bundle ngay tai day.
    set -a
    # shellcheck disable=SC1090
    . "$ENV_FILE"
    set +a
    NODE_ENV=production "$NPM_BIN" run build

    # output: 'standalone' CO Y khong copy .next/static va public/ sang
    # .next/standalone. Thieu buoc nay -> moi /_next/static/*.js tra 404 ->
    # trinh duyet bao "Application error: a client-side exception has occurred".
    # package.json da co postbuild:standalone; day la luoi an toan neu script do
    # bi go bo hoac build chay bang cong cu khac.
    local sa="${UI_DIR}/.next/standalone"
    [[ -d "$sa" ]] || die "Khong thay ${sa}. next.config.js phai dat output: 'standalone'."

    if [[ -d "${UI_DIR}/.next/static" && ! -d "${sa}/.next/static" ]]; then
        log_warn "Thieu .next/standalone/.next/static - dang copy thu cong."
        mkdir -p "${sa}/.next"
        cp -a "${UI_DIR}/.next/static" "${sa}/.next/static"
    fi
    if [[ -d "${UI_DIR}/public" && ! -d "${sa}/public" ]]; then
        log_warn "Thieu .next/standalone/public - dang copy thu cong."
        cp -a "${UI_DIR}/public" "${sa}/public"
    fi

    [[ -f "${sa}/server.js" ]] ||
        die "Khong thay ${sa}/server.js sau khi build. Giao dien se khong khoi dong duoc."
    [[ -d "${sa}/.next/static" ]] ||
        die "Khong thay ${sa}/.next/static. Panel se bao 'Application error: a client-side exception has occurred'."

    chown -R "$VKAI_USER:$VKAI_GROUP" "$UI_DIR"
    log_info "Giao dien da build: ${sa}/server.js"
}

# =============================================================================
# 12. systemd
# =============================================================================
install_systemd_units() {
    log_step "Cai systemd unit"

    local src="${SRC_DIR}/deploy/systemd"
    [[ -d "$src" ]] || die "Khong thay ${src}"

    local unit
    for unit in vkai-api.service vkai-ui.service vkai-agent.service; do
        [[ -f "${src}/${unit}" ]] || die "Thieu ${src}/${unit}"
        install -m 0644 "${src}/${unit}" "/etc/systemd/system/${unit}"
        log_info "  -> /etc/systemd/system/${unit}"
    done

    # Don sach ten unit cu tu cac ban cai truoc.
    local old
    for old in vkai-frontend.service vkai-panel-api.service vkai-panel-frontend.service; do
        if [[ -f "/etc/systemd/system/${old}" ]]; then
            systemctl disable --now "${old%.service}" >/dev/null 2>&1 || true
            rm -f "/etc/systemd/system/${old}"
            log_info "  Da go unit cu: ${old}"
        fi
    done

    systemctl daemon-reload
    systemctl enable vkai-api vkai-ui >/dev/null 2>&1
    # Agent la tuy chon: chi enable khi nguoi dung tu bat.
    log_info "Da bat vkai-api va vkai-ui khi khoi dong may."
}

# =============================================================================
# 13. Nginx
# =============================================================================
setup_nginx() {
    log_step "Cau hinh nginx cho panel (cong ${PANEL_PORT})"

    has nginx || { log_warn "Khong co nginx, bo qua reverse proxy."; return 0; }

    local conf_dir="/etc/nginx/conf.d"
    [[ -d "$conf_dir" ]] || mkdir -p "$conf_dir"

    local template="${SRC_DIR}/deploy/nginx/vkai-panel.conf"
    [[ -f "$template" ]] || die "Khong thay ${template}"

    # File goc duoc giu o dang dung cho Docker (upstream la ten dich vu
    # compose) de docker-compose.yml mount thang duoc. Ban cai bang systemd
    # phai doi upstream sang loopback va dat dung cong panel.
    sed -e "s|^\( *\)server vkai-ui:3000;|\\1server 127.0.0.1:${UI_BIND_PORT};|" \
        -e "s|^\( *\)server vkai-core:30110;|\\1server 127.0.0.1:${API_BIND_PORT};|" \
        -e "s|^\( *\)listen 8888;|\\1listen ${PANEL_PORT};|" \
        -e "s|^\( *\)listen \[::\]:8888;|\\1listen [::]:${PANEL_PORT};|" \
        -e "s|/var/log/nginx/vkai-panel|${LOG_DIR}/vkai-panel|g" \
        "$template" >"${conf_dir}/vkai-panel.conf"
    chmod 644 "${conf_dir}/vkai-panel.conf"

    # Kiem tra chac chan da khong con tro vao ten dich vu Docker.
    if grep -qE '^[[:space:]]*server (vkai-ui|vkai-core):' "${conf_dir}/vkai-panel.conf"; then
        rm -f "${conf_dir}/vkai-panel.conf"
        die "Khong doi duoc upstream nginx sang 127.0.0.1. Mau cau hinh da bi sua doi?"
    fi

    # Ban cai Debian/Ubuntu co sites-enabled/default chiem cong 80 - de nguyen,
    # cong 80 khong phai viec cua panel.
    if ! nginx -t; then
        rm -f "${conf_dir}/vkai-panel.conf"
        die "nginx -t that bai. Da go bo ${conf_dir}/vkai-panel.conf de khong lam hong nginx dang chay."
    fi

    pkg_enable_service nginx || log_warn "nginx khong khoi dong duoc."
    systemctl reload nginx 2>/dev/null || true
    log_info "nginx dang phuc vu panel tren cong ${PANEL_PORT}."
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
    log_info "Da cau hinh xoay vong nhat ky."
}

# =============================================================================
# 14. SELinux
# =============================================================================
setup_selinux() {
    has getenforce || return 0
    local mode
    mode="$(getenforce 2>/dev/null || echo Disabled)"
    if [[ "$mode" == "Disabled" ]]; then
        log_info "SELinux dang tat, khong can xu ly ngu canh."
        return 0
    fi

    log_step "Xu ly SELinux (che do ${mode})"

    if has semanage; then
        # Cong panel phai duoc gan nhan http_port_t thi nginx moi bind duoc.
        if ! semanage port -l 2>/dev/null | grep -E '^http_port_t' | grep -qw "$PANEL_PORT"; then
            semanage port -a -t http_port_t -p tcp "$PANEL_PORT" 2>/dev/null ||
            semanage port -m -t http_port_t -p tcp "$PANEL_PORT" 2>/dev/null ||
            log_warn "Khong gan duoc nhan http_port_t cho cong ${PANEL_PORT}."
        fi
        # Thu muc website: nginx doc va ghi (upload, cache).
        semanage fcontext -a -t httpd_sys_rw_content_t "${WWW_DIR}(/.*)?" 2>/dev/null ||
        semanage fcontext -m -t httpd_sys_rw_content_t "${WWW_DIR}(/.*)?" 2>/dev/null || true
        semanage fcontext -a -t httpd_log_t "${LOG_SITES_DIR}(/.*)?" 2>/dev/null ||
        semanage fcontext -m -t httpd_log_t "${LOG_SITES_DIR}(/.*)?" 2>/dev/null || true
    else
        log_warn "Khong co 'semanage' (goi policycoreutils-python-utils). Bo qua nhan cong/ngu canh."
    fi

    if has restorecon; then
        restorecon -R "$WWW_DIR" >/dev/null 2>&1 || true
        restorecon -R "$LOG_SITES_DIR" >/dev/null 2>&1 || true
    fi

    if has setsebool; then
        # nginx can ket noi toi upstream loopback (API + UI).
        setsebool -P httpd_can_network_connect 1 2>/dev/null ||
            log_warn "Khong bat duoc httpd_can_network_connect. nginx co the khong proxy duoc."
    fi
    log_info "Da xu ly ngu canh SELinux."
}

# =============================================================================
# 15. Tuong lua
# =============================================================================
setup_firewall() {
    if [[ "$OPT_NO_FIREWALL" == "true" ]]; then
        log_warn "--no-firewall: khong dong toi tuong lua. Tu mo cong ${PANEL_PORT}/tcp."
        FIREWALL_TOOL="bo qua (--no-firewall)"
        return 0
    fi

    log_step "Mo cong panel tren tuong lua"

    if has ufw && ufw status 2>/dev/null | grep -qi '^Status: active'; then
        FIREWALL_TOOL="ufw"
        ufw allow "${PANEL_PORT}/tcp" >/dev/null 2>&1 || log_warn "ufw khong mo duoc cong ${PANEL_PORT}."
        ufw allow 80/tcp  >/dev/null 2>&1 || true
        ufw allow 443/tcp >/dev/null 2>&1 || true
        log_info "ufw: da mo ${PANEL_PORT}/tcp (panel) va 80,443/tcp (website khach)."
    elif has firewall-cmd && systemctl is-active --quiet firewalld; then
        FIREWALL_TOOL="firewalld"
        firewall-cmd --permanent --add-port="${PANEL_PORT}/tcp" >/dev/null 2>&1 || log_warn "firewalld khong mo duoc cong ${PANEL_PORT}."
        firewall-cmd --permanent --add-service=http  >/dev/null 2>&1 || true
        firewall-cmd --permanent --add-service=https >/dev/null 2>&1 || true
        firewall-cmd --reload >/dev/null 2>&1 || true
        log_info "firewalld: da mo ${PANEL_PORT}/tcp (panel) va http,https (website khach)."
    else
        FIREWALL_TOOL="none"
        log_warn "Khong tim thay ufw dang bat lan firewalld dang chay."
        log_warn "Neu may co tuong lua khac (iptables/nftables/security group cua nha cung cap),"
        log_warn "hay tu mo cong ${PANEL_PORT}/tcp, neu khong ban se khong vao duoc panel."
    fi

    log_warn "Khuyen nghi: gioi han cong ${PANEL_PORT} theo IP quan tri thay vi mo cho ca Internet."
}

# =============================================================================
# 16. Khoi dong + kiem tra
# =============================================================================
start_services() {
    log_step "Khoi dong dich vu"

    systemctl restart vkai-api || die "vkai-api khong khoi dong duoc. Xem: journalctl -u vkai-api -n 80"
    systemctl restart vkai-ui  || die "vkai-ui khong khoi dong duoc. Xem: journalctl -u vkai-ui -n 80"

    local i ok="false"
    for i in $(seq 1 30); do
        if curl -fsS --max-time 3 "http://127.0.0.1:${API_BIND_PORT}/health" >/dev/null 2>&1; then
            ok="true"; break
        fi
        sleep 1
    done
    if [[ "$ok" == "true" ]]; then
        log_info "API phan hoi /health."
    else
        log_warn "API chua tra loi /health sau 30 giay. Kiem tra: journalctl -u vkai-api -n 80"
    fi

    ok="false"
    for i in $(seq 1 30); do
        if curl -fsS --max-time 3 -o /dev/null "http://127.0.0.1:${UI_BIND_PORT}/" 2>/dev/null; then
            ok="true"; break
        fi
        sleep 1
    done
    if [[ "$ok" == "true" ]]; then
        log_info "Giao dien phan hoi tren cong ${UI_BIND_PORT}."
    else
        log_warn "Giao dien chua phan hoi. Kiem tra: journalctl -u vkai-ui -n 80"
    fi
}

install_vkai_cli() {
    local src="${SRC_DIR}/deploy/vkai.sh"
    [[ -f "$src" ]] || die "Khong thay ${src}"
    install -m 0755 "$src" /usr/local/bin/vkai
    log_info "Lenh quan tri: vkai (/usr/local/bin/vkai)"
}

# =============================================================================
# 17. Tong ket
# =============================================================================
print_summary() {
    local url="http://${SERVER_IP}:${PANEL_PORT}${PANEL_ENTRANCE}/"
    local pass_note=""
    [[ "$ADMIN_PASS_CHANGED" == "true" ]] || pass_note="  (!) Day la mat khau MAC DINH - doi ngay sau khi dang nhap."

    local summary
    summary="$(cat <<SUM
=============================================================================
 ${BRAND_NAME} v${VKAI_VERSION} - CAI DAT HOAN TAT
 ${BRAND_ORG}
 Thoi diem: $(ts)
 He dieu hanh: ${OS_PRETTY} (${ARCH})
=============================================================================

TRUY CAP PANEL
  URL day du : ${url}
  Cong       : ${PANEL_PORT}   (80/443 danh RIENG cho website cua khach)
  Loi vao    : ${PANEL_ENTRANCE}
  Vao sai duong dan se nhan 404 trung tinh - do la co y.

TAI KHOAN QUAN TRI
  Ten dang nhap : ${ADMIN_USER}
  Mat khau      : ${ADMIN_PASS}
  Email         : ${ADMIN_EMAIL}
${pass_note}

DUONG DAN DU LIEU
  Goc cai dat       : ${PANEL_ROOT}
  Ma/binary API     : ${CORE_DIR}            (bin/vkai-api)
  Ban build UI      : ${UI_DIR}              (.next/standalone/server.js)
  Agent             : ${AGENT_DIR}           (bin/vkai-agent)
  Ma nguon website  : ${WWW_DOMAINS_DIR}/<domain>
  Sao luu           : ${WWW_BACKUP_DIR}
  Trang mac dinh    : ${WWW_DEFAULT_DIR}
  Cau hinh          : ${ENV_FILE}
  Nhat ky panel     : ${LOG_DIR}
  Nhat ky theo site : ${LOG_SITES_DIR}/<domain>
  Chung chi         : ${SSL_DIR}
  Tam               : ${TMP_DIR}

DICH VU
  vkai-api   -> ${CORE_DIR}/bin/vkai-api        (loopback:${API_BIND_PORT})
  vkai-ui    -> ${UI_DIR}/.next/standalone/server.js (loopback:${UI_BIND_PORT})
  vkai-agent -> ${AGENT_DIR}/bin/vkai-agent     (tuy chon, chua bat)
  nginx      -> reverse proxy cong ${PANEL_PORT}
  CSDL       : ${DB_NAME} / nguoi dung ${DB_USER}
  Redis      : ${REDIS_SERVICE:-khong ro}
  Tuong lua  : ${FIREWALL_TOOL}

LENH QUAN TRI "vkai"
  vkai status              Xem trang thai dich vu
  vkai start|stop|restart  Dieu khien dich vu
  vkai logs api|ui|agent   Xem nhat ky
  vkai info                In lai thong tin truy cap
  vkai port 9001           Doi cong panel
  vkai entrance random     Sinh loi vao an toan moi
  vkai backup              Sao luu CSDL + cau hinh
  vkai update              Cap nhat va build lai
  vkai uninstall           Go cai dat
  vkai site create ...     Cac lenh nghiep vu (uy quyen cho vkai-cli)

CANH BAO
  * LUU LAI URL va mat khau tren. Mat khau khong the xem lai o dang ro.
  * Ban sao cua bang nay: ${SUMMARY_FILE} (quyen 600)
  * Nen gioi han cong ${PANEL_PORT} theo IP quan tri.
  * Doi mat khau quan tri va bat 2FA ngay khi dang nhap lan dau.
=============================================================================
SUM
)"

    printf '\n%s%s%s\n' "$C_GREEN" "$summary" "$C_OFF"

    umask 077
    printf '%s\n' "$summary" >"$SUMMARY_FILE"
    chown "$VKAI_USER:$VKAI_GROUP" "$SUMMARY_FILE"
    chmod 600 "$SUMMARY_FILE"
}

# =============================================================================
# 18. Go cai dat
# =============================================================================
do_uninstall() {
    banner
    log_warn "Chuan bi GO CAI DAT ${BRAND_NAME}."
    log_warn "Se dung va xoa: dich vu vkai-api/vkai-ui/vkai-agent, ${PANEL_ROOT}, cau hinh nginx cua panel."

    confirm "Xac nhan go cai dat?" || die "Da huy."

    local drop_db="false"
    if confirm "Xoa luon CSDL '${DB_NAME}' va toan bo du lieu website trong ${WWW_DIR}?"; then
        drop_db="true"
    fi

    log_step "Dung dich vu"
    local svc
    for svc in vkai-ui vkai-api vkai-agent; do
        systemctl disable --now "$svc" >/dev/null 2>&1 || true
        rm -f "/etc/systemd/system/${svc}.service"
    done
    for svc in vkai-frontend vkai-panel-api vkai-panel-frontend; do
        systemctl disable --now "$svc" >/dev/null 2>&1 || true
        rm -f "/etc/systemd/system/${svc}.service"
    done
    systemctl daemon-reload

    log_step "Go cau hinh nginx"
    rm -f /etc/nginx/conf.d/vkai-panel.conf
    rm -f /etc/nginx/sites-available/vkai-panel /etc/nginx/sites-enabled/vkai-panel
    if has nginx && nginx -t >/dev/null 2>&1; then
        systemctl reload nginx >/dev/null 2>&1 || true
    fi

    rm -f /etc/logrotate.d/vkai-panel /etc/profile.d/vkai-golang.sh
    rm -f /usr/local/bin/vkai /usr/local/bin/vkai-panelctl /usr/local/bin/vkai-cli
    [[ -L /etc/vkai ]] && rm -f /etc/vkai

    if [[ "$drop_db" == "true" ]]; then
        log_step "Xoa CSDL va du lieu"
        local name user
        name="$(env_get VKAI_DB_NAME || echo "$DB_NAME")"
        user="$(env_get VKAI_DB_USER || echo "$DB_USER")"
        sudo -u postgres psql -qc "DROP DATABASE IF EXISTS \"${name}\";" 2>/dev/null || true
        sudo -u postgres psql -qc "DROP ROLE IF EXISTS \"${user}\";" 2>/dev/null || true
        rm -rf "$PANEL_ROOT"
        userdel "$VKAI_USER" 2>/dev/null || true
        groupdel "$VKAI_GROUP" 2>/dev/null || true
        log_info "Da xoa CSDL, ${PANEL_ROOT} va nguoi dung ${VKAI_USER}."
    else
        log_step "Giu lai du lieu"
        rm -rf "$CORE_DIR" "$UI_DIR" "$AGENT_DIR"
        log_info "Da giu: ${WWW_DIR}, ${ETC_DIR}, ${LOG_DIR}, ${SSL_DIR} va CSDL '${DB_NAME}'."
    fi

    printf '\n%s%s da duoc go cai dat.%s\n\n' "$C_GREEN" "$BRAND_NAME" "$C_OFF"
}

# =============================================================================
# Luong chinh
# =============================================================================
main() {
    parse_args "$@"
    preflight_root "$@"
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

    # Chi tao duoc log sau khi biet chac se cai.
    mkdir -p "$LOG_DIR"
    start_logging
    log_info "Tuy chon: port='${OPT_PORT:-mac dinh}' entrance='${OPT_ENTRANCE:-ngau nhien}' skip-deps=${OPT_SKIP_DEPS} no-firewall=${OPT_NO_FIREWALL}"

    install_dependencies
    install_golang
    install_nodejs
    setup_user
    setup_directories

    # Cong panel duoc chot truoc khi kiem tra chiem dung.
    resolve_panel_access
    preflight_ports

    sync_sources

    # >>> THU TU BAT BUOC: cau hinh TRUOC, build UI SAU. <<<
    setup_config

    setup_database
    run_migrations
    setup_redis

    build_core
    build_agent
    build_ui           # doc NEXT_PUBLIC_API_URL tu .env vua sinh

    setup_admin_account
    install_systemd_units
    install_vkai_cli
    setup_nginx
    setup_logrotate
    setup_selinux
    setup_firewall
    start_services

    print_summary
}

main "$@"
