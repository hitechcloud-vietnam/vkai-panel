package nodeapp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// serviceNameRe pins the shape of a generated unit name so it can never carry a
// path separator into /etc/systemd/system.
var serviceNameRe = regexp.MustCompile(`^vkai-nodeapp-[0-9a-f]{8}$`)

const (
	systemdUnitPath = "/etc/systemd/system"
	servicePrefix   = "vkai-nodeapp"
)

// SystemdServiceManager manages Node.js applications as systemd services
type SystemdServiceManager struct {
	logger  *zap.Logger
	binPath string // Path to node binary
	npmPath string // Path to npm binary
}

// NewSystemdServiceManager creates a new systemd service manager
func NewSystemdServiceManager(logger *zap.Logger) *SystemdServiceManager {
	return &SystemdServiceManager{
		logger:  logger,
		binPath: "/usr/bin/node",
		npmPath: "/usr/bin/npm",
	}
}

// ServiceConfig represents the configuration for a systemd service
type ServiceConfig struct {
	ServiceName    string
	Description    string
	WorkingDir     string
	ExecStart      string
	ExecStop       string
	ExecReload     string
	Environment    map[string]string
	User           string
	Group          string
	RestartPolicy  string
	RestartSec     int
	WatchdogSec    int
	TimeoutStopSec int
	PIDFile        string
	LogFile        string
	EnvFile        string
	WantedBy       string
}

// systemdServiceTemplate is the template for systemd service files
// Every dynamic value is validated before rendering (see validateServiceConfig)
// and the identity plus sandboxing directives are emitted AFTER the dynamic
// block, so a value that somehow smuggled in a newline still cannot override
// them - systemd keeps the last assignment of a directive.
const systemdServiceTemplate = `[Unit]
Description={{.Description}}
After=network.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory={{.WorkingDir}}
{{- if .EnvFile}}
EnvironmentFile={{.EnvFile}}
{{- end}}
{{- range $key, $value := .Environment}}
Environment={{$value}}
{{- end}}
ExecStart={{.ExecStart}}
{{- if .ExecStop}}
ExecStop={{.ExecStop}}
{{- end}}
{{- if .ExecReload}}
ExecReload={{.ExecReload}}
{{- end}}
Restart={{.RestartPolicy}}
RestartSec={{.RestartSec}}
TimeoutStopSec={{.TimeoutStopSec}}
{{- if .PIDFile}}
PIDFile={{.PIDFile}}
{{- end}}
StandardOutput=journal
StandardError=journal
SyslogIdentifier={{.ServiceName}}
User={{.User}}
Group={{.Group}}
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictSUIDSGID=true
ReadWritePaths={{.WorkingDir}}

[Install]
WantedBy={{.WantedBy}}
`

// validateServiceConfig rejects any value that could break out of its INI line
// or redefine a directive further down the unit file.
func validateServiceConfig(app *models.NodeApp, envVars map[string]string) error {
	if err := utils.ValidateNodeStartScript(app.StartScript, "start_script"); err != nil {
		return err
	}
	if app.StopScript != "" && app.StopScript != "kill $PID" {
		if err := utils.ValidateNodeStartScript(app.StopScript, "stop_script"); err != nil {
			return err
		}
	}
	if app.RestartScript != "" {
		if err := utils.ValidateNodeStartScript(app.RestartScript, "restart_script"); err != nil {
			return err
		}
	}
	if err := utils.ValidateAbsolutePath(app.Path, "path"); err != nil {
		return err
	}
	if app.EnvFile != "" {
		if err := utils.ValidateAbsolutePath(app.EnvFile, "env_file"); err != nil {
			return err
		}
	}
	if app.PIDFile != "" {
		if err := utils.ValidateAbsolutePath(app.PIDFile, "pid_file"); err != nil {
			return err
		}
	}
	if err := utils.ValidateSingleLine(app.Name, "name"); err != nil {
		return err
	}
	for k, v := range envVars {
		if err := utils.ValidateEnvKey(k); err != nil {
			return err
		}
		if err := utils.ValidateSingleLine(v, "environment value "+k); err != nil {
			return err
		}
	}
	return nil
}

// GenerateServiceFile generates a systemd service file for a Node.js app
func (m *SystemdServiceManager) GenerateServiceFile(ctx context.Context, app *models.NodeApp, envVars map[string]string) (string, error) {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return "", err
	}

	if err := validateServiceConfig(app, envVars); err != nil {
		return "", err
	}

	// Build ExecStart command
	execStart := fmt.Sprintf("%s %s", m.binPath, app.StartScript)
	if app.StartScript == "npm start" {
		execStart = fmt.Sprintf("%s start", m.npmPath)
	}

	// Build ExecStop command
	execStop := ""
	if app.StopScript != "" && app.StopScript != "kill $PID" {
		execStop = fmt.Sprintf("%s %s", m.binPath, app.StopScript)
	}

	// Build ExecReload command
	execReload := ""
	if app.RestartScript != "" {
		execReload = fmt.Sprintf("%s %s", m.binPath, app.RestartScript)
	}

	// Environment values are rendered as quoted "KEY=VALUE" pairs produced by
	// strconv.Quote, which escapes quotes, backslashes and newlines.
	quotedEnv := make(map[string]string, len(envVars))
	for k, v := range envVars {
		quotedEnv[k] = strconv.Quote(k + "=" + v)
	}

	// Set restart policy
	restartPolicy := "on-failure"
	if app.AutoRestart {
		restartPolicy = "always"
	}

	config := ServiceConfig{
		ServiceName:    serviceName,
		Description:    fmt.Sprintf("VKAI Panel - Node.js App: %s", app.Name),
		WorkingDir:     app.Path,
		ExecStart:      execStart,
		ExecStop:       execStop,
		ExecReload:     execReload,
		Environment:    quotedEnv,
		User:           "www-data",
		Group:          "www-data",
		RestartPolicy:  restartPolicy,
		RestartSec:     5,
		TimeoutStopSec: 30,
		PIDFile:        app.PIDFile,
		LogFile:        app.LogFile,
		EnvFile:        app.EnvFile,
		WantedBy:       "multi-user.target",
	}

	// Parse template
	tmpl, err := template.New("systemd").Parse(systemdServiceTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// Generate service file content
	var result strings.Builder
	if err := tmpl.Execute(&result, config); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return result.String(), nil
}

// InstallService installs a systemd service for a Node.js app
func (m *SystemdServiceManager) InstallService(ctx context.Context, app *models.NodeApp, envVars map[string]string) error {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return err
	}
	serviceFilePath := filepath.Join(systemdUnitPath, serviceName+".service")

	// Generate service file content
	content, err := m.GenerateServiceFile(ctx, app, envVars)
	if err != nil {
		return fmt.Errorf("failed to generate service file: %w", err)
	}

	// Write service file
	if err := os.WriteFile(serviceFilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd
	if err := m.reloadSystemd(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable service
	if err := m.enableService(ctx, serviceName); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	m.logger.Info("Systemd service installed",
		zap.String("service", serviceName),
		zap.String("path", serviceFilePath),
	)

	return nil
}

// UninstallService removes a systemd service for a Node.js app
func (m *SystemdServiceManager) UninstallService(ctx context.Context, app *models.NodeApp) error {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return err
	}
	serviceFilePath := filepath.Join(systemdUnitPath, serviceName+".service")

	// Stop service if running
	_ = m.StopService(ctx, app)

	// Disable service
	_ = m.disableService(ctx, serviceName)

	// Remove service file
	if err := os.Remove(serviceFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	// Reload systemd
	if err := m.reloadSystemd(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	m.logger.Info("Systemd service uninstalled",
		zap.String("service", serviceName),
	)

	return nil
}

// StartService starts a systemd service for a Node.js app
func (m *SystemdServiceManager) StartService(ctx context.Context, app *models.NodeApp) error {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return err
	}

	if err := m.systemctl(ctx, "start", serviceName); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	m.logger.Info("Service started", zap.String("service", serviceName))
	return nil
}

// StopService stops a systemd service for a Node.js app
func (m *SystemdServiceManager) StopService(ctx context.Context, app *models.NodeApp) error {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return err
	}

	if err := m.systemctl(ctx, "stop", serviceName); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	m.logger.Info("Service stopped", zap.String("service", serviceName))
	return nil
}

// RestartService restarts a systemd service for a Node.js app
func (m *SystemdServiceManager) RestartService(ctx context.Context, app *models.NodeApp) error {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return err
	}

	if err := m.systemctl(ctx, "restart", serviceName); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}

	m.logger.Info("Service restarted", zap.String("service", serviceName))
	return nil
}

// ReloadService reloads a systemd service for a Node.js app
func (m *SystemdServiceManager) ReloadService(ctx context.Context, app *models.NodeApp) error {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return err
	}

	if err := m.systemctl(ctx, "reload", serviceName); err != nil {
		return fmt.Errorf("failed to reload service: %w", err)
	}

	m.logger.Info("Service reloaded", zap.String("service", serviceName))
	return nil
}

// GetServiceStatus gets the status of a systemd service
func (m *SystemdServiceManager) GetServiceStatus(ctx context.Context, app *models.NodeApp) (string, error) {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return "", err
	}

	output, err := m.systemctlOutput(ctx, "is-active", serviceName)
	if err != nil {
		// Service might not be installed or might have failed
		return "unknown", nil
	}

	status := strings.TrimSpace(output)
	switch status {
	case "active":
		return "running", nil
	case "inactive":
		return "stopped", nil
	case "failed":
		return "failed", nil
	case "activating":
		return "starting", nil
	case "deactivating":
		return "stopping", nil
	default:
		return status, nil
	}
}

// GetServiceLogs gets the logs of a systemd service
func (m *SystemdServiceManager) GetServiceLogs(ctx context.Context, app *models.NodeApp, lines int) ([]string, error) {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return nil, err
	}

	args := []string{
		"-u", serviceName,
		"--no-pager",
		"-n", fmt.Sprintf("%d", lines),
		"--output", "short-iso",
	}

	cmd := exec.CommandContext(ctx, "journalctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	logLines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return logLines, nil
}

// IsServiceInstalled checks if a systemd service is installed
func (m *SystemdServiceManager) IsServiceInstalled(ctx context.Context, app *models.NodeApp) bool {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return false
	}
	serviceFilePath := filepath.Join(systemdUnitPath, serviceName+".service")

	_, err = os.Stat(serviceFilePath)
	return err == nil
}

// IsServiceRunning checks if a systemd service is running
func (m *SystemdServiceManager) IsServiceRunning(ctx context.Context, app *models.NodeApp) bool {
	status, err := m.GetServiceStatus(ctx, app)
	if err != nil {
		return false
	}
	return status == "running"
}

// Helper functions

func (m *SystemdServiceManager) getServiceName(app *models.NodeApp) (string, error) {
	id := app.ID.String()
	if len(id) < 8 {
		return "", fmt.Errorf("invalid app id")
	}
	name := fmt.Sprintf("%s-%s", servicePrefix, id[:8])
	if !serviceNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid service name derived from app id")
	}
	return name, nil
}

func (m *SystemdServiceManager) systemctl(ctx context.Context, action, service string) error {
	cmd := exec.CommandContext(ctx, "systemctl", action, service)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s %s failed: %s: %w", action, service, string(output), err)
	}
	return nil
}

func (m *SystemdServiceManager) systemctlOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}

func (m *SystemdServiceManager) reloadSystemd(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "daemon-reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("daemon-reload failed: %s: %w", string(output), err)
	}
	return nil
}

func (m *SystemdServiceManager) enableService(ctx context.Context, service string) error {
	return m.systemctl(ctx, "enable", service)
}

func (m *SystemdServiceManager) disableService(ctx context.Context, service string) error {
	return m.systemctl(ctx, "disable", service)
}

// GetServiceUptime gets the uptime of a systemd service
func (m *SystemdServiceManager) GetServiceUptime(ctx context.Context, app *models.NodeApp) (time.Duration, error) {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return 0, err
	}

	args := []string{
		"show", serviceName,
		"--property=ActiveEnterTimestamp",
		"--no-pager",
	}

	output, err := m.systemctlOutput(ctx, args...)
	if err != nil {
		return 0, err
	}

	// Parse the timestamp
	parts := strings.SplitN(strings.TrimSpace(string(output)), "=", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("unexpected output format")
	}

	timestamp := strings.TrimSpace(parts[1])
	if timestamp == "" || timestamp == "n/a" {
		return 0, nil
	}

	// Parse systemd timestamp format
	enterTime, err := time.Parse("Mon 2006-01-02 15:04:05 MST", timestamp)
	if err != nil {
		return 0, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return time.Since(enterTime), nil
}

// GetServicePID gets the PID of a systemd service
func (m *SystemdServiceManager) GetServicePID(ctx context.Context, app *models.NodeApp) (int, error) {
	serviceName, err := m.getServiceName(app)
	if err != nil {
		return 0, err
	}

	args := []string{
		"show", serviceName,
		"--property=MainPID",
		"--no-pager",
	}

	output, err := m.systemctlOutput(ctx, args...)
	if err != nil {
		return 0, err
	}

	parts := strings.SplitN(strings.TrimSpace(string(output)), "=", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("unexpected output format")
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &pid); err != nil {
		return 0, fmt.Errorf("failed to parse PID: %w", err)
	}

	return pid, nil
}
