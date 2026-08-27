// vkai-crypt — mã hoá mã nguồn trong repository Git, giữ bản làm việc dưới máy ở dạng gốc.
//
// Mô hình: dùng bộ lọc clean/smudge của Git.
//
//	Thư mục làm việc (plaintext)  --clean-->   Blob trong .git và trên GitHub (ciphertext)
//	Blob trong .git (ciphertext)  --smudge-->  Thư mục làm việc (plaintext)
//
// Nhờ vậy code dưới máy KHÔNG bị mã hoá, còn thứ đẩy lên GitHub thì đã mã hoá.
//
// Mã hoá tất định (deterministic): IV được dẫn xuất bằng HMAC của chính nội dung gốc,
// nên cùng một nội dung luôn cho ra cùng một ciphertext. Nếu không làm vậy, mỗi lần
// `git status` Git sẽ thấy file "đã đổi" dù nội dung không đổi.
//
// Thuật toán: AES-256-CTR để mã hoá, HMAC-SHA256 để dẫn xuất IV và để xác thực.
// Khoá gốc 32 byte nằm NGOÀI repository (mặc định ~/.vkai/keys/vkai-repo.key).
//
// Chỉ dùng thư viện chuẩn của Go — bộ giải mã luôn build được ở mọi nơi có Go.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	magic       = "VKAICRYPT1\n" // 11 byte nhận dạng file đã mã hoá
	ivLen       = 16
	macLen      = 32
	keyFileMode = 0o600
	keyDirMode  = 0o700
)

// headerLen là phần cố định đứng trước ciphertext: magic + IV + MAC.
const headerLen = len(magic) + ivLen + macLen

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit()
	case "clean":
		err = cmdClean()
	case "smudge":
		err = cmdSmudge()
	case "textconv":
		err = cmdTextconv(os.Args[2:])
	case "encrypt-file":
		err = cmdEncryptFile(os.Args[2:])
	case "decrypt-file":
		err = cmdDecryptFile(os.Args[2:])
	case "decrypt-tree":
		err = cmdDecryptTree(os.Args[2:])
	case "status":
		err = cmdStatus()
	case "verify":
		err = cmdVerify()
	case "export-key":
		err = cmdExportKey()
	case "import-key":
		err = cmdImportKey(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		err = fmt.Errorf("lệnh không hợp lệ: %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "vkai-crypt: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`vkai-crypt — mã hoá mã nguồn khi đẩy lên Git, giữ nguyên bản dưới máy.

  vkai-crypt init                 Sinh khoá (nếu chưa có) và cài bộ lọc Git cho repo hiện tại
  vkai-crypt status               Liệt kê file thuộc diện mã hoá và trạng thái hiện tại
  vkai-crypt verify               Tự kiểm tra vòng mã hoá/giải mã (không đụng repo)

  vkai-crypt export-key           In khoá dạng hex để sao lưu (GIỮ TUYỆT MẬT)
  vkai-crypt import-key <hex>     Khôi phục khoá từ bản sao lưu ("-" để đọc từ stdin)

  vkai-crypt encrypt-file <in> <out>    Mã hoá một file rời
  vkai-crypt decrypt-file <in> <out>    Giải mã một file rời ("-" = stdout)
  vkai-crypt decrypt-tree <dir>         BỘ GIẢI MÃ: quét thư mục, giải mã mọi file đã mã hoá tại chỗ

  (nội bộ, do Git gọi) clean | smudge | textconv <file>

Khoá lấy theo thứ tự: biến môi trường VKAI_CRYPT_KEY (hex 64 ký tự),
rồi file VKAI_CRYPT_KEY_FILE, rồi ~/.vkai/keys/vkai-repo.key.

Khôi phục khi chỉ còn repo đã mã hoá và khoá sao lưu:
  git clone <repo> ma-nguon && cd ma-nguon
  VKAI_CRYPT_KEY=<hex> vkai-crypt decrypt-tree .
`)
}

// ───────────────────────────── quản lý khoá

func keyPath() string {
	if p := os.Getenv("VKAI_CRYPT_KEY_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".vkai-repo.key"
	}
	return filepath.Join(home, ".vkai", "keys", "vkai-repo.key")
}

func loadKey() ([]byte, error) {
	if h := strings.TrimSpace(os.Getenv("VKAI_CRYPT_KEY")); h != "" {
		return decodeKey(h)
	}
	p := keyPath()
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("chưa có khoá tại %s — chạy `vkai-crypt init` hoặc `vkai-crypt import-key`", p)
		}
		return nil, err
	}
	if fi, err := os.Stat(p); err == nil && fi.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "vkai-crypt: cảnh báo — %s đang mở quyền cho người khác, nên chmod 600\n", p)
	}
	return decodeKey(strings.TrimSpace(string(raw)))
}

func decodeKey(h string) ([]byte, error) {
	k, err := hex.DecodeString(h)
	if err != nil {
		return nil, fmt.Errorf("khoá không phải chuỗi hex hợp lệ: %w", err)
	}
	if len(k) != 32 {
		return nil, fmt.Errorf("khoá phải dài 32 byte (64 ký tự hex), đang có %d byte", len(k))
	}
	return k, nil
}

// subKey tách khoá gốc thành các khoá con theo mục đích, để không dùng chung
// một khoá cho vừa mã hoá vừa xác thực.
func subKey(master []byte, purpose string) []byte {
	m := hmac.New(sha256.New, master)
	m.Write([]byte("vkai-crypt/v1/" + purpose))
	return m.Sum(nil)
}

// ───────────────────────────── lõi mã hoá

func encrypt(master, plaintext []byte) ([]byte, error) {
	encKey := subKey(master, "enc")
	ivKey := subKey(master, "iv")
	macKey := subKey(master, "mac")

	// IV tất định: HMAC của nội dung gốc. Cùng nội dung -> cùng ciphertext,
	// nhờ đó Git không báo file thay đổi khi nội dung không đổi.
	m := hmac.New(sha256.New, ivKey)
	m.Write(plaintext)
	iv := m.Sum(nil)[:ivLen]

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	ct := make([]byte, len(plaintext))
	cipher.NewCTR(block, iv).XORKeyStream(ct, plaintext)

	mac := hmac.New(sha256.New, macKey)
	mac.Write(iv)
	mac.Write(ct)
	tag := mac.Sum(nil)

	out := make([]byte, 0, headerLen+len(ct))
	out = append(out, magic...)
	out = append(out, iv...)
	out = append(out, tag...)
	out = append(out, ct...)
	return out, nil
}

func decrypt(master, blob []byte) ([]byte, error) {
	if !isEncrypted(blob) {
		return nil, errors.New("dữ liệu không phải định dạng vkai-crypt")
	}
	iv := blob[len(magic) : len(magic)+ivLen]
	tag := blob[len(magic)+ivLen : headerLen]
	ct := blob[headerLen:]

	macKey := subKey(master, "mac")
	mac := hmac.New(sha256.New, macKey)
	mac.Write(iv)
	mac.Write(ct)
	if subtle.ConstantTimeCompare(tag, mac.Sum(nil)) != 1 {
		return nil, errors.New("sai khoá hoặc dữ liệu đã bị sửa đổi (HMAC không khớp)")
	}

	block, err := aes.NewCipher(subKey(master, "enc"))
	if err != nil {
		return nil, err
	}
	pt := make([]byte, len(ct))
	cipher.NewCTR(block, iv).XORKeyStream(pt, ct)
	return pt, nil
}

func isEncrypted(b []byte) bool {
	return len(b) >= headerLen && bytes.HasPrefix(b, []byte(magic))
}

// ───────────────────────────── bộ lọc Git

// cmdClean chạy khi Git đọc file từ thư mục làm việc để ghi vào kho (add/commit).
func cmdClean() error {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	if isEncrypted(in) { // đã mã hoá rồi thì không mã hoá chồng
		_, err = os.Stdout.Write(in)
		return err
	}
	key, err := loadKey()
	if err != nil {
		// Không có khoá mà vẫn commit thì sẽ đẩy plaintext lên -> phải chặn.
		return fmt.Errorf("từ chối mã hoá khi commit: %w", err)
	}
	out, err := encrypt(key, in)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}

// cmdSmudge chạy khi Git ghi file từ kho ra thư mục làm việc (checkout/clone).
func cmdSmudge() error {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	if !isEncrypted(in) { // file cũ chưa mã hoá -> giữ nguyên
		_, err = os.Stdout.Write(in)
		return err
	}
	key, err := loadKey()
	if err != nil {
		// Không có khoá: trả nguyên ciphertext thay vì làm hỏng checkout.
		fmt.Fprintln(os.Stderr, "vkai-crypt: không có khoá — file giữ nguyên dạng đã mã hoá")
		_, werr := os.Stdout.Write(in)
		return werr
	}
	out, err := decrypt(key, in)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}

// cmdTextconv giúp `git diff`/`git log -p` hiển thị nội dung gốc thay vì rác nhị phân.
func cmdTextconv(args []string) error {
	if len(args) < 1 {
		return errors.New("textconv cần đường dẫn file")
	}
	in, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	if !isEncrypted(in) {
		_, err = os.Stdout.Write(in)
		return err
	}
	key, err := loadKey()
	if err != nil {
		fmt.Fprintln(os.Stdout, "<vkai-crypt: nội dung đã mã hoá, không có khoá để hiển thị>")
		return nil
	}
	out, err := decrypt(key, in)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}

// ───────────────────────────── lệnh cho người dùng

func cmdInit() error {
	p := keyPath()
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(p), keyDirMode); err != nil {
			return err
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(hex.EncodeToString(key)+"\n"), keyFileMode); err != nil {
			return err
		}
		fmt.Printf("Đã sinh khoá mới: %s\n", p)
	} else if err != nil {
		return err
	} else {
		fmt.Printf("Dùng khoá sẵn có: %s\n", p)
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}
	for _, kv := range [][2]string{
		{"filter.vkai-crypt.clean", self + " clean"},
		{"filter.vkai-crypt.smudge", self + " smudge"},
		{"filter.vkai-crypt.required", "true"},
		{"diff.vkai-crypt.textconv", self + " textconv"},
		{"diff.vkai-crypt.cachetextconv", "false"},
	} {
		if err := exec.Command("git", "config", kv[0], kv[1]).Run(); err != nil {
			return fmt.Errorf("git config %s: %w", kv[0], err)
		}
	}
	fmt.Println("Đã cài bộ lọc Git cho repository hiện tại.")
	fmt.Println()
	fmt.Println("Việc cần làm ngay:")
	fmt.Println("  1. Sao lưu khoá ra nơi an toàn:  vkai-crypt export-key")
	fmt.Println("     MẤT KHOÁ = MẤT VĨNH VIỄN mã nguồn đã đẩy lên (không có cách khôi phục).")
	fmt.Println("  2. Áp bộ lọc cho file đang theo dõi:  git add --renormalize .")
	fmt.Println("  3. Kiểm tra blob đã mã hoá thật chưa:  vkai-crypt status")
	return nil
}

func cmdExportKey() error {
	key, err := loadKey()
	if err != nil {
		return err
	}
	fmt.Println("# KHOÁ GIẢI MÃ VKAI PANEL — TUYỆT MẬT, KHÔNG COMMIT, KHÔNG GỬI QUA CHAT")
	fmt.Println(hex.EncodeToString(key))
	fmt.Println("#")
	fmt.Println("# Khôi phục:  vkai-crypt import-key <chuỗi hex ở trên>")
	fmt.Println("# Giải mã một bản clone đã mã hoá:")
	fmt.Println("#   VKAI_CRYPT_KEY=<hex> vkai-crypt decrypt-tree .")
	return nil
}

func cmdImportKey(args []string) error {
	var h string
	if len(args) == 1 && args[0] != "-" {
		h = args[0]
	} else {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				h = line
				break
			}
		}
	}
	key, err := decodeKey(strings.TrimSpace(h))
	if err != nil {
		return err
	}
	p := keyPath()
	if err := os.MkdirAll(filepath.Dir(p), keyDirMode); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(hex.EncodeToString(key)+"\n"), keyFileMode); err != nil {
		return err
	}
	fmt.Printf("Đã ghi khoá vào %s\n", p)
	return nil
}

func cmdEncryptFile(args []string) error {
	if len(args) != 2 {
		return errors.New("cú pháp: vkai-crypt encrypt-file <đầu vào> <đầu ra>")
	}
	key, err := loadKey()
	if err != nil {
		return err
	}
	in, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	out, err := encrypt(key, in)
	if err != nil {
		return err
	}
	return writeOut(args[1], out, 0o600)
}

func cmdDecryptFile(args []string) error {
	if len(args) != 2 {
		return errors.New("cú pháp: vkai-crypt decrypt-file <đầu vào> <đầu ra|->")
	}
	key, err := loadKey()
	if err != nil {
		return err
	}
	in, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	out, err := decrypt(key, in)
	if err != nil {
		return err
	}
	return writeOut(args[1], out, 0o644)
}

func writeOut(path string, data []byte, mode os.FileMode) error {
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, mode)
}

// cmdDecryptTree là bộ giải mã độc lập: dùng khi chỉ còn bản clone đã mã hoá
// và khoá sao lưu, không cần cấu hình Git nào.
func cmdDecryptTree(args []string) error {
	if len(args) != 1 {
		return errors.New("cú pháp: vkai-crypt decrypt-tree <thư mục>")
	}
	key, err := loadKey()
	if err != nil {
		return err
	}
	root := args[0]
	var done, skipped, failed int
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".next", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		head := make([]byte, len(magic))
		n, _ := io.ReadFull(f, head)
		f.Close()
		if n < len(magic) || !bytes.Equal(head, []byte(magic)) {
			skipped++
			return nil
		}
		blob, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		pt, err := decrypt(key, blob)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  LỖI  %s: %v\n", path, err)
			failed++
			return nil
		}
		fi, err := os.Stat(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, pt, fi.Mode().Perm()); err != nil {
			return err
		}
		done++
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("Đã giải mã %d file, bỏ qua %d file không mã hoá, lỗi %d file.\n", done, skipped, failed)
	if failed > 0 {
		return errors.New("có file không giải mã được — kiểm tra lại khoá")
	}
	return nil
}

func cmdStatus() error {
	out, err := exec.Command("git", "ls-files", "-z").Output()
	if err != nil {
		return fmt.Errorf("không đọc được danh sách file của Git: %w", err)
	}
	files := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")

	var guarded, plain int
	var plainSamples []string
	for _, f := range files {
		if f == "" {
			continue
		}
		chk, err := exec.Command("git", "check-attr", "filter", "--", f).Output()
		if err != nil {
			continue
		}
		if !strings.Contains(string(chk), "filter: vkai-crypt") {
			continue
		}
		blob, err := exec.Command("git", "cat-file", "-p", ":"+f).Output()
		if err != nil {
			continue
		}
		if isEncrypted(blob) {
			guarded++
		} else {
			plain++
			if len(plainSamples) < 10 {
				plainSamples = append(plainSamples, f)
			}
		}
	}
	fmt.Printf("File thuộc diện mã hoá và ĐÃ mã hoá trong kho: %d\n", guarded)
	fmt.Printf("File thuộc diện mã hoá nhưng CÒN plaintext:    %d\n", plain)
	if plain > 0 {
		for _, f := range plainSamples {
			fmt.Printf("   - %s\n", f)
		}
		fmt.Println("Khắc phục: git add --renormalize . && git commit")
	}
	return nil
}

func cmdVerify() error {
	key, err := loadKey()
	if err != nil {
		return err
	}
	samples := [][]byte{
		[]byte(""),
		[]byte("package main\n\nfunc main() {}\n"),
		bytes.Repeat([]byte{0x00, 0xff, 0x7f, 0x80}, 4096),
	}
	for i, s := range samples {
		ct, err := encrypt(key, s)
		if err != nil {
			return err
		}
		ct2, err := encrypt(key, s)
		if err != nil {
			return err
		}
		if !bytes.Equal(ct, ct2) {
			return fmt.Errorf("mẫu %d: mã hoá không tất định", i)
		}
		pt, err := decrypt(key, ct)
		if err != nil {
			return fmt.Errorf("mẫu %d: %w", i, err)
		}
		if !bytes.Equal(pt, s) {
			return fmt.Errorf("mẫu %d: giải mã sai nội dung", i)
		}
		bad := append([]byte(nil), ct...)
		bad[len(bad)-1] ^= 0x01
		if _, err := decrypt(key, bad); err == nil {
			return fmt.Errorf("mẫu %d: HMAC không phát hiện được sửa đổi", i)
		}
	}
	fmt.Println("Kiểm tra đạt: mã hoá tất định, giải mã đúng, phát hiện được sửa đổi.")
	return nil
}
