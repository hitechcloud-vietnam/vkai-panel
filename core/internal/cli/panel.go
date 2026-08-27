package cli

// The panel access commands: the panel's own port, its security entrance, the
// IP allow list and the pinned domain.
//
//	vkai panel info
//	vkai panel port 8888
//	vkai panel entrance random
//	vkai panel entrance /vkai_a1b2c3d4
//	vkai panel allow-ip 203.0.113.7,10.0.0.0/8
//	vkai panel domain panel.example.com
//
// Every change is written to the panel access state file
// (/vkai-panel/etc/panel_access.json by default) and takes effect after
// "systemctl restart vkai-api". They live here rather than in a separate
// binary so that the escape hatch an operator needs after changing the
// entrance is on the same command they already have on their PATH.

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// NewPanelCmd builds the "vkai panel" command tree.
func NewPanelCmd() *cobra.Command {
	return newPanelCmd()
}

func newPanelCmd() *cobra.Command {
	panelCmd := &cobra.Command{
		Use:   "panel",
		Short: "Cau hinh cong panel va loi vao an toan",
		Long: "Xem va thay doi cong panel (mac dinh 8888), loi vao an toan, gioi han IP\n" +
			"va ten mien. Cong 80/443 danh rieng cho website khach.",
	}

	panelCmd.AddCommand(newInfoCmd(), newPortCmd(), newEntranceCmd(), newAllowIPCmd(), newDomainCmd())
	return panelCmd
}

func newInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Hien thi thong tin truy cap panel",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := load()
			if err != nil {
				return err
			}
			fmt.Print(cfg.Banner())
			printEnvOverrides(cfg)
			return nil
		},
	}
}

func newPortCmd() *cobra.Command {
	var random bool
	var public bool

	cmd := &cobra.Command{
		Use:   "port [so-cong]",
		Short: "Xem hoac doi cong panel",
		Long: "Doi cong panel. Cong 80 va 443 bi tu choi vi thuoc ve website khach.\n" +
			"Nho mo tuong lua cho cong moi TRUOC khi khoi dong lai panel.\n\n" +
			"Khi panel dung sau reverse proxy, --public doi cong ma nguoi dung go\n" +
			"tren trinh duyet (nho sua ca 'listen' trong cau hinh nginx cua panel).",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := load()
			if err != nil {
				return err
			}

			if len(args) == 0 && !random {
				if public {
					fmt.Println(cfg.EffectivePort())
					return nil
				}
				fmt.Println(cfg.Port)
				return nil
			}

			var port int
			if random {
				port, err = config.RandomPanelPort()
				if err != nil {
					return err
				}
			} else {
				port, err = strconv.Atoi(strings.TrimSpace(args[0]))
				if err != nil {
					return fmt.Errorf("%q khong phai so cong hop le", args[0])
				}
			}

			field := "port"
			old := cfg.Port
			if public {
				field = "public_port"
				old = cfg.EffectivePort()
				cfg.PublicPort = port
			} else {
				cfg.Port = port
			}

			if err := commit(cfg, field); err != nil {
				return err
			}

			fmt.Printf("Da doi cong panel: %d -> %d\n", old, port)
			fmt.Printf("Mo tuong lua:\n  ufw allow %d/tcp\n", cfg.EffectivePort())
			fmt.Printf("  firewall-cmd --permanent --add-port=%d/tcp && firewall-cmd --reload\n", cfg.EffectivePort())
			if old != port {
				fmt.Printf("Dong cong cu khi khong con dung:\n  ufw delete allow %d/tcp\n", old)
			}
			if cfg.IsProxied() && !public {
				fmt.Printf("Luu y: panel dang sau reverse proxy, nguoi dung van vao qua cong %d.\n", cfg.EffectivePort())
			}
			printApply(cfg)
			return nil
		},
	}

	cmd.Flags().BoolVar(&random, "random", false, "Sinh cong ngau nhien trong khoang 8000-65535")
	cmd.Flags().BoolVar(&public, "public", false, "Thao tac tren cong cong khai (khi panel nam sau reverse proxy)")
	return cmd
}

func newEntranceCmd() *cobra.Command {
	var disable bool

	cmd := &cobra.Command{
		Use:   "entrance [duong-dan|random]",
		Short: "Xem hoac doi loi vao an toan",
		Long: "Loi vao an toan la tien to bi mat trong URL, vi du /vkai_a1b2c3d4.\n" +
			"Moi truy cap sai loi vao deu nhan 404 trung tinh.\n" +
			"Doi loi vao se huy toan bo phien dang mo.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := load()
			if err != nil {
				return err
			}

			if disable {
				cfg.EntranceEnabled = false
				if err := commit(cfg, "entrance_enabled"); err != nil {
					return err
				}
				fmt.Println("Da TAT loi vao an toan. Panel chi con duoc bao ve boi cong rieng,")
				fmt.Println("gioi han IP va dang nhap - khong khuyen nghi tren Internet cong cong.")
				printApply(cfg)
				return nil
			}

			if len(args) == 0 {
				if !cfg.EntranceEnabled {
					fmt.Println("(loi vao an toan dang tat)")
					return nil
				}
				fmt.Println(cfg.Entrance)
				return nil
			}

			value := strings.TrimSpace(args[0])
			if strings.EqualFold(value, "random") {
				entrance, err := config.RandomEntrance()
				if err != nil {
					return err
				}
				cfg.Entrance = entrance
			} else {
				entrance := config.NormalizeEntrance(value)
				if err := config.ValidateEntrance(entrance); err != nil {
					return err
				}
				cfg.Entrance = entrance
			}
			cfg.EntranceEnabled = true

			if err := commit(cfg, "entrance"); err != nil {
				return err
			}

			fmt.Printf("Loi vao an toan moi: %s\n", cfg.Entrance)
			fmt.Printf("URL truy cap: %s\n", cfg.AccessURL())
			printApply(cfg)
			return nil
		},
	}

	cmd.Flags().BoolVar(&disable, "disable", false, "Tat loi vao an toan (khong khuyen nghi)")
	return cmd
}

func newAllowIPCmd() *cobra.Command {
	var clear bool

	cmd := &cobra.Command{
		Use:   "allow-ip [ip-hoac-cidr,...]",
		Short: "Xem hoac dat danh sach IP duoc phep vao panel",
		Long: "Danh sach rong nghia la cho phep tat ca. Ho tro IP don va CIDR,\n" +
			"vi du: 203.0.113.7,10.0.0.0/8",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := load()
			if err != nil {
				return err
			}

			if clear {
				cfg.AllowedIPs = []string{}
				if err := commit(cfg, "allowed_ips"); err != nil {
					return err
				}
				fmt.Println("Da xoa gioi han IP: moi IP deu co the vao panel.")
				printApply(cfg)
				return nil
			}

			if len(args) == 0 {
				if len(cfg.AllowedIPs) == 0 {
					fmt.Println("(khong gioi han - tat ca IP)")
					return nil
				}
				fmt.Println(strings.Join(cfg.AllowedIPs, ","))
				return nil
			}

			var list []string
			for _, raw := range strings.FieldsFunc(args[0], func(r rune) bool { return r == ',' || r == ' ' }) {
				value := strings.TrimSpace(raw)
				if value == "" {
					continue
				}
				if _, err := config.ParseIPMatcher(value); err != nil {
					return fmt.Errorf("gia tri khong hop le %q: %w", value, err)
				}
				list = append(list, value)
			}
			cfg.AllowedIPs = list

			if err := commit(cfg, "allowed_ips"); err != nil {
				return err
			}

			fmt.Printf("Chi cac dia chi sau duoc vao panel: %s\n", strings.Join(cfg.AllowedIPs, ", "))
			fmt.Println("Kiem tra ky IP hien tai cua ban co trong danh sach truoc khi khoi dong lai.")
			printApply(cfg)
			return nil
		},
	}

	cmd.Flags().BoolVar(&clear, "clear", false, "Xoa gioi han IP (cho phep tat ca)")
	return cmd
}

func newDomainCmd() *cobra.Command {
	var clear bool

	cmd := &cobra.Command{
		Use:   "domain [ten-mien]",
		Short: "Xem hoac rang buoc panel theo ten mien",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := load()
			if err != nil {
				return err
			}

			if clear {
				cfg.Domain = ""
				if err := commit(cfg, "domain"); err != nil {
					return err
				}
				fmt.Println("Da bo rang buoc ten mien.")
				printApply(cfg)
				return nil
			}

			if len(args) == 0 {
				if cfg.Domain == "" {
					fmt.Println("(khong rang buoc)")
					return nil
				}
				fmt.Println(cfg.Domain)
				return nil
			}

			cfg.Domain = strings.ToLower(strings.TrimSpace(args[0]))
			if err := commit(cfg, "domain"); err != nil {
				return err
			}

			fmt.Printf("Panel chi chap nhan Host = %s\n", cfg.Domain)
			printApply(cfg)
			return nil
		},
	}

	cmd.Flags().BoolVar(&clear, "clear", false, "Bo rang buoc ten mien")
	return cmd
}

func load() (*config.PanelAccessConfig, error) {
	cfg, err := config.LoadPanelAccess()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// commit validates the whole configuration before writing, so a bad value can
// never be persisted into a state file that the panel then refuses to start on.
func commit(cfg *config.PanelAccessConfig, changed string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("%w (chay bang quyen root?)", err)
	}
	if cfg.IsEnvOverridden(changed) {
		fmt.Fprintf(os.Stderr,
			"Canh bao: gia tri %q dang bi dat boi bien moi truong, file cau hinh se bi ghi de khi khoi dong.\n"+
				"          Sua trong /vkai-panel/etc/.env (hoac unit systemd) roi khoi dong lai.\n", changed)
	}
	return nil
}

func printApply(cfg *config.PanelAccessConfig) {
	fmt.Printf("Da luu vao %s. Ap dung bang:\n  systemctl restart vkai-api\n", cfg.StateFile)
}

func printEnvOverrides(cfg *config.PanelAccessConfig) {
	overrides := cfg.EnvOverrides
	if len(overrides) == 0 {
		return
	}
	fmt.Printf("  Dang bi dat boi bien moi truong: %s\n", strings.Join(overrides, ", "))
}
