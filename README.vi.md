# VKAI Panel

[English](README.md) · **Tiếng Việt**

**Bảng điều khiển máy chủ & hosting đa máy chủ** — sản phẩm của **HiTechCloud** ([hitechcloud.vn](https://hitechcloud.vn)).

VKAI Panel quản lý máy chủ, website, cơ sở dữ liệu, DNS, chứng chỉ TLS, container,
tường lửa, sao lưu và giám sát từ một giao diện web duy nhất. Panel chạy trên
**cổng riêng (mặc định 8888)** phía sau một **lối vào an toàn**; cổng **80/443 dành
riêng cho website của khách hàng**.

---

## Mục lục

- [Điểm khác biệt](#điểm-khác-biệt)
- [Docker trong VKAI Panel: hai vai trò khác nhau](#docker-trong-vkai-panel-hai-vai-trò-khác-nhau)
- [Tính năng](#tính-năng)
- [Hệ điều hành hỗ trợ](#hệ-điều-hành-hỗ-trợ)
- [Cài đặt một dòng lệnh](#cài-đặt-một-dòng-lệnh)
- [Truy cập panel lần đầu](#truy-cập-panel-lần-đầu)
- [Giao diện](#giao-diện)
- [Kiến trúc](#kiến-trúc)
- [Cấu trúc mã nguồn](#cấu-trúc-mã-nguồn)
- [Đường dẫn chuẩn trên máy chủ](#đường-dẫn-chuẩn-trên-máy-chủ)
- [Dịch vụ systemd](#dịch-vụ-systemd)
- [Bản phát hành và triển khai](#bản-phát-hành-và-triển-khai)
- [Vận hành hằng ngày](#vận-hành-hằng-ngày)
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
| Lối vào | Đường dẫn bí mật dạng `/vkai_a1b2c3d4`; sai đường dẫn trả về 404 trung tính |
| Chặn theo IP / tên miền | Có, kiểm tra trước cả lối vào |
| Website khách | Toàn quyền dùng 80/443, tách hoàn toàn khỏi panel |
| Triển khai | systemd thuần (`vkai-api`, `vkai-ui`, `vkai-agent`) — binary Go + Next.js standalone, **không dùng Docker** |
| Đa máy chủ | Một panel điều khiển nhiều node qua `vkai-agent` |

## Docker trong VKAI Panel: hai vai trò khác nhau

Đây là chỗ dễ hiểu nhầm nhất, nên nói rõ ngay từ đầu. Chữ "Docker" trong dự án này
mang **hai nghĩa hoàn toàn tách biệt**.

**1. Docker như hạ tầng để dựng chính panel — đã bỏ hẳn.**
Core API, giao diện và agent đều build và chạy **trần** trên Linux: binary Go,
bản build Next.js standalone, quản lý bằng systemd. Kho mã không còn `Dockerfile`,
`docker-compose.yml`, `.dockerignore` hay bất kỳ hướng dẫn `docker compose up` nào
để dựng panel. PostgreSQL và Redis được cài **trực tiếp lên máy** bởi
`deploy/install.sh`. Máy chủ chạy panel **không cần** cài Docker Engine.

**2. Docker như tính năng dành cho khách hàng — giữ nguyên, đầy đủ.**
Khách dùng panel để quản lý container, image, volume, network và compose stack
**của chính họ**. Màn hình Docker trong giao diện, nhóm API `/api/v1/docker/*` và
các quyền `docker:*` trong RBAC đều **không thay đổi**. Tính năng này hoàn toàn
không bị cắt bỏ.

| | Docker để dựng panel | Docker như tính năng cho khách |
|---|---|---|
| Trạng thái | **Đã bỏ** | **Giữ nguyên, hỗ trợ đầy đủ** |
| Thể hiện trong mã | `Dockerfile`, `docker-compose.yml` (đã xoá) | Màn hình Docker, `/api/v1/docker/*`, quyền `docker:*` |
| Thay thế bằng | `deploy/install.sh` + systemd | Không thay thế — vẫn là tính năng chính thức |
| Cần Docker Engine trên máy chủ? | Không | Có, chỉ khi khách muốn dùng tính năng này |

Nói ngắn gọn: **panel không chạy trong Docker, nhưng panel quản lý Docker.**

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

## Hệ điều hành hỗ trợ

Trình cài đặt nhận diện hệ điều hành qua `/etc/os-release` và chỉ chạy trên các
họ dưới đây.

| Hệ điều hành | Phiên bản khuyến nghị | Trạng thái | Ghi chú |
|---|---|---|---|
| Ubuntu Server | 22.04 LTS, 24.04 LTS | Hỗ trợ đầy đủ | Nền tảng kiểm thử chính |
| Ubuntu Server | 20.04 LTS | Hỗ trợ | Cần kho Node.js 20 từ NodeSource |
| Debian | 12 (Bookworm), 11 (Bullseye) | Hỗ trợ đầy đủ | |
| Rocky Linux | 9, 8 | Hỗ trợ | Dùng nhánh `dnf`/`yum` của trình cài |
| AlmaLinux | 9, 8 | Hỗ trợ | Dùng nhánh `dnf`/`yum` của trình cài |
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

Cài từ mã nguồn, khi cần build tại chỗ:

```bash
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel
sudo bash deploy/install.sh
```

> Đọc kỹ [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) trước khi cài lên máy chủ đang
> chạy dịch vụ thật.

## Truy cập panel lần đầu

Kết thúc cài đặt, panel in một lần duy nhất thông tin truy cập:

```
=============================================================================
 VKAI Panel v1.0.0 - INSTALLATION COMPLETE (fresh)
 HiTechCloud
 System  : Ubuntu 24.04.1 LTS (x86_64)
=============================================================================

PANEL ACCESS
  Full URL   : https://203.0.113.10:8888/vkai_91ac5b65/
  Port       : 8888   (80/443 stay reserved for the customer websites)
  Entrance   : /vkai_91ac5b65
  Domain     : (none - reached by IP 203.0.113.10)
  Allowed IPs: any source address
  Any other path returns a neutral 404. That is deliberate.

ADMINISTRATOR
  Username : admin
  Password : <generated>
  (!) This is the DEFAULT password - change it immediately after logging in.

CERTIFICATE
  Mode        : letsencrypt
  Source      : Let's Encrypt
  Expires     : 2026-11-26
  SHA-256     : <fingerprint>
```

(Đã lược bớt. Bản in thật còn liệt kê toàn bộ đường dẫn trên đĩa, các dịch vụ
systemd, cơ sở dữ liệu và Redis, trạng thái tường lửa, kênh cập nhật, và việc máy
này đã được ghi danh sẵn làm node đầu tiên được quản lý. Trình cài in bằng tiếng
Anh.)

Ba việc phải làm ngay:

1. **Mở tường lửa cho cổng panel trước khi đóng console.**

   ```bash
   sudo ufw allow 8888/tcp                                                          # Ubuntu / Debian
   sudo firewall-cmd --permanent --add-port=8888/tcp && sudo firewall-cmd --reload  # RHEL / Rocky / Alma
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
Mono cho mã và nhật ký. Bố cục gồm sidebar điều hướng bên trái, thanh trên hiển thị
máy chủ đang chọn và thông báo, phần thân là nội dung từng màn hình.

Ảnh chụp màn hình đặt trong `docs/images/` và được nhúng vào tài liệu tương ứng.

| Màn hình | Nội dung |
|---|---|
| Bảng điều khiển | CPU, RAM, đĩa, băng thông theo thời gian thực; cảnh báo đang mở; tác vụ gần đây |
| Máy chủ | Danh sách node, trạng thái agent, thêm và gỡ máy chủ |
| Website | Danh sách site, loại runtime, tình trạng TLS, thao tác nhanh; quản lý WordPress |
| Cơ sở dữ liệu | Instance, database, người dùng, sao lưu, console truy vấn |
| SSL | Chứng chỉ, ngày hết hạn, phát hành và gia hạn Let's Encrypt |
| DNS | Vùng và bản ghi DNS |
| Docker | Container, image, volume, network, compose |
| Trình quản lý tệp | Duyệt, sửa, tải lên và tải xuống trong thư mục gốc đã giới hạn |
| Cron | Lịch chạy, lịch sử, nhật ký từng lần chạy |
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
                                       |  duong vao duy nhat, loi    |
                                       |  vao duoc kiem tra tai day  |
                                       +--------------+--------------+
                                                      | chi khi qua cong gac
                                                      v
                                       +-----------------------------+
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

Cổng `30110` (API) và `3000` (giao diện) chỉ lắng nghe nội bộ; mọi truy cập từ bên
ngoài đi qua cổng panel `8888`. nginx chỉ có **một** upstream là `vkai-api`: lối vào
an toàn được kiểm tra ở đó, rồi API mới chuyển tiếp phần giao diện sang Next.js.

### Công nghệ

| Thành phần | Công nghệ |
|---|---|
| API (`core/`) | Go 1.22, Gin, JWT, pgx, go-redis, asynq |
| Giao diện (`panel/`) | Next.js 14, React 18, TypeScript, Tailwind CSS |
| Cơ sở dữ liệu | PostgreSQL 16, Redis 7 |
| Agent (`agent/`) | Binary Go (`vkaid`) |
| Web server | Nginx (mặc định), Apache, OpenLiteSpeed, LiteSpeed, Caddy, Traefik |
| Chạy dịch vụ | systemd — binary Go + bản build Next.js standalone, không dùng Docker |

## Cấu trúc mã nguồn

```
vkai-panel/
├── core/                       # Máy chủ API viết bằng Go (trước đây là backend/)
│   ├── cmd/
│   │   ├── api/                # Điểm vào dịch vụ vkai-api
│   │   ├── cli/                # Lệnh quản trị
│   │   └── panelctl/           # vkai-panelctl: cổng, lối vào, IP, tên miền, chứng chỉ
│   ├── internal/
│   │   ├── acme/               # Client ACME (RFC 8555)
│   │   ├── auth/               # Xác thực JWT
│   │   ├── config/             # Cấu hình + cổng và lối vào panel
│   │   ├── database/           # Kết nối cơ sở dữ liệu
│   │   ├── handler/            # HTTP handler
│   │   ├── middleware/         # HTTP middleware
│   │   ├── models/             # Mô hình dữ liệu
│   │   ├── rbac/               # Phân quyền theo vai trò
│   │   ├── repository/         # Lớp truy cập dữ liệu
│   │   ├── service/            # Nghiệp vụ
│   │   ├── terminal/           # Shell đăng nhập trên pseudo-terminal
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
├── deploy/                     # install.sh, deploy.sh, unit systemd, cấu hình nginx
│   ├── install.sh              # Bộ cài đa hệ điều hành (cài trần lên máy)
│   ├── systemd/                # vkai-api.service, vkai-ui.service, vkai-agent.service
│   ├── nginx/                  # vhost cho cổng panel
│   └── scripts/deploy.sh       # Triển khai gói .tar.gz và quay lui
├── scripts/                    # Script tiện ích
├── docs/                       # Tài liệu
├── setup-dev.sh                # Dựng môi trường phát triển
└── Makefile                    # build / test / lint / package
```

Không có `Dockerfile` hay `docker-compose.yml` trong kho mã: panel được build và
chạy trần. Xem [Docker trong VKAI Panel: hai vai trò khác nhau](#docker-trong-vkai-panel-hai-vai-trò-khác-nhau).

> Đường dẫn import Go **không đổi**: module vẫn là
> `github.com/hitechcloud-vietnam/vkai-panel` (và `.../agent`). Chỉ tên thư mục
> trên đĩa đổi từ `backend/` sang `core/` và `frontend/` sang `panel/`.

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
`VKAI_SSL_ROOT` hoặc `VKAI_TMP_ROOT` chỉ dời nhánh tương ứng — đó là cách gắn một ổ
đĩa riêng cho sao lưu hoặc cho nhật ký.

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

Panel chạy dưới người dùng hệ thống **`vkai`**, không chạy bằng `root`. Riêng
`vkai-agent` chạy bằng `root` vì phải thao tác ở mức hệ thống, và là dịch vụ
**tuỳ chọn**. Cả ba unit đều đã bật gia cố systemd: `NoNewPrivileges`,
`ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, và `ReadWritePaths` chỉ mở
đúng các thư mục cần ghi.

## Bản phát hành và triển khai

Bản phát hành được đóng thành gói `.tar.gz` rồi đẩy xuống máy chủ. Mỗi lần triển
khai giải nén vào **một thư mục riêng theo phiên bản** và trỏ symlink `current`
sang đó, nên quay lui chỉ là trỏ lại symlink.

```
/vkai-panel/releases/20250315_101500/    # bản cũ, vẫn giữ để quay lui
/vkai-panel/releases/20250316_143000/    # bản vừa triển khai
/vkai-panel/current -> /vkai-panel/releases/20250316_143000
```

`etc/`, `logs/`, `www/`, `ssl/` nằm **ngoài** release: triển khai không ghi đè,
quay lui không đưa chúng về cũ.

Gói phải có đúng cấu trúc sau. CI đóng gói tự động; nếu đóng tay thì theo mẫu này:

```
core/bin/vkai-api                    # binary API
core/migrations/*.sql                # migration
panel/.next/standalone/server.js     # bản build giao diện
panel/.next/standalone/.next/static  # BẮT BUỘC, thiếu là giao diện lỗi client-side
agent/bin/vkai-agent                 # tuỳ chọn
```

```bash
# Trên máy build
make build                   # binary Go + bản build Next.js standalone
tar -czf vkai-panel-1.2.0.tar.gz -C dist .

# Trên máy chủ
sudo bash deploy/scripts/deploy.sh deploy /tmp/vkai-panel-1.2.0.tar.gz
sudo bash deploy/scripts/deploy.sh list         # các bản đang giữ
sudo bash deploy/scripts/deploy.sh status
sudo bash deploy/scripts/deploy.sh rollback     # quay về bản trước
```

Lệnh `deploy` kiểm tra gói hợp lệ **trước khi** động vào hệ thống đang chạy, sao
lưu cơ sở dữ liệu, chạy migration của bản mới **trước khi** đổi symlink, rồi mới
trỏ `current` sang bản mới và khởi động lại dịch vụ. Health check bao gồm **cả API
lẫn giao diện**; **hỏng thì tự động quay lui** về bản trước. Hệ thống giữ bản đang
chạy cộng 5 bản gần nhất.

> Quay lui chỉ đưa **mã nguồn** về bản cũ, **không** hoàn tác migration cơ sở dữ liệu.

Chi tiết: [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Vận hành hằng ngày

```bash
# Xem nhật ký
sudo journalctl -u vkai-api -f                 # API, theo dõi trực tiếp
sudo journalctl -u vkai-ui -n 200 --no-pager   # 200 dòng cuối của giao diện
sudo journalctl -u vkai-agent --since "1 hour ago"
sudo journalctl -u vkai-api -p err --since today   # chỉ lỗi trong hôm nay

# Kiểm tra sức khoẻ
curl -fsS http://127.0.0.1:30110/health        # API còn sống
curl -fsS http://127.0.0.1:30110/ready         # sẵn sàng (đã nối CSDL và Redis)
curl -fsS http://127.0.0.1:3000/ -o /dev/null  # giao diện phản hồi
systemctl is-active vkai-api vkai-ui

# Quay lui bản phát hành
sudo bash deploy/scripts/deploy.sh rollback
readlink -f /vkai-panel/current                # bản đang chạy
```

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
vkai port 8888              # Đổi cổng panel (80 và 443 bị từ chối)
vkai port random            # Cổng ngẫu nhiên trong khoảng 8000-65535
vkai entrance random        # Sinh lối vào an toàn mới
vkai panel allow-ip 203.0.113.7,10.0.0.0/8
vkai panel domain panel.example.com

# Chứng chỉ TLS của chính panel
vkai cert status            # Nhà phát hành, chủ thể, hạn dùng, số ngày còn lại
vkai cert issue             # Đặt chứng chỉ từ Let's Encrypt
vkai cert renew             # Gia hạn nếu sắp hết hạn; chưa cần thì không làm gì

# Vận hành panel
vkai backup                 # Sao lưu CSDL và cấu hình vào /vkai-panel/www/backup
vkai update                 # Build lại core/ và panel/, rồi khởi động lại dịch vụ
vkai upgrade --check        # Kiểm tra có bản mới hay không (không bao giờ tự cài)
vkai uninstall              # Gỡ cài đặt

# Lệnh nghiệp vụ, uỷ quyền cho vkai-cli
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

`vkai cert renew` chính là lệnh mà timer systemd `vkai-cert-renew` chạy hai lần mỗi
ngày. Nó chỉ đặt chứng chỉ mới khi chứng chỉ hiện tại sắp hết hạn, bị thiếu, hoặc
không còn bao phủ định danh đã cấu hình; ngoài các trường hợp đó nó báo số ngày còn
lại và kết thúc thành công mà không liên hệ nhà cung cấp chứng chỉ. Một lần gia hạn
thất bại **không bao giờ** xoá mất chứng chỉ đang dùng.

Dạng cũ `vkai panel info` / `vkai panel port` / `vkai panel entrance` /
`vkai panel cert` vẫn hoạt động để tương thích ngược.

## Cấu hình

Cấu hình đọc theo thứ tự ưu tiên tăng dần: giá trị mặc định → `config.yaml` →
biến môi trường. Biến môi trường **luôn thắng**.

Tệp cấu hình đặt tại `/vkai-panel/etc/.env` và `/vkai-panel/etc/config.yaml`
(quyền `0600`, thuộc người dùng `vkai`).

### Biến môi trường chính

Mọi biến đều mang tiền tố **`VKAI_`**.

| Biến | Mô tả | Mặc định |
|---|---|---|
| `VKAI_PANEL_PORT` | Cổng của panel quản trị. 80, 443, 22, 25, 3306, 5432 và 6379 bị từ chối | `8888` |
| `VKAI_PANEL_BIND` | Địa chỉ panel lắng nghe | `0.0.0.0` |
| `VKAI_PANEL_ENTRANCE` | Lối vào an toàn, ví dụ `/vkai_a1b2c3d4`. Để trống để tự sinh | (tự sinh) |
| `VKAI_PANEL_ENTRANCE_ENABLED` | Bật lối vào an toàn | `true` |
| `VKAI_PANEL_ALLOWED_IPS` | Danh sách IP/CIDR được vào panel. Trống nghĩa là mọi IP | (trống) |
| `VKAI_PANEL_TRUSTED_PROXIES` | Chỉ tin `X-Forwarded-For` từ các địa chỉ này | (trống) |
| `VKAI_PANEL_DOMAIN` | Ràng buộc panel theo một tên miền | (trống) |
| `VKAI_PANEL_TLS_CERT` / `VKAI_PANEL_TLS_KEY` | Chứng chỉ và khoá TLS riêng của panel | (trống) |
| `VKAI_PANEL_SESSION_TTL` | Hiệu lực cookie lối vào | `12h` |
| `VKAI_PANEL_CONFIG_FILE` | Nơi lưu cổng và lối vào đã sinh | `/vkai-panel/etc/panel_access.json` |
| `VKAI_SERVER_PORT` | Cổng API nội bộ | `30110` |
| `VKAI_DB_HOST` / `VKAI_DB_PORT` | PostgreSQL | `localhost` / `5432` |
| `VKAI_DB_USER` / `VKAI_DB_NAME` | Người dùng và tên cơ sở dữ liệu | `vkai` / `vkai_panel` |
| `VKAI_DB_PASSWORD` | Mật khẩu PostgreSQL | **bắt buộc, không có mặc định** |
| `VKAI_DB_SSLMODE` | Chế độ SSL tới PostgreSQL | `require` |
| `VKAI_REDIS_HOST` / `VKAI_REDIS_PORT` | Redis | `localhost` / `6379` |
| `VKAI_JWT_SECRET` | Khoá ký JWT, tối thiểu 32 ký tự ngẫu nhiên | **bắt buộc, không có mặc định** |
| `VKAI_SECRET_KEY` | Khoá mã hoá bí mật lưu trong CSDL (32 byte, hex hoặc base64) | **bắt buộc để tạo hoặc đổi user CSDL** |
| `VKAI_CORS_ALLOWED_ORIGINS` | Danh sách origin trình duyệt được phép | (trống) |
| `VKAI_AGENT_PORT` / `VKAI_AGENT_ENROLMENT_TOKEN` | Cổng kênh điều khiển agent, và token ghi danh dùng một lần, chỉ ở lần khởi động đầu tiên của agent. Không có bí mật dùng chung: xem [docs/AGENT_CHANNEL.md](docs/AGENT_CHANNEL.md) | `30111` / (trống) |

Danh sách đầy đủ: [`.env.example`](.env.example) và
[docs/CONFIGURATION.md](docs/CONFIGURATION.md).

### Tương thích ngược với tên biến cũ

Panel vẫn chấp nhận các tên cũ, nhưng chúng đã lỗi thời và sẽ bị gỡ trong một bản
phát hành lớn sau này. Tên có tiền tố `VKAI_` luôn được ưu tiên.

| Tên chuẩn | Tên cũ vẫn được chấp nhận |
|---|---|
| `VKAI_PANEL_PORT` | `PANEL_PORT` |
| `VKAI_PANEL_BIND` | `PANEL_BIND`, `PANEL_HOST`, `VKAI_PANEL_HOST` |
| `VKAI_PANEL_ENTRANCE` | `PANEL_ENTRANCE` |
| `VKAI_PANEL_ALLOWED_IPS` | `PANEL_ALLOWED_IPS`, `PANEL_ALLOW_IPS`, `VKAI_PANEL_ALLOW_IPS` |
| `VKAI_PANEL_TLS_CERT` / `VKAI_PANEL_TLS_KEY` | `PANEL_TLS_CERT_FILE`, `PANEL_TLS_KEY_FILE`, kèm các biến thể không tiền tố |
| `VKAI_DB_HOST`, `VKAI_DB_PORT`, `VKAI_DB_USER`, `VKAI_DB_PASSWORD`, `VKAI_DB_NAME`, `VKAI_DB_SSLMODE` | `VKAI_DATABASE_HOST`, `VKAI_DATABASE_PORT`, `VKAI_DATABASE_USER`, `VKAI_DATABASE_PASSWORD`, `VKAI_DATABASE_DBNAME`, `VKAI_DATABASE_SSLMODE` |

## Môi trường phát triển

```bash
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel

# Cài PostgreSQL và Redis trực tiếp lên máy, cài phụ thuộc, sinh .env cho dev
bash setup-dev.sh

# Cửa sổ 1 — API
cd core
cp ../.env.example ../.env      # điền VKAI_DB_PASSWORD, VKAI_JWT_SECRET, VKAI_SECRET_KEY
go run ./cmd/api

# Cửa sổ 2 — giao diện
cd panel
npm install
npm run dev
```

Khi phát triển, giao diện chạy ở `http://localhost:3000` và gọi API qua
`NEXT_PUBLIC_API_URL`. Trên máy chủ thật, giao diện chỉ truy cập được qua cổng
panel, phía sau lối vào an toàn.

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

2. Viết commit theo Conventional Commits:
   `feat(website): add Node.js 22 support`.

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
| [docs/INSTALL.md](docs/INSTALL.md) | Hướng dẫn cài đặt đầy đủ |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | Triển khai lên máy chủ thật |
| [docs/UPGRADE.md](docs/UPGRADE.md) | Nâng cấp bản đã cài |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Kiến trúc hệ thống |
| [docs/AGENT_CHANNEL.md](docs/AGENT_CHANNEL.md) | Kênh panel–agent và quy trình ghi danh |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | Thiết lập môi trường phát triển |
| [docs/DEVELOPER_GUIDE.md](docs/DEVELOPER_GUIDE.md) | Hướng dẫn cho lập trình viên |
| [docs/TESTING.md](docs/TESTING.md) | Chiến lược kiểm thử |
| [docs/SECURITY.md](docs/SECURITY.md) | Hướng dẫn bảo mật vận hành |
| [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) | Khắc phục sự cố |
| [docs/FAQ.md](docs/FAQ.md) | Câu hỏi thường gặp |
| [docs/ROADMAP.md](docs/ROADMAP.md) · [docs/ENTERPRISE_ROADMAP.md](docs/ENTERPRISE_ROADMAP.md) | Lộ trình phát triển |
| [CHANGELOG.md](CHANGELOG.md) | Lịch sử thay đổi |

## Giấy phép & hỗ trợ

Phát hành theo giấy phép MIT. Bản quyền (c) 2024 HiTechCloud Vietnam. Xem
[LICENSE](LICENSE).

- Website: https://hitechcloud.vn
- Tài liệu: https://docs.vkai.vn
- Báo lỗi: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- Thảo luận: https://github.com/hitechcloud-vietnam/vkai-panel/discussions
- Báo cáo lỗ hổng bảo mật: [SECURITY.md](SECURITY.md) — **không** mở issue công khai
- Email hỗ trợ: support@hitechcloud.vn
