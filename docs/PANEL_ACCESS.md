# Cổng truy cập Panel & Lối vào an toàn

> Panel **không bao giờ** dùng cổng 80/443. Hai cổng đó dành riêng cho website
> của khách hàng chạy trên máy chủ này. Panel nghe trên **cổng riêng** (mặc định
> `8888`, giống aaPanel) và nằm sau một **lối vào an toàn** dạng
> `/vkai_a1b2c3d4`.

Mục lục:

- [Vì sao tách cổng](#vì-sao-tách-cổng)
- [Truy cập lần đầu](#truy-cập-lần-đầu)
- [Đổi cổng panel](#đổi-cổng-panel)
- [Mở tường lửa](#mở-tường-lửa)
- [Lối vào an toàn](#lối-vào-an-toàn)
- [Giới hạn theo IP](#giới-hạn-theo-ip)
- [Ràng buộc theo tên miền](#ràng-buộc-theo-tên-miền)
- [Bật TLS cho panel](#bật-tls-cho-panel)
- [Chạy sau reverse proxy](#chạy-sau-reverse-proxy)
- [Docker](#docker)
- [Toàn bộ biến môi trường](#toàn-bộ-biến-môi-trường)
- [Khắc phục sự cố](#khắc-phục-sự-cố)

---

## Vì sao tách cổng

Cổng 80/443 là nơi mọi trình quét trên Internet gõ cửa đầu tiên. Nếu giao diện
quản trị nằm ở đó thì:

- mọi bot dò `/admin`, `/login`, `/api` đều chạm tới panel;
- một lỗi cấu hình trong vhost của khách có thể vô tình lộ panel;
- log của panel lẫn với log của hàng trăm website.

VKAI Panel tách hẳn:

| Thành phần | Cổng | Ai vào |
|---|---|---|
| Website của khách (`/vkai-panel/www/domains/<domain>`) | 80 / 443 | Internet |
| **Panel quản trị** | **8888** (`VKAI_PANEL_PORT`) | Chỉ quản trị viên |
| API nội bộ (`vkai-api`) | 30110 | Chỉ localhost / mạng nội bộ |
| Giao diện Next.js (`vkai-ui`) | 3000 | Chỉ localhost / mạng nội bộ |
| Agent (`vkai-agent`) | 30111 | Chỉ localhost / mạng nội bộ |

Ba lớp bảo vệ trên cổng panel, theo thứ tự kiểm tra:

1. **Tên miền** (`VKAI_PANEL_DOMAIN`) — sai `Host` thì trả 404.
2. **Địa chỉ IP** (`VKAI_PANEL_ALLOWED_IPS`) — ngoài danh sách thì trả 404.
3. **Lối vào an toàn** (`VKAI_PANEL_ENTRANCE`) — sai đường dẫn thì trả 404.

Mọi trường hợp bị chặn đều nhận **404 trung tính** giống hệt một cổng trống:
không chuyển hướng, không gợi ý, không lộ dấu vết là có panel ở đây.

Ngoại lệ duy nhất: `/api/v1/health`, `/health`, `/ready`, `/live` luôn trả lời
để healthcheck của Docker/Kubernetes hoạt động mà không cần biết lối vào.

---

## Truy cập lần đầu

Lần khởi động đầu tiên, nếu chưa cấu hình gì, panel **tự sinh** lối vào an toàn,
ghi vào `/vkai-panel/etc/panel_access.json` (quyền `0600`) và in ra console:

```
==============================================================================
  VKAI PANEL - THONG TIN TRUY CAP (khong dung cong 80/443)
==============================================================================
  URL truy cap       : http://203.0.113.10:8888/vkai_91ac5b65/
  Cong panel         : 8888
  Dia chi bind       : 0.0.0.0
  Loi vao an toan    : /vkai_91ac5b65
  Gioi han IP        : tat ca IP
  Ten mien           : (khong rang buoc)
  TLS                : tat (HTTP)
  File cau hinh      : /vkai-panel/etc/panel_access.json
==============================================================================
  MO TUONG LUA cho cong panel truoc khi dong console:
    ufw allow 8888/tcp        # Ubuntu / Debian
    firewall-cmd --permanent --add-port=8888/tcp && firewall-cmd --reload   # RHEL
  Cong 80/443 danh RIENG cho website khach - khong dung de vao panel.
  Truy cap sai duong dan se tra ve 404 trung tinh. Hay luu lai URL o tren.
==============================================================================
```

Xem lại bất cứ lúc nào:

```bash
vkai panel info                 # hoặc: vkai-panelctl panel info
journalctl -u vkai-api | grep -A20 "THONG TIN TRUY CAP"
```

---

## Đổi cổng panel

```bash
vkai port                       # xem cổng hiện tại
sudo vkai port 9001             # đổi sang 9001
sudo vkai port random           # cổng ngẫu nhiên
sudo systemctl restart vkai-api
```

Dạng cũ `vkai panel port ...` vẫn hoạt động. Hoặc sửa `/vkai-panel/etc/.env`:

```bash
VKAI_PANEL_PORT=9001
```

rồi `sudo systemctl restart vkai-api`.

Quy tắc:

- Cổng **80, 443, 22, 25, 3306, 5432, 6379** bị từ chối — panel không được
  chiếm cổng của website khách hay của dịch vụ khác.
- **Mở tường lửa cho cổng mới TRƯỚC khi restart**, nếu không bạn tự khoá mình
  ra ngoài.
- Biến môi trường luôn thắng file cấu hình. Nếu `.env` đặt `VKAI_PANEL_PORT`,
  lệnh `vkai panel port` sẽ cảnh báo và bạn phải sửa trong `.env`.

---

## Mở tường lửa

**ufw (Ubuntu/Debian):**

```bash
sudo ufw allow 8888/tcp
sudo ufw reload

# An toàn hơn: chỉ cho IP quản trị
sudo ufw allow from 203.0.113.7 to any port 8888 proto tcp
sudo ufw delete allow 8888/tcp          # gỡ luật mở rộng cũ
```

**firewalld (RHEL/CentOS/Rocky/Alma):**

```bash
sudo firewall-cmd --permanent --add-port=8888/tcp
sudo firewall-cmd --reload

# Chỉ cho IP quản trị
sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="203.0.113.7" port port="8888" protocol="tcp" accept'
sudo firewall-cmd --permanent --remove-port=8888/tcp
sudo firewall-cmd --reload
```

**iptables:**

```bash
sudo iptables -A INPUT -p tcp -s 203.0.113.7 --dport 8888 -j ACCEPT
sudo iptables-save > /etc/iptables/rules.v4
```

**Nhà cung cấp cloud:** đừng quên Security Group / Cloud Firewall của
AWS, GCP, Azure, Vultr, DigitalOcean...

**Đóng cổng cũ** sau khi đổi cổng:

```bash
sudo ufw delete allow 8888/tcp
sudo firewall-cmd --permanent --remove-port=8888/tcp && sudo firewall-cmd --reload
```

---

## Lối vào an toàn

Lối vào an toàn là tiền tố bí mật trong URL, đúng mô hình *security entrance*
của aaPanel:

```
http://203.0.113.10:8888/vkai_91ac5b65/
                        └────┬────────┘
                          lối vào
```

Cách hoạt động:

1. Vào đúng lối vào → panel đặt cookie phiên `vkai_entrance` (HttpOnly,
   SameSite=Lax, ký bằng HMAC-SHA256) rồi phục vụ như bình thường; phần tiền tố
   được cắt bỏ trước khi định tuyến, nên `/vkai_91ac5b65/api/v1/...` và
   `/api/v1/...` là cùng một route.
2. Các request sau đó (XHR của giao diện) chỉ cần cookie, không cần tiền tố.
3. Sai lối vào và không có cookie hợp lệ → **404 trung tính**.
4. Cookie hết hạn sau `VKAI_PANEL_SESSION_TTL` (mặc định `12h`). Đổi lối vào sẽ
   **huỷ toàn bộ** cookie đang có (chữ ký bao gồm chính lối vào).

Quản lý:

```bash
vkai entrance                                # xem lối vào hiện tại
sudo vkai entrance random                    # sinh lối vào mới
sudo vkai entrance /cong_quan_tri_2026       # đặt lối vào tự chọn
```

Dạng cũ `vkai panel entrance ...` vẫn hoạt động. Tắt lối vào an toàn
(**KHÔNG khuyến nghị**) bằng cách đặt trong `/vkai-panel/etc/.env`:

```bash
VKAI_PANEL_ENTRANCE_ENABLED=false
sudo systemctl restart vkai-api
```

Ràng buộc của đường dẫn: bắt đầu bằng `/`, dài 4–64 ký tự, chỉ gồm chữ, số,
`-`, `_`, `.`, `/`; không được trùng các đường dẫn hệ thống `/api`, `/health`,
`/ready`, `/live`, `/ws`, `/_next`, `/static`. Lệnh `vkai entrance` của trình
bao bọc chỉ nhận chữ, số, `-` và `_`; muốn dùng `.` hoặc `/` thì sửa thẳng
`VKAI_PANEL_ENTRANCE` trong `.env`.

> Lưu ý: lối vào là **bí mật, không phải xác thực**. Nó cắt sạch bot dò quét,
> nhưng vẫn phải giữ mật khẩu mạnh và bật 2FA. Đừng dán URL kèm lối vào vào
> chat nhóm, issue tracker hay ảnh chụp màn hình.

---

## Giới hạn theo IP

```bash
vkai panel allow-ip 203.0.113.7,10.0.0.0/8   # chỉ các địa chỉ này
vkai panel allow-ip                          # xem danh sách
sudo vkai panel allow-ip --clear             # bỏ giới hạn
sudo systemctl restart vkai-api
```

Hoặc trong `.env`:

```bash
VKAI_PANEL_ALLOWED_IPS=203.0.113.7,10.0.0.0/8
```

Danh sách rỗng = cho phép tất cả. Hỗ trợ cả IPv4/IPv6, IP đơn và CIDR.

**X-Forwarded-For chỉ được tin khi có proxy tin cậy:**

```bash
VKAI_PANEL_TRUSTED_PROXIES=127.0.0.1,::1
```

- Danh sách rỗng → panel **bỏ qua hoàn toàn** `X-Forwarded-For` và chỉ dùng địa
  chỉ TCP thật. Đây là mặc định an toàn: nếu tin header vô điều kiện, bất kỳ ai
  cũng có thể tự phong cho mình một IP nằm trong danh sách cho phép.
- Có danh sách → panel lấy địa chỉ **ngoài cùng bên phải mà không phải proxy tin
  cậy** trong chuỗi `X-Forwarded-For`, tức là client thật sự.

Chỉ khai báo đúng địa chỉ của reverse proxy do bạn quản lý.

---

## Ràng buộc theo tên miền

```bash
vkai panel domain panel.congty.vn
vkai panel domain --clear
sudo systemctl restart vkai-api
```

Khi đặt, mọi request có `Host` khác (kể cả gõ thẳng IP) đều nhận 404. Rất hiệu
quả để chặn quét theo dải IP.

---

## Bật TLS cho panel

Panel có chứng chỉ **riêng**, độc lập với chứng chỉ website của khách.

**Chứng chỉ thật (khuyến nghị):**

```bash
VKAI_PANEL_TLS_CERT=/etc/letsencrypt/live/panel.congty.vn/fullchain.pem
VKAI_PANEL_TLS_KEY=/etc/letsencrypt/live/panel.congty.vn/privkey.pem
```

**Chứng chỉ tự ký (nội bộ / dùng ngay):**

```bash
VKAI_PANEL_TLS_SELF_SIGNED=true
```

Panel tự sinh `/vkai-panel/ssl/panel/panel.crt` và `/vkai-panel/ssl/panel/panel.key` (khoá riêng
quyền `0600`, hiệu lực 2 năm, SAN gồm `localhost`, `127.0.0.1`, IP chính của máy
và `VKAI_PANEL_DOMAIN` nếu có). Trình duyệt sẽ cảnh báo — đúng như mong đợi với
chứng chỉ tự ký.

Sau khi bật, URL chuyển thành `https://IP:8888/vkai_.../` và cookie lối vào được
đánh dấu `Secure`.

---

## Chạy sau reverse proxy

Khi nginx giữ cổng panel công khai còn API nghe loopback (đây là cách
`deploy/install.sh` cài đặt):

```bash
VKAI_PANEL_BIND=127.0.0.1        # API chỉ nghe nội bộ
VKAI_PANEL_PORT=30110            # cổng API nội bộ
VKAI_PANEL_PUBLIC_PORT=8888      # cổng người dùng gõ (nginx mở)
VKAI_PANEL_PUBLIC_SCHEME=http    # hoặc https nếu nginx cắt TLS
VKAI_PANEL_TRUSTED_PROXIES=127.0.0.1,::1
```

`VKAI_PANEL_PUBLIC_PORT` chỉ ảnh hưởng URL in ra và gợi ý tường lửa; tiến trình
vẫn bind `VKAI_PANEL_PORT`.

Mẫu cấu hình nginx cho panel: `deploy/nginx/vkai-panel.conf` — server block chỉ
`listen 8888`, **không đụng** vhost 80/443 của khách. Cài đặt:

```bash
sudo cp deploy/nginx/vkai-panel.conf /etc/nginx/conf.d/vkai-panel.conf
sudo sed -i 's/8888/9001/g' /etc/nginx/conf.d/vkai-panel.conf   # nếu đổi cổng
sudo nginx -t && sudo systemctl reload nginx
```

Trong file mẫu có sẵn (đang comment) các khối: TLS cho panel, `allow/deny` theo
IP ở tầng nginx, và location cho lối vào an toàn.

> Ở mô hình này, lối vào an toàn được **API** kiểm tra cho `/api/` và `/ws/`.
> Giao diện web không đăng nhập được nếu chưa đi qua lối vào, vì mọi lời gọi API
> đều bị trả 404. Muốn chặn ngay từ nginx cho cả giao diện, bỏ comment khối
> `location ^~ /vkai_.../` trong file mẫu và thay bằng lối vào thật.

---

## Docker

`docker-compose.yml` chỉ publish **một** cổng cho panel:

```yaml
nginx:
  ports:
    - "${VKAI_PANEL_PORT:-8888}:8888"   # KHÔNG map 80/443
```

Các cổng khác (`3000`, `30110`, `30111`, `5432`, `6379`) chỉ nghe `127.0.0.1`
hoặc chỉ nằm trong mạng nội bộ của compose.

Các service trong `docker-compose.yml`: `vkai-core` (API), `vkai-ui` (giao diện),
`vkai-agent`, `nginx`, `postgres`, `redis`.

```bash
# Đổi cổng panel
VKAI_PANEL_PORT=9001 docker compose up -d

# Xem thông tin truy cập (lối vào sinh tự động)
docker compose logs vkai-core | grep -A20 "THONG TIN TRUY CAP"
docker compose exec vkai-core vkai-panelctl panel info
```

Lối vào sinh tự động được lưu trong thư mục cấu hình gắn từ máy chủ
(`${VKAI_PANEL_ROOT:-/vkai-panel}/etc`) nên vẫn giữ nguyên sau khi build lại image.

**Website của khách** phải chạy ở stack/nginx riêng giữ 80/443 — không thêm hai
cổng đó vào service nginx của panel.

---

## Toàn bộ biến môi trường

Mọi biến đều nhận cả tên có tiền tố `VKAI_` lẫn tên trần (`PANEL_PORT`).

| Biến | Mặc định | Ý nghĩa |
|---|---|---|
| `VKAI_PANEL_ENABLED` | `true` | Bật cổng truy cập riêng + các lớp kiểm tra |
| `VKAI_PANEL_PORT` | `8888` | Cổng panel lắng nghe (80/443 bị từ chối) |
| `VKAI_PANEL_BIND` | `0.0.0.0` | Địa chỉ bind (`127.0.0.1` khi có proxy) |
| `VKAI_PANEL_PUBLIC_PORT` | (theo `PORT`) | Cổng người dùng gõ khi có proxy |
| `VKAI_PANEL_PUBLIC_SCHEME` | (theo TLS) | `http` / `https` mà proxy phục vụ |
| `VKAI_PANEL_ENTRANCE` | *(tự sinh)* | Lối vào an toàn, vd `/vkai_a1b2c3d4`; `random` để sinh mới |
| `VKAI_PANEL_ENTRANCE_ENABLED` | `true` | Bật/tắt kiểm tra lối vào |
| `VKAI_PANEL_SESSION_TTL` | `12h` | Hạn cookie phiên lối vào |
| `VKAI_PANEL_RANDOM_PORT` | `false` | Sinh cổng ngẫu nhiên 8000-65535 lần đầu |
| `VKAI_PANEL_ALLOWED_IPS` | *(rỗng)* | IP/CIDR được phép; rỗng = tất cả |
| `VKAI_PANEL_TRUSTED_PROXIES` | *(rỗng)* | Chỉ tin `X-Forwarded-For` từ các địa chỉ này |
| `VKAI_PANEL_DOMAIN` | *(rỗng)* | Chỉ chấp nhận `Host` khớp tên miền này |
| `VKAI_PANEL_TLS_CERT` | *(rỗng)* | Chứng chỉ TLS của panel |
| `VKAI_PANEL_TLS_KEY` | *(rỗng)* | Khoá riêng TLS của panel |
| `VKAI_PANEL_TLS_SELF_SIGNED` | `false` | Tự sinh chứng chỉ tự ký nếu chưa có |
| `VKAI_PANEL_CONFIG_FILE` | `/vkai-panel/etc/panel_access.json` | Nơi lưu cổng/lối vào đã sinh |

Thứ tự ưu tiên: **mặc định < file cấu hình < biến môi trường**.

---

## Khắc phục sự cố

### Quên lối vào an toàn

```bash
vkai panel info
# hoặc
sudo cat /vkai-panel/etc/panel_access.json
# hoặc
sudo grep VKAI_PANEL_ENTRANCE /vkai-panel/etc/.env
# hoặc sinh lối vào mới
vkai panel entrance random && sudo systemctl restart vkai-api
```

### Quên cổng panel

```bash
vkai panel port
sudo ss -tlnp | grep vkai-api
```

### Vào đâu cũng ra 404

Đúng thiết kế khi chưa qua lối vào. Kiểm tra lần lượt:

```bash
vkai panel info                                   # URL đầy đủ, cổng, lối vào
curl -sI http://127.0.0.1:8888/health             # 200 => panel đang chạy
journalctl -u vkai-api | grep "panel access denied"   # lý do bị chặn
```

Log ghi rõ nguyên nhân: `host khong khop PANEL_DOMAIN`,
`IP khong nam trong PANEL_ALLOWED_IPS`, hoặc `sai loi vao an toan`.

### Tự khoá mình bằng danh sách IP

Đăng nhập qua SSH/console của nhà cung cấp rồi:

```bash
sudo vkai panel allow-ip --clear
sudo systemctl restart vkai-api
```

### Không kết nối được sau khi đổi cổng

```bash
sudo ss -tlnp | grep vkai-api          # panel có lắng nghe đúng cổng không
sudo ufw status | grep 8888            # tường lửa đã mở chưa
sudo journalctl -u vkai-api -n 50      # lỗi bind: cổng đang bị chiếm?
```

### Panel không khởi động sau khi sửa cấu hình

```bash
sudo journalctl -u vkai-api -n 50
```

Panel **cố tình dừng hẳn** khi cấu hình truy cập không hợp lệ (cổng 80/443, lối
vào sai định dạng, CIDR sai) thay vì âm thầm chạy với cấu hình yếu hơn. Thông
báo lỗi ghi rõ giá trị nào sai.

### Đổi cổng nhưng nginx vẫn nghe cổng cũ

Ở mô hình có reverse proxy phải sửa cả hai nơi:

```bash
sudo sed -i 's/listen 8888/listen 9001/' /etc/nginx/conf.d/vkai-panel.conf
sudo nginx -t && sudo systemctl reload nginx
vkai panel port --public 9001
```

### Reset toàn bộ cấu hình truy cập

```bash
sudo rm /vkai-panel/etc/panel_access.json
sudo systemctl restart vkai-api
sudo journalctl -u vkai-api -n 40      # cổng + lối vào mới được in ra
```
