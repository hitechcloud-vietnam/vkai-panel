#!/usr/bin/env bash
# =============================================================================
# VKAI Panel - chuan bi moi truong PHAT TRIEN (khong phai cai dat may chu)
# HiTech Cloud (hitechcloud.vn)
#
# Cai dat len may chu that: sudo bash deploy/install.sh
# =============================================================================

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly REPO_ROOT
readonly CORE_DIR="${REPO_ROOT}/core"     # truoc day la backend/
readonly UI_DIR="${REPO_ROOT}/panel"      # truoc day la frontend/
readonly AGENT_DIR="${REPO_ROOT}/agent"

if [[ -t 1 ]]; then
    C_RED=$'\033[0;31m'; C_GREEN=$'\033[0;32m'; C_YELLOW=$'\033[1;33m'; C_OFF=$'\033[0m'
else
    C_RED=""; C_GREEN=""; C_YELLOW=""; C_OFF=""
fi

ok()   { printf '%s[OK]%s %s\n'   "$C_GREEN"  "$C_OFF" "$*"; }
warn() { printf '%s[!]%s %s\n'    "$C_YELLOW" "$C_OFF" "$*" >&2; }
die()  { printf '%s[X]%s %s\n'    "$C_RED"    "$C_OFF" "$*" >&2; exit 1; }

trap 'die "That bai o dong $LINENO: $BASH_COMMAND"' ERR

has() { command -v "$1" >/dev/null 2>&1; }

echo "========================================="
echo "  VKAI Panel - chuan bi moi truong dev"
echo "  HiTech Cloud (hitechcloud.vn)"
echo "========================================="
echo

# --- Kiem tra cong cu --------------------------------------------------------
echo "Kiem tra cong cu can thiet..."

has go || die "Chua co Go. Can Go 1.22 tro len."
ok "Go: $(go version | awk '{print $3}')"

has node || die "Chua co Node.js. Can Node.js 20 tro len."
ok "Node.js: $(node --version)"

has npm || die "Chua co npm."
ok "npm: $(npm --version)"

if has psql; then
    ok "PostgreSQL client: $(psql --version | awk '{print $3}')"
else
    warn "Khong co psql. Dung docker-compose de chay PostgreSQL."
fi

if has redis-cli; then
    ok "Redis client: $(redis-cli --version | awk '{print $2}')"
else
    warn "Khong co redis-cli. Dung docker-compose de chay Redis."
fi

# --- core/ (API Go) ----------------------------------------------------------
echo
echo "Chuan bi core/ (API Go)..."
cd "$CORE_DIR"
go mod tidy
ok "Da tai phu thuoc Go cho core/"

if [[ ! -f "${CORE_DIR}/config.yaml" && -f "${CORE_DIR}/config.yaml.example" ]]; then
    cp "${CORE_DIR}/config.yaml.example" "${CORE_DIR}/config.yaml"
    ok "Da tao core/config.yaml tu ban mau"
fi

# --- panel/ (giao dien Next.js) ---------------------------------------------
echo
echo "Chuan bi panel/ (giao dien Next.js)..."
cd "$UI_DIR"
if [[ -f package-lock.json ]]; then
    npm ci --no-audit --no-fund
else
    npm install --no-audit --no-fund
fi
ok "Da tai phu thuoc Node cho panel/"

# NEXT_PUBLIC_* duoc nhung vao bundle luc build, khong doc luc chay. O moi
# truong dev "next dev" doc .env moi lan khoi dong lai, nhung file phai co mat
# tai GOC du an panel/ - Next.js khong doc .env o thu muc khac.
if [[ ! -f "${UI_DIR}/.env" && ! -L "${UI_DIR}/.env" ]]; then
    if [[ -f "${REPO_ROOT}/.env" ]]; then
        ln -sfn "${REPO_ROOT}/.env" "${UI_DIR}/.env"
        ok "Da lien ket panel/.env -> .env o goc repo"
    else
        warn "Chua co panel/.env. Sao chep tu .env.example va dien gia tri that."
    fi
fi

# --- agent/ ------------------------------------------------------------------
echo
echo "Chuan bi agent/..."
cd "$AGENT_DIR"
go mod tidy
ok "Da tai phu thuoc Go cho agent/"

cd "$REPO_ROOT"

cat <<GUIDE

=========================================
  Xong!
=========================================

Chay o che do phat trien:

  1. Dich vu nen (CSDL, cache):
     docker compose -f docker-compose.dev.yml up -d postgres redis

  2. API (core/):
     cd core && go run ./cmd/api

  3. Giao dien (panel/):
     cd panel && npm run dev

  4. Agent (tuy chon):
     cd agent && VKAI_PANEL_URL=http://localhost:30110 \\
        VKAI_AGENT_TOKEN=<token> go run ./cmd

Giao dien dev: http://localhost:3000

Luu y:
  * Cai len may chu that thi dung: sudo bash deploy/install.sh
    (cai vao /vkai-panel, cong panel rieng + loi vao an toan).
  * Cong 80/443 danh RIENG cho website cua khach, panel khong dung.
  * Tai khoan mac dinh admin/admin123 CHI danh cho moi truong dev.

GUIDE
