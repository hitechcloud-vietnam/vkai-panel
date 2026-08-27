# Bảo vệ mã nguồn VKAI Panel

Hai lớp bảo vệ khác nhau, giải quyết hai mối lo khác nhau. Đừng lẫn lộn.

| | Lớp 1 — mã hoá repository | Lớp 2 — làm rối bản phát hành |
|---|---|---|
| Chống ai đọc | Người xem repository trên GitHub | Khách hàng cầm bản cài trên máy chủ của họ |
| Công cụ | `vkai-crypt` (bộ lọc clean/smudge của Git) | `garble`, obfuscator JS, bytenode |
| Đối tượng | Mã nguồn trong kho Git | Binary và bundle đã biên dịch |
| Dưới máy dev | Mã nguồn gốc, không đổi | Không ảnh hưởng |
| Có giải mã ngược không | Có, bằng khoá | Không — làm rối là một chiều |

---

## Lớp 1 — Mã hoá mã nguồn trên GitHub

### Nguyên lý

```
Thư mục làm việc (mã nguồn gốc)  ──clean──▶  Blob trong .git và trên GitHub (ciphertext)
Blob trong .git (ciphertext)     ──smudge─▶  Thư mục làm việc (mã nguồn gốc)
```

Code dưới máy không bao giờ bị mã hoá. Chỉ nội dung nằm trong kho Git là ciphertext.

Thuật toán: AES-256-CTR, xác thực bằng HMAC-SHA256, khoá gốc 32 byte.
Mã hoá **tất định** — IV dẫn xuất từ HMAC của chính nội dung — nên cùng một nội dung
luôn cho ra cùng ciphertext. Nếu dùng IV ngẫu nhiên, mỗi lần chạy `git status`
Git sẽ tưởng mọi file đều thay đổi.

### Cài đặt lần đầu

```bash
cd /home/vkai-panel
go build -o /usr/local/bin/vkai-crypt ./tools/protect/cmd/vkai-crypt

vkai-crypt init                 # sinh khoá + cài bộ lọc cho repo này
vkai-crypt export-key           # SAO LƯU NGAY ra nơi an toàn
vkai-crypt verify               # tự kiểm tra vòng mã hoá/giải mã

git add --renormalize .         # áp bộ lọc cho toàn bộ file đang theo dõi
vkai-crypt status               # xác nhận blob trong kho đã là ciphertext
git commit -m "chore: bật mã hoá mã nguồn"
```

Khoá nằm ở `~/.vkai/keys/vkai-repo.key` (chmod 600), **ngoài repository**.
Ghi đè bằng `VKAI_CRYPT_KEY` (hex) hoặc `VKAI_CRYPT_KEY_FILE`.

### Máy mới / người mới trong nhóm

```bash
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel
go build -o /usr/local/bin/vkai-crypt ./tools/protect/cmd/vkai-crypt
vkai-crypt import-key <chuỗi-hex-được-cấp>
vkai-crypt init
git checkout -f HEAD             # checkout lại để smudge giải mã
```

### Bộ giải mã độc lập

Khi chỉ còn một bản clone đã mã hoá và khoá sao lưu, không cần cấu hình Git nào:

```bash
git clone <repo> ma-nguon && cd ma-nguon
VKAI_CRYPT_KEY=<hex> vkai-crypt decrypt-tree .
```

`decrypt-tree` quét toàn thư mục, nhận diện file có header `VKAICRYPT1`, giải mã tại chỗ,
bỏ qua file không mã hoá, và báo lỗi nếu HMAC không khớp (sai khoá hoặc dữ liệu bị sửa).

Định dạng file mã hoá — đủ để tự viết lại bộ giải mã nếu cần:

```
"VKAICRYPT1\n"  11 byte nhận dạng
IV              16 byte  = HMAC-SHA256(subkey "iv", plaintext)[0:16]
MAC             32 byte  = HMAC-SHA256(subkey "mac", IV || ciphertext)
ciphertext      còn lại  = AES-256-CTR(subkey "enc", IV, plaintext)

subkey(mục_đích) = HMAC-SHA256(khoá_gốc, "vkai-crypt/v1/" + mục_đích)
```

### Phạm vi mã hoá

Khai báo trong `.gitattributes` ở gốc repo. Cố ý **không** mã hoá:
`tools/protect/**` (chính bộ giải mã, CI phải chạy được trước tiên), `.github/**`,
`go.mod`, `go.sum`, `package.json`, `package-lock.json`, `LICENSE`, `README.md`.
Các file manifest chỉ lộ danh sách thư viện, đổi lại giữ được cache CI, Dependabot và quét lỗ hổng.

### CI phải giải mã trước khi build

Thêm secret `VKAI_CRYPT_KEY` trong Settings → Secrets and variables → Actions,
rồi chèn bước này ngay sau `actions/checkout` và `actions/setup-go`:

```yaml
      - name: Giải mã mã nguồn
        env:
          VKAI_CRYPT_KEY: ${{ secrets.VKAI_CRYPT_KEY }}
        run: |
          if [ -z "$VKAI_CRYPT_KEY" ]; then
            echo "::error::Thiếu secret VKAI_CRYPT_KEY — không build được mã nguồn đã mã hoá."
            exit 1
          fi
          go run ./tools/protect/cmd/vkai-crypt decrypt-tree .
```

Secret không được cấp cho workflow chạy từ fork, nên PR từ người ngoài sẽ không giải mã được — đúng ý đồ.

---

## Lớp 2 — Làm rối bản phát hành

```bash
./tools/protect/build-protected.sh
```

Biến điều khiển:

| Biến | Mặc định | Tác dụng |
|---|---|---|
| `VKAI_OBFUSCATE_GO` | `1` | `garble -literals -tiny` + `-trimpath -ldflags "-s -w"` |
| `VKAI_OBFUSCATE_JS` | `0` | webpack-obfuscator cho phần logic lõi phía trình duyệt |
| `VKAI_BYTECODE_SERVER` | `0` | biên dịch server Node sang V8 bytecode (thử nghiệm) |

Cần cài trước: `go install mvdan.cc/garble@latest`.

Đầu ra `dist/<phiên-bản>/` gồm `bin/vkai-api`, `bin/vkai-agent`, bản build UI standalone,
`SHA256SUMS` và `MANIFEST.txt`. Bản này **không chứa mã nguồn**.

### Ba điều cần biết trước khi kỳ vọng quá nhiều

**Làm rối JS phía trình duyệt là lớp bảo vệ yếu.** Code chạy trên trình duyệt bắt buộc
phải ở dạng máy đọc được, nên luôn dịch ngược được — chỉ tốn thời gian hơn. Đổi lại,
bundle phình 2–5 lần và parse chậm hơn thấy rõ. Vì vậy mặc định TẮT: chỉ nên bật cho
logic kiểm tra bản quyền, đừng bật cho toàn bộ UI.

**Bytecode V8 không phải mã hoá.** File `.jsc` vẫn dịch ngược được về mã gần với JS gốc
bằng công cụ có sẵn. Nó chặn người đọc lướt, không chặn người quyết tâm. Với Next.js còn
dễ vỡ: `.next/server` nạp động nhiều chunk, mỗi lần nâng Next lại phải làm lại.

**Kết luận thực dụng:** logic nào thật sự cần giữ kín thì đặt ở phía Go, nơi `garble`
bảo vệ có ý nghĩa; hoặc giữ trên máy chủ của mình và cho panel gọi qua API. Không có
cách nào bảo vệ tuyệt đối thứ đã nằm trên máy của người khác.

---

## Rủi ro phải xử lý trước khi bật

**Repository đang PUBLIC.** Toàn bộ lịch sử commit từ trước tới nay là mã nguồn dạng
plaintext, ai cũng đọc và clone được. Mã hoá từ bây giờ chỉ bảo vệ commit mới. Xử lý:

1. Chuyển repo sang private: `gh repo edit hitechcloud-vietnam/vkai-panel --visibility private`
2. Nếu cần xoá dấu vết cũ: viết lại lịch sử bằng `git filter-repo`, hoặc dứt khoát hơn là
   tạo repo mới và đẩy lên một commit khởi tạo duy nhất đã mã hoá.
3. Coi mọi bí mật từng xuất hiện trong lịch sử là đã lộ, phải xoay vòng lại toàn bộ.

**Mất khoá là mất mã nguồn.** Không có cửa hậu, không có khôi phục. Sao lưu khoá ở ít nhất
hai nơi tách biệt (trình quản lý mật khẩu của công ty + bản in cất két), và cấp cho ít nhất
hai người.

**Đánh đổi khi làm việc nhóm.** Diff và review trên giao diện GitHub trở thành vô nghĩa
(chỉ thấy ciphertext); `git blame`, tìm kiếm mã trên GitHub, Dependabot với code scanning
đều mất tác dụng. Dưới máy vẫn xem diff bình thường nhờ `diff=vkai-crypt` (textconv).
Nếu cần review trên GitHub, cân nhắc chỉ mã hoá phần lõi thay vì toàn bộ — sửa danh sách
trong `.gitattributes`.

**Đừng commit khi chưa có khoá.** Bộ lọc `clean` cố ý báo lỗi thay vì để lọt plaintext lên kho.
