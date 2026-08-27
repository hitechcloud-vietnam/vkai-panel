# VKAI Panel — Cài đặt & Triển khai

HiTech Cloud (hitechcloud.vn)

Panel **không bao giờ** dùng cổng 80/443. Hai cổng đó dành riêng cho website của
khách. Panel nghe trên **cổng riêng** (mặc định `8888`) kèm một **lối vào an
toàn** dạng `/vkai_a1b2c3d4` — vào sai đường dẫn sẽ nhận 404 trung tính.

---

## 1. Cài đặt nhanh

```bash
# Trên máy chủ trắng, chạy bằng root
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git /usr/src/vkai-panel
cd /usr/src/vkai-panel
sudo bash deploy/install.sh
```

Bộ cài in ra bảng tổng kết (URL đầy đủ kèm lối vào, tài khoản quản trị, đường dẫn
dữ liệu) và lưu một bản sao vào `/vkai-panel/etc/install-summary.txt` (quyền 600).
Toàn bộ quá trình được ghi vào `/vkai-panel/logs/install.log`.

> `scripts/install.sh` chỉ là lối tắt chuyển tiếp sang `deploy/install.sh`.
> Chỉ có **một** bộ cài duy nhất.

---

## 2. Ma trận hệ điều hành hỗ trợ

| Hệ điều hành | Phiên bản | Họ | Trình quản lý gói |
|---|---|---|---|
| Ubuntu | 20.04, 22.04, 24.04 | debian | `apt-get` |
| Debian | 11, 12 | debian | `apt-get` |
| CentOS Stream | 8, 9 | rhel | `dnf` |
| RHEL | 8, 9 | rhel | `dnf` |
| Rocky Linux | 8, 9 | rhel | `dnf` |
| AlmaLinux | 8, 9 | rhel | `dnf` |
| Fedora | 38 trở lên | rhel | `dnf` |
| openSUSE Leap | 15.x | suse | `zypper` |
| Amazon Linux | 2023 | rhel | `dnf` |

**Kiến trúc:** `x86_64` (amd64) và `aarch64` (arm64).

Không nhận diện được hệ điều hành thì bộ cài **dừng lại có thông báo rõ ràng**,
không đoán bừa. Phiên bản nằm ngoài bảng trên thì cảnh báo và yêu cầu `--force-os`.

Đã xử lý sẵn cho từng họ OS:

- **apt** — `-o DPkg::Lock::Timeout=600` để không kẹt khi `unattended-upgrades` đang giữ lock.
- **RHEL/Rocky/Alma/CentOS** — bật kho **EPEL** và **CRB/PowerTools**.
- **Amazon Linux 2023** — dùng `postgresql15*` và `redis6` thay cho tên gói chuẩn.
- **openSUSE** — `git-core`, `gpg2` thay cho `git`, `gnupg`.
- **SELinux** (RHEL/Rocky/Alma/Fedora) — `semanage port` cho cổng panel,
  ngữ cảnh `httpd_sys_rw_content_t` cho `www/`, bật `httpd_can_network_connect`.
- **Tường lửa** — tự mở cổng panel qua `ufw` hoặc `firewalld`; nếu không có công
  cụ nào thì in cảnh báo rõ ràng thay vì im lặng.

---

## 3. Cờ dòng lệnh

| Cờ | Ý nghĩa |
|---|---|
| `--port <số>` | Cổng truy cập panel (mặc định 8888). Từ chối 80/443. |
| `--random-port` | Sinh cổng ngẫu nhiên 10000–60000. |
| `--entrance <đường>` | Lối vào an toàn, ví dụ `/vkai_a1b2c3d4`. Bỏ trống = ngẫu nhiên. |
| `--admin-user <tên>` | Tên tài khoản quản trị đầu tiên (mặc định `admin`). |
| `--api-url <url>` | Gán cứng `NEXT_PUBLIC_API_URL`. Bỏ trống = same-origin qua nginx. |
| `--no-firewall` | Không động tới ufw/firewalld. |
| `--skip-deps` | Bỏ qua bước cài gói hệ thống. |
| `--skip-checksum` | Bỏ qua kiểm tra checksum khi tải Go/Node (chỉ khi buộc phải). |
| `--force-os` | Vẫn cài trên phiên bản OS ngoài ma trận. |
| `-y`, `--yes` | Không hỏi lại. |
| `--uninstall` | Gỡ cài đặt (hỏi trước khi xoá dữ liệu). |
| `-h`, `--help` | Trợ giúp. |
| `-v`, `--version` | Phiên bản. |

Ví dụ:

```bash
sudo bash deploy/install.sh --port 9001 --entrance /quantri_x9f2 --yes
sudo bash deploy/install.sh --random-port --admin-user sysop
sudo bash deploy/install.sh --uninstall
```

---

## 4. Các bước bộ cài thực hiện (đúng thứ tự)

1. Đọc tham số, kiểm tra **quyền root**, nhận diện **kiến trúc** và **hệ điều hành**
   (`/etc/os-release` → `ID`/`VERSION_ID`/`ID_LIKE`, dự phòng `/etc/redhat-release`).
2. Chọn trình quản lý gói: `apt-get` | `dnf` | `yum` | `zypper`.
3. Kiểm tra trước: systemd, **RAM/đĩa tối thiểu**, **panel khác** (cPanel/aaPanel/
   Plesk/DirectAdmin/Vesta/Hestia).
4. Bật ghi log ra `/vkai-panel/logs/install.log`.
5. Cài phụ thuộc hệ thống theo tên gói đúng của từng họ OS (kèm EPEL/CRB).
6. Cài **Go** và **Node.js** theo kiến trúc, **kiểm tra SHA256** từ nguồn chính thức;
   bỏ qua nếu máy đã có bản đủ mới.
7. Tạo người dùng/nhóm hệ thống `vkai` và cây thư mục `/vkai-panel/**`.
8. Chốt **cổng panel + lối vào**, kiểm tra **cổng 80/443/panel** có bị chiếm.
9. Đồng bộ mã nguồn vào `/vkai-panel/{core,panel,agent}`.
10. **Sinh cấu hình `/vkai-panel/etc/.env`** — mật khẩu CSDL, JWT (≥64 ký tự),
    secret key, agent token, cổng, lối vào — rồi **symlink sang `panel/.env`**.
    > ⚠️ Bước này **bắt buộc phải trước** bước build UI: `NEXT_PUBLIC_API_URL` là
    > biến được Next.js nhúng thẳng vào bundle lúc build, và Next.js chỉ đọc
    > `.env` tại **gốc dự án**.
11. Khởi tạo PostgreSQL (initdb khi cần, `pg_hba` cho loopback), tạo role + CSDL,
    cài `uuid-ossp`, chạy migration theo thứ tự (ghi nhớ trong `etc/migrations.applied`).
12. Bật Redis.
13. Build `core/` (vkai-api, vkai-panelctl, vkai-cli) và `agent/`.
14. Build `panel/` — sau đó **bắt buộc** đảm bảo `.next/static` và `public/` đã nằm
    trong `.next/standalone`, rồi kiểm tra `.next/standalone/server.js` tồn tại.
    > ⚠️ Thiếu bước copy này, panel trả HTML nhưng mọi `/_next/static/*.js` đều 404
    > và trình duyệt báo *"Application error: a client-side exception has occurred"*.
15. Tạo **tài khoản quản trị đầu tiên** với mật khẩu ngẫu nhiên (bcrypt cost 12).
16. Cài systemd unit, cài lệnh `vkai`, cấu hình nginx, logrotate, SELinux, tường lửa.
17. Khởi động dịch vụ, kiểm tra `/health`, in **bảng tổng kết** và ghi
    `/vkai-panel/etc/install-summary.txt`.

Chạy lại bộ cài là **idempotent**: cổng, lối vào, mật khẩu CSDL, JWT và mật khẩu
quản trị đang dùng đều được giữ nguyên.

---

## 5. Cây thư mục sau khi cài

| Đường dẫn | Nội dung |
|---|---|
| `/vkai-panel/` | Gốc cài đặt (750) |
| `/vkai-panel/core/` | Mã + binary API (`bin/vkai-api`) |
| `/vkai-panel/panel/` | Bản build UI (`.next/standalone/server.js`) |
| `/vkai-panel/agent/` | Node agent (`bin/vkai-agent`) |
| `/vkai-panel/www/domains/<domain>` | **Mã nguồn website của khách** |
| `/vkai-panel/www/backup/` | Sao lưu website / CSDL |
| `/vkai-panel/www/default/` | Trang mặc định |
| `/vkai-panel/logs/` | Nhật ký panel (`install.log`) |
| `/vkai-panel/logs/sites/<domain>/` | Nhật ký web server theo site |
| `/vkai-panel/etc/` | Cấu hình (`.env`, `install-summary.txt`) — quyền 700 |
| `/vkai-panel/ssl/` | Chứng chỉ |
| `/vkai-panel/tmp/` | Tạm |

`/etc/vkai` được liên kết về `/vkai-panel/etc` để chỉ còn một nguồn sự thật.

---

## 6. Dịch vụ

| Dịch vụ | Vai trò | Cổng | Unit |
|---|---|---|---|
| API (Go) | `core/bin/vkai-api` | 30110 (loopback) | `vkai-api` |
| Giao diện (Next.js) | `panel/.next/standalone/server.js` | 3000 (loopback) | `vkai-ui` |
| Agent | `agent/bin/vkai-agent` | 30111 | `vkai-agent` (tuỳ chọn) |
| nginx — panel | Reverse proxy panel | **8888** (`VKAI_PANEL_PORT`) | `nginx` |
| nginx — website khách | Vhost của khách | **80/443** | `nginx` |
| PostgreSQL | CSDL | 5432 | `postgresql` |
| Redis | Cache / hàng đợi | 6379 | `redis-server` \| `redis` \| `redis6` |

3000 / 30110 / 30111 / 5432 / 6379 **chỉ nghe loopback**, không mở ra Internet.

Unit systemd có sẵn hardening: `NoNewPrivileges`, `ProtectSystem=strict`,
`ProtectHome`, `PrivateTmp`, và `ReadWritePaths` chỉ mở đúng thư mục cần ghi.

---

## 7. Lệnh quản trị `vkai`

```bash
vkai status                 # trạng thái dịch vụ + tài nguyên máy
vkai start | stop | restart
vkai logs api|ui|agent|nginx|install

vkai info                   # URL đầy đủ, cổng, lối vào, đường dẫn dữ liệu
vkai port 9001              # đổi cổng panel (cập nhật nginx + tường lửa + SELinux)
vkai port random
vkai entrance random        # sinh lối vào an toàn mới

vkai backup                 # sao lưu CSDL + cấu hình vào www/backup
vkai update [đường-dẫn-mã-nguồn]   # build lại core/ và panel/, khởi động lại
vkai uninstall

vkai site create example.com   # nghiệp vụ — uỷ quyền cho vkai-cli
vkai db | ssl | firewall | server ...
```

Quên lối vào an toàn? `vkai info` hoặc `vkai-panelctl panel info`.

---

## 8. Triển khai bản phát hành đóng gói

`deploy/scripts/deploy.sh` dành cho quy trình CI đẩy gói `.tar.gz` xuống máy chủ:

```bash
sudo bash deploy/scripts/deploy.sh deploy /tmp/vkai-panel-1.2.0.tar.gz
sudo bash deploy/scripts/deploy.sh status
sudo bash deploy/scripts/deploy.sh rollback
```

Gói phải chứa `core/bin/vkai-api`, `core/migrations/*.sql`,
`panel/.next/standalone/server.js` **kèm** `panel/.next/standalone/.next/static`.
Script kiểm tra đủ các thành phần này **trước khi** dừng dịch vụ, sao lưu CSDL,
chạy migration còn thiếu, khởi động lại rồi kiểm tra `/health`; giữ 5 bản gần nhất
để `rollback`.

> Rollback chỉ quay lui **mã**, không quay lui **migration CSDL**.

---

## 9. Nginx

`deploy/nginx/vkai-panel.conf` được giữ ở dạng dùng được cho Docker (upstream là
tên dịch vụ compose `vkai-core` / `vkai-ui`). Bộ cài bằng systemd tự đổi hai
dòng đó sang `127.0.0.1` và đặt đúng cổng panel khi copy sang
`/etc/nginx/conf.d/vkai-panel.conf`.

File này **không chứa** `listen 80` hay `listen 443` — và không được phép chứa.

Bật HTTPS riêng cho panel: đổi `listen <cổng>;` thành `listen <cổng> ssl;` rồi bỏ
comment bốn dòng `ssl_*` (chứng chỉ đặt trong `/vkai-panel/ssl/`).

---

## 10. Xử lý sự cố

**Dịch vụ không lên**

```bash
systemctl status vkai-api vkai-ui
journalctl -u vkai-api -n 100 --no-pager
cat /vkai-panel/etc/.env          # quyền 600, chỉ root/vkai đọc được
```

**Panel báo "Application error: a client-side exception has occurred"**

Thiếu tài nguyên tĩnh trong bản standalone:

```bash
ls /vkai-panel/panel/.next/standalone/.next/static   # phải tồn tại
sudo vkai update                                     # build lại đúng cách
```

**Không vào được panel từ ngoài**

```bash
vkai info                        # xem đúng cổng và lối vào
ss -ltnp | grep <cổng-panel>     # nginx có nghe không
ufw status | grep <cổng-panel>   # hoặc: firewall-cmd --list-ports
```

Ngoài ra kiểm tra security group / firewall của nhà cung cấp máy chủ.

**Lỗi kết nối CSDL**

```bash
systemctl status postgresql
PGPASSWORD=$(grep ^VKAI_DB_PASSWORD= /vkai-panel/etc/.env | cut -d= -f2-) \
  psql -h 127.0.0.1 -U vkai -d vkai_panel -c 'select 1'
```

---

## 11. Khuyến nghị bảo mật

1. Đổi mật khẩu quản trị ngay sau lần đăng nhập đầu tiên.
2. Giới hạn cổng panel theo IP quản trị — `VKAI_PANEL_ALLOWED_IPS` trong `.env`
   và/hoặc khối `allow`/`deny` trong `vkai-panel.conf`.
3. Giữ lối vào an toàn; đổi định kỳ bằng `vkai entrance random`.
4. **Không** đặt panel lên 80/443.
5. Bật TLS cho panel bằng chứng chỉ riêng, tách khỏi chứng chỉ website khách.
6. Sao lưu định kỳ: `0 2 * * * /usr/local/bin/vkai backup nightly`.
7. Cập nhật hệ thống và panel thường xuyên; theo dõi `vkai logs api`.
