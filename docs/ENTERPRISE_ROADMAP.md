# VKAI Panel - Lộ trình Doanh nghiệp (Enterprise Roadmap)

Tài liệu định hướng sản phẩm cho VKAI Panel - HiTech Cloud.
Phiên bản tài liệu: 1.0 - Cập nhật: 2026-08-28.
Đối tượng đọc: ban lãnh đạo, quản lý sản phẩm, kiến trúc sư hệ thống, đội phát triển.

---

## 1. Bối cảnh và định vị sản phẩm

### 1.1. Tại sao tài liệu này tồn tại

VKAI Panel là control panel đầu tiên do HiTech Cloud tự phát triển. Khác với một dự án nội bộ,
đây là **bộ mặt kỹ thuật của doanh nghiệp**: khách hàng hosting sẽ đăng nhập vào panel nhiều lần
mỗi tuần, còn website công ty thì họ chỉ xem một lần khi mua hàng. Panel là điểm chạm thường
xuyên nhất giữa doanh nghiệp và khách hàng, đồng thời là nơi mọi sự cố kỹ thuật bộc lộ ra ngoài.

Hệ quả về mặt yêu cầu:

- Một lỗi giao diện nhỏ trong panel gây tổn hại uy tín lớn hơn một lỗi tương tự trong công cụ nội bộ.
- Một sự cố bảo mật ở panel không chỉ mất một máy chủ, mà mất **toàn bộ hạ tầng của mọi khách hàng**
  đang được panel quản lý, kèm theo trách nhiệm pháp lý.
- Panel phải chạy được trên VPS của khách hàng (nơi ta không kiểm soát), chứ không chỉ trong hạ tầng nội bộ.

Tài liệu này xác định: hiện panel đang ở đâu, thiếu gì so với một sản phẩm thương mại, và cần làm
gì theo thứ tự nào để có thể bán được, cạnh tranh được, rồi khác biệt hoá.

### 1.2. Định vị so với đối thủ

| Sản phẩm | Điểm mạnh | Điểm yếu khai thác được | Mô hình giá |
|---|---|---|---|
| **cPanel/WHM** | Chuẩn de-facto, hệ sinh thái WHMCS/Softaculous, tài liệu và nhân sự dồi dào, migration tool trưởng thành | Giá tăng liên tục theo số tài khoản; giao diện cũ; nặng; chỉ RHEL-family; không hỗ trợ tiếng Việt tử tế | Theo tài khoản, rất đắt ở quy mô lớn |
| **Plesk** | Giao diện tốt nhất nhóm cũ, hỗ trợ Windows, extension marketplace | Đắt, khoá tính năng theo edition, nặng, mô hình license phức tạp | Theo edition + số domain |
| **aaPanel/BT Panel** | Miễn phí, cài nhanh, phổ biến ở thị trường châu Á, nhẹ | Nguồn gốc Trung Quốc gây lo ngại tuân thủ dữ liệu; kiến trúc bảo mật yếu; tính năng doanh nghiệp nằm sau paywall; tiếng Việt máy dịch | Freemium |
| **DirectAdmin** | Nhẹ, ổn định, giá dễ chịu, reseller tốt | Giao diện lạc hậu; API hạn chế; ít đổi mới | Theo license máy chủ |
| **CyberPanel** | OpenLiteSpeed/LSCache sẵn, miễn phí, nhanh cho WordPress | Chất lượng code không đồng đều, lịch sử CVE nghiêm trọng, hỗ trợ yếu | Free/Enterprise |

**Định vị của VKAI Panel:**

> Control panel doanh nghiệp, đa máy chủ, đa tenant, **do người Việt phát triển và hỗ trợ bằng
> tiếng Việt**, với kiến trúc bảo mật minh bạch và dữ liệu nằm trong lãnh thổ Việt Nam.

Ba trục khác biệt cần bám:

1. **Đa máy chủ ngay từ lõi.** cPanel/DirectAdmin/CyberPanel về bản chất là panel một máy chủ,
   phải mua thêm WHM/cluster để quản lý nhiều máy. VKAI đã có mô hình `servers` + agent `vkaid`
   + `clusters`/`load_balancers`/`ha_pairs` (`core/internal/models/cluster.go`) từ đầu.
   Đây là lợi thế kiến trúc, cần khai thác thay vì bắt chước mô hình một máy chủ.
2. **Tuân thủ và chủ quyền dữ liệu.** Khách hàng ngân hàng, fintech, cơ quan nhà nước Việt Nam
   không thể chấp nhận panel gửi telemetry ra nước ngoài. Nhật ký kiểm toán bất biến và
   báo cáo tuân thủ là điểm bán hàng, không phải chi phí.
3. **Trải nghiệm tiếng Việt thật.** Không phải dịch chuỗi UI, mà là: thông báo lỗi giải thích
   được cho khách hàng cuối, tài liệu tiếng Việt, cảnh báo qua Zalo/Telegram, hỗ trợ theo giờ Việt Nam.

**Không nên cạnh tranh ở:** số lượng ứng dụng one-click (Softaculous có hơn 400), hỗ trợ Windows Server,
hoặc số lượng extension. Đó là những trận đánh không thắng được ở giai đoạn này.

---

## 2. Đánh giá hiện trạng

### 2.1. Bức tranh kỹ thuật

Nền tảng: Go 1.22 + Gin (API, `core/`), Next.js 14 App Router + TypeScript + Tailwind
(giao diện, `panel/`), PostgreSQL 16 + Redis 7, agent Go `vkaid` (`agent/cmd/main.go`),
triển khai bằng systemd qua `deploy/install.sh`.

Quy mô mã nguồn hiện tại: 43 handler, khoảng 35 service, 33 repository, 18 file migration SQL,
7 adapter web server, hàng đợi công việc asynq, WebSocket hub.

### 2.2. Những gì ĐÃ CÓ (không đề xuất lại)

| Nhóm | Trạng thái | Vị trí trong repo |
|---|---|---|
| Xác thực JWT + refresh token | Hoạt động | `core/internal/auth/jwt.go`, `middleware/auth.go` |
| RBAC 8 vai trò, quyền dạng `resource.action` | Hoạt động ở mức khung | `core/internal/rbac/rbac.go`, `migrations/015_create_multi_user_tables.sql` |
| Đa tenant, cách ly theo `tenant_id` ở mọi bảng | Hoạt động | `models/models.go`, toàn bộ repository |
| Quản lý máy chủ + agent heartbeat | Cơ bản | `agent/cmd/main.go`, `handler/server.go` |
| Website, domain, vhost | Hoạt động | `service/website.go`, `webserver/*.go` |
| Adapter web server: nginx, apache, openlitespeed, litespeed, caddy, traefik | Nginx đầy đủ (300 dòng), các adapter khác đã có khung nhưng còn TODO | `core/internal/webserver/` |
| SSL Let's Encrypt + chứng chỉ tự tải lên | Hoạt động | `service/ssl.go`, `handler/ssl.go` |
| CSDL MySQL/MariaDB/PostgreSQL/Redis | Hoạt động | `service/database.go` |
| DNS zone + record (A/AAAA/CNAME/MX/TXT/NS/SRV) | Hoạt động | `service/dns.go` |
| File manager (duyệt, sửa, nén, phân quyền) | Hoạt động | `service/file_manager.go` |
| Cron job | Hoạt động | `service/cron.go` |
| Firewall (iptables/UFW) | Hoạt động | `service/firewall.go` |
| Quản lý dịch vụ systemd | Hoạt động | `service/service_manager.go` |
| Docker (container, image, volume) | Cơ bản | `handler/docker.go` |
| PHP đa phiên bản: version, pool FPM, extension, config | **Có mô hình dữ liệu và CRUD**, chưa tác động thật lên hệ thống | `service/php.go`, `migrations/002_php_management.sql` |
| Node.js app + systemd unit | Có phần thực thi | `service/nodeapp.go`, `internal/nodeapp/systemd.go` |
| Reverse proxy | CRUD | `service/reverseproxy.go` |
| Git deployment + webhook secret | CRUD + khung | `service/gitdeployment.go` |
| WordPress: site, plugin, theme | **Chỉ là metadata trong DB**, chưa gọi WP-CLI | `service/wordpress.go` |
| Mail server: domain, account, alias, DKIM key, queue, spam filter | Mô hình dữ liệu khá đầy đủ | `models/mail_server.go`, `migrations/013_create_mail_server_tables.sql` |
| Email marketing | CRUD | `service/email_marketing.go` |
| Sao lưu website/CSDL/file, lịch, dọn dẹp | **Chỉ ghi ra thư mục local** (`/vkai-panel/www/backup`, đổi được bằng `VKAI_BACKUP_ROOT`) | `service/backup.go` |
| Giám sát: metric, alert, dashboard | Ghi metric và đánh giá ngưỡng | `service/monitoring.go` |
| Quản lý log tập trung | CRUD | `service/log.go` |
| Thông báo: notification, template, channel, preference | **Lưu DB, chưa có bộ gửi thật** | `service/notification.go` |
| Nhật ký kiểm toán | Có bảng và truy vấn | `models/audit.go`, `migrations/011_audit_logging.sql` |
| WAF: rule, policy, event | CRUD | `service/waf.go`, `migrations/010_create_waf_tables.sql` |
| Tamper Proof (giám sát toàn vẹn file, baseline SHA-256/512, alert, scan result) | **Điểm mạnh thật sự**, hiếm panel nào có | `models/tamper_proof.go`, `migrations/018_create_tamper_proof_tables.sql` |
| File Protection | CRUD | `service/file_protection.go` |
| Thống kê website (visitor, quốc gia) | Có | `repository/website_stats.go` |
| Báo cáo hàng ngày | Có | `service/daily_report.go` |
| Scheduled tasks | Có | `service/scheduled_task.go` |
| Cluster / Load balancer / HA pair | Mô hình dữ liệu + CRUD | `models/cluster.go` |
| API key có scope, hash SHA-256, lưu prefix 12 ký tự | Có tạo/thu hồi | `service/apikey.go` |
| Hàng đợi công việc asynq (backup, restore, deploy, ssl, cleanup, healthcheck, metric, logrotate, notification) | Khung đầy đủ, 9 handler còn TODO | `internal/job/queue.go` |
| WebSocket hub + terminal web (xterm.js) | Có | `handler/websocket.go`, `app/(dashboard)/terminal` |
| Rollback cấu hình | Có | `service/config.go`, `migrations/014_config_rollback.sql` |
| CLI `vkai` (server, site, ssl, db, backup, firewall, user, service) | Có | `core/internal/cli/` |
| Cài đặt systemd không cần Docker | Có | `deploy/install.sh`, `deploy/systemd/` |
| Security headers HTTP (HSTS, CSP, X-Frame-Options...) | Có | `middleware/middleware.go` |

Nhận xét chung: **bề rộng tính năng đã rất ấn tượng** - 34 màn hình trong
`panel/src/app/(dashboard)`. Vấn đề không nằm ở bề rộng.

### 2.3. Những gì THIẾU so với một control panel thương mại

Chia thành bốn nhóm theo mức độ nghiêm trọng.

**Nhóm A - Khoảng cách giữa "có bảng dữ liệu" và "thực sự làm việc"**

Đây là rủi ro lớn nhất hiện nay. Nhiều module có đầy đủ model, repository, service, handler và
màn hình, nhưng phần chạm vào hệ điều hành thì chưa có:

- `service/wordpress.go` tạo bản ghi WordPress site trong PostgreSQL nhưng không hề tải mã nguồn
  WordPress, không tạo `wp-config.php`, không chạy `wp core install`. Không có bất kỳ tham chiếu
  `wp-cli` nào trong toàn bộ mã nguồn.
- `service/backup.go` chỉ tạo file tar tại `job.Destination` mặc định `/vkai-panel/www/backup`
  (`config.BackupRoot()`, đổi được bằng `VKAI_BACKUP_ROOT`).
  Trường `Destination` trong `models.BackupJob` ghi chú `local, s3, sftp` nhưng không có mã
  nào tải lên S3. Sao lưu nằm cùng máy với dữ liệu gốc **không phải là sao lưu**.
- `service/notification.go` lưu notification và channel vào DB, không có mã gửi SMTP,
  không có Telegram, không có webhook outbound.
- `service/security.go` có `runScan()` với nội dung là `// TODO: Implement actual security scanning logic`.
- `internal/job/queue.go` có 9 hàm `handleXxxTask` đều còn TODO ở phần thân.
- `handler/router.go` dòng 908-916: nhóm `/api/v1/agent` gồm `/heartbeat` và `/register` là
  hai hàm rỗng có comment TODO, trong khi agent thực tế đang POST tới đó.
- `middleware.RateLimit()` trả về `c.Next()` không giới hạn gì.

Hệ quả kinh doanh: nếu bán hàng ngay bây giờ, khách hàng bấm nút và không có gì xảy ra ở tầng hệ
thống. Đây là loại lỗi phá huỷ niềm tin nhanh nhất.

**Nhóm B - Tính năng thương mại bắt buộc mà chưa có mô hình dữ liệu**

- **Gói dịch vụ và hạn mức (hosting package/quota).** `models.Tenant` chỉ có `Plan`, `MaxServers`,
  `MaxWebsites`. Không có hạn mức disk, băng thông, số CSDL, số email, số subdomain, số cron,
  số tiến trình, giới hạn CPU/RAM/IO. Không có cơ chế cưỡng chế hạn mức. Không bán hosting được nếu không có cái này.
- **Đại lý / phân cấp tài khoản (reseller).** Không có từ khoá `reseller` nào trong repo.
  Tenant hiện phẳng, không có quan hệ cha-con, không có phân bổ hạn mức từ đại lý xuống khách.
- **Di trú từ cPanel/DirectAdmin.** Không có. Đây là rào cản chuyển đổi lớn nhất của thị trường.
- **Hoá đơn/thanh toán.** Không có tích hợp WHMCS, không có API cung cấp/thu hồi tài khoản tự động.
- **i18n.** Không có hệ thống dịch. Toàn bộ chuỗi UI hard-code tiếng Anh trong TSX.
  Một panel Việt Nam không có tiếng Việt là mâu thuẫn với chính định vị của mình.
- **2FA/TOTP.** `models.User` đã có `MFAEnabled` và `MFASecret`, nhưng không có mã TOTP nào
  (0 kết quả cho `totp`, `otpauth`). Trường có sẵn nhưng chưa được dùng.

**Nhóm C - Vận hành và vận hành sự cố**

- Không có trang trạng thái dịch vụ (status page) cho khách hàng.
- Không có cam kết SLA hay đo lường uptime.
- Không có kiểm thử khôi phục (restore drill) - sao lưu chưa từng được xác minh là khôi phục được.
- Không có đường vào khẩn cấp khi panel/mạng hỏng.
- Không có API công khai có tài liệu (OpenAPI) và webhook outbound.
- Không có test: theo `PROGRESS.md`, toàn bộ mục unit/integration/E2E đều chưa đánh dấu.

**Nhóm D - Bảo mật kiến trúc**

Xem chi tiết mục 4. Ba vấn đề nghiêm trọng nhất:

1. Agent xác thực bằng **một token tĩnh chia sẻ** trong header `X-Agent-Token`
   (`agent/cmd/main.go` dòng 156, 294-296), qua HTTP thường, không TLS, không xoay khoá.
   Agent có endpoint `/execute` nhận lệnh tuỳ ý. Ai lấy được token là có shell trên mọi máy chủ.
2. `README.md` công bố công khai mật khẩu mặc định `admin/admin123`, và không có cơ chế
   bắt buộc đổi ở lần đăng nhập đầu.
3. Nhật ký kiểm toán (`models.AuditLog`) là bảng PostgreSQL thông thường - bất kỳ ai có
   quyền ghi DB đều xoá được dấu vết. Không có chuỗi hash.

---

## 3. Đề xuất tính năng theo ba đợt

Quy ước:
- **Độ khó**: Thấp (dưới 1 tuần-người) | Trung bình (1-3 tuần-người) | Cao (1-2 tháng-người) | Rất cao (trên 2 tháng-người).
- Mỗi tính năng ghi rõ giá trị kinh doanh, độ khó, phụ thuộc.

---

### ĐỢT P0 - Bắt buộc hoàn tất trước khi bán hàng

Nguyên tắc P0: **không bán một tính năng chưa chạy thật**. Đợt này ưu tiên biến những module
"có bảng dữ liệu" thành "thực sự làm việc", cộng với những thứ không bán hosting được nếu thiếu.

#### P0-1. Đóng khoảng cách thực thi (Execution Gap Closure)

- **Nội dung:** Rà soát toàn bộ 24 điểm TODO trong `core/internal`, hoàn thiện 9 task handler
  trong `job/queue.go`, viết hai handler agent thật trong `handler/router.go`, cài đặt rate limit
  thật bằng `golang.org/x/time/rate` (đã có sẵn trong dependency tree). Với mỗi module, xác định
  rõ: module này thực sự thay đổi được gì trên hệ điều hành?
- **Giá trị kinh doanh:** Đây là điều kiện cần tuyệt đối. Bán một panel mà nút bấm không có tác dụng
  sẽ tạo làn sóng hoàn tiền và mất uy tín không phục hồi được. Không có giá trị dương, chỉ có
  việc tránh giá trị âm rất lớn.
- **Độ khó:** Cao.
- **Phụ thuộc:** Không. Phải làm trước mọi thứ khác.

#### P0-2. Gói dịch vụ và hạn mức (Package & Quota)

- **Nội dung:** Bảng `hosting_packages` (disk MB, băng thông GB/tháng, số website, số CSDL,
  số tài khoản email, số subdomain, số cron, số tiến trình đồng thời, giới hạn CPU %, RAM MB,
  IOPS, phiên bản PHP được phép, tính năng được bật). Bảng `tenant_quota_usage` cập nhật định kỳ.
  Cưỡng chế ở ba tầng: (a) middleware chặn khi tạo tài nguyên vượt hạn mức;
  (b) `quota` hệ thống trên filesystem cho disk; (c) cgroup v2 hoặc `LimitNPROC`/`MemoryMax`
  trong systemd unit cho CPU/RAM. Hành vi khi vượt hạn mức phải cấu hình được:
  cảnh báo, khoá ghi, hoặc treo dịch vụ.
- **Giá trị kinh doanh:** Rất cao. Không có package/quota thì không có sản phẩm hosting - chỉ có
  công cụ quản trị. Đây là thứ biến panel thành nguồn doanh thu.
- **Độ khó:** Cao.
- **Phụ thuộc:** Cần agent đáng tin cậy (P0-1) để đo lường và cưỡng chế trên máy đích.

#### P0-3. Trình cài đặt một lệnh trên VPS khách hàng

- **Nội dung:** Nâng cấp `deploy/install.sh` thành một trình cài đặt sản xuất: phát hiện distro
  (Ubuntu 22.04/24.04, Debian 12, AlmaLinux/Rocky 8/9), kiểm tra điều kiện tiên quyết
  (RAM, disk, cổng đang bận, SELinux), cài đặt idempotent (chạy lại không hỏng),
  sinh mật khẩu và secret ngẫu nhiên thay vì `admin123`, tự cấp SSL cho chính panel,
  ghi nhật ký cài đặt đầy đủ, và có `deploy/uninstall.sh` sạch. Bổ sung chế độ không tương tác
  cho tự động hoá. Có kịch bản nâng cấp phiên bản với migration DB an toàn.
- **Giá trị kinh doanh:** Rất cao. Cài đặt là ấn tượng đầu tiên. cPanel mất một giờ và hay hỏng;
  nếu VKAI cài xong trong 5 phút và chạy được, đó là một luận điểm bán hàng tự nó.
  Đồng thời giảm mạnh chi phí hỗ trợ trước bán hàng.
- **Độ khó:** Trung bình.
- **Phụ thuộc:** Không.

#### P0-4. Sao lưu ra S3/Google Drive + khôi phục một chạm + kiểm thử khôi phục

- **Nội dung:** Ba phần tách biệt.
  (a) *Đích lưu trữ từ xa*: S3 tương thích (AWS S3, VNG Cloud, Bizfly, Wasabi, MinIO), Google Drive,
  FTP/SFTP. Mã hoá phía client trước khi tải lên (AES-256-GCM, khoá lưu trong keystore riêng).
  Sao lưu tăng dần cho website lớn.
  (b) *Khôi phục một chạm*: chọn bản sao lưu, xem trước nội dung, khôi phục toàn bộ hoặc từng phần
  (chỉ CSDL, chỉ thư mục, chỉ một file). Luôn tạo bản chụp trước khi ghi đè để có đường lùi.
  (c) *Kiểm thử khôi phục tự động*: hàng tháng, chọn ngẫu nhiên một bản sao lưu, khôi phục vào
  môi trường tạm cách ly, kiểm tra tính toàn vẹn (checksum, `mysqlcheck`, HTTP 200 trang chủ),
  ghi kết quả vào báo cáo. Đây là điểm bán hàng: "chúng tôi chứng minh bản sao lưu khôi phục được".
- **Giá trị kinh doanh:** Rất cao và mang tính sống còn. Mất dữ liệu khách hàng là sự cố duy nhất
  có thể chấm dứt một doanh nghiệp hosting. Sao lưu ngoài máy chủ cũng là yêu cầu của mọi
  hợp đồng doanh nghiệp.
- **Độ khó:** Cao.
- **Phụ thuộc:** P0-1 (job queue thật), hạ tầng mã hoá (mục 4.6).

#### P0-5. Giám sát và cảnh báo thật (Telegram/Zalo/Email)

- **Nội dung:** Hoàn thiện bộ gửi cho `notification_channels` đã có sẵn trong DB:
  SMTP (email), Telegram Bot API, Zalo Official Account API, webhook chung, và tuỳ chọn SMS.
  Bộ quy tắc cảnh báo: CPU, RAM, disk, load, dịch vụ chết, website trả mã lỗi, chứng chỉ SSL
  sắp hết hạn, sao lưu thất bại, cảnh báo Tamper Proof. Kèm chống nhiễu: gom nhóm (deduplication),
  cửa sổ im lặng (silence window), leo thang (escalation) khi không ai xác nhận.
- **Giá trị kinh doanh:** Cao. Biết trước sự cố trước khi khách hàng gọi điện là khác biệt giữa
  nhà cung cấp chuyên nghiệp và nghiệp dư. Zalo đặc biệt quan trọng ở Việt Nam - đối thủ
  quốc tế không có, và đây là chi tiết khiến khách hàng Việt cảm thấy sản phẩm hiểu họ.
- **Độ khó:** Trung bình.
- **Phụ thuộc:** P0-1 (job queue để gửi bất đồng bộ).

#### P0-6. Trình cài WordPress thật + WP-CLI + Staging

- **Nội dung:**
  (a) *Cài đặt thật*: tải WordPress, tạo CSDL và người dùng, sinh `wp-config.php` với salt ngẫu nhiên,
  chạy `wp core install`, đặt quyền file đúng, cấp SSL, cấu hình vhost. Thay thế `service/wordpress.go`
  hiện chỉ ghi metadata.
  (b) *WP-CLI làm nền*: mọi thao tác plugin/theme/core/user/db đi qua WP-CLI chạy dưới
  người dùng của site (không phải root), có timeout và giới hạn tài nguyên.
  (c) *Staging*: nhân bản site sang subdomain `staging.<domain>`, tìm-thay URL trong CSDL bằng
  `wp search-replace` (an toàn với dữ liệu serialize), chặn index bởi công cụ tìm kiếm,
  và **đẩy ngược lên production có chọn lọc** (chỉ file, chỉ DB, hoặc cả hai) kèm sao lưu tự động.
  (d) Bổ sung: khoá lõi WordPress (chống ghi vào `wp-includes`), tự động cập nhật bảo mật,
  quét plugin có lỗ hổng đã biết.
- **Giá trị kinh doanh:** Rất cao. Phần lớn thị trường hosting chia sẻ Việt Nam là WordPress.
  Staging là tính năng khách hàng sẵn sàng trả thêm tiền và là lý do phổ biến để rời cPanel.
- **Độ khó:** Cao.
- **Phụ thuộc:** P0-1, quản lý PHP thật (P0-7), file manager (đã có).

#### P0-7. Quản lý PHP đa phiên bản thật

- **Nội dung:** Biến `service/php.go` từ CRUD thành điều khiển thật: cài/gỡ PHP 7.4-8.4 từ kho
  (Ondrej cho Debian/Ubuntu, Remi cho RHEL), sinh file pool FPM thật vào
  `/etc/php/<ver>/fpm/pool.d/`, đổi phiên bản theo từng website (cập nhật vhost + reload),
  quản lý extension qua `pecl`/gói hệ thống, chỉnh `php.ini` theo pool với kiểm tra hợp lệ,
  hiển thị `phpinfo` an toàn, tách người dùng FPM theo từng site.
- **Giá trị kinh doanh:** Cao. Đây là tính năng được dùng hàng ngày và là mục so sánh trực tiếp
  với mọi đối thủ. Thiếu nó, panel không dùng được cho hosting chia sẻ.
- **Độ khó:** Trung bình đến Cao.
- **Phụ thuộc:** P0-1, adapter web server (đã có nginx).

#### P0-8. 2FA/TOTP + mã dự phòng

- **Nội dung:** Xem mục 4.1. Ghi ở đây vì đây là tính năng P0 không thể lùi.
- **Giá trị kinh doanh:** Rất cao. Panel không có 2FA sẽ bị loại ngay trong bất kỳ vòng đánh giá
  bảo mật nào của khách hàng doanh nghiệp.
- **Độ khó:** Thấp đến Trung bình.
- **Phụ thuộc:** Không (trường `MFAEnabled`/`MFASecret` đã có trong `models.User`).

#### P0-9. Bảo mật kênh panel - agent (mTLS + xoay khoá)

- **Nội dung:** Xem mục 4.8. Đây là lỗ hổng kiến trúc nghiêm trọng nhất hiện tại.
- **Giá trị kinh doanh:** Rất cao ở dạng rủi ro tránh được. Một sự cố ở đây là sự cố toàn hệ thống.
- **Độ khó:** Trung bình đến Cao.
- **Phụ thuộc:** Không.

#### P0-10. i18n tiếng Việt/tiếng Anh

- **Nội dung:** Đưa toàn bộ chuỗi UI ra file tài nguyên (`next-intl` hoặc giải pháp tự viết nhẹ
  dựa trên React Context - tránh thêm dependency nếu có thể). Hai ngôn ngữ: `vi` mặc định, `en`.
  Bao gồm: chuỗi UI, thông báo lỗi từ backend (mã lỗi + bảng tra cứu, không dịch ở backend),
  định dạng ngày/giờ/số theo `vi-VN`, múi giờ `Asia/Ho_Chi_Minh`, và tài liệu người dùng tiếng Việt.
  Chọn ngôn ngữ theo từng người dùng, lưu trong hồ sơ.
- **Giá trị kinh doanh:** Cao và mang tính chiến lược. Đây là lý do tồn tại của sản phẩm.
  Một panel Việt Nam chỉ có tiếng Anh không có lý do để khách chọn thay vì DirectAdmin.
- **Độ khó:** Trung bình (khối lượng lớn nhưng kỹ thuật đơn giản; chi phí tăng theo cấp số nếu
  làm muộn - mỗi màn hình mới sẽ phải sửa lại).
- **Phụ thuộc:** Không. **Nên làm càng sớm càng tốt vì chi phí trì hoãn cao.**

#### P0-11. Nhật ký kiểm toán bất biến

- **Nội dung:** Xem mục 4.11.
- **Giá trị kinh doanh:** Cao. Yêu cầu bắt buộc cho khách hàng có nghĩa vụ tuân thủ, và là công cụ
  tự bảo vệ khi có tranh chấp với khách hàng ("ai đã xoá website này?").
- **Độ khó:** Trung bình.
- **Phụ thuộc:** Bảng `audit_logs` đã có.

---

### ĐỢT P1 - Cạnh tranh ngang hàng

Sau P0, sản phẩm bán được. P1 làm cho nó đủ tốt để **thắng khi so sánh trực tiếp** với đối thủ.

#### P1-1. Đại lý và phân cấp tài khoản (Reseller)

- **Nội dung:** Chuyển `tenants` thành cây có `parent_tenant_id` và độ sâu tối đa (khuyến nghị 3 cấp:
  nhà cung cấp - đại lý - khách hàng cuối). Đại lý được cấp một khối hạn mức tổng và tự chia nhỏ
  cho khách của mình, không bao giờ vượt được hạn mức cha (kiểm tra ở tầng service, không chỉ UI).
  Đại lý tạo gói riêng, đặt giá riêng, gắn thương hiệu riêng (logo, màu chủ đạo, tên miền panel,
  email gửi đi) - white label. Đại lý xem được tài nguyên của khách nhưng **không** đăng nhập
  vào panel khách nếu không có cơ chế mạo danh có ghi log rõ ràng.
- **Giá trị kinh doanh:** Rất cao. Reseller là kênh phân phối tự nhân bản: một đại lý mang về
  hàng chục khách hàng cuối mà doanh nghiệp không phải bán trực tiếp. Đây cũng là lý do
  DirectAdmin sống được nhiều năm.
- **Độ khó:** Cao. Ảnh hưởng tới mọi truy vấn có `tenant_id`.
- **Phụ thuộc:** P0-2 (package/quota) là điều kiện tiên quyết tuyệt đối.

#### P1-2. Di trú từ cPanel/DirectAdmin

- **Nội dung:** Bộ nhập liệu đọc được:
  (a) file cPanel backup (`cpmove-*.tar.gz`) - phân tích `homedir`, `mysql.sql`, `dnszones`,
  `userdata`, `va/` (email), `cron`, `ssl`;
  (b) DirectAdmin backup (`user.admin.<username>.tar.gz`);
  (c) di trú trực tiếp qua SSH từ máy chủ nguồn còn sống, có chế độ đồng bộ trước rồi
  chuyển đổi sau (pre-sync + final sync) để giảm downtime xuống dưới 5 phút.
  Ánh xạ được: website, tài liệu gốc, CSDL và người dùng, bản ghi DNS, tài khoản email và thư,
  chứng chỉ SSL, cron, tài khoản FTP. Có báo cáo kiểm tra sau di trú và chế độ chạy thử (dry-run).
- **Giá trị kinh doanh:** Rất cao. Đây là **rào cản chuyển đổi số một** của toàn thị trường.
  Khách hàng ghét cPanel nhưng ở lại vì sợ di chuyển. Ai gỡ được nút thắt này sẽ lấy được thị phần.
  Đồng thời đây là công cụ bán hàng trực tiếp: đội kinh doanh có thể chào "chúng tôi chuyển giúp miễn phí".
- **Độ khó:** Rất cao. Định dạng cPanel không có tài liệu chính thức đầy đủ, nhiều biến thể theo phiên bản.
- **Phụ thuộc:** P0-2, P0-6, P0-7, quản lý email (P1-4).

#### P1-3. API công khai + Webhook

- **Nội dung:**
  (a) *API*: đặc tả OpenAPI 3.1 đầy đủ cho toàn bộ `/api/v1`, đánh phiên bản rõ ràng, phân trang
  nhất quán, mã lỗi có cấu trúc, giới hạn tần suất theo khoá, idempotency key cho thao tác ghi.
  Sinh SDK cho PHP (cho WHMCS), Python, JavaScript. Trang tài liệu tương tác.
  (b) *Webhook outbound*: đăng ký endpoint theo sự kiện (`website.created`, `backup.failed`,
  `quota.exceeded`, `ssl.expiring`, `security.alert`...), ký payload bằng HMAC-SHA256,
  thử lại theo cấp số nhân, hàng đợi thư chết, nhật ký gửi để gỡ lỗi.
- **Giá trị kinh doanh:** Cao. API là điều kiện cần cho tự động hoá bán hàng và cho khách hàng
  doanh nghiệp muốn tích hợp vào quy trình DevOps của họ. Không có API tốt thì mọi tích hợp
  hoá đơn, CRM, hệ thống nội bộ đều bất khả thi.
- **Độ khó:** Trung bình đến Cao.
- **Phụ thuộc:** P0-1. Nên làm trước P1-5.

#### P1-4. Email doanh nghiệp hoàn chỉnh

- **Nội dung:** Hiện đã có mô hình dữ liệu tốt (`models/mail_server.go` với `SPFRecord`,
  `DKIMEnabled`, `DMARCRecord`, `MailDKIMKey`, `MailQueueItem`, `MailSpamFilter`).
  Cần phần thực thi:
  (a) Postfix + Dovecot cấu hình tự động theo domain, xác thực qua PostgreSQL;
  (b) DKIM thật: sinh cặp khoá 2048-bit, cấu hình OpenDKIM, **tự tạo bản ghi DNS** trong module
  DNS đã có, và nút kiểm tra xác minh SPF/DKIM/DMARC có hoạt động thật không;
  (c) Chống spam: Rspamd (khuyến nghị hơn SpamAssassin - nhanh hơn, cấu hình tốt hơn),
  greylisting, RBL, học Bayes theo từng người dùng, thư mục cách ly (quarantine) để người dùng tự xem lại;
  (d) Chống *gửi* spam - quan trọng không kém: giới hạn tần suất gửi theo tài khoản,
  phát hiện tài khoản bị chiếm dụng, cảnh báo khi hàng đợi tăng đột biến. Một máy chủ bị lọt
  vào danh sách đen sẽ làm hỏng email của mọi khách hàng trên đó;
  (e) Webmail (Roundcube hoặc SnappyMail), IMAP/SMTP có TLS bắt buộc, autodiscover cho Outlook/Apple Mail;
  (f) Trang chẩn đoán khả năng gửi thư: kiểm tra reverse DNS, danh sách đen, điểm DMARC.
- **Giá trị kinh doanh:** Cao. Email là tính năng gây khó chịu nhất cho khách hàng khi hỏng và là
  nguyên nhân hàng đầu của ticket hỗ trợ. Làm tốt sẽ giảm chi phí vận hành đáng kể.
- **Độ khó:** Rất cao. Email là lĩnh vực có nhiều cạm bẫy vận hành nhất.
- **Phụ thuộc:** Module DNS (đã có), P0-5 (cảnh báo).

#### P1-5. Hoá đơn và thanh toán (WHMCS + API riêng)

- **Nội dung:** Hai đường song song.
  (a) *Module WHMCS*: viết provisioning module PHP chuẩn (`CreateAccount`, `SuspendAccount`,
  `UnsuspendAccount`, `TerminateAccount`, `ChangePackage`, `ChangePassword`, `ClientArea` SSO).
  WHMCS là chuẩn thực tế của ngành hosting; có module WHMCS đồng nghĩa với việc panel
  cắm được vào quy trình bán hàng đã tồn tại của hàng nghìn nhà cung cấp.
  (b) *API cung cấp dịch vụ riêng*: cho khách hàng dùng hệ thống hoá đơn khác hoặc tự viết.
  Kèm tích hợp cổng thanh toán Việt Nam (VNPay, MoMo, chuyển khoản ngân hàng) cho mô hình
  bán trực tiếp; xử lý gia hạn, quá hạn, treo và thu hồi tự động theo chính sách cấu hình được.
- **Giá trị kinh doanh:** Rất cao. Đây là chỗ tiền thực sự chảy vào. Tự động hoá vòng đời
  đăng ký - thanh toán - cung cấp - treo - thu hồi cắt bỏ phần lớn lao động thủ công.
- **Độ khó:** Cao.
- **Phụ thuộc:** P0-2 (package), P1-1 (reseller), P1-3 (API).

#### P1-6. Node.js và Python App Manager

- **Nội dung:** Mở rộng `service/nodeapp.go` và `internal/nodeapp/systemd.go` đã có.
  (a) *Node.js*: quản lý nhiều phiên bản qua `nvm`/`fnm` theo từng ứng dụng, cài phụ thuộc
  (`npm ci`/`yarn`/`pnpm`), lệnh build, biến môi trường có mã hoá cho secret, systemd unit
  với tự khởi động lại, reverse proxy tự động, log gộp vào module log, khởi động lại không gián đoạn
  (zero-downtime) qua socket activation hoặc cụm tiến trình.
  (b) *Python*: virtualenv theo ứng dụng, nhiều phiên bản Python, WSGI/ASGI
  (Gunicorn/Uvicorn) sau Nginx, `requirements.txt`/`poetry`, quản lý migration cho Django/Flask.
  (c) Chung: giới hạn tài nguyên theo cgroup, kiểm tra sức khoẻ, chính sách khởi động lại,
  triển khai từ Git (nối vào `service/gitdeployment.go` đã có).
- **Giá trị kinh doanh:** Trung bình đến Cao. Mở ra phân khúc khách hàng lập trình viên và agency
  - phân khúc trả tiền cao hơn hosting chia sẻ và ít yêu cầu hỗ trợ hơn. cPanel làm mảng này rất kém,
  đây là cơ hội rõ ràng.
- **Độ khó:** Cao.
- **Phụ thuộc:** P0-1, P0-2 (giới hạn tài nguyên).

#### P1-7. Trạng thái dịch vụ và SLA

- **Nội dung:**
  (a) *Status page công khai* tại tên miền riêng: trạng thái từng dịch vụ (web, mail, DNS, CSDL,
  chính panel), sự cố đang diễn ra, lịch bảo trì, lịch sử 90 ngày, đăng ký nhận thông báo qua email/Zalo.
  Phải chạy trên hạ tầng **tách biệt** với hệ thống nó giám sát - nếu không, khi hệ thống sập thì
  status page cũng sập.
  (b) *Đo lường SLA*: tính uptime theo từng khách hàng và từng dịch vụ, báo cáo hàng tháng,
  tính tín dụng SLA tự động khi vi phạm cam kết.
  (c) *Quản lý sự cố*: quy trình khai báo sự cố, cập nhật tiến độ, báo cáo hậu sự cố (postmortem).
- **Giá trị kinh doanh:** Cao. SLA có số liệu chứng minh là điều kiện bắt buộc trong hợp đồng
  doanh nghiệp, và status page giảm mạnh lượng ticket "website tôi có vấn đề không?" trong sự cố.
  Đây cũng là tín hiệu trưởng thành mà khách hàng lớn đánh giá.
- **Độ khó:** Trung bình.
- **Phụ thuộc:** P0-5 (giám sát).

#### P1-8. Truy cập khẩn cấp khi mất kết nối

- **Nội dung:** Nhiều lớp phòng thủ khi panel không truy cập được:
  (a) *CLI cục bộ*: mở rộng `core/internal/cli/` thành công cụ vận hành đầy đủ chạy được
  qua SSH khi web UI chết - đặt lại mật khẩu admin, tắt 2FA có xác thực ngoài băng,
  gỡ khoá IP, khởi động lại dịch vụ, khôi phục cấu hình từ `service/config.go`;
  (b) *Chế độ cứu hộ*: panel khởi động ở chế độ tối giản chỉ với chức năng chẩn đoán khi
  phát hiện lỗi khởi động;
  (c) *Cổng khẩn cấp*: cổng riêng, chỉ nghe trên localhost, dùng token dùng một lần có thời hạn
  do quản trị viên sinh sẵn và cất giữ ngoài hệ thống;
  (d) *Tự phục hồi*: watchdog systemd, tự khôi phục cấu hình nginx sai cú pháp,
  tự khởi động lại dịch vụ chết theo chính sách;
  (e) *Không bao giờ tự khoá mình*: mọi thao tác firewall thay đổi quy tắc phải có cơ chế
  tự động lùi lại sau N phút nếu quản trị viên không xác nhận vẫn kết nối được.
- **Giá trị kinh doanh:** Cao ở dạng rủi ro tránh được. Một quản trị viên tự khoá mình khỏi
  máy chủ sản xuất lúc 2 giờ sáng là sự cố tốn kém và hoàn toàn có thể phòng ngừa.
  Đây cũng là chi tiết mà kỹ sư đánh giá sản phẩm sẽ chú ý.
- **Độ khó:** Trung bình.
- **Phụ thuộc:** `internal/cli/` đã có, `service/config.go` (rollback) đã có.

#### P1-9. CDN, cache và tăng tốc

- **Nội dung:**
  (a) *Redis object cache* cho WordPress: cài, cấu hình, gắn plugin, cách ly theo site
  (mỗi site một database hoặc một prefix, tránh rò rỉ dữ liệu giữa khách hàng);
  (b) *LSCache*: khi dùng OpenLiteSpeed/LiteSpeed (adapter đã có trong `webserver/`),
  bật cache trang, cấu hình quy tắc, xoá cache theo site;
  (c) *FastCGI cache cho Nginx*: cấu hình sẵn cho WordPress với quy tắc bỏ qua khi đăng nhập/giỏ hàng;
  (d) *OPcache* theo pool PHP với thống kê;
  (e) *Brotli/Gzip*, HTTP/2, HTTP/3 (QUIC), tối ưu ảnh (WebP/AVIF tự động);
  (f) *Tích hợp CDN*: Cloudflare API (xoá cache, chế độ phát triển, cấu hình DNS),
  và CDN Việt Nam (VNPT, Viettel, BizFly) cho khách hàng cần độ trễ thấp trong nước.
- **Giá trị kinh doanh:** Cao. Tốc độ website là chỉ số khách hàng cảm nhận được ngay và
  đo được bằng công cụ công khai. "Hosting nhanh" là luận điểm bán hàng dễ chứng minh nhất.
- **Độ khó:** Trung bình đến Cao.
- **Phụ thuộc:** P0-6 (WordPress), P0-7 (PHP).

#### P1-10. Quét mã độc website

- **Nội dung:** Xem mục 4.12.
- **Giá trị kinh doanh:** Cao. Website WordPress bị nhiễm mã độc là loại ticket phổ biến nhất,
  tốn thời gian nhất và dễ khiến IP máy chủ bị đưa vào danh sách đen nhất.
- **Độ khó:** Trung bình đến Cao.
- **Phụ thuộc:** P0-5 (cảnh báo), Tamper Proof (đã có).

---

### ĐỢT P2 - Khác biệt hoá

P2 là những thứ đối thủ không có hoặc làm kém, tạo lý do để khách hàng chọn VKAI thay vì
chỉ chấp nhận VKAI vì rẻ hơn.

#### P2-1. Chế độ nhiều máy chủ trưởng thành (Multi-node)

- **Nội dung:** Nâng `models/cluster.go` (hiện là CRUD) thành năng lực vận hành thật:
  (a) *Panel điều khiển tập trung, nhiều máy chủ được quản lý* - một giao diện, N máy chủ,
  triển khai website lên máy chủ được chọn hoặc tự động theo tải;
  (b) *Nhóm máy chủ theo vai trò*: web, database, mail, cache, storage - tách vai trò để
  mở rộng độc lập;
  (c) *Di chuyển website giữa các máy chủ* khi máy chủ quá tải, có giai đoạn đồng bộ trước;
  (d) *Lưu trữ chia sẻ* (NFS/GlusterFS/CephFS) cho tài liệu gốc, hoặc đồng bộ nếu không có SAN;
  (e) *DNS và mail phân tán*, CSDL sao chép chủ-tớ có chuyển đổi dự phòng;
  (f) *Đặt lịch nhận biết tài nguyên*: đặt khách hàng mới lên máy chủ còn dư địa.
- **Giá trị kinh doanh:** Rất cao và là **trục khác biệt số một**. cPanel/DirectAdmin/CyberPanel
  về bản chất là panel một máy chủ. Nhà cung cấp có 50 máy chủ hiện phải mở 50 tab.
  VKAI đã có nền kiến trúc đúng, chỉ cần hoàn thiện. Đây là thứ biện minh cho giá cao hơn.
- **Độ khó:** Rất cao.
- **Phụ thuộc:** P0-1, P0-9 (mTLS), P0-2 (quota), P1-1 (reseller).

#### P2-2. Cân bằng tải và sẵn sàng cao

- **Nội dung:** Hoàn thiện `LoadBalancer` và `HAPair` trong `models/cluster.go`:
  cấu hình HAProxy/Nginx làm bộ cân bằng tải, kiểm tra sức khoẻ backend, thuật toán phân phối
  (round-robin, least-conn, IP hash cho phiên dính), rút máy chủ ra khỏi cụm mà không rớt kết nối,
  keepalived/VRRP cho IP nổi, tự động chuyển đổi dự phòng có kiểm soát chống split-brain,
  và tuỳ chọn tự mở rộng khi tích hợp API của nhà cung cấp đám mây.
- **Giá trị kinh doanh:** Cao cho phân khúc doanh nghiệp và thương mại điện tử - phân khúc
  có giá trị hợp đồng cao nhất. Cũng là điều kiện để cam kết SLA 99.9% trở lên.
- **Độ khó:** Rất cao.
- **Phụ thuộc:** P2-1.

#### P2-3. Marketplace ứng dụng và hệ thống plugin

- **Nội dung:** Kiến trúc module cho phép bên thứ ba mở rộng panel: manifest plugin, API ổn định,
  sandbox thực thi, ký số plugin, và bộ cài ứng dụng một chạm cho nhóm ứng dụng phổ biến ở
  Việt Nam (WordPress, WooCommerce, Laravel, Magento, Odoo, n8n, Nextcloud, Moodle, phpBB).
  Không cạnh tranh về số lượng với Softaculous; cạnh tranh về chất lượng cấu hình sau khi cài
  (SSL, cache, sao lưu, bảo mật đã được thiết lập đúng).
- **Giá trị kinh doanh:** Trung bình đến Cao. Mở rộng hệ sinh thái và cho phép đối tác đóng góp
  thay vì mọi thứ phải do nội bộ làm.
- **Độ khó:** Rất cao.
- **Phụ thuộc:** P1-3 (API), mô hình bảo mật sandbox (mục 4.14).

#### P2-4. Trợ lý vận hành thông minh

- **Nội dung:** Dựa trên dữ liệu đã thu thập được (`monitoring_metrics`, `website_stats`,
  `audit_logs`, `waf_events`, `tamper_alerts`):
  phát hiện bất thường (tăng đột biến CPU, lưu lượng lạ, mẫu tấn công), dự báo cạn disk
  và cạn hạn mức trước 30 ngày, gợi ý tối ưu cụ thể ("site X nên bật cache, giảm 60% thời gian tải"),
  phân tích nguyên nhân gốc khi có sự cố, và trợ lý hỏi-đáp bằng tiếng Việt trên tài liệu và trạng thái hệ thống.
- **Giá trị kinh doanh:** Cao ở khía cạnh khác biệt hoá và giảm tải cho đội hỗ trợ.
  Không đối thủ nào trong nhóm so sánh làm điều này bằng tiếng Việt.
- **Độ khó:** Cao.
- **Phụ thuộc:** Dữ liệu giám sát chất lượng (P0-5), API (P1-3).

#### P2-5. Ứng dụng di động và cảnh báo đẩy

- **Nội dung:** Ứng dụng iOS/Android tối giản: xem trạng thái máy chủ, nhận cảnh báo đẩy,
  thao tác khẩn cấp (khởi động lại dịch vụ, khôi phục sao lưu, chặn IP), xác thực sinh trắc học,
  và duyệt yêu cầu 2FA. Không nhân bản toàn bộ panel - chỉ những việc cần làm ngay khi
  quản trị viên không ở bàn làm việc.
- **Giá trị kinh doanh:** Trung bình. Giá trị cảm nhận cao, chi phí vừa phải, tạo ấn tượng
  sản phẩm hiện đại. Là điểm cộng trong so sánh nhưng hiếm khi là lý do quyết định mua.
- **Độ khó:** Cao.
- **Phụ thuộc:** P1-3 (API), P0-5 (cảnh báo).

#### P2-6. Báo cáo tuân thủ và chứng nhận

- **Nội dung:** Bộ báo cáo phục vụ kiểm toán: truy vết truy cập dữ liệu cá nhân theo
  Nghị định 13/2023/NĐ-CP về bảo vệ dữ liệu cá nhân, báo cáo lưu trữ dữ liệu trong nước
  theo Luật An ninh mạng, sổ đăng ký xử lý dữ liệu, quy trình phản hồi yêu cầu của chủ thể dữ liệu,
  và bộ chứng cứ chuẩn bị cho ISO 27001. Xuất báo cáo theo kỳ, đóng dấu thời gian, không sửa được.
- **Giá trị kinh doanh:** Cao đối với phân khúc doanh nghiệp lớn, ngân hàng, cơ quan nhà nước -
  phân khúc mà đối thủ nước ngoài gặp bất lợi cấu trúc. Đây là nơi lợi thế "panel Việt Nam"
  chuyển thành hợp đồng thật.
- **Độ khó:** Trung bình (kỹ thuật) nhưng cần đầu tư nghiên cứu pháp lý.
- **Phụ thuộc:** P0-11 (nhật ký kiểm toán bất biến).

---

## 4. Lộ trình bảo mật nâng cao

Nguyên tắc bao trùm: **panel là mục tiêu tấn công có giá trị cao nhất trong toàn hạ tầng.**
Chiếm được panel là chiếm được mọi máy chủ nó quản lý. Mọi quyết định bảo mật phải xuất phát
từ giả định này.

### 4.1. 2FA/TOTP và mã dự phòng (P0)

- **Hiện trạng:** `models.User` có `MFAEnabled bool` và `MFASecret *string` nhưng không có
  mã cài đặt TOTP nào trong repo.
- **Cần làm:** TOTP theo RFC 6238 (SHA-1, 6 chữ số, chu kỳ 30 giây, cửa sổ trượt ±1 để bù lệch đồng hồ).
  Sinh mã QR `otpauth://` cho Google Authenticator/Authy. **10 mã dự phòng dùng một lần**,
  lưu dưới dạng băm bcrypt, hiển thị đúng một lần khi bật, đếm số mã còn lại và cảnh báo khi cạn.
  `MFASecret` phải được mã hoá trong DB (mục 4.6), không lưu dạng thô. Bắt buộc 2FA cho vai trò
  `super_admin` và `admin` (`rbac.go`), cho phép tenant tự đặt chính sách bắt buộc.
  Chống dò mã: khoá sau 5 lần sai. Có quy trình khôi phục khi mất cả thiết bị lẫn mã dự phòng
  (yêu cầu xác minh ngoài băng, ghi log ở mức nghiêm trọng cao nhất).
- **Nâng cấp về sau:** WebAuthn/Passkey - chống lừa đảo tốt hơn TOTP và không cần nhập mã.
- **Độ khó:** Thấp đến Trung bình. **Phụ thuộc:** Không.

### 4.2. Khoá phiên theo IP và thiết bị (P0-P1)

- **Hiện trạng:** `models/multi_user.go` đã có `UserSession` với `IPAddress`, `UserAgent`,
  `ExpiresAt`, `LastActiveAt` - hạ tầng đã sẵn, chưa được cưỡng chế.
- **Cần làm:** Gắn phiên với dấu vân tay thiết bị (User-Agent + đặc điểm ổn định của trình duyệt).
  Chính sách IP cấu hình được ở ba mức: khoá cứng theo IP, khoá theo dải/24, hoặc chỉ cảnh báo khi đổi.
  Cảnh báo và yêu cầu xác thực lại khi phiên chuyển sang IP khác quốc gia hoặc khác ASN.
  Màn hình "thiết bị đang đăng nhập" cho phép người dùng tự thu hồi từng phiên,
  và cho phép quản trị viên thu hồi mọi phiên của một tài khoản ngay lập tức.
  Đăng xuất toàn cục khi đổi mật khẩu. Danh sách IP cho phép ở cấp tài khoản
  (đặc biệt cho tài khoản dịch vụ và API key).
- **Độ khó:** Trung bình. **Phụ thuộc:** Bảng `user_sessions` đã có.

### 4.3. SSO/OIDC/LDAP cho doanh nghiệp (P1)

- **Cần làm:** OIDC (Google Workspace, Microsoft Entra ID, Keycloak, Authentik) và SAML 2.0
  cho khách hàng lớn; LDAP/Active Directory cho doanh nghiệp truyền thống Việt Nam.
  Ánh xạ nhóm từ IdP sang vai trò trong `rbac.go`. Tự động cấp và **tự động thu hồi** tài khoản
  khi nhân viên rời tổ chức (SCIM nếu IdP hỗ trợ) - đây mới là giá trị thật của SSO.
  Bắt buộc SSO theo tenant (chặn đăng nhập bằng mật khẩu cục bộ), nhưng luôn giữ
  một tài khoản dự phòng có 2FA để không mất quyền khi IdP hỏng.
- **Giá trị kinh doanh:** Đây thường là yêu cầu chặn cửa (blocker) trong đấu thầu doanh nghiệp.
- **Độ khó:** Cao. **Phụ thuộc:** 4.1, RBAC ổn định (4.4).

### 4.4. RBAC chi tiết theo tài nguyên (P1)

- **Hiện trạng:** `rbac.go` có 8 vai trò và khoảng 19 quyền dạng `resource.action`, kiểm tra ở
  cấp loại tài nguyên. Nghĩa là ai có `website.write` thì sửa được **mọi** website trong tenant.
- **Cần làm:** Thêm chiều tài nguyên: `(subject, action, resource_type, resource_id | scope)`.
  Hỗ trợ phạm vi theo máy chủ, theo nhóm website, theo môi trường (production/staging),
  và theo nhãn. Vai trò tuỳ chỉnh do khách hàng tự định nghĩa (bảng `roles` đã có `tenant_id`
  và `is_system` - nền tảng đã đúng). Quyền hiệu lực = hợp của các vai trò, với quy tắc từ chối
  luôn thắng. Bổ sung: phê duyệt hai người cho thao tác huỷ diệt (xoá website, xoá CSDL,
  xoá tenant), cửa sổ thời gian cho quyền tạm (nâng quyền có thời hạn), và
  màn hình "người này thực sự làm được gì" để kiểm tra quyền hiệu lực.
  Cache kết quả kiểm tra quyền trong Redis với vô hiệu hoá chính xác khi vai trò thay đổi.
- **Độ khó:** Cao. **Phụ thuộc:** Không, nhưng nên làm trước P1-1 (reseller).

### 4.5. Khoá API scoped và xoay khoá (P0-P1)

- **Hiện trạng:** `service/apikey.go` đã làm đúng nhiều điểm: sinh khoá ngẫu nhiên, băm SHA-256,
  chỉ lưu băm, lưu `KeyPrefix` 12 ký tự để tra cứu, có `Scopes`.
- **Cần cải thiện:**
  (a) Chuyển từ SHA-256 sang HMAC-SHA-256 với khoá bí mật phía máy chủ, để một bản sao DB rò rỉ
  cũng không cho phép dò khoá ngoại tuyến;
  (b) Cưỡng chế scope thật ở middleware - hiện `ValidateKey` xác thực khoá nhưng chưa thấy
  middleware nào áp scope lên các nhóm route trong `router.go`;
  (c) Thêm hạn dùng bắt buộc (mặc định 90 ngày) và cảnh báo trước khi hết hạn;
  (d) **Xoay khoá không gián đoạn**: cho phép hai khoá cùng hiệu lực trong cửa sổ chuyển đổi;
  (e) Giới hạn IP nguồn theo từng khoá;
  (f) Giới hạn tần suất theo từng khoá;
  (g) Ghi lần dùng cuối và tự động vô hiệu hoá khoá không dùng quá 90 ngày;
  (h) Quét bí mật trong CI để phát hiện khoá bị commit nhầm.
- **Độ khó:** Trung bình. **Phụ thuộc:** 4.4 để scope có ý nghĩa đầy đủ.

### 4.6. Mã hoá dữ liệu nhạy cảm trong CSDL (P0)

- **Hiện trạng:** Đây là vấn đề nghiêm trọng. Nhiều bí mật đang nằm dạng thô trong PostgreSQL:
  `WordPressSite.DBPassword` và `AdminPassword` (`models/wordpress.go`),
  `MailDKIMKey.PrivateKey` (`models/mail_server.go`), `GitDeployment.DeployKey` và `WebhookSecret`,
  `User.MFASecret`, cấu hình `NotificationChannel.Config` (chứa token bot),
  và thông tin đăng nhập máy chủ. Việc đánh dấu `json:"-"` chỉ ẩn khỏi API, **không** bảo vệ dữ liệu.
- **Cần làm:** Mã hoá cấp trường bằng AES-256-GCM với khoá dữ liệu riêng cho từng tenant,
  các khoá dữ liệu này lại được bọc bởi một khoá gốc (envelope encryption). Khoá gốc lưu ngoài
  CSDL: biến môi trường ở mức tối thiểu, HashiCorp Vault hoặc KMS của nhà cung cấp đám mây
  ở mức chuẩn. Hỗ trợ xoay khoá có phiên bản (mỗi bản ghi ghi rõ đã mã hoá bằng khoá phiên bản nào).
  Kiểu Go tự mã hoá/giải mã qua `driver.Valuer`/`sql.Scanner` để lập trình viên không thể quên.
  Mật khẩu người dùng giữ nguyên bcrypt (đang đúng), nhưng nâng cost factor và cân nhắc Argon2id.
  Bổ sung TLS bắt buộc cho kết nối tới PostgreSQL và Redis, mã hoá đĩa ở tầng hệ điều hành.
- **Độ khó:** Cao. **Phụ thuộc:** Không. **Nên làm sớm** - chi phí di trú dữ liệu tăng theo thời gian.

### 4.7. Ký và xác minh gói agent (P1)

- **Cần làm:** Ký số mọi binary `vkaid` và gói cập nhật bằng khoá riêng cất trong HSM hoặc
  quy trình ký cách ly. Agent xác minh chữ ký trước khi tự cập nhật và **từ chối khởi động**
  nếu binary không khớp chữ ký. Kênh cập nhật có phiên bản, hỗ trợ lùi phiên bản,
  và chống tấn công hạ cấp (không chấp nhận phiên bản cũ hơn phiên bản đang chạy).
  Công bố SBOM và checksum công khai. Xây dựng có thể tái lập (reproducible build) trong CI
  (`.github/workflows/`) để bên thứ ba xác minh được binary khớp mã nguồn.
- **Giá trị kinh doanh:** Chuỗi cung ứng phần mềm là vector tấn công đang tăng nhanh nhất.
  Đây cũng là câu hỏi mà mọi đội bảo mật doanh nghiệp sẽ hỏi.
- **Độ khó:** Trung bình đến Cao. **Phụ thuộc:** Hạ tầng CI đã có.

### 4.8. mTLS giữa panel và agent (P0 - ưu tiên cao nhất)

- **Hiện trạng - rủi ro nghiêm trọng:** `agent/cmd/main.go` dùng một chuỗi tĩnh
  `VKAI_AGENT_TOKEN` gửi trong header `X-Agent-Token` (dòng 156), agent mở HTTP server
  ở `/execute` và chỉ so sánh chuỗi token (dòng 294-296). Không TLS, so sánh chuỗi không
  chống tấn công phân tích thời gian, không xoay khoá, cùng một token cho mọi máy chủ.
  Phía panel, `/api/v1/agent/heartbeat` và `/register` (`handler/router.go` dòng 908-916)
  là hàm rỗng. Bất kỳ ai đọc được token - từ file cấu hình, biến môi trường, log,
  hay nghe lén mạng - đều thực thi được lệnh tuỳ ý trên mọi máy chủ.
- **Cần làm:**
  (a) CA nội bộ do panel vận hành, mỗi agent có chứng chỉ khách riêng, xác thực TLS hai chiều;
  (b) Chứng chỉ ngắn hạn (24 giờ) tự động gia hạn, có danh sách thu hồi;
  (c) Ghim chứng chỉ (certificate pinning) hai chiều;
  (d) Quy trình đăng ký agent dùng token một lần có thời hạn ngắn, phải được quản trị viên
  chấp thuận thủ công, và ghi lại dấu vân tay chứng chỉ;
  (e) Bỏ mô hình `/execute` nhận lệnh tuỳ ý; thay bằng **danh sách thao tác được phép**
  có tham số kiểu chặt chẽ (xem 4.13);
  (f) Đảo chiều kết nối - agent chủ động kết nối ra panel (gRPC stream hoặc WebSocket)
  thay vì mở cổng nghe, để agent không cần mở cổng vào từ Internet;
  (g) So sánh bí mật bằng `crypto/subtle.ConstantTimeCompare`.
- **Độ khó:** Cao. **Phụ thuộc:** Không. **Đây là hạng mục bảo mật số một.**

### 4.9. Chống dò mật khẩu và tích hợp fail2ban (P0)

- **Hiện trạng:** `middleware.RateLimit()` không giới hạn gì (`c.Next()` trực tiếp).
  Không có `fail2ban` trong repo.
- **Cần làm:**
  (a) Giới hạn tần suất thật, nhiều tầng: theo IP, theo tài khoản, theo endpoint, theo tenant.
  Dùng token bucket lưu trong Redis (Redis đã có sẵn) để hiệu lực trên nhiều instance;
  (b) Trễ tăng dần và khoá tạm thời sau N lần đăng nhập sai, với thời gian khoá tăng theo cấp số nhân;
  (c) CAPTCHA sau ngưỡng thất bại;
  (d) Tích hợp fail2ban thật: panel sinh filter và jail cho log của chính nó, cho SSH, cho
  đăng nhập WordPress, cho SMTP/IMAP, cho FTP. Quản lý jail và IP bị cấm từ giao diện,
  kết nối với `service/firewall.go` đã có;
  (e) Danh sách IP bị cấm dùng chung giữa các máy chủ trong cụm - một kẻ tấn công bị chặn
  ở một máy chủ thì bị chặn ở tất cả;
  (f) Danh sách trắng luôn ưu tiên để không tự khoá quản trị viên;
  (g) Thông báo cho người dùng khi có đăng nhập bất thường vào tài khoản của họ.
- **Độ khó:** Trung bình. **Phụ thuộc:** Redis (đã có), `service/firewall.go` (đã có).

### 4.10. Nâng cấp Tamper Proof - phát hiện xâm nhập file (P1)

- **Hiện trạng - điểm mạnh cần khai thác:** `models/tamper_proof.go` và
  `migrations/018_create_tamper_proof_tables.sql` đã có kiến trúc tốt: `ProtectedPath`
  (đường dẫn, đệ quy, thuật toán, mẫu bỏ qua, cảnh báo theo loại thay đổi),
  `FileBaseline` (checksum, kích thước, quyền, chủ sở hữu, thời gian sửa),
  `TamperAlert` (phân loại và mức nghiêm trọng, quy trình xử lý),
  `TamperScanResult`, `TamperAuditLog`. Rất ít panel thương mại có tính năng này.
  Đây là tài sản khác biệt hoá cần được đầu tư thêm chứ không phải làm lại.
- **Hướng nâng cấp:**
  (a) *Thời gian thực thay vì quét định kỳ*: dùng `fanotify` (Linux) hoặc `inotify` để phát hiện
  thay đổi ngay lập tức thay vì chờ chu kỳ quét. Đây là bước nhảy về giá trị - phát hiện
  webshell trong vài giây thay vì vài giờ;
  (b) *Baseline chống sửa*: hiện baseline nằm trong PostgreSQL cùng nơi kẻ tấn công có thể
  chạm tới nếu chiếm được panel. Cần ký baseline bằng khoá không nằm trên máy chủ được giám sát,
  và đẩy bản sao baseline sang nơi lưu trữ chỉ-ghi-thêm;
  (c) *Giảm cảnh báo giả*: hiểu ngữ cảnh - cập nhật WordPress hợp lệ, `composer install`,
  `npm install`, triển khai Git từ chính panel phải tự động cập nhật baseline thay vì báo động;
  (d) *Phân loại thông minh*: một file `.php` mới trong `wp-content/uploads` là mức nghiêm trọng
  cao nhất; một file `.log` thay đổi là mức thấp nhất. Chấm điểm theo vị trí, phần mở rộng,
  nội dung và ngữ cảnh;
  (e) *Phản ứng tự động cấu hình được*: cách ly file, thu hồi quyền ghi, chuyển site sang
  chế độ chỉ đọc, chặn IP nguồn - kèm cơ chế hoàn tác;
  (f) *So sánh chéo giữa các máy chủ*: cùng một file lạ xuất hiện trên nhiều máy chủ
  là dấu hiệu chiến dịch tấn công, cần leo thang ngay;
  (g) *Giám sát cả những thứ ngoài file*: cron mới, người dùng hệ thống mới, khoá SSH mới,
  dịch vụ systemd mới, quy tắc firewall thay đổi - đây là nơi kẻ tấn công cắm chốt duy trì.
- **Độ khó:** Cao. **Phụ thuộc:** P0-5 (cảnh báo), agent đáng tin cậy (P0-1, P0-9).

### 4.11. Nhật ký kiểm toán chống sửa (P0)

- **Hiện trạng:** `models.AuditLog` là bảng PostgreSQL thường. Ai có quyền ghi CSDL - kể cả
  kẻ tấn công đã chiếm panel - đều có thể `DELETE` xoá sạch dấu vết.
- **Cần làm:**
  (a) *Chuỗi băm*: mỗi bản ghi chứa băm của bản ghi trước (`prev_hash`) tạo thành chuỗi
  không cắt được ở giữa mà không bị phát hiện. Thêm băm gốc (Merkle root) theo ngày,
  ký số và công bố;
  (b) *Chỉ ghi thêm*: thu hồi quyền `UPDATE`/`DELETE` trên bảng audit ở cấp PostgreSQL,
  kể cả với tài khoản ứng dụng; dùng trigger `BEFORE UPDATE OR DELETE` để từ chối;
  (c) *Bản sao ngoài*: đẩy đồng thời sang nơi lưu trữ chỉ-ghi-thêm bên ngoài
  (S3 Object Lock ở chế độ compliance, hoặc syslog tới máy chủ log riêng);
  (d) *Ghi đầy đủ*: hiện `AuditLog` chưa có giá trị trước/sau. Cần lưu ảnh chụp trước và
  sau mỗi thay đổi để trả lời được câu hỏi "cấu hình này trước đây là gì";
  (e) *Bao phủ toàn diện*: mọi thao tác ghi phải sinh audit log, kể cả thao tác từ CLI
  (`internal/cli/`) và từ API key, không chỉ từ web UI;
  (f) *Không thể tắt*: quản trị viên không được phép tắt ghi log; thao tác thử tắt cũng phải được ghi;
  (g) *Xác minh và xuất*: nút kiểm tra tính toàn vẹn chuỗi, và xuất báo cáo có chữ ký cho kiểm toán viên;
  (h) *Lưu giữ*: tối thiểu 12 tháng nóng, 7 năm lạnh cho khách hàng có nghĩa vụ tuân thủ.
- **Độ khó:** Trung bình. **Phụ thuộc:** Bảng `audit_logs` đã có, hạ tầng mã hoá (4.6).

### 4.12. Quét mã độc website (P1)

- **Cần làm:**
  (a) *Nhiều lớp*: ClamAV với bộ mẫu bổ sung, Linux Malware Detect (maldet) cho mẫu webshell,
  YARA rules cho phát hiện theo mẫu, cộng với heuristic riêng (phát hiện `eval(base64_decode(`,
  file PHP trong thư mục upload, file có entropy cao, mã bị làm rối);
  (b) *Quét theo lịch và theo sự kiện*: quét toàn bộ định kỳ, quét ngay khi Tamper Proof
  phát hiện file mới, quét khi tải file lên qua file manager;
  (c) *Chuyên biệt cho WordPress*: đối chiếu checksum lõi WordPress với bản chính thức,
  kiểm tra plugin/theme với cơ sở dữ liệu lỗ hổng (WPScan/Patchstack), phát hiện
  plugin nulled, phát hiện tài khoản admin lạ;
  (d) *Xử lý sau phát hiện*: cách ly file thay vì xoá ngay (tránh phá website vì cảnh báo giả),
  báo cáo rõ ràng bằng tiếng Việt cho khách hàng cuối, hướng dẫn khắc phục,
  và tuỳ chọn tự động làm sạch cho các mẫu đã biết chắc chắn;
  (e) *Chống lây lan*: khi phát hiện, kiểm tra ngay các site khác cùng máy chủ và cùng chủ sở hữu;
  (f) *Giới hạn tài nguyên khi quét*: quét là tác vụ nặng, phải chạy với `nice`/`ionice`
  và cgroup để không làm chậm website đang phục vụ.
- **Độ khó:** Trung bình đến Cao. **Phụ thuộc:** Tamper Proof (đã có), P0-5.

### 4.13. Tách quyền tiến trình và sandbox lệnh (P1-P2)

- **Hiện trạng:** Agent chạy với quyền root và có endpoint `/execute` nhận lệnh tuỳ ý.
  Đây là mô hình quyền lực tối đa - một lỗ hổng bất kỳ trong xác thực agent dẫn thẳng tới
  root trên toàn hạ tầng.
- **Cần làm:**
  (a) *Chạy không cần root khi có thể*: tách agent thành tiến trình chính chạy dưới người dùng
  không đặc quyền, và một trợ lý đặc quyền nhỏ (privileged helper) chỉ thực hiện một
  **danh sách hữu hạn các thao tác đã định nghĩa trước** với tham số được kiểm tra kiểu chặt chẽ.
  Trợ lý này phải nhỏ đủ để đọc và kiểm toán được hết;
  (b) *Loại bỏ ghép chuỗi lệnh shell*: mọi lời gọi hệ thống dùng `exec.Command` với mảng
  tham số, không bao giờ dựng chuỗi rồi đưa qua `sh -c`. Rà soát toàn bộ mã hiện có;
  (c) *Giới hạn năng lực Linux*: dùng capabilities cụ thể (`CAP_NET_ADMIN` cho firewall,
  `CAP_CHOWN` cho file manager) thay vì root toàn phần;
  (d) *Sandbox*: bộ lọc seccomp-bpf, `systemd` hardening (`ProtectSystem=strict`,
  `PrivateTmp=yes`, `NoNewPrivileges=yes`, `RestrictAddressFamilies`,
  `SystemCallFilter`) cho mọi unit trong `deploy/systemd/`;
  (e) *Thao tác của người dùng cuối chạy dưới người dùng của họ*: WP-CLI, composer, npm,
  cron của khách hàng phải chạy dưới UID của khách hàng đó, không bao giờ dưới root;
  (f) *Timeout và giới hạn tài nguyên* cho mọi lệnh, không có lệnh nào chạy vô hạn;
  (g) *Terminal web* (`app/(dashboard)/terminal`) là bề mặt tấn công đặc biệt lớn: cần
  quyền riêng, ghi log toàn bộ phiên, giới hạn thời gian, và tuỳ chọn tắt hoàn toàn theo chính sách tenant.
- **Độ khó:** Rất cao (đòi hỏi tái cấu trúc agent). **Phụ thuộc:** 4.8.

### 4.14. Cô lập tenant (P1)

- **Hiện trạng:** Cô lập hiện ở tầng ứng dụng - mọi truy vấn lọc theo `tenant_id`.
  Điều này đúng nhưng chưa đủ: một lỗi lập trình quên mệnh đề `WHERE tenant_id` sẽ làm rò rỉ
  dữ liệu giữa các khách hàng, và không có lớp phòng thủ thứ hai.
- **Cần làm:**
  (a) *Tầng CSDL*: bật Row Level Security của PostgreSQL trên mọi bảng có `tenant_id`,
  đặt `app.current_tenant_id` theo phiên kết nối. Đây là lưới an toàn cho lỗi lập trình;
  (b) *Tầng hệ điều hành*: mỗi website có UID/GID riêng, `open_basedir` cho PHP,
  `ProtectHome`, `PrivateTmp` cho mỗi pool FPM; cân nhắc namespace hoặc container nhẹ
  cho khách hàng ở gói cao;
  (c) *Tầng file*: quyền chặt chẽ, không có thư mục đọc được chung, `noexec` trên thư mục upload;
  (d) *Tầng CSDL của khách hàng*: mỗi khách một người dùng MySQL/PostgreSQL riêng với
  quyền tối thiểu, không bao giờ dùng chung tài khoản;
  (e) *Tầng mạng*: cách ly lưu lượng giữa các tenant khi có nhiều máy chủ,
  chặn khách hàng quét mạng nội bộ;
  (f) *Tầng tài nguyên*: cgroup v2 để một khách hàng không thể làm chậm khách hàng khác
  (vấn đề "hàng xóm ồn ào" - nguyên nhân phổ biến của ticket khiếu nại);
  (g) *Tầng Redis*: nếu dùng Redis chung cho object cache, phải tách database hoặc prefix
  và chặn lệnh `KEYS`/`FLUSHALL`;
  (h) *Kiểm thử*: bộ test tự động cố tình truy cập chéo tenant, chạy trong CI, phải luôn thất bại đúng cách.
- **Độ khó:** Cao. **Phụ thuộc:** P0-2 (quota), 4.4 (RBAC).

### 4.15. Nền tảng bảo mật chung (xuyên suốt)

Những việc không thuộc một tính năng cụ thể nhưng phải làm:

- Loại bỏ mật khẩu mặc định `admin/admin123` khỏi `README.md` và khỏi mã nguồn;
  trình cài đặt sinh mật khẩu ngẫu nhiên và bắt buộc đổi ở lần đăng nhập đầu.
- Siết CSP: hiện `middleware.SecurityHeaders()` dùng `'unsafe-inline' 'unsafe-eval'` cho script,
  vô hiệu hoá phần lớn tác dụng chống XSS. Chuyển sang nonce hoặc hash.
- Siết CORS: kiểm tra `middleware.CORS()` không cho phép mọi origin.
- Lưu token: frontend đang lưu `access_token` trong `localStorage`
  (`app/(dashboard)/layout.tsx`), dễ bị đánh cắp qua XSS. Chuyển sang cookie
  `HttpOnly` + `Secure` + `SameSite=Strict` kèm chống CSRF.
- Quét phụ thuộc tự động (`govulncheck`, `npm audit`) trong CI, chặn merge khi có lỗ hổng nghiêm trọng.
- Chương trình khai báo lỗ hổng có trách nhiệm và quy trình phản hồi (`SECURITY.md` đã có, cần cập nhật).
- Kiểm thử xâm nhập độc lập trước khi phát hành thương mại. Đây là chi phí bắt buộc, không phải tuỳ chọn.
- Bộ test bảo mật tự động trong CI: kiểm tra ủy quyền, cô lập tenant, giới hạn tần suất.

---

## 5. Chuẩn giao diện (Design System)

### 5.1. Hệ thống hiện đang áp dụng

VKAI Panel dùng **light enterprise**: nền sáng, mật độ thông tin cao, ưu tiên sự rõ ràng và
độ tin cậy hơn hiệu ứng thị giác. Đây là lựa chọn đúng cho một công cụ quản trị được nhìn
nhiều giờ mỗi ngày và được trình bày trước khách hàng doanh nghiệp.

Nền tảng kỹ thuật: Tailwind CSS 3.4 (`panel/tailwind.config.js`), biến CSS trong
`panel/src/styles/globals.css`, các primitive trong `panel/src/components/ui/`
(`button`, `card`, `input`, `select`, `tabs`, `badge`), icon từ `lucide-react`,
biểu đồ từ `recharts`, thông báo từ `react-hot-toast`, bảng từ `@tanstack/react-table`.

Bảng màu và token đang dùng:

| Vai trò | Giá trị |
|---|---|
| Nền trang | `bg-gray-50` |
| Nền thẻ, panel, bảng | `bg-white` |
| Đường viền | `border-gray-200` (mặc định), `border-gray-300` (nhấn) |
| Chữ chính | `text-gray-900` |
| Chữ phụ | `text-gray-600` |
| Chữ mờ, nhãn, chú thích | `text-gray-500` |
| Màu chủ đạo | `primary-600` (`#2563eb`), hover `primary-700` |
| Nền nhấn nhẹ | `primary-50`, `bg-gray-100` (trạng thái hover hàng) |
| Thành công | `#22c55e` |
| Cảnh báo | `#f59e0b` |
| Lỗi | `#ef4444` |
| Thông tin | `#06b6d4` |
| Bo góc | `rounded-lg` (thẻ), `rounded-md` (nút, input), `rounded-full` (badge trạng thái) |
| Đổ bóng | `shadow-sm` là mặc định; không dùng bóng nặng |
| Chữ | `Inter` cho giao diện, `JetBrains Mono` cho mã, log, terminal |

### 5.2. Quy tắc bắt buộc cho mọi màn hình mới

1. **Không dùng lớp tối.** Không `bg-dark-*`, không `bg-gray-700/800/900` làm nền vùng nội dung.
   Thang `dark-*` trong `tailwind.config.js` chỉ còn tồn tại vì lý do lịch sử; không dùng cho màn hình mới.
2. **Chỉ dùng token, không dùng mã màu tuỳ tiện.** Không viết `#1a2b3c` trong TSX.
   Cần màu mới thì bổ sung vào `tailwind.config.js` và ghi vào tài liệu này trước.
3. **Tái sử dụng primitive trong `components/ui/`.** Không tạo nút hay input cục bộ trong một trang.
   Nếu primitive thiếu biến thể, mở rộng primitive đó chứ không sao chép.
4. **Cấu trúc trang nhất quán:** tiêu đề trang + mô tả ngắn một dòng, hàng thẻ số liệu tổng quan
   (nếu có), thanh công cụ (tìm kiếm, bộ lọc, nút hành động chính bên phải), rồi vùng nội dung
   (bảng hoặc lưới thẻ). Tham chiếu mẫu tốt: `app/(dashboard)/users/page.tsx`,
   `app/(dashboard)/ssl/page.tsx`.
5. **Bảng:** tiêu đề cột chữ nhỏ in hoa `text-gray-500`, hàng có `hover:bg-gray-50`,
   đường kẻ ngang `border-gray-200`, không kẻ dọc. Bảng rộng phải cuộn ngang trong
   `overflow-x-auto` riêng - trang không bao giờ được cuộn ngang.
6. **Trạng thái badge dùng đúng ngữ nghĩa màu:** xanh lá = đang chạy/khoẻ mạnh,
   vàng = cảnh báo/đang xử lý, đỏ = lỗi/dừng, xám = không hoạt động/không rõ, xanh dương = thông tin.
   Không dùng màu cho mục đích trang trí.
7. **Ba trạng thái bắt buộc cho mọi vùng dữ liệu:** đang tải (skeleton, không phải spinner
   toàn trang), rỗng (giải thích rõ và có hành động tiếp theo), lỗi (nói rõ chuyện gì xảy ra
   và làm gì tiếp, kèm nút thử lại). Không để màn hình trắng.
8. **Hành động huỷ diệt** (xoá website, xoá CSDL, xoá tenant) phải có hộp thoại xác nhận
   yêu cầu gõ đúng tên tài nguyên, nêu rõ hậu quả và cho biết có sao lưu hay không.
9. **Mọi chuỗi phải đi qua hệ thống i18n** ngay khi P0-10 hoàn tất. Không hard-code chuỗi
   hiển thị trong TSX. Áp dụng cho cả thông báo lỗi và văn bản trong `toast`.
10. **Số liệu phải có ngữ cảnh.** Không hiển thị "78%" trần trụi - phải là
    "78% của 100 GB" kèm xu hướng. Ngày giờ theo `dd/MM/yyyy HH:mm` giờ Việt Nam.
    Dung lượng dùng đơn vị nhị phân nhất quán.
11. **Khả năng tiếp cận:** tương phản tối thiểu 4.5:1 cho chữ thường, mọi thao tác thực hiện
    được bằng bàn phím, focus ring rõ ràng, `aria-label` cho nút chỉ có icon.
12. **Đáp ứng đa thiết bị:** bố cục hoạt động từ 1280px trở lên là bắt buộc;
    từ 768px trở lên phải dùng được ở mức đọc và thao tác cơ bản.
13. **Icon từ `lucide-react`**, kích thước nhất quán (16px trong nút và bảng, 20px trong tiêu đề).
    Không trộn nhiều bộ icon.
14. **Không dùng emoji trong giao diện sản phẩm.** Dùng icon.
15. **Không để `className` rỗng hay thừa dấu cách** sau khi chỉnh sửa. Không để lại lớp CSS không dùng.

### 5.3. Việc cần làm với design system

`globals.css` hiện vẫn khai báo các biến `--bg-primary: #0f172a` (tối) ở `:root` trong khi
giao diện đã chuyển sang sáng, và trong repo vẫn còn khoảng 13 file dùng lớp `bg-dark-*`
song song với 17 file đã dùng `bg-white`. Cần một đợt thống nhất: cập nhật biến CSS gốc sang
bảng màu sáng, chuyển nốt các trang còn lại, và gỡ thang `dark-*` khỏi `tailwind.config.js`
khi không còn tham chiếu nào. Sau đó viết `docs/DESIGN_SYSTEM.md` làm nguồn tham chiếu duy nhất.

---

## 6. Bảng ưu tiên tổng hợp

Thang đo: **Tác động** = giá trị kinh doanh cộng rủi ro tránh được (1-5, 5 là cao nhất).
**Công sức** = tuần-người ước tính. **Thứ tự** = trình tự đề xuất thực hiện.

| # | Tính năng | Đợt | Tác động | Công sức (tuần-người) | Thứ tự |
|---|---|---|---|---|---|
| 1 | Bảo mật kênh panel-agent: mTLS, bỏ token tĩnh, bỏ `/execute` tuỳ ý (4.8) | P0 | 5 | 6-8 | 1 |
| 2 | Đóng khoảng cách thực thi: 24 TODO, 9 task handler, agent handler thật (P0-1) | P0 | 5 | 10-14 | 2 |
| 3 | Chống dò mật khẩu, rate limit thật, fail2ban (4.9) | P0 | 5 | 3-4 | 3 |
| 4 | 2FA/TOTP + mã dự phòng (P0-8, 4.1) | P0 | 5 | 2-3 | 4 |
| 5 | Mã hoá dữ liệu nhạy cảm trong DB (4.6) | P0 | 5 | 5-7 | 5 |
| 6 | Gói dịch vụ và hạn mức (P0-2) | P0 | 5 | 8-10 | 6 |
| 7 | Sao lưu S3/Drive + khôi phục 1 chạm + kiểm thử khôi phục (P0-4) | P0 | 5 | 8-10 | 7 |
| 8 | i18n tiếng Việt/tiếng Anh (P0-10) | P0 | 4 | 4-6 | 8 |
| 9 | Trình cài đặt một lệnh trên VPS khách (P0-3) | P0 | 5 | 3-4 | 9 |
| 10 | Quản lý PHP đa phiên bản thật (P0-7) | P0 | 5 | 5-7 | 10 |
| 11 | Giám sát và cảnh báo thật: Telegram/Zalo/Email (P0-5) | P0 | 5 | 4-5 | 11 |
| 12 | Trình cài WordPress + WP-CLI + Staging (P0-6) | P0 | 5 | 8-10 | 12 |
| 13 | Nhật ký kiểm toán bất biến (P0-11, 4.11) | P0 | 4 | 3-4 | 13 |
| 14 | Khoá phiên theo IP/thiết bị (4.2) | P0 | 4 | 2-3 | 14 |
| 15 | Khoá API scoped + xoay khoá (4.5) | P0 | 4 | 2-3 | 15 |
| 16 | Cô lập tenant nhiều lớp: RLS, UID riêng, cgroup (4.14) | P1 | 5 | 6-8 | 16 |
| 17 | RBAC chi tiết theo tài nguyên (4.4) | P1 | 4 | 5-7 | 17 |
| 18 | Đại lý và phân cấp tài khoản (P1-1) | P1 | 5 | 8-10 | 18 |
| 19 | API công khai + webhook + OpenAPI (P1-3) | P1 | 4 | 5-7 | 19 |
| 20 | Truy cập khẩn cấp khi mất kết nối (P1-8) | P1 | 4 | 3-4 | 20 |
| 21 | Quét mã độc website (P1-10, 4.12) | P1 | 4 | 4-6 | 21 |
| 22 | Nâng cấp Tamper Proof: fanotify, baseline ký số, giảm cảnh báo giả (4.10) | P1 | 4 | 5-7 | 22 |
| 23 | Trạng thái dịch vụ và SLA (P1-7) | P1 | 4 | 4-5 | 23 |
| 24 | CDN, cache, Redis, LSCache, tăng tốc (P1-9) | P1 | 4 | 5-7 | 24 |
| 25 | Di trú từ cPanel/DirectAdmin (P1-2) | P1 | 5 | 12-16 | 25 |
| 26 | Email doanh nghiệp: Postfix/Dovecot/DKIM/Rspamd (P1-4) | P1 | 4 | 12-16 | 26 |
| 27 | Hoá đơn/thanh toán: module WHMCS + API + cổng VN (P1-5) | P1 | 5 | 8-10 | 27 |
| 28 | Node.js và Python App Manager (P1-6) | P1 | 3 | 8-10 | 28 |
| 29 | Ký và xác minh gói agent (4.7) | P1 | 4 | 3-4 | 29 |
| 30 | SSO/OIDC/LDAP/SAML (4.3) | P1 | 3 | 6-8 | 30 |
| 31 | Tách quyền tiến trình và sandbox lệnh (4.13) | P1-P2 | 5 | 10-14 | 31 |
| 32 | Chế độ nhiều máy chủ trưởng thành (P2-1) | P2 | 5 | 16-20 | 32 |
| 33 | Cân bằng tải và HA (P2-2) | P2 | 4 | 10-14 | 33 |
| 34 | Báo cáo tuân thủ Nghị định 13, ISO 27001 (P2-6) | P2 | 3 | 5-7 | 34 |
| 35 | Trợ lý vận hành thông minh (P2-4) | P2 | 3 | 8-12 | 35 |
| 36 | Marketplace và hệ thống plugin (P2-3) | P2 | 3 | 14-18 | 36 |
| 37 | Ứng dụng di động và cảnh báo đẩy (P2-5) | P2 | 2 | 8-12 | 37 |

### 6.1. Diễn giải thứ tự

- **Thứ tự 1-5 là nhóm không thể thương lượng.** Đây là những lỗ hổng mà nếu phát hành ra
  thị trường với hiện trạng, một sự cố duy nhất có thể chấm dứt sản phẩm. Chúng phải xong
  trước khi có khách hàng trả tiền đầu tiên, không phải trước khi có khách hàng thứ một trăm.
- **Thứ tự 6-15 là điều kiện cần để bán được.** Không có package/quota thì không có sản phẩm
  hosting. Không có sao lưu ngoài máy chủ thì không có hợp đồng doanh nghiệp. Không có tiếng Việt
  thì không có lý do tồn tại.
- **i18n (số 8) được đặt sớm có chủ ý.** Về mặt tác động nó không cao bằng các mục xung quanh,
  nhưng chi phí trì hoãn tăng theo số màn hình. Mỗi tuần chậm là thêm mã cần sửa lại.
- **Di trú cPanel (số 25) và email (số 26) là hai hạng mục nặng nhất trong P1.** Chúng có
  tác động cao nhưng công sức lớn và rủi ro kỹ thuật cao. Nên bắt đầu nghiên cứu khả thi
  song song với các mục trước đó thay vì chờ tới lượt.
- **Multi-node (số 32) là trục khác biệt hoá chiến lược.** Nó nằm ở P2 vì phụ thuộc vào gần như
  toàn bộ nền tảng bên dưới, nhưng mọi quyết định kiến trúc từ bây giờ phải giữ đường
  mở cho nó - đặc biệt là mô hình agent (số 1) và cô lập tenant (số 16).

### 6.2. Ước tính tổng thể

| Đợt | Tổng công sức | Ghi chú |
|---|---|---|
| P0 (mục 1-15) | 76-100 tuần-người | Với đội 4 người, khoảng 5-6 tháng |
| P1 (mục 16-31) | 105-140 tuần-người | Với đội 5 người, khoảng 6-7 tháng tiếp theo |
| P2 (mục 32-37) | 61-83 tuần-người | Làm chọn lọc theo tín hiệu thị trường, không làm hết |

Các con số này là ước tính thô cho việc lập kế hoạch, chưa tính kiểm thử, tài liệu và
thời gian ổn định sau phát hành. Kinh nghiệm ngành cho thấy nên cộng thêm 30-40% cho
những phần này, và cộng thêm dự phòng cho di trú cPanel và email - hai hạng mục có
độ bất định cao nhất.

---

## 7. Ba việc cần quyết định ngay

1. **Không phát hành thương mại cho tới khi xong nhóm ưu tiên 1-5.** Rủi ro của việc phát hành
   sớm với mô hình xác thực agent hiện tại lớn hơn nhiều so với lợi ích của việc ra mắt sớm vài tháng.
2. **Dừng mở rộng bề rộng tính năng.** Panel đã có 34 màn hình và 43 handler - nhiều hơn mức
   cần thiết để bán hàng. Mọi nỗ lực nên chuyển sang làm cho những gì đã có chạy thật.
   Thêm một module CRUD nữa không tạo thêm giá trị, chỉ thêm bề mặt tấn công và chi phí bảo trì.
3. **Cập nhật `PROGRESS.md`.** Tài liệu này hiện mô tả nhiều mục là "COMPLETE" trong khi mã nguồn
   cho thấy chúng chỉ ở mức CRUD metadata. Sai lệch giữa tài liệu và thực tế dẫn tới quyết định sai
   ở mọi cấp - từ lập kế hoạch sprint tới cam kết với khách hàng.

---

*Tài liệu này cần được xem lại mỗi quý và cập nhật khi hoàn thành mỗi đợt.
Mọi tính năng mới đề xuất bổ sung phải nêu rõ giá trị kinh doanh, độ khó và phụ thuộc
theo cùng định dạng đang dùng ở mục 3.*
