/**
 * next.config.js dùng output: 'standalone'. Ở chế độ này Next.js CỐ Ý không đưa
 * .next/static và public/ vào .next/standalone — người triển khai phải tự copy.
 *
 * Nếu bỏ qua bước này, server standalone vẫn trả HTML nhưng mọi
 * /_next/static/chunks/*.js đều 404 -> ChunkLoadError -> trình duyệt hiện
 * "Application error: a client-side exception has occurred".
 *
 * Chạy tự động sau `next build` (xem script "build" trong package.json).
 */
const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..');
const standalone = path.join(root, '.next', 'standalone');

if (!fs.existsSync(standalone)) {
  console.log('[standalone] Bỏ qua: .next/standalone không tồn tại (output không phải standalone).');
  process.exit(0);
}

function copyDir(from, to, label) {
  if (!fs.existsSync(from)) {
    console.log(`[standalone] Bỏ qua ${label}: ${from} không tồn tại.`);
    return;
  }
  fs.rmSync(to, { recursive: true, force: true });
  fs.cpSync(from, to, { recursive: true });
  console.log(`[standalone] Đã copy ${label} -> ${path.relative(root, to)}`);
}

copyDir(path.join(root, '.next', 'static'), path.join(standalone, '.next', 'static'), '.next/static');
copyDir(path.join(root, 'public'), path.join(standalone, 'public'), 'public');

const server = path.join(standalone, 'server.js');
if (!fs.existsSync(server)) {
  console.error('[standalone] LỖI: không tìm thấy .next/standalone/server.js sau khi build.');
  process.exit(1);
}
console.log('[standalone] Sẵn sàng: node .next/standalone/server.js');
