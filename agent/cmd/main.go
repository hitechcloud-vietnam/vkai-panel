package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	AgentVersion = "1.0.0"

	// AgentName is the binary and systemd unit name of the node agent.
	AgentName = "vkaid"

	// AgentProduct is what an operator reading the console sees.
	AgentProduct = "VKAI Panel Agent"

	// AgentVendor appears in the startup line so a support ticket carries the
	// product it belongs to.
	AgentVendor = "HiTech Cloud (hitechcloud.vn)"
)

type AgentConfig struct {
	PanelURL   string
	AgentToken string
	ListenPort int
}

type SystemInfo struct {
	Hostname  string  `json:"hostname"`
	OS        string  `json:"os"`
	Kernel    string  `json:"kernel"`
	Arch      string  `json:"arch"`
	CPUCores  int     `json:"cpu_cores"`
	RAMTotal  int64   `json:"ram_total"`
	DiskTotal int64   `json:"disk_total"`
	Uptime    int64   `json:"uptime"`
	Load1     float64 `json:"load1"`
	Load5     float64 `json:"load5"`
	Load15    float64 `json:"load15"`
}

type HeartbeatPayload struct {
	ServerToken string     `json:"server_token"`
	Timestamp   time.Time  `json:"timestamp"`
	SystemInfo  SystemInfo `json:"system_info"`
	Metrics     Metrics    `json:"metrics"`
	Services    []Service  `json:"services"`
}

type Metrics struct {
	CPUPercent float64 `json:"cpu_percent"`
	RAMUsed    int64   `json:"ram_used"`
	RAMTotal   int64   `json:"ram_total"`
	DiskUsed   int64   `json:"disk_used"`
	DiskTotal  int64   `json:"disk_total"`
	NetIn      int64   `json:"net_in"`
	NetOut     int64   `json:"net_out"`
}

type Service struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func main() {
	fmt.Printf("%s (%s) v%s - %s\n", AgentProduct, AgentName, AgentVersion, AgentVendor)

	cfg := loadConfig()

	// Validate configuration
	if cfg.PanelURL == "" {
		fmt.Println("Loi: thieu bien moi truong VKAI_PANEL_URL (URL cua VKAI Panel)")
		os.Exit(1)
	}
	if cfg.AgentToken == "" {
		fmt.Println("Loi: thieu bien moi truong VKAI_AGENT_TOKEN")
		os.Exit(1)
	}

	// Start heartbeat loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go heartbeatLoop(ctx, cfg)

	// Start command listener
	go commandListener(ctx, cfg)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Shutting down agent...")
	cancel()
}

// env returns the first non-empty value among the given variables. The VKAI_
// name is listed first and the pre-whitelabel name after it, so an agent that
// was deployed with the old environment file keeps running after an upgrade.
func env(names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

func loadConfig() *AgentConfig {
	port := 30111
	if p := env("VKAI_AGENT_PORT", "AGENT_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	return &AgentConfig{
		PanelURL:   env("VKAI_PANEL_URL", "PANEL_URL"),
		AgentToken: env("VKAI_AGENT_TOKEN", "AGENT_TOKEN"),
		ListenPort: port,
	}
}

func heartbeatLoop(ctx context.Context, cfg *AgentConfig) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Send initial heartbeat
	sendHeartbeat(cfg)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sendHeartbeat(cfg)
		}
	}
}

func sendHeartbeat(cfg *AgentConfig) {
	info := collectSystemInfo()
	metrics := collectMetrics()
	services := collectServices()

	payload := HeartbeatPayload{
		ServerToken: cfg.AgentToken,
		Timestamp:   time.Now(),
		SystemInfo:  info,
		Metrics:     metrics,
		Services:    services,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error marshaling heartbeat: %v\n", err)
		return
	}

	url := fmt.Sprintf("%s/api/v1/agent/heartbeat", cfg.PanelURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", cfg.AgentToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending heartbeat: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Heartbeat response: %d\n", resp.StatusCode)
	}
}

func collectSystemInfo() SystemInfo {
	info := SystemInfo{
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}

	// Hostname
	info.Hostname, _ = os.Hostname()

	// OS
	info.OS = runtime.GOOS

	// Kernel
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		info.Kernel = strings.TrimSpace(string(out))
	}

	// RAM
	if out, err := exec.Command("grep", "MemTotal", "/proc/meminfo").Output(); err == nil {
		var total int64
		fmt.Sscanf(string(out), "MemTotal: %d kB", &total)
		info.RAMTotal = total * 1024
	}

	// Disk
	if out, err := exec.Command("df", "-B1", "/").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) > 1 {
				fmt.Sscanf(fields[1], "%d", &info.DiskTotal)
			}
		}
	}

	// Uptime
	if out, err := exec.Command("cat", "/proc/uptime").Output(); err == nil {
		var uptime float64
		fmt.Sscanf(string(out), "%f", &uptime)
		info.Uptime = int64(uptime)
	}

	// Load average
	if out, err := exec.Command("cat", "/proc/loadavg").Output(); err == nil {
		fmt.Sscanf(string(out), "%f %f %f", &info.Load1, &info.Load5, &info.Load15)
	}

	return info
}

func collectMetrics() Metrics {
	var m Metrics

	// RAM usage
	if out, err := exec.Command("free", "-b").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) > 2 {
				fmt.Sscanf(fields[1], "%d", &m.RAMTotal)
				fmt.Sscanf(fields[2], "%d", &m.RAMUsed)
			}
		}
	}

	// Disk usage
	if out, err := exec.Command("df", "-B1", "/").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) > 2 {
				fmt.Sscanf(fields[1], "%d", &m.DiskTotal)
				fmt.Sscanf(fields[2], "%d", &m.DiskUsed)
			}
		}
	}

	return m
}

func collectServices() []Service {
	services := []Service{
		{Name: "nginx", Status: checkService("nginx")},
		{Name: "apache2", Status: checkService("apache2")},
		{Name: "mariadb", Status: checkService("mariadb")},
		{Name: "mysql", Status: checkService("mysql")},
		{Name: "redis", Status: checkService("redis-server")},
		{Name: "docker", Status: checkService("docker")},
		{Name: "php-fpm", Status: checkService("php*-fpm")},
	}
	return services
}

func checkService(name string) string {
	cmd := exec.Command("systemctl", "is-active", name)
	out, err := cmd.Output()
	if err != nil {
		return "inactive"
	}
	status := strings.TrimSpace(string(out))
	if status == "active" {
		return "active"
	}
	return status
}

func commandListener(ctx context.Context, cfg *AgentConfig) {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "healthy",
			"version": AgentVersion,
		})
	})

	mux.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Verify agent token
		token := r.Header.Get("X-Agent-Token")
		if token != cfg.AgentToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			Timeout int      `json:"timeout"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		timeout := 30
		if req.Timeout > 0 {
			timeout = req.Timeout
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, req.Command, req.Args...)
		output, err := cmd.CombinedOutput()

		response := map[string]interface{}{
			"output": string(output),
		}
		if err != nil {
			response["error"] = err.Error()
		}

		json.NewEncoder(w).Encode(response)
	})

	addr := fmt.Sprintf(":%d", cfg.ListenPort)
	fmt.Printf("Agent listening on %s\n", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Agent server error: %v\n", err)
	}
}
