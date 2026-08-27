#!/usr/bin/env bash
# =============================================================================
# VKAI Panel - dung moi truong PHAT TRIEN chay TRAN tren may lap trinh vien
# HiTechCloud (hitechcloud.vn)
#
#   bash setup-dev.sh                  # cai day du (co hoi truoc khi dong cham)
#   bash setup-dev.sh --yes            # khong hoi gi ca
#   bash setup-dev.sh --skip-deps      # khong dung toi trinh quan ly goi
#
# KHONG CO DOCKER trong luong nay. PostgreSQL va Redis duoc cai TRAN bang
# trinh quan ly goi cua he dieu hanh, giong het cach bo cai may chu
# deploy/install.sh lam. Docker trong san pham chi con la TINH NANG cho khach
# (quan ly container cua ho), khong phai cach dung panel.
#
# Script nay idempotent: chay lai nhieu lan khong hong gi, khong ghi de .env
# da co.
# =============================================================================

set -Eeuo pipefail

# -----------------------------------------------------------------------------
# Hang so
# -----------------------------------------------------------------------------
readonly BRAND_NAME="VKAI Panel"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly ROOT_DIR
readonly CORE_DIR="${ROOT_DIR}/core"
readonly UI_DIR="${ROOT_DIR}/panel"
readonly AGENT_DIR="${ROOT_DIR}/agent"
readonly DEV_DIR="${ROOT_DIR}/.dev"
readonly MIGRATIONS_STATE="${DEV_DIR}/migrations.applied"

# Giu dong bo voi deploy/install.sh.
readonly GO_VERSION="1.22.5"
readonly GO_MIN_MINOR="22"
readonly NODE_VERSION="20.18.1"
readonly NODE_MIN_MAJOR="20"

# CSDL cua may phat trien. Mat khau nay CHI dung tren may lap trinh vien.
readonly DB_NAME="vkai_panel"
readonly DB_USER="vkai"
readonly DB_PASS="vkai_dev_password"

readonly API_PORT="30110"
readonly UI_PORT="3000"
readonly PANEL_PORT="8888"

# -----------------------------------------------------------------------------
# Tuy chon dong lenh
# -----------------------------------------------------------------------------
OPT_SKIP_DEPS="false"
OPT_ASSUME_YES="false"

# -----------------------------------------------------------------------------
# Trang thai suy ra
# -----------------------------------------------------------------------------
OS_ID=""; OS_VERSION_ID=""; OS_MAJOR=""; OS_LIKE=""; OS_PRETTY=""
OS_FAMILY=""          # debian | rhel | suse
PKG=""                # apt-get | dnf | yum | zypper
ARCH=""; NODE_ARCH=""; GO_ARCH=""
PG_SERVICE=""
REDIS_SERVICE=""
SUDO=""

# -----------------------------------------------------------------------------
# Ghi nhat ky
# -----------------------------------------------------------------------------
if [[ -t 1 ]]; then
    C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'; C_YELLOW=$'\033[1;33m'
    C_BLUE=$'\033[0;34m'; C_BOLD=$'\033[1m'; C_OFF=$'\033[0m'
else
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_BOLD=""; C_OFF=""
fi

ts() { date '+%H:%M:%S'; }
log_info()  { printf '%s[INFO ]%s %s %s\n' "$C_GREEN"  "$C_OFF" "$(ts)" "$*"; }
log_step()  { printf '\n%s[BUOC ]%s %s %s%s%s\n' "$C_BLUE" "$C_OFF" "$(ts)" "$C_BOLD" "$*" "$C_OFF"; }
log_warn()  { printf '%s[CANH ]%s %s %s\n' "$C_YELLOW" "$C_OFF" "$(ts)" "$*" >&2; }
log_error() { printf '%s[LOI  ]%s %s %s\n' "$C_RED"    "$C_OFF" "$(ts)" "$*" >&2; }

die() { log_error "$*"; exit 1; }
on_error() { log_error "That bai o dong $2 (ma loi $1): $3"; }
trap 'on_error $? $LINENO "$BASH_COMMAND"' ERR

has() { command -v "$1" >/dev/null 2>&1; }

confirm() {
    [[ "$OPT_ASSUME_YES" == "true" ]] && return 0
    local ans
    read -r -p "$1 [y/N] " ans </dev/tty || return 1
    [[ "$ans" =~ ^[Yy]$ ]]
}

banner() {
    printf '%s\n' "$C_BLUE"
    echo "============================================================"
    echo "  ${BRAND_NAME} - moi truong phat trien (chay tran, khong Docker)"
    echo "  HiTechCloud - hitechcloud.vn"
    echo "============================================================"
    printf '%s\n' "$C_OFF"
}

usage() {
    cat <<USAGE
${BRAND_NAME} - dung moi truong phat trien tren may lap trinh vien

  bash setup-dev.sh [tuy chon]

Tuy chon:
  --skip-deps     Khong cai goi he thong / Go / Node (chi sinh cau hinh,
                  tai phu thuoc du an va chuan bi CSDL neu da co san).
  --yes, -y       Khong hoi xac nhan.
  --help, -h      In tro giup nay.

Script can quyen root (qua sudo) de cai PostgreSQL, Redis, Go va Node.
Phan con lai chay bang tai khoan thuong.
USAGE
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --skip-deps) OPT_SKIP_DEPS="true" ;;
            --yes|-y)    OPT_ASSUME_YES="true" ;;
            --help|-h)   usage; exit 0 ;;
            *)           usage >&2; die "Tuy chon khong hop le: $1" ;;
        esac
        shift
    done
}

# Chay mot lenh bang quyen root. Tren may dev thuong khong dang nhap bang root.
run_root() {
    if [[ "${EUID}" -eq 0 ]]; then
        "$@"
    elif [[ -n "$SUDO" ]]; then
        $SUDO "$@"
    else
        die "Can quyen root de chay: $* — hay cai sudo, hoac chay lai script bang root."
    fi
}

detect_sudo() {
    if [[ "${EUID}" -eq 0 ]]; then
        SUDO=""
        log_info "Dang chay bang root."
    elif has sudo; then
        SUDO="sudo"
        log_info "Se dung sudo cho cac buoc can quyen root."
    else
        SUDO=""
        log_warn "Khong co sudo va khong phai root: cac buoc cai goi/dich vu se dung lai."
    fi
}

# =============================================================================
# 1. Nhan dien he dieu hanh
#    Giong het deploy/install.sh: doc /etc/os-release, suy ra ho OS, KHONG doan.
#    Sua o mot noi thi sua ca hai.
# =============================================================================
detect_arch() {
    local machine
    machine="$(uname -m)"
    case "$machine" in
        x86_64|amd64)   ARCH="amd64"; GO_ARCH="amd64"; NODE_ARCH="x64" ;;
        aarch64|arm64)  ARCH="arm64"; GO_ARCH="arm64"; NODE_ARCH="arm64" ;;
        *) die "Kien truc '${machine}' khong duoc ho tro (chi x86_64 va aarch64)." ;;
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
        local rel
        rel="$(cat /etc/redhat-release)"
        OS_PRETTY="$rel"
        OS_VERSION_ID="$(grep -oE '[0-9]+(\.[0-9]+)?' <<<"$rel" | head -n1)"
        case "${rel,,}" in
            *centos*)   OS_ID="centos" ;;
            *rocky*)    OS_ID="rocky" ;;
            *alma*)     OS_ID="almalinux" ;;
            *fedora*)   OS_ID="fedora" ;;
            *red\ hat*) OS_ID="rhel" ;;
            *)          OS_ID="" ;;
        esac
        OS_LIKE="rhel fedora"
    else
        die "Khong doc duoc /etc/os-release lan /etc/redhat-release. Script khong doan bua he dieu hanh."
    fi

    [[ -n "$OS_ID" ]] || die "Khong xac dinh duoc ID he dieu hanh trong /etc/os-release."
    OS_MAJOR="${OS_VERSION_ID%%.*}"
    [[ -n "$OS_MAJOR" ]] || OS_MAJOR="0"
}

resolve_os_family() {
    case "$OS_ID" in
        ubuntu|debian|linuxmint|pop|elementary|raspbian)
            OS_FAMILY="debian" ;;
        centos|rhel|redhat|rocky|almalinux|ol|oracle|fedora|amzn)
            OS_FAMILY="rhel" ;;
        opensuse-leap|opensuse|sles|sled)
            OS_FAMILY="suse" ;;
        *)
            case " ${OS_LIKE} " in
                *debian*|*ubuntu*)        OS_FAMILY="debian" ;;
                *rhel*|*fedora*|*centos*) OS_FAMILY="rhel" ;;
                *suse*)                   OS_FAMILY="suse" ;;
                *) OS_FAMILY="" ;;
            esac
            ;;
    esac

    if [[ -z "$OS_FAMILY" ]]; then
        die "He dieu hanh '${OS_PRETTY}' (ID=${OS_ID}) khong suy ra duoc ho OS.
May phat trien duoc ho tro: Ubuntu/Debian, RHEL/CentOS/Rocky/Alma/Fedora, openSUSE."
    fi
    log_info "He dieu hanh: ${OS_PRETTY} (ID=${OS_ID}, ho=${OS_FAMILY}, kien truc=${ARCH})"
}

# =============================================================================
# 2. Lop truu tuong trinh quan ly goi (giong deploy/install.sh)
# =============================================================================
detect_pkg_manager() {
    case "$OS_FAMILY" in
        debian) has apt-get || die "Khong tim thay apt-get."; PKG="apt-get" ;;
        rhel)
            if has dnf; then PKG="dnf"
            elif has yum; then PKG="yum"
            else die "Khong tim thay dnf lan yum."
            fi
            ;;
        suse) has zypper || die "Khong tim thay zypper."; PKG="zypper" ;;
        *)    die "Ho he dieu hanh khong xac dinh: '${OS_FAMILY}'" ;;
    esac
    log_info "Trinh quan ly goi: ${PKG}"
}

apt_run() {
    run_root env DEBIAN_FRONTEND=noninteractive apt-get \
        -o DPkg::Lock::Timeout=600 \
        -o Dpkg::Options::=--force-confdef \
        -o Dpkg::Options::=--force-confold \
        "$@"
}

pkg_update() {
    case "$PKG" in
        apt-get) apt_run update -qq ;;
        dnf)     run_root dnf -y makecache ;;
        yum)     run_root yum -y makecache ;;
        zypper)  run_root zypper --non-interactive --gpg-auto-import-keys refresh ;;
    esac
}

pkg_install() {
    [[ $# -gt 0 ]] || return 0
    case "$PKG" in
        apt-get) apt_run install -y "$@" ;;
        dnf)     run_root dnf install -y "$@" ;;
        yum)     run_root yum install -y "$@" ;;
        zypper)  run_root zypper --non-interactive install --auto-agree-with-licenses "$@" ;;
    esac
}

pkg_install_optional() {
    local p
    for p in "$@"; do
        [[ -n "$p" ]] || continue
        if ! pkg_install "$p" >/dev/null 2>&1; then
            log_warn "Kho goi khong co '${p}', bo qua."
        fi
    done
}

# Enable + start mot dich vu systemd, chiu duoc ten khac nhau giua cac ho OS.
svc_enable() {
    local svc="$1"
    has systemctl || { log_warn "Khong co systemd, tu khoi dong '${svc}' bang tay."; return 1; }
    if ! systemctl list-unit-files 2>/dev/null | grep -q "^${svc}\.service"; then
        log_warn "Khong tim thay unit '${svc}.service'."
        return 1
    fi
    run_root systemctl enable "$svc" >/dev/null 2>&1 || log_warn "Khong enable duoc ${svc}"
    run_root systemctl restart "$svc" || {
        log_error "Khong khoi dong duoc ${svc}. Xem: journalctl -u ${svc} -n 50"
        return 1
    }
    return 0
}

pkg_names_for() {
    local generic="$1"
    case "$generic" in
        base)
            case "$OS_FAMILY" in
                debian) echo "curl wget git tar unzip xz-utils openssl ca-certificates jq lsof procps" ;;
                rhel)   echo "curl wget git tar unzip xz openssl ca-certificates jq lsof procps-ng" ;;
                suse)   echo "curl wget git-core tar unzip xz openssl ca-certificates jq lsof procps" ;;
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
    esac
}

# =============================================================================
# 3. Goi he thong
# =============================================================================
install_dependencies() {
    if [[ "$OPT_SKIP_DEPS" == "true" ]]; then
        log_warn "--skip-deps: bo qua buoc cai goi he thong."
        return 0
    fi

    log_step "Cai goi he thong (PostgreSQL, Redis, cong cu build)"
    confirm "Cho phep cai goi bang ${PKG}?" || die "Da huy. Chay lai voi --skip-deps neu ban tu cai."

    pkg_update
    # shellcheck disable=SC2046
    pkg_install $(pkg_names_for base)
    # shellcheck disable=SC2046
    pkg_install_optional $(pkg_names_for buildtools)
    # shellcheck disable=SC2046
    pkg_install $(pkg_names_for postgresql)
    # shellcheck disable=SC2046
    pkg_install $(pkg_names_for redis)
    log_info "Da cai xong goi he thong."
}

# =============================================================================
# 4. Go va Node (chi cai khi thieu hoac qua cu)
# =============================================================================
verify_sha256() {
    local file="$1" expected="$2" actual
    if [[ -z "$expected" ]]; then
        log_warn "Khong lay duoc checksum chinh thuc cho $(basename "$file")."
        confirm "Van dung tep vua tai ve?" || die "Da huy vi khong xac minh duoc checksum."
        return 0
    fi
    actual="$(sha256sum "$file" | awk '{print $1}')"
    [[ "$actual" == "$expected" ]] ||
        die "Checksum sai cho $(basename "$file"): cho doi ${expected}, nhan duoc ${actual}."
}

go_needs_install() {
    local v major minor
    has go || return 0
    v="$(go version 2>/dev/null | awk '{print $3}')"; v="${v#go}"
    major="${v%%.*}"; minor="${v#*.}"; minor="${minor%%.*}"
    [[ "$major" =~ ^[0-9]+$ ]] || return 0
    [[ "$minor" =~ ^[0-9]+$ ]] || return 0
    (( major > 1 )) && return 1
    (( major == 1 && minor >= GO_MIN_MINOR )) && return 1
    return 0
}

install_golang() {
    if ! go_needs_install; then
        log_info "Go da dat yeu cau: $(go version)"
        return 0
    fi
    if [[ "$OPT_SKIP_DEPS" == "true" ]]; then
        die "Thieu Go >= 1.${GO_MIN_MINOR} nhung dang bat --skip-deps. Cai Go roi chay lai."
    fi

    log_step "Cai Go ${GO_VERSION} (${GO_ARCH})"
    local tarball url sha_url expected tmp
    tarball="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
    url="https://go.dev/dl/${tarball}"
    sha_url="https://dl.google.com/go/${tarball}.sha256"
    tmp="$(mktemp -d)"

    curl -fsSL --retry 3 --connect-timeout 20 -o "${tmp}/${tarball}" "$url" ||
        die "Khong tai duoc Go tu ${url}"
    expected="$(curl -fsSL --retry 3 --connect-timeout 20 "$sha_url" 2>/dev/null | tr -d ' \n' || true)"
    verify_sha256 "${tmp}/${tarball}" "$expected"

    run_root rm -rf /usr/local/go
    run_root tar -C /usr/local -xzf "${tmp}/${tarball}"
    rm -rf "$tmp"

    run_root ln -sf /usr/local/go/bin/go    /usr/local/bin/go
    run_root ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
    export PATH="/usr/local/go/bin:$PATH"
    log_info "Da cai: $(go version)"
}

node_needs_install() {
    local v major
    has node || return 0
    v="$(node --version 2>/dev/null)"; v="${v#v}"; major="${v%%.*}"
    [[ "$major" =~ ^[0-9]+$ ]] || return 0
    (( major >= NODE_MIN_MAJOR )) && return 1
    return 0
}

install_nodejs() {
    if ! node_needs_install; then
        log_info "Node.js da dat yeu cau: $(node --version)"
        return 0
    fi
    if [[ "$OPT_SKIP_DEPS" == "true" ]]; then
        die "Thieu Node.js >= ${NODE_MIN_MAJOR} nhung dang bat --skip-deps. Cai Node roi chay lai."
    fi

    log_step "Cai Node.js ${NODE_VERSION} (${NODE_ARCH})"
    local tarball url expected tmp
    tarball="node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz"
    url="https://nodejs.org/dist/v${NODE_VERSION}/${tarball}"
    tmp="$(mktemp -d)"

    curl -fsSL --retry 3 --connect-timeout 20 -o "${tmp}/${tarball}" "$url" ||
        die "Khong tai duoc Node.js tu ${url}"
    expected="$(curl -fsSL --retry 3 --connect-timeout 20 \
        "https://nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt" 2>/dev/null |
        awk -v f="$tarball" '$2 == f {print $1}' || true)"
    verify_sha256 "${tmp}/${tarball}" "$expected"

    run_root rm -rf "/usr/local/lib/nodejs-v${NODE_VERSION}"
    run_root mkdir -p /usr/local/lib
    run_root tar -C /usr/local/lib -xJf "${tmp}/${tarball}"
    run_root mv "/usr/local/lib/node-v${NODE_VERSION}-linux-${NODE_ARCH}" \
                "/usr/local/lib/nodejs-v${NODE_VERSION}"
    rm -rf "$tmp"

    run_root ln -sfn "/usr/local/lib/nodejs-v${NODE_VERSION}" /usr/local/lib/nodejs
    local b
    for b in node npm npx; do
        run_root ln -sf "/usr/local/lib/nodejs/bin/${b}" "/usr/local/bin/${b}"
    done
    export PATH="/usr/local/lib/nodejs/bin:$PATH"
    log_info "Da cai: $(node --version)"
}

# =============================================================================
# 5. PostgreSQL cho may phat trien
# =============================================================================
detect_pg_service() {
    local svc
    for svc in postgresql postgresql-16 postgresql-15 postgresql-14 postgresql-13; do
        if systemctl list-unit-files 2>/dev/null | grep -q "^${svc}\.service"; then
            PG_SERVICE="$svc"
            return 0
        fi
    done
    PG_SERVICE="postgresql"
}

pg_initdb_if_needed() {
    # Debian/Ubuntu tu khoi tao cum khi cai goi. RHEL thi khong.
    [[ "$OS_FAMILY" == "rhel" ]] || return 0
    local setup_bin
    setup_bin="$(command -v postgresql-setup || true)"
    [[ -n "$setup_bin" ]] || return 0
    [[ -f /var/lib/pgsql/data/PG_VERSION ]] && return 0
    log_info "Khoi tao cum PostgreSQL..."
    run_root "$setup_bin" --initdb >/dev/null 2>&1 ||
        run_root "$setup_bin" initdb >/dev/null 2>&1 || true
}

# Chay mot lenh bang tai khoan he thong "postgres". Uu tien sudo (chay duoc ca
# khi dang la root), du phong runuser khi may khong co sudo.
as_postgres() {
    if has sudo; then
        sudo -u postgres "$@"
    elif [[ "${EUID}" -eq 0 ]] && has runuser; then
        runuser -u postgres -- "$@"
    else
        die "Can quyen root de chay lenh bang tai khoan postgres: $*"
    fi
}

psql_super() { as_postgres psql "$@"; }

# Cho phep core/ ket noi qua 127.0.0.1 bang mat khau. Phuong thuc PHAI khop
# cach may chu bam mat khau (md5 tren PG<=13, scram-sha-256 tu PG14).
pg_hba_allow_local_tcp() {
    local hba method tmp
    hba="$(psql_super -tAc 'SHOW hba_file' 2>/dev/null || true)"
    [[ -n "$hba" ]] || { log_warn "Khong xac dinh duoc pg_hba.conf, bo qua."; return 0; }

    if run_root grep -qE "^host[[:space:]]+${DB_NAME}[[:space:]]+${DB_USER}[[:space:]]" "$hba"; then
        log_info "pg_hba.conf da co quy tac cho ${DB_USER}."
        return 0
    fi

    method="$(psql_super -tAc 'SHOW password_encryption' 2>/dev/null | tr -d '[:space:]' || true)"
    case "$method" in
        scram-sha-256|md5) : ;;
        *) method="scram-sha-256" ;;
    esac

    log_info "Them quy tac pg_hba.conf cho ${DB_USER}@127.0.0.1 (${method})."
    run_root cp -a "$hba" "${hba}.vkai.bak.$(date +%Y%m%d%H%M%S)"
    tmp="$(mktemp)"
    {
        echo "# --- ${BRAND_NAME} (dev): truy cap CSDL cua panel qua loopback ---"
        echo "host    ${DB_NAME}    ${DB_USER}    127.0.0.1/32    ${method}"
        echo "host    ${DB_NAME}    ${DB_USER}    ::1/128         ${method}"
        run_root cat "$hba"
    } >"$tmp"
    run_root tee "$hba" >/dev/null <"$tmp"
    rm -f "$tmp"
    if has systemctl; then
        run_root systemctl reload "$PG_SERVICE" 2>/dev/null ||
            run_root systemctl restart "$PG_SERVICE"
    else
        log_warn "Khong co systemd: hay nap lai cau hinh PostgreSQL bang tay de quy tac pg_hba co hieu luc."
    fi
}

setup_database() {
    log_step "Chuan bi PostgreSQL (chay tran, khong container)"

    if ! has psql && [[ "$OPT_SKIP_DEPS" == "true" ]]; then
        log_warn "Khong thay psql va dang bat --skip-deps: bo qua buoc CSDL."
        return 0
    fi

    detect_pg_service
    pg_initdb_if_needed
    if has systemctl; then
        svc_enable "$PG_SERVICE" ||
            die "PostgreSQL khong khoi dong duoc. Xem: journalctl -u ${PG_SERVICE} -n 50"
    else
        log_warn "Khong co systemd tren may nay: hay tu khoi dong PostgreSQL truoc (vd: service postgresql start)."
    fi

    local i
    for i in $(seq 1 30); do
        psql_super -tAc 'SELECT 1' >/dev/null 2>&1 && break
        sleep 1
        (( i == 30 )) && die "PostgreSQL khong phan hoi sau 30 giay."
    done

    local esc_pass="${DB_PASS//\'/\'\'}"
    if psql_super -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1; then
        psql_super -qc "ALTER ROLE \"${DB_USER}\" WITH LOGIN PASSWORD '${esc_pass}';"
        log_info "Da cap nhat mat khau vai tro '${DB_USER}'."
    else
        psql_super -qc "CREATE ROLE \"${DB_USER}\" WITH LOGIN PASSWORD '${esc_pass}';"
        log_info "Da tao vai tro CSDL '${DB_USER}'."
    fi

    if psql_super -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
        log_info "CSDL '${DB_NAME}' da ton tai."
    else
        as_postgres createdb -O "$DB_USER" "$DB_NAME"
        log_info "Da tao CSDL '${DB_NAME}' (chu so huu ${DB_USER})."
    fi

    # migration 001 can uuid_generate_v4(); extension phai do superuser cai.
    psql_super -q -d "$DB_NAME" -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp";' ||
        die "Khong cai duoc extension uuid-ossp. Thieu goi postgresql-contrib?"

    pg_hba_allow_local_tcp
}

run_migrations() {
    local dir="${CORE_DIR}/migrations"
    [[ -d "$dir" ]] || { log_warn "Khong thay ${dir}, bo qua migration."; return 0; }
    has psql || { log_warn "Khong co psql, bo qua migration."; return 0; }

    log_step "Ap dung migration CSDL cho may phat trien"
    mkdir -p "$DEV_DIR"
    touch "$MIGRATIONS_STATE"

    local f name applied=0
    while IFS= read -r f; do
        name="$(basename "$f")"
        grep -qxF "$name" "$MIGRATIONS_STATE" && continue
        log_info "  -> ${name}"
        PGPASSWORD="$DB_PASS" psql -h 127.0.0.1 -p 5432 -U "$DB_USER" -d "$DB_NAME" \
            -v ON_ERROR_STOP=1 --quiet -f "$f" >/dev/null ||
            die "Migration '${name}' that bai. CSDL dev dang do dang."
        echo "$name" >>"$MIGRATIONS_STATE"
        applied=$((applied + 1))
    done < <(find "$dir" -maxdepth 1 -name '*.sql' -type f | sort)

    log_info "Da ap dung ${applied} migration moi."
}

# =============================================================================
# 6. Redis cho may phat trien
# =============================================================================
setup_redis() {
    log_step "Chuan bi Redis (chay tran, khong container)"

    case "$OS_FAMILY" in
        debian) REDIS_SERVICE="redis-server" ;;
        *)      REDIS_SERVICE="redis" ;;
    esac
    if ! systemctl list-unit-files 2>/dev/null | grep -q "^${REDIS_SERVICE}\.service"; then
        local alt
        for alt in redis6 redis redis-server; do
            if systemctl list-unit-files 2>/dev/null | grep -q "^${alt}\.service"; then
                REDIS_SERVICE="$alt"
                break
            fi
        done
    fi

    if svc_enable "$REDIS_SERVICE"; then
        log_info "Dich vu Redis: ${REDIS_SERVICE}"
    else
        log_warn "Redis chua chay. API van khoi dong duoc nhung mat cache/hang doi."
    fi
}

# =============================================================================
# 7. Cau hinh: core/.env va panel/.env.local
# =============================================================================
setup_core_env() {
    log_step "Sinh cau hinh core/.env"

    if [[ -f "${CORE_DIR}/.env" ]]; then
        log_info "core/.env da co, giu nguyen (xoa di roi chay lai neu muon sinh moi)."
        return 0
    fi

    mkdir -p "${DEV_DIR}/vkai-panel/www/domains" "${DEV_DIR}/vkai-panel/www/backup"

    # Cau hinh doc qua viper voi SetEnvPrefix("VKAI") nen chi bien co tien to
    # VKAI_ moi duoc doc.
    cat >"${CORE_DIR}/.env" <<COREENV
# =============================================================================
# ${BRAND_NAME} - cau hinh core/ cho MAY PHAT TRIEN
# Sinh boi setup-dev.sh. KHONG dung tren may that.
# PostgreSQL va Redis chay TRAN tren chinh may nay (khong Docker).
# =============================================================================
VKAI_PANEL_ENABLED=true
VKAI_PANEL_PORT=${PANEL_PORT}
VKAI_PANEL_BIND=127.0.0.1
# De trong: loi vao an toan se duoc sinh va in ra trong banner khoi dong.
VKAI_PANEL_ENTRANCE=
VKAI_PANEL_CONFIG_FILE=./.panel_access.json

VKAI_SERVER_HOST=127.0.0.1
VKAI_SERVER_PORT=${API_PORT}
VKAI_SERVER_MODE=debug

# --- CSDL (dich vu he thong tren may nay) ------------------------------------
VKAI_DB_HOST=127.0.0.1
VKAI_DB_PORT=5432
VKAI_DB_NAME=${DB_NAME}
VKAI_DB_USER=${DB_USER}
VKAI_DB_PASSWORD=${DB_PASS}
VKAI_DB_SSLMODE=disable

# --- Redis (dich vu he thong tren may nay) -----------------------------------
VKAI_REDIS_HOST=127.0.0.1
VKAI_REDIS_PORT=6379
VKAI_REDIS_PASSWORD=
VKAI_REDIS_DB=0

# --- Bi mat: CHI danh cho may phat trien -------------------------------------
VKAI_JWT_SECRET=dev-only-a1b2c3d4e5f60718293a4b5c6d7e8f90
VKAI_JWT_ISSUER=vkai-panel
VKAI_AGENT_TOKEN=dev-only-agent-token
VKAI_AGENT_PORT=30111

# --- Duong dan (tro vao .dev/ trong repo, khong dung / that) -----------------
VKAI_PANEL_ROOT=${DEV_DIR}/vkai-panel
VKAI_WEB_ROOT=${DEV_DIR}/vkai-panel/www/domains
VKAI_FILEMANAGER_ROOT=${DEV_DIR}/vkai-panel/www/domains
VKAI_BACKUP_ROOT=${DEV_DIR}/vkai-panel/www/backup

# --- Nhat ky -----------------------------------------------------------------
VKAI_LOG_LEVEL=debug
VKAI_LOG_FORMAT=text
COREENV
    chmod 600 "${CORE_DIR}/.env"
    log_info "Da tao ${CORE_DIR}/.env"
}

setup_panel_env() {
    log_step "Sinh cau hinh panel/.env.local"

    if [[ -f "${UI_DIR}/.env.local" ]]; then
        log_info "panel/.env.local da co, giu nguyen."
        return 0
    fi

    # NEXT_PUBLIC_* la bien INLINE LUC BUILD: Next.js thay thang gia tri vao
    # bundle khi "npm run build", KHONG doc lai luc chay. Doi gia tri thi phai
    # build lai (o dev: restart "npm run dev").
    cat >"${UI_DIR}/.env.local" <<PANELENV
# ${BRAND_NAME} - cau hinh giao dien cho MAY PHAT TRIEN (sinh boi setup-dev.sh)
#
# URL API tinh theo goc nhin cua TRINH DUYET.
# LUU Y: day la bien inline LUC BUILD. Sua xong phai khoi dong lai "npm run dev"
# (hoac build lai) thi gia tri moi co tac dung.
NEXT_PUBLIC_API_URL=http://localhost:${API_PORT}
PANELENV
    log_info "Da tao ${UI_DIR}/.env.local"
}

# =============================================================================
# 8. Phu thuoc cua du an
# =============================================================================
setup_project_deps() {
    log_step "Tai phu thuoc cua du an"

    ( cd "$CORE_DIR"  && go mod download )
    log_info "core/: xong."
    ( cd "$AGENT_DIR" && go mod download )
    log_info "agent/: xong."

    if [[ -f "${UI_DIR}/package-lock.json" ]]; then
        ( cd "$UI_DIR" && npm ci --no-audit --no-fund )
    else
        ( cd "$UI_DIR" && npm install --no-audit --no-fund )
    fi
    log_info "panel/: xong."
}

setup_hooks() {
    if [[ -d "${ROOT_DIR}/.git" && -d "${ROOT_DIR}/githooks" ]]; then
        git -C "$ROOT_DIR" config core.hooksPath githooks
        log_info "Da dat core.hooksPath=githooks"
    fi
}

# =============================================================================
# 9. Huong dan chay
# =============================================================================
print_usage_guide() {
    cat <<GUIDE

${C_GREEN}Moi truong phat trien da san sang.${C_OFF}

${C_BLUE}1) API (core/) - cua so thu nhat${C_OFF}
   cd ${CORE_DIR}
   go run ./cmd/api
   -> API: http://127.0.0.1:${API_PORT}
   -> Cong panel (khi bat access gate): http://127.0.0.1:${PANEL_PORT}/<loi-vao>/
      Loi vao duoc in ra trong banner khoi dong cua API.

${C_BLUE}2) Giao dien (panel/) - cua so thu hai${C_OFF}
   cd ${UI_DIR}
   npm run dev
   -> UI: http://localhost:${UI_PORT}

${C_BLUE}3) Agent (tuy chon) - cua so thu ba${C_OFF}
   cd ${AGENT_DIR}
   VKAI_PANEL_URL=http://127.0.0.1:${API_PORT} \\
   VKAI_AGENT_TOKEN=dev-only-agent-token \\
   go run ./cmd

${C_BLUE}Dich vu nen (dich vu he thong, khong container)${C_OFF}
   PostgreSQL : ${PG_SERVICE:-postgresql}  (127.0.0.1:5432, CSDL ${DB_NAME}, user ${DB_USER})
   Redis      : ${REDIS_SERVICE:-redis}    (127.0.0.1:6379)
   systemctl status ${PG_SERVICE:-postgresql} ${REDIS_SERVICE:-redis}

${C_BLUE}Kiem tra ban build that (giong may chu)${C_OFF}
   cd ${UI_DIR} && npm run build
   ls .next/standalone/server.js .next/standalone/.next/static
   Thieu mot trong hai -> trinh duyet bao
   "Application error: a client-side exception has occurred".

${C_YELLOW}Cai len may chu that:${C_OFF} sudo bash deploy/install.sh
${C_YELLOW}Cong 80/443 danh RIENG cho website cua khach, panel khong dung.${C_OFF}

GUIDE
}

# =============================================================================
main() {
    parse_args "$@"
    banner
    detect_sudo
    detect_arch
    detect_os
    resolve_os_family
    detect_pkg_manager

    install_dependencies
    install_golang
    install_nodejs

    setup_database
    setup_redis

    setup_core_env
    setup_panel_env
    run_migrations

    setup_project_deps
    setup_hooks

    print_usage_guide
}

main "$@"
