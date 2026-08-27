#!/usr/bin/env bash
#
# build-protected.sh — dựng bản PHÁT HÀNH đã được làm rối, dùng để giao cho khách.
#
# Khác biệt cần nắm rõ:
#   - tools/protect/cmd/vkai-crypt  : mã hoá MÃ NGUỒN trong repository (chống đọc trên GitHub).
#   - build-protected.sh (file này) : làm rối SẢN PHẨM BIÊN DỊCH (chống đọc trên máy khách).
# Hai lớp độc lập nhau, không thay thế cho nhau.
#
# Đầu ra: dist/<phiên bản>/ gồm binary API + agent đã garble, bản build UI, checksum và manifest.
#
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST="${ROOT}/dist"
VERSION="${VKAI_VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUT="${DIST}/${VERSION}"

# Mặc định: làm rối Go (rẻ, không ảnh hưởng hiệu năng đáng kể).
# Làm rối JS phía trình duyệt TẮT mặc định vì làm phình bundle và chậm parse;
# chỉ nên bật cho phần logic lõi/kiểm tra bản quyền.
OBFUSCATE_GO="${VKAI_OBFUSCATE_GO:-1}"
OBFUSCATE_JS="${VKAI_OBFUSCATE_JS:-0}"
BYTECODE_SERVER="${VKAI_BYTECODE_SERVER:-0}"

log()  { printf '\033[0;34m[build]\033[0m %s\n' "$*"; }
warn() { printf '\033[0;33m[cảnh báo]\033[0m %s\n' "$*"; }
die()  { printf '\033[0;31m[lỗi]\033[0m %s\n' "$*" >&2; exit 1; }
trap 'die "thất bại tại dòng $LINENO"' ERR

need() { command -v "$1" >/dev/null 2>&1 || die "thiếu công cụ: $1"; }

# ─────────────────────────────────────────── chuẩn bị

need go
need node
need npm

if [ "$OBFUSCATE_GO" = "1" ] && ! command -v garble >/dev/null 2>&1; then
  warn "chưa có garble. Cài bằng: go install mvdan.cc/garble@latest"
  warn "tạm thời build thường (không làm rối)."
  OBFUSCATE_GO=0
fi

rm -rf "$OUT"
mkdir -p "$OUT/bin" "$OUT/panel" "$OUT/deploy"
log "phiên bản: $VERSION"
log "thư mục đầu ra: $OUT"

# ─────────────────────────────────────────── 1. Go: API + agent

# -trimpath   : bỏ đường dẫn máy build khỏi binary
# -ldflags -s : bỏ bảng ký hiệu   | -w : bỏ thông tin DWARF debug
# garble -literals : mã hoá hằng chuỗi trong binary
# garble -tiny     : bỏ thêm tên hàm/số dòng (stack trace sẽ khó đọc — chấp nhận đánh đổi)
GOFLAGS_COMMON=(-trimpath -ldflags "-s -w -X main.version=${VERSION}")

build_go() {
  local name="$1" dir="$2" pkg="$3"
  log "biên dịch ${name} (garble=${OBFUSCATE_GO})"
  if [ "$OBFUSCATE_GO" = "1" ]; then
    ( cd "$dir" && garble -literals -tiny build "${GOFLAGS_COMMON[@]}" -o "$OUT/bin/$name" "$pkg" )
  else
    ( cd "$dir" && go build "${GOFLAGS_COMMON[@]}" -o "$OUT/bin/$name" "$pkg" )
  fi
  # Kiểm chứng đã strip: không được còn bảng ký hiệu.
  if command -v file >/dev/null 2>&1; then
    file "$OUT/bin/$name" | grep -q "not stripped" && warn "$name vẫn còn ký hiệu debug"
  fi
}

API_DIR="${ROOT}/core"; [ -d "$API_DIR" ] || API_DIR="${ROOT}/backend"
build_go vkai-api "$API_DIR" ./cmd/api 2>/dev/null || build_go vkai-api "$API_DIR" ./cmd/server 2>/dev/null || {
  # Fallback: tìm package main đầu tiên trong cmd/
  MAIN_PKG=$(cd "$API_DIR" && for d in cmd/*/; do grep -qs '^package main' "$d"*.go && echo "./$d" && break; done)
  [ -n "${MAIN_PKG:-}" ] || die "không tìm thấy package main trong ${API_DIR}/cmd"
  build_go vkai-api "$API_DIR" "$MAIN_PKG"
}
build_go vkai-agent "${ROOT}/agent" ./cmd

# ─────────────────────────────────────────── 2. Panel UI (Next.js)

PANEL_DIR="${ROOT}/panel"; [ -d "$PANEL_DIR" ] || PANEL_DIR="${ROOT}/frontend"
log "build UI (obfuscate=${OBFUSCATE_JS})"
(
  cd "$PANEL_DIR"
  [ -d node_modules ] || npm ci
  # next.config.js đọc biến này để bật webpack-obfuscator cho phần logic lõi.
  VKAI_OBFUSCATE_JS="$OBFUSCATE_JS" NEXT_TELEMETRY_DISABLED=1 npm run build
)

[ -f "$PANEL_DIR/.next/standalone/server.js" ] || die "không thấy .next/standalone/server.js — kiểm tra output:'standalone' và bước postbuild"
[ -d "$PANEL_DIR/.next/standalone/.next/static" ] || die "thiếu .next/standalone/.next/static — UI sẽ lỗi 'Application error' khi chạy"

cp -r "$PANEL_DIR/.next/standalone/." "$OUT/panel/"
log "đã đóng gói UI dạng standalone"

# ─────────────────────────────────────────── 3. (tuỳ chọn) V8 bytecode cho server Node

if [ "$BYTECODE_SERVER" = "1" ]; then
  warn "biên dịch bytecode cho Next.js standalone là THỬ NGHIỆM."
  warn "Next.js nạp động nhiều chunk trong .next/server; chuyển toàn bộ sang .jsc dễ vỡ khi nâng cấp Next."
  warn "Khuyến nghị: giữ logic bản quyền/nghiệp vụ nhạy cảm ở phía Go, nơi garble bảo vệ thật sự."
  if node -e "require('bytenode')" >/dev/null 2>&1; then
    node -e "
      const bytenode = require('bytenode');
      const out = process.env.OUT_DIR + '/panel/server.jsc';
      bytenode.compileFile({ filename: process.env.OUT_DIR + '/panel/server.js', output: out });
      console.log('[build] đã sinh', out);
    " OUT_DIR="$OUT"
    cat > "$OUT/panel/server.js" <<'JS'
// Điểm vào: nạp bytecode đã biên dịch (mã nguồn JS gốc không đi kèm bản phát hành).
require('bytenode');
require('./server.jsc');
JS
  else
    warn "chưa cài bytenode — bỏ qua bước này (npm i -g bytenode)"
  fi
fi

# ─────────────────────────────────────────── 4. Tài nguyên triển khai + manifest

for d in deploy docker; do
  [ -d "${ROOT}/$d" ] && cp -r "${ROOT}/$d/." "$OUT/deploy/" 2>/dev/null || true
done

( cd "$OUT" && find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS )

cat > "$OUT/MANIFEST.txt" <<EOF
VKAI Panel — bản phát hành
Phiên bản      : ${VERSION}
Dựng lúc       : $(date -u '+%Y-%m-%d %H:%M:%S UTC')
Làm rối Go     : ${OBFUSCATE_GO} (garble -literals -tiny, -trimpath, -s -w)
Làm rối JS     : ${OBFUSCATE_JS}
Bytecode server: ${BYTECODE_SERVER}

Bản phát hành này KHÔNG chứa mã nguồn. Kiểm tra toàn vẹn: sha256sum -c SHA256SUMS
EOF

log "hoàn tất: $OUT"
ls -la "$OUT" "$OUT/bin"
