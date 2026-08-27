package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// managedServicePrefixes limits which units the panel API may control. Without
// this a tenant could stop postgresql, sshd or the panel itself.
var managedServicePrefixes = []string{"vkai-", "vkai_"}

// alwaysManageable are the well known units a hosting panel legitimately
// operates on top of the units it created itself.
var serviceUserRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

var alwaysManageable = map[string]bool{
	"nginx": true, "apache2": true, "httpd": true, "caddy": true,
	"php-fpm": true, "mysql": true, "mariadb": true, "postgresql": true,
	"redis": true, "redis-server": true, "memcached": true,
	"vsftpd": true, "proftpd": true, "pure-ftpd": true,
	"postfix": true, "dovecot": true, "named": true, "bind9": true,
}

// checkServiceName validates the unit name and confirms the panel is allowed to
// act on it.
func checkServiceName(name string) error {
	if err := utils.ValidateServiceName(name); err != nil {
		return err
	}
	base := strings.TrimSuffix(name, ".service")
	if alwaysManageable[base] {
		return nil
	}
	for _, prefix := range managedServicePrefixes {
		if strings.HasPrefix(base, prefix) {
			return nil
		}
	}
	return fmt.Errorf("service %q is not managed by this panel", name)
}

// ServiceManager manages systemd services (background services, not Docker)
type ServiceManager struct{}

func NewServiceManager() *ServiceManager {
	return &ServiceManager{}
}

// ServiceInfo represents a systemd service
type ServiceInfo struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Description string `json:"description"`
	PID         int    `json:"pid"`
	Memory      int64  `json:"memory"`
}

// ListServices lists all vkai-managed services
func (m *ServiceManager) ListServices(ctx context.Context) ([]ServiceInfo, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--plain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	var services []ServiceInfo
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.HasSuffix(fields[0], ".service") {
			name := strings.TrimSuffix(fields[0], ".service")
			services = append(services, ServiceInfo{
				Name:        name,
				ActiveState: fields[2],
				SubState:    fields[3],
				Status:      fields[2],
			})
		}
	}

	return services, nil
}

// GetServiceStatus gets the status of a specific service
func (m *ServiceManager) GetServiceStatus(ctx context.Context, name string) (*ServiceInfo, error) {
	if err := utils.ValidateServiceName(name); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "systemctl", "show", "--", name,
		"--property=ActiveState,SubState,Description,MainPID,MemoryCurrent",
		"--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get service status: %w", err)
	}

	info := &ServiceInfo{Name: name}
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "ActiveState":
			info.ActiveState = parts[1]
			info.Status = parts[1]
		case "SubState":
			info.SubState = parts[1]
		case "Description":
			info.Description = parts[1]
		case "MainPID":
			fmt.Sscanf(parts[1], "%d", &info.PID)
		case "MemoryCurrent":
			fmt.Sscanf(parts[1], "%d", &info.Memory)
		}
	}

	return info, nil
}

// StartService starts a systemd service
func (m *ServiceManager) StartService(ctx context.Context, name string) error {
	if err := checkServiceName(name); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "systemctl", "start", "--", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start service: %s: %w", string(output), err)
	}
	return nil
}

// StopService stops a systemd service
func (m *ServiceManager) StopService(ctx context.Context, name string) error {
	if err := checkServiceName(name); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "systemctl", "stop", "--", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stop service: %s: %w", string(output), err)
	}
	return nil
}

// RestartService restarts a systemd service
func (m *ServiceManager) RestartService(ctx context.Context, name string) error {
	if err := checkServiceName(name); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "--", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restart service: %s: %w", string(output), err)
	}
	return nil
}

// EnableService enables a service to start on boot
func (m *ServiceManager) EnableService(ctx context.Context, name string) error {
	if err := checkServiceName(name); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "systemctl", "enable", "--", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable service: %s: %w", string(output), err)
	}
	return nil
}

// DisableService disables a service from starting on boot
func (m *ServiceManager) DisableService(ctx context.Context, name string) error {
	if err := checkServiceName(name); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "systemctl", "disable", "--", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to disable service: %s: %w", string(output), err)
	}
	return nil
}

// CreateService creates a new systemd service unit file
func (m *ServiceManager) CreateService(ctx context.Context, name, description, execStart, workDir, user string, env map[string]string) error {
	// The unit file is written as root, so every field is validated before it
	// reaches the file: a newline in any of them would otherwise let the caller
	// append arbitrary directives (User=root, ExecStartPre=...).
	if err := checkServiceName(name); err != nil {
		return err
	}
	if err := utils.ValidateSingleLine(description, "description"); err != nil {
		return err
	}
	if err := utils.ValidateSingleLine(execStart, "exec_start"); err != nil {
		return err
	}
	if !filepath.IsAbs(strings.Fields(execStart + " ")[0]) {
		return fmt.Errorf("exec_start must start with an absolute path")
	}
	if workDir == "" {
		workDir = "/"
	}
	if err := utils.ValidateAbsolutePath(workDir, "work_dir"); err != nil {
		return err
	}
	if !serviceUserRe.MatchString(user) {
		return fmt.Errorf("user must match ^[a-z_][a-z0-9_-]*$")
	}
	if user == "root" {
		return fmt.Errorf("services created through the API may not run as root")
	}
	for k, v := range env {
		if err := utils.ValidateEnvKey(k); err != nil {
			return err
		}
		if err := utils.ValidateSingleLine(v, "environment value "+k); err != nil {
			return err
		}
	}

	unitContent := fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=5
`, description, workDir, execStart)

	// Add environment variables, each quoted so nothing can escape its line.
	for k, v := range env {
		unitContent += fmt.Sprintf("Environment=%s\n", strconv.Quote(k+"="+v))
	}

	// Identity and sandboxing come last so they win over anything above.
	unitContent += fmt.Sprintf(`User=%s
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
RestrictSUIDSGID=true
ReadWritePaths=%s

[Install]
WantedBy=multi-user.target
`, user, workDir)

	unitPath := filepath.Join("/etc/systemd/system", name+".service")
	if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd
	cmd := exec.CommandContext(ctx, "systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	return nil
}

// DeleteService removes a systemd service
func (m *ServiceManager) DeleteService(ctx context.Context, name string) error {
	if err := checkServiceName(name); err != nil {
		return err
	}

	// Stop and disable first
	_ = m.StopService(ctx, name)
	_ = m.DisableService(ctx, name)

	unitPath := filepath.Join("/etc/systemd/system", name+".service")
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	// Reload systemd
	cmd := exec.CommandContext(ctx, "systemctl", "daemon-reload")
	return cmd.Run()
}

// GetServiceLogs gets recent logs for a service
func (m *ServiceManager) GetServiceLogs(ctx context.Context, name string, lines int) (string, error) {
	if err := checkServiceName(name); err != nil {
		return "", err
	}
	if lines <= 0 || lines > 5000 {
		lines = 100
	}
	cmd := exec.CommandContext(ctx, "journalctl", "-u", name,
		"--no-pager", "-n", fmt.Sprintf("%d", lines), "--output=short-iso")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get service logs: %w", err)
	}
	return string(output), nil
}

// CreateAppService creates a systemd service for a user application
func (m *ServiceManager) CreateAppService(ctx context.Context, tenantID uuid.UUID, appName, appType, appDir string, port int, env map[string]string) error {
	serviceName := fmt.Sprintf("vkai-app-%s-%s", tenantID.String()[:8], appName)

	var execStart string
	var user string = "www-data"

	switch appType {
	case "node":
		execStart = fmt.Sprintf("/usr/bin/node %s", filepath.Join(appDir, "server.js"))
	case "python":
		execStart = fmt.Sprintf("/usr/bin/python3 %s", filepath.Join(appDir, "app.py"))
	case "go":
		execStart = filepath.Join(appDir, appName)
	default:
		return fmt.Errorf("unsupported app type: %s", appType)
	}

	if env == nil {
		env = make(map[string]string)
	}
	env["PORT"] = fmt.Sprintf("%d", port)

	return m.CreateService(ctx, serviceName, fmt.Sprintf("VKAI Panel - App: %s", appName), execStart, appDir, user, env)
}
