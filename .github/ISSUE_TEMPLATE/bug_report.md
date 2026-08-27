---
name: Báo lỗi
about: Báo một lỗi của VKAI Panel
title: '[BUG] '
labels: bug
assignees: ''
---

<!--
  KHÔNG dùng mẫu này để báo lỗ hổng bảo mật.
  Lỗ hổng bảo mật gửi riêng theo hướng dẫn trong SECURITY.md.
  KHÔNG dán mật khẩu, token, khoá riêng hay lối vào an toàn (entrance) vào đây.
-->

## Mô tả lỗi

Mô tả ngắn gọn, rõ ràng lỗi đang gặp.

## Các bước tái hiện

1. Vào màn hình '...'
2. Bấm '...'
3. Quan sát '...'

## Kết quả mong đợi

Điều lẽ ra phải xảy ra.

## Kết quả thực tế

Điều thực sự xảy ra.

## Môi trường

- Phiên bản VKAI Panel: (ví dụ 1.0.0)
- Hệ điều hành máy chủ: (ví dụ Ubuntu 22.04, Rocky Linux 9)
- Kiến trúc CPU: (`x86_64` hoặc `aarch64`)
- Cách cài đặt: (một dòng lệnh / `deploy/install.sh` / Docker / build từ mã nguồn)
- Thư mục cài: (mặc định `/vkai-panel`, hay đường dẫn khác)
- Thành phần liên quan: (`vkai-api` / `vkai-ui` / `vkai-agent` / CLI `vkai`)
- Trình duyệt: (ví dụ Chrome 120) — nếu lỗi ở giao diện
- Web server: (nginx / apache / openlitespeed / caddy / traefik)
- Go / Node.js: (nếu build từ mã nguồn)

## Nhật ký

Che hết thông tin nhạy cảm (IP thật, tên miền, token, đường dẫn lối vào an toàn)
trước khi dán.

```
sudo journalctl -u vkai-api -n 200 --no-pager
sudo journalctl -u vkai-ui -n 200 --no-pager
sudo tail -n 200 /vkai-panel/logs/*.log
```

```
Dán nhật ký liên quan vào đây
```

## Ảnh chụp màn hình

Nếu là lỗi giao diện, đính kèm ảnh chụp.

## Ngữ cảnh thêm

Bất cứ thông tin nào khác giúp tái hiện hoặc khoanh vùng lỗi.

## Hướng khắc phục đề xuất

Nếu bạn đã biết nguyên nhân hoặc cách sửa, mô tả tại đây.

## Issue liên quan

Liên kết tới các issue liên quan nếu có.
