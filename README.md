# VKAI Panel

**Bảng điều khiển máy chủ & hosting đa máy chủ** — sản phẩm của **HiTechCloud** ([hitechcloud.vn](https://hitechcloud.vn)).

VKAI Panel quản lý máy chủ, website, cơ sở dữ liệu, DNS, chứng chỉ SSL, container,
tường lửa, sao lưu và giám sát từ một giao diện web duy nhất. Panel chạy trên
**cổng riêng (mặc định 8888)** phía sau một **lối vào an toàn**; cổng **80/443 dành
riêng cho website của khách hàng**.

---

## Mục lục

- [Điểm khác biệt](#điểm-khác-biệt)
- [Tính năng](#tính-năng)
- [Ma trận hệ điều hành hỗ trợ](#ma-trận-hệ-điều-hành-hỗ-trợ)
- [Cài đặt một dòng lệnh](#cài-đặt-một-dòng-lệnh)
- [Truy cập panel lần đầu](#truy-cập-panel-lần-đầu)
- [Giao diện](#giao-diện)
- [Kiến trúc](#kiến-trúc)
- [Cấu trúc mã nguồn](#cấu-trúc-mã-nguồn)
- [Đường dẫn chuẩn trên máy chủ](#đường-dẫn-chuẩn-trên-máy-chủ)
- [Dịch vụ systemd](#dịch-vụ-systemd)
- [Lệnh quản trị `vkai`](#lệnh-quản-trị-vkai)
- [Cấu hình](#cấu-hình)
- [Môi trường phát triển](#môi-trường-phát-triển)
- [Quy trình đóng góp](#quy-trình-đóng-góp)
- [Tài liệu](#tài-liệu)
- [Giấy phép & hỗ trợ](#giấy-phép--hỗ-trợ)

---

## Điểm khác biệt

| | VKAI Panel |
|---|---|
| Cổng quản trị | Cổng riêng, mặc định `8888` — **không bao giờ** chiếm 80/443 |
| Lối vào | Đường dẫn bí mật dạng `/vkai_a1b2c3d4`, sai đường dẫn trả về 404 trung tính |
| Chặn theo IP / tên miền | Có, kiểm tra trước cả lối vào |
| Website khách | Toàn quyền dùng 80/443, tách hoàn toàn khỏi panel |
| Triển khai | systemd thuần (`vkai-api`, `vkai-ui`, `vkai-agent`), không bắt buộc Docker |
| Đa máy chủ | Một panel điều khiển nhiều node qua `vkai-agent` |

## Tính năng

### Nhóm chính

- **Quản lý đa máy chủ** — điều khiển nhiều node từ một panel.
- **Website** — PHP, Node.js, Python, reverse proxy, site tĩnh, WordPress.
- **Cơ sở dữ liệu** — MySQL, MariaDB, PostgreSQL, Redis, MongoDB.
- **SSL/TLS** — Let's Encrypt, chứng chỉ tự cấp, tự động gia hạn.
- **DNS** — tích hợp BIND, PowerDNS.
- **Docker** — container, image, volume, compose.
- **Trình quản lý tệp** — trình soạn thảo web có tô màu cú pháp, giới hạn trong thư mục gốc cấu hình được.
- **Cron** — quản lý tác vụ định kỳ bằng giao diện.
- **Tường lửa** — UFW, firewalld, CSF.
- **Sao lưu** — sao lưu tự động ra S3, FTP, SFTP, Dropbox.
- **Triển khai** — deploy từ Git kèm webhook.
- **Giám sát** — số liệu máy chủ thời gian thực và cảnh báo.
- **Bảo mật** — quản lý khoá SSH, 2FA, danh sách IP cho phép, WAF, chống sửa đổi tệp.
- **Đa người thuê (multi-tenant)** — cô lập tenant kèm RBAC 8 vai trò.

### Web server hỗ trợ

Nginx, Apache, OpenLiteSpeed, LiteSpeed Enterprise, Caddy, Traefik.

> Adapter Nginx đã đầy đủ; các adapter còn lại đã có khung và đang hoàn thiện —
> xem [docs/ENTERPRISE_ROADMAP.md](docs/ENTERPRISE_ROADMAP.md).

## Ma trận hệ điều hành hỗ trợ

Trình cài đặt nhận diện hệ điều hành qua `/etc/os-release` và chỉ chạy trên các
họ dưới đây.

| Hệ điều hành | Phiên bản khuyến nghị | Trạng thái | Ghi chú |
|---|---|---|---|
| Ubuntu Server | 22.04 LTS, 24.04 LTS | Hỗ trợ đầy đủ | Nền tảng kiểm thử chính |
| Ubuntu Server | 20.04 LTS | Hỗ trợ | Cần kho Node.js 20 từ NodeSource |
| Debian | 12 (Bookworm), 11 (Bullseye) | Hỗ trợ đầy đủ | |
| Rocky Linux | 9, 8 | Hỗ trợ | Dùng nhánh `dnf/yum` của trình cài |
| AlmaLinux | 9, 8 | Hỗ trợ | Dùng nhánh `dnf/yum` của trình cài |
| RHEL | 9, 8 | Hỗ trợ | Cần đăng ký kho phần mềm hợp lệ |
| CentOS Stream | 9 | Hỗ trợ | CentOS 7 đã hết vòng đời, không hỗ trợ |
| Các bản Linux khác | — | Không hỗ trợ | Trình cài dừng với thông báo rõ ràng |

| Kiến trúc CPU | Trạng thái |
|---|---|
| `x86_64` / `amd64` | Hỗ trợ đầy đủ |
| `aarch64` / `arm64` | Hỗ trợ |
| Kiến trúc khác | Không hỗ trợ |

Yêu cầu tối thiểu: 2 vCPU, 4 GB RAM, 50 GB đĩa SSD, quyền `root`, và một máy chủ
mới chưa cài panel khác (aaPanel, cPanel, Plesk...). Khuyến nghị cho môi trường
sản xuất: 4 vCPU, 8 GB RAM, 100 GB SSD.

## Cài đặt một dòng lệnh

```bash
curl -sSL https://install.vkai.vn | sudo bash
```

Trình cài sẽ: nhận diện hệ điều hành và kiến trúc, cài PostgreSQL / Redis / Nginx,
sinh mật khẩu và bí mật ngẫu nhiên, cài binary panel, tạo dịch vụ systemd, rồi in
ra thông tin truy cập.

Cài từ mã nguồn (khi cần build tại chỗ):

```bash
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel
sudo bash deploy/install.sh
```

> Đọc kỹ [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) trước khi cài lên máy chủ đang chạy dịch vụ thật.

## Truy cập panel lần đầu

Kết thúc cài đặt, panel in một lần duy nhất thông tin truy cập:

```
==============================================================================
  VKAI PANEL - THONG TIN TRUY CAP (khong dung cong 80/443)
==============================================================================
  URL truy cap       : http://203.0.113.10:8888/vkai_91ac5b65/
  Cong panel         : 8888
  Loi vao an toan    : /vkai_91ac5b65
  File cau hinh      : /vkai-panel/etc/panel_access.json
==============================================================================
```

Ba việc phải làm ngay:

1. **Mở tường lửa cho cổng panel trước khi đóng console**

   ```bash
   sudo ufw allow 8888/tcp                                                    # Ubuntu / Debian
   sudo firewall-cmd --permanent --add-port=8888/tcp && sudo firewall-cmd --reload   # RHEL / Rocky / Alma
   ```

2. **Lưu lại URL kèm lối vào an toàn.** Truy cập sai đường dẫn chỉ nhận 404 trung tính.
3. **Đổi mật khẩu quản trị mặc định** ngay trong lần đăng nhập đầu tiên.

Xem lại thông tin bất cứ lúc nào:

```bash
vkai panel info
```

Chi tiết đầy đủ (đổi cổng, đổi lối vào, giới hạn IP, TLS, reverse proxy):
[docs/PANEL_ACCESS.md](docs/PANEL_ACCESS.md).

## Giao diện

Giao diện là ứng dụng Next.js 14 (App Router) chạy trên nền sáng, hai màu thương
hiệu **navy `#0B398C`** và **cyan `#1791C8`**, font Inter cho văn bản và JetBrains
Mono cho mã/log. Bố cục gồm sidebar điều hướng bên trái, thanh trên hiển thị máy
chủ đang chọn và thông báo, phần thân là nội dung từng màn hình.

Ảnh chụp màn hình đặt trong `docs/images/` và được nhúng vào tài liệu tương ứng.

| Màn hình | Nội dung |
|---|---|
| Dashboard | CPU, RAM, đĩa, băng thông theo thời gian thực; cảnh báo đang mở; tác vụ gần đây |
| Máy chủ | Danh sách node, trạng thái agent, thêm/gỡ máy chủ |
| Website | Danh sách site, loại runtime, tình trạng SSL, thao tác nhanh; quản lý WordPress |
| Cơ sở dữ liệu | Instance, database, người dùng, sao lưu, console truy vấn |
| SSL | Chứng chỉ, ngày hết hạn, phát hành và gia hạn Let's Encrypt |
| DNS | Vùng và bản ghi DNS |
| Docker | Container, image, volume, network, compose |
| Trình quản lý tệp | Duyệt, sửa, tải lên/xuống trong thư mục gốc đã giới hạn |
| Cron / Tác vụ định kỳ | Lịch chạy, lịch sử, nhật ký từng lần chạy |
| Bảo mật | Tường lửa, WAF, quét bảo mật, bảo vệ tệp, chống sửa đổi |
| Giám sát & Nhật ký | Biểu đồ số liệu, cảnh báo, nhật ký panel và web server |
| Sao lưu | Chính sách, điểm khôi phục, đích lưu trữ từ xa |
| Người dùng & API key | Tài khoản, vai trò RBAC, khoá API, nhật ký kiểm toán |
| Terminal | Phiên shell trên máy chủ đã chọn, ngay trong trình duyệt |

## Kiến trúc

```
                       Internet
                          |
        +-----------------+------------------+
        |                                    |
        v                                    v
  Cong 80 / 443                        Cong 8888 (VKAI_PANEL_PORT)
  Website cua khach                    Panel quan tri
  (nginx/apache/... vhost)             + loi vao an toan /vkai_xxxxxxxx
        |                                    |
        v                                    v
  /vkai-panel/www/domains/<domain>     +-----------------------------+
                                       |  vkai-api (Go, cong 30110)  |
                                       |  vkai-ui  (Next.js, 3000)   |
                                       +--------------+--------------+
                                                      |
                            +-------------------------+-------------------------+
                            |                         |                         |
                            v                         v                         v
                     +-------------+           +-------------+          +----------------+
                     | PostgreSQL  |           |    Redis    |          |   vkai-agent   |
                     |  cong 5432  |           |  cong 6379  |          |   cong 30111   |
                     +-------------+           +-------------+          +----------------+
```

Cổng `30110` (API) và `3000` (UI) chỉ lắng nghe nội bộ; mọi truy cập từ bên ngoài
đi qua cổng panel `8888`.

### Công nghệ

| Thành phần | Công nghệ |
|---|---|
| API (`core/`) | Go 1.22, Gin, JWT, pgx, go-redis, asynq |
| Giao diện (`panel/`) | Next.js 14, React 18, TypeScript, Tailwind CSS |
| Cơ sở dữ liệu | PostgreSQL 16, Redis 7 |
| Agent (`agent/`) | Binary Go (`vkaid`) |
| Web server | Nginx (mặc định), Apache, OpenLiteSpeed, LiteSpeed, Caddy, Traefik |
| Chạy dịch vụ | systemd (Docker tuỳ chọn) |

## Cấu trúc mã nguồn

```
vkai-panel/
├── core/                       # Máy chủ API viết bằng Go (trước đây là backend/)
│   ├── cmd/
│   │   ├── api/                # Điểm vào dịch vụ vkai-api
│   │   ├── cli/                # Lệnh quản trị
│   │   └── panelctl/           # vkai-panelctl: cổng, lối vào, IP, tên miền
│   ├── internal/
│   │   ├── auth/               # Xác thực JWT
│   │   ├── config/             # Cấu hình + cổng/lối vào panel
│   │   ├── database/           # Kết nối cơ sở dữ liệu
│   │   ├── handler/            # HTTP handler
│   │   ├── middleware/         # HTTP middleware
│   │   ├── models/             # Mô hình dữ liệu
│   │   ├── rbac/               # Phân quyền theo vai trò
│   │   ├── repository/         # Lớp truy cập dữ liệu
│   │   ├── service/            # Nghiệp vụ
│   │   ├── utils/              # Tiện ích
│   │   └── webserver/          # Adapter web server
│   ├── migrations/             # Migration SQL
│   └── config.yaml             # Cấu hình mẫu
├── panel/                      # Giao diện Next.js (trước đây là frontend/)
│   ├── src/
│   │   ├── app/                # App Router
│   │   ├── components/         # Component React
│   │   ├── services/           # Lớp gọi API
│   │   ├── store/              # Zustand store
│   │   └── styles/             # CSS
│   └── package.json
├── agent/                      # VKAI Agent chạy trên từng node
│   └── cmd/main.go
├── deploy/                     # install.sh, unit systemd, cấu hình nginx
├── docker/                     # Cấu hình Docker
├── scripts/                    # Script tiện ích
├── docs/                       # Tài liệu
├── Dockerfile
└── docker-compose.yml
```

> Đường dẫn import Go **không đổi**: module vẫn là
> `github.com/hitechcloud-vietnam/vkai-panel` (và `.../agent`). Chỉ tên thư mục
> trên đĩa đổi từ `backend/`→`core/` và `frontend/`→`panel/`.

## Đường dẫn chuẩn trên máy chủ

| Đường dẫn | Nội dung |
|---|---|
| `/vkai-panel/` | Thư mục gốc của panel sau khi cài |
| `/vkai-panel/core/` | Mã nguồn và binary của API (`vkai-api`) |
| `/vkai-panel/panel/` | Bản build giao diện (`vkai-ui`) |
| `/vkai-panel/www/domains/<domain>/` | **Mã nguồn website của khách hàng** |
| `/vkai-panel/www/backup/` | Sao lưu website và cơ sở dữ liệu |
| `/vkai-panel/www/default/` | Trang mặc định cho vhost chưa khớp tên miền |
| `/vkai-panel/logs/` | Nhật ký của panel |
| `/vkai-panel/logs/sites/<domain>/` | Nhật ký web server tách theo từng site |
| `/vkai-panel/etc/` | Cấu hình panel (`.env`, `config.yaml`) |
| `/vkai-panel/ssl/` | Chứng chỉ TLS |
| `/vkai-panel/tmp/` | Tệp tạm |

Cổng và lối vào an toàn đã sinh được lưu trong `/vkai-panel/etc/panel_access.json`
(quyền `0600`). Đổi `VKAI_PANEL_ROOT` sẽ dời toàn bộ cây thư mục trên; đổi riêng
`VKAI_WEB_ROOT`, `VKAI_BACKUP_ROOT`, `VKAI_LOG_ROOT`, `VKAI_ETC_ROOT`,
`VKAI_SSL_ROOT` hoặc `VKAI_TMP_ROOT` chỉ dời nhánh tương ứng — đó là cách gắn
một ổ đĩa riêng cho sao lưu hoặc cho nhật ký.

## Dịch vụ systemd

| Dịch vụ | Vai trò | Cổng |
|---|---|---|
| `vkai-api` | API Go, đồng thời phục vụ cổng panel và lối vào an toàn | 8888 (công khai), 30110 (nội bộ) |
| `vkai-ui` | Giao diện Next.js | 3000 (chỉ nội bộ) |
| `vkai-agent` | Agent chạy trên từng node được quản lý | 30111 |

```bash
sudo systemctl status vkai-api vkai-ui vkai-agent
sudo journalctl -u vkai-api -f
```

Panel chạy dưới người dùng hệ thống **`vkai`**, không chạy bằng `root`.

## Lệnh quản trị `vkai`

```bash
# Dịch vụ
vkai start                  # Khởi động vkai-api, vkai-ui, nginx
vkai stop                   # Dừng vkai-ui, vkai-api
vkai restart                # Khởi động lại toàn bộ
vkai status                 # Trạng thái dịch vụ và tài nguyên máy

# Nhật ký
vkai logs api               # Nhật ký vkai-api
vkai logs ui                # Nhật ký vkai-ui
vkai logs agent             # Nhật ký vkai-agent
vkai logs nginx             # Nhật ký web server
vkai logs install           # Nhật ký lần cài đặt gần nhất

# Cổng truy cập panel và lối vào an toàn
vkai info                   # URL panel, cổng, lối vào, đường dẫn dữ liệu
vkai port                   # Xem cổng hiện tại
vkai port 8888              # Đổi cổng panel (80/443 bị từ chối)
vkai port random            # Cổng ngẫu nhiên trong 8000-65535
vkai entrance random        # Sinh lối vào an toàn mới
vkai panel allow-ip 203.0.113.7,10.0.0.0/8
vkai panel domain panel.example.com

# Vận hành panel
vkai backup                 # Sao lưu CSDL + cấu hình vào /vkai-panel/www/backup
vkai update                 # Build lại core/ và panel/, khởi động lại dịch vụ
vkai uninstall              # Gỡ cài đặt

# Nghiệp vụ (uỷ quyền cho vkai-cli)
vkai site list
vkai site create example.com
vkai db backup
vkai db restore
vkai ssl request example.com
vkai ssl renew
vkai ssl list
vkai firewall list
vkai server status
vkai user list
```

Dạng cũ `vkai panel info` / `vkai panel port` / `vkai panel entrance` vẫn hoạt
động để tương thích ngược.

## Cấu hình

Cấu hình đọc theo thứ tự ưu tiên tăng dần: giá trị mặc định → `config.yaml` →
biến môi trường. Biến môi trường **luôn thắng**.

Tệp cấu hình đặt tại `/vkai-panel/etc/.env` và `/vkai-panel/etc/config.yaml`
(quyền `0600`, thuộc người dùng `vkai`).

### Biến môi trường chính

Mọi biến đều mang tiền tố **`VKAI_`**.

| Biến | Mô tả | Mặc định |
|---|---|---|
| `VKAI_PANEL_PORT` | Cổng của panel quản trị. 80/443/22/25/3306/5432/6379 bị từ chối | `8888` |
| `VKAI_PANEL_BIND` | Địa chỉ panel lắng nghe | `0.0.0.0` |
| `VKAI_PANEL_ENTRANCE` | Lối vào an toàn, ví dụ `/vkai_a1b2c3d4`. Để trống để tự sinh | (tự sinh) |
| `VKAI_PANEL_ENTRANCE_ENABLED` | Bật lối vào an toàn | `true` |
| `VKAI_PANEL_ALLOWED_IPS` | Danh sách IP/CIDR được vào panel. Trống = mọi IP | (trống) |
| `VKAI_PANEL_TRUSTED_PROXIES` | Chỉ tin `X-Forwarded-For` từ các địa chỉ này | (trống) |
| `VKAI_PANEL_DOMAIN` | Ràng buộc panel theo một tên miền | (trống) |
| `VKAI_PANEL_TLS_CERT` / `VKAI_PANEL_TLS_KEY` | Chứng chỉ TLS riêng của panel | (trống) |
| `VKAI_PANEL_SESSION_TTL` | Hiệu lực cookie lối vào | `12h` |
| `VKAI_PANEL_CONFIG_FILE` | Nơi lưu cổng/lối vào đã sinh | `/vkai-panel/etc/panel_access.json` |
| `VKAI_SERVER_PORT` | Cổng API nội bộ | `30110` |
| `VKAI_DB_HOST` / `VKAI_DB_PORT` | PostgreSQL | `localhost` / `5432` |
| `VKAI_DB_USER` / `VKAI_DB_NAME` | Người dùng và tên cơ sở dữ liệu | `vkai` / `vkai_panel` |
| `VKAI_DB_PASSWORD` | Mật khẩu PostgreSQL | **bắt buộc, không có mặc định** |
| `VKAI_DB_SSLMODE` | Chế độ SSL tới PostgreSQL | `require` |
| `VKAI_REDIS_HOST` / `VKAI_REDIS_PORT` | Redis | `localhost` / `6379` |
| `VKAI_JWT_SECRET` | Khoá ký JWT, tối thiểu 32 ký tự ngẫu nhiên | **bắt buộc, không có mặc định** |
| `VKAI_SECRET_KEY` | Khoá mã hoá bí mật lưu trong CSDL (32 byte hex/base64) | **bắt buộc để tạo/đổi user CSDL** |
| `VKAI_CORS_ALLOWED_ORIGINS` | Danh sách origin trình duyệt được phép | (trống) |
| `VKAI_AGENT_PORT` / `VKAI_AGENT_TOKEN` | Agent và bí mật dùng chung | `30111` / **bắt buộc** |

Danh sách đầy đủ: [`.env.example`](.env.example) và [docs/CONFIGURATION.md](docs/CONFIGURATION.md).

### Tương thích ngược với tên biến cũ

Panel vẫn chấp nhận các tên cũ, nhưng chúng đã lỗi thời và sẽ bị gỡ trong một
bản phát hành lớn sau này. Tên có tiền tố `VKAI_` luôn được ưu tiên.

| Tên chuẩn | Tên cũ vẫn được chấp nhận |
|---|---|
| `VKAI_PANEL_PORT` | `PANEL_PORT` |
| `VKAI_PANEL_BIND` | `PANEL_BIND`, `PANEL_HOST`, `VKAI_PANEL_HOST` |
| `VKAI_PANEL_ENTRANCE` | `PANEL_ENTRANCE` |
| `VKAI_PANEL_ALLOWED_IPS` | `PANEL_ALLOWED_IPS`, `PANEL_ALLOW_IPS`, `VKAI_PANEL_ALLOW_IPS` |
| `VKAI_PANEL_TLS_CERT` / `VKAI_PANEL_TLS_KEY` | `PANEL_TLS_CERT_FILE`, `PANEL_TLS_KEY_FILE` (kèm biến thể không tiền tố) |
| `VKAI_DB_HOST`, `VKAI_DB_PORT`, `VKAI_DB_USER`, `VKAI_DB_PASSWORD`, `VKAI_DB_NAME`, `VKAI_DB_SSLMODE` | `VKAI_DATABASE_HOST`, `VKAI_DATABASE_PORT`, `VKAI_DATABASE_USER`, `VKAI_DATABASE_PASSWORD`, `VKAI_DATABASE_DBNAME`, `VKAI_DATABASE_SSLMODE` |

## Môi trường phát triển

```bash
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel

# PostgreSQL + Redis cho môi trường dev (Docker chỉ dùng cho CSDL)
bash setup-dev.sh

# Cửa sổ 1 - API
cd core
cp ../.env.example ../.env      # điền VKAI_DB_PASSWORD, VKAI_JWT_SECRET, VKAI_SECRET_KEY
go run ./cmd/api

# Cửa sổ 2 - giao diện
cd panel
npm install
npm run dev
```

Giao diện dev chạy ở `http://localhost:3000` và gọi API qua
`NEXT_PUBLIC_API_URL`. Trên máy chủ thật, giao diện chỉ được truy cập qua cổng
panel kèm lối vào an toàn.

Hướng dẫn chi tiết: [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) và
[docs/DEVELOPER_GUIDE.md](docs/DEVELOPER_GUIDE.md).

## Quy trình đóng góp

**Cấm push thẳng vào `main`.** Nhánh `main` được bảo vệ; mọi thay đổi đều phải đi
qua Pull Request và được ít nhất một người review chấp thuận.

1. Tạo nhánh phụ từ `main`:

   ```bash
   git checkout main
   git pull origin main
   git checkout -b feat/ten-tinh-nang
   ```

   Quy ước tên nhánh: `feat/...`, `fix/...`, `docs/...`, `refactor/...`, `chore/...`.

2. Commit theo Conventional Commits: `feat(website): them ho tro Node.js 22`.

3. Chạy kiểm thử và lint trước khi đẩy:

   ```bash
   make lint
   make test
   ```

4. Đẩy nhánh phụ và mở Pull Request vào `main`:

   ```bash
   git push origin feat/ten-tinh-nang
   ```

5. Điền đầy đủ [mẫu Pull Request](.github/PULL_REQUEST_TEMPLATE.md): CI xanh, đã
   test, ảnh chụp giao diện nếu đổi UI, đánh giá ảnh hưởng bảo mật và migration.

6. Chờ review, xử lý góp ý, rồi squash merge. Nhánh phụ được xoá sau khi merge.

Chi tiết: [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md) và
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Tài liệu

| Tài liệu | Nội dung |
|---|---|
| [docs/PANEL_ACCESS.md](docs/PANEL_ACCESS.md) | Cổng panel, lối vào an toàn, giới hạn IP, TLS |
| [docs/USER_GUIDE.md](docs/USER_GUIDE.md) | Hướng dẫn sử dụng cho quản trị viên |
| [docs/API.md](docs/API.md) | Tài liệu REST API |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | Toàn bộ tuỳ chọn cấu hình |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | Triển khai lên máy chủ thật |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Kiến trúc hệ thống |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | Thiết lập môi trường phát triển |
| [docs/DEVELOPER_GUIDE.md](docs/DEVELOPER_GUIDE.md) | Hướng dẫn cho lập trình viên |
| [docs/TESTING.md](docs/TESTING.md) | Chiến lược kiểm thử |
| [docs/SECURITY.md](docs/SECURITY.md) | Hướng dẫn bảo mật vận hành |
| [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) | Khắc phục sự cố |
| [docs/FAQ.md](docs/FAQ.md) | Câu hỏi thường gặp |
| [docs/ROADMAP.md](docs/ROADMAP.md) · [docs/ENTERPRISE_ROADMAP.md](docs/ENTERPRISE_ROADMAP.md) | Lộ trình phát triển |
| [CHANGELOG.md](CHANGELOG.md) | Lịch sử thay đổi |

## Giấy phép & hỗ trợ

Phát hành theo giấy phép MIT. Bản quyền (c) 2024 HiTechCloud Vietnam. Xem [LICENSE](LICENSE).

- Website: https://hitechcloud.vn
- Tài liệu: https://docs.vkai.vn
- Báo lỗi: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- Thảo luận: https://github.com/hitechcloud-vietnam/vkai-panel/discussions
- Báo cáo lỗ hổng bảo mật: [SECURITY.md](SECURITY.md) — **không** mở issue công khai
- Email hỗ trợ: support@hitechcloud.vn
