package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

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
	cmd := exec.CommandContext(ctx, "systemctl", "show", name,
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
	cmd := exec.CommandContext(ctx, "systemctl", "start", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start service: %s: %w", string(output), err)
	}
	return nil
}

// StopService stops a systemd service
func (m *ServiceManager) StopService(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "systemctl", "stop", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stop service: %s: %w", string(output), err)
	}
	return nil
}

// RestartService restarts a systemd service
func (m *ServiceManager) RestartService(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "systemctl", "restart", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restart service: %s: %w", string(output), err)
	}
	return nil
}

// EnableService enables a service to start on boot
func (m *ServiceManager) EnableService(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "systemctl", "enable", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable service: %s: %w", string(output), err)
	}
	return nil
}

// DisableService disables a service from starting on boot
func (m *ServiceManager) DisableService(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "systemctl", "disable", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to disable service: %s: %w", string(output), err)
	}
	return nil
}

// CreateService creates a new systemd service unit file
func (m *ServiceManager) CreateService(ctx context.Context, name, description, execStart, workDir, user string, env map[string]string) error {
	unitContent := fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
Type=simple
User=%s
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=5
`, description, user, workDir, execStart)

	// Add environment variables
	for k, v := range env {
		unitContent += fmt.Sprintf("Environment=%s=%s\n", k, v)
	}

	unitContent += `
[Install]
WantedBy=multi-user.target
`

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

	return m.CreateService(ctx, serviceName, fmt.Sprintf("vKAI App: %s", appName), execStart, appDir, user, env)
}
