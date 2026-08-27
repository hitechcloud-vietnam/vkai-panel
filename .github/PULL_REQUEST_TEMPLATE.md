<!--
  VKAI Panel - HiTechCloud
  Cấm push thẳng vào `main`. Mọi thay đổi phải đi qua nhánh phụ + Pull Request.
  Workflow "Guard main" sẽ báo lỗi nếu phát hiện commit vào main không qua PR.
-->

## Mô tả thay đổi

Tóm tắt thay đổi này làm gì và vì sao cần làm.

Đóng issue: # (số issue)

## Loại thay đổi

- [ ] Sửa lỗi (không phá vỡ tương thích)
- [ ] Tính năng mới (không phá vỡ tương thích)
- [ ] Thay đổi phá vỡ tương thích (breaking change)
- [ ] Chỉ tài liệu
- [ ] Refactor / dọn dẹp mã, không đổi hành vi
- [ ] Hạ tầng, CI/CD, script triển khai

## Phạm vi ảnh hưởng

- [ ] `core/` (API Go, dịch vụ `vkai-api`)
- [ ] `panel/` (giao diện Next.js, dịch vụ `vkai-ui`)
- [ ] `agent/` (dịch vụ `vkai-agent`)
- [ ] `deploy/` (trình cài đặt, unit systemd, cấu hình nginx)
- [ ] `docs/` hoặc tài liệu ở thư mục gốc

## Checklist bắt buộc

- [ ] **CI xanh** — cả ba job `Core API`, `Panel UI`, `Agent` đều pass.
- [ ] **Không push thẳng `main`** — thay đổi này nằm trên nhánh phụ
      (`feat/...`, `fix/...`, `docs/...`, `refactor/...`, `chore/...`) và được merge qua PR.
- [ ] **Đã test** — mô tả rõ ở mục dưới, không chỉ ghi "đã chạy thử".
- [ ] `make lint` và `make test` chạy sạch trên máy cục bộ.
- [ ] Đã tự review lại diff của chính mình.
- [ ] Đã cập nhật tài liệu tương ứng trong `docs/` hoặc `README.md`.
- [ ] Không lộ bí mật: không có mật khẩu, token, khoá riêng, chuỗi kết nối thật trong diff.
- [ ] Tên hiển thị, lệnh, dịch vụ, biến môi trường đúng chuẩn thương hiệu:
      **VKAI Panel**, `vkai`, `vkai-api` / `vkai-ui` / `vkai-agent`, tiền tố `VKAI_`,
      đường dẫn `/vkai-panel/...`.

## Đã test như thế nào

Mô tả cách kiểm chứng: lệnh đã chạy, môi trường, kết quả quan sát được.

- [ ] Unit test
- [ ] Integration test
- [ ] Test thủ công trên giao diện
- [ ] Test trên máy chủ thật / máy ảo

**Môi trường test**

- Hệ điều hành: (ví dụ Ubuntu 22.04)
- Trình duyệt: (ví dụ Chrome 120) — nếu có đổi UI
- Phiên bản panel: (ví dụ 1.0.0)
- Go / Node.js: (ví dụ Go 1.22, Node 20)

## Ảnh chụp giao diện (bắt buộc nếu đổi UI)

Nếu PR này thay đổi giao diện, **phải** đính kèm ảnh chụp màn hình trước và sau.

| Trước | Sau |
|---|---|
| (ảnh) | (ảnh) |

- [ ] PR này không đổi giao diện, nên không cần ảnh chụp.

## Ảnh hưởng bảo mật

- [ ] Không ảnh hưởng bảo mật.
- [ ] Có ảnh hưởng — mô tả bên dưới.

Nếu có, trả lời rõ:

- Thay đổi có chạm vào xác thực, phiên đăng nhập, JWT, RBAC hay không?
- Có thêm/đổi endpoint công khai (không cần đăng nhập) nào không?
- Có chạm vào cổng panel, lối vào an toàn, danh sách IP cho phép, TLS không?
- Có thực thi lệnh hệ thống, đọc/ghi tệp ngoài thư mục gốc đã giới hạn,
  hoặc nâng quyền không?
- Có thêm phụ thuộc mới không? Nếu có, nguồn gốc và giấy phép là gì?

## Ảnh hưởng migration

- [ ] Không có migration cơ sở dữ liệu.
- [ ] Có migration — mô tả bên dưới.

Nếu có:

- Tệp migration: `core/migrations/...`
- Thay đổi lược đồ: (thêm/sửa/xoá bảng, cột, index)
- Có phá vỡ tương thích với bản cũ đang chạy không?
- Có script/quy trình rollback không? Mô tả rõ.
- Ước lượng thời gian chạy và mức khoá bảng trên dữ liệu lớn.
- [ ] Đã test migration trên bản sao dữ liệu thật hoặc dữ liệu mẫu tương đương.

## Thay đổi phá vỡ tương thích

Nếu có, liệt kê những gì hỏng và hướng dẫn nâng cấp cho người đang dùng bản cũ.

## Ảnh hưởng hiệu năng

Nếu có, mô tả và kèm số đo nếu có thể.

## Ghi chú thêm

Bất cứ điều gì người review cần biết trước khi đọc diff.
