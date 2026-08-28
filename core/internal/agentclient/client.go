// Package agentclient is how the panel talks to an agent.
//
// Every call goes over mutual TLS: the panel presents the client certificate
// its own CA issued it, and it accepts only the certificate that same CA issued
// to that specific agent. Neither side checks a host name, because a customer's
// server changes address; identity is the certificate itself.
//
// The surface is a closed set of named operations. There is no Exec method and
// no way to pass a program name, because the agent no longer offers one - see
// agent/internal/ops for why.
package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/agentpki"
)

// DefaultPort is the port the agent listens on.
const DefaultPort = 30111

// DefaultTimeout bounds one operation. A named operation is a status query or a
// systemctl call; nothing here should take minutes.
const DefaultTimeout = 60 * time.Second

// Target names one agent: which certificate to demand, and where to dial.
type Target struct {
	// AgentID is the identity the certificate must carry. It is the pin.
	AgentID string
	// Address is host or host:port. It carries no authority: a wrong address
	// reaches a machine that cannot complete the handshake.
	Address string
}

// Response is the envelope every agent operation returns.
type Response struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Client dials agents. It is safe for concurrent use.
type Client struct {
	authority *agentpki.Authority
	logger    *zap.Logger
	timeout   time.Duration

	mu         sync.Mutex
	transports map[string]*http.Transport
}

// New builds a client on top of the panel's certificate authority.
//
// It subscribes to revocations. A deny list is checked when a handshake
// happens, and a connection already in the pool does not make one, so a
// revocation would otherwise take effect only once the idle connection timed
// out. Dropping the transport here makes the next call handshake again and be
// refused, which is what "rejected at the next handshake, not at the next
// expiry" has to mean in a process that keeps connections.
func New(authority *agentpki.Authority, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}
	c := &Client{
		authority:  authority,
		logger:     logger,
		timeout:    DefaultTimeout,
		transports: make(map[string]*http.Transport),
	}
	if authority != nil {
		authority.OnRevoke(c.Forget)
	}
	return c
}

// SetTimeout overrides the per-operation timeout.
func (c *Client) SetTimeout(d time.Duration) {
	if d > 0 {
		c.timeout = d
	}
}

// transportFor caches one transport per agent, because the TLS configuration is
// pinned to that agent's certificate and cannot be shared between them.
func (c *Client) transportFor(agentID string) *http.Transport {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.transports[agentID]; ok {
		return t
	}
	t := &http.Transport{
		TLSClientConfig:     c.authority.ClientTLSConfig(agentID),
		TLSHandshakeTimeout: 15 * time.Second,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     90 * time.Second,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}
	c.transports[agentID] = t
	return t
}

// Forget drops the cached transport for an agent. Call it after revoking or
// re-enrolling, so no pooled connection outlives the decision that ended it.
func (c *Client) Forget(agentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.transports[agentID]; ok {
		t.CloseIdleConnections()
		delete(c.transports, agentID)
	}
}

// Call runs one named operation and decodes its result into out, which may be
// nil when the result is not needed.
func (c *Client) Call(ctx context.Context, target Target, operation string, args any, out any) error {
	if target.AgentID == "" {
		return fmt.Errorf("agentclient: an agent id is required, so the certificate can be pinned")
	}
	endpoint, err := operationURL(target.Address, operation)
	if err != nil {
		return err
	}
	body := []byte("{}")
	if args != nil {
		body, err = json.Marshal(args)
		if err != nil {
			return err
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Transport: c.transportFor(target.AgentID)}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("agentclient: %s on %s failed: %w", operation, target.AgentID, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	var envelope Response
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("agentclient: %s on %s returned status %d and an unreadable body",
			operation, target.AgentID, resp.StatusCode)
	}
	if !envelope.OK {
		return fmt.Errorf("agentclient: %s on %s was refused: %s", operation, target.AgentID, envelope.Error)
	}
	if out == nil || len(envelope.Result) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}

func operationURL(address, operation string) (string, error) {
	host := strings.TrimSpace(address)
	if host == "" {
		return "", fmt.Errorf("agentclient: no address for the agent")
	}
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	host = strings.TrimSuffix(host, "/")
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, fmt.Sprint(DefaultPort))
	}
	if operation == "" || strings.ContainsAny(operation, "/?#") {
		return "", fmt.Errorf("agentclient: %q is not an operation name", operation)
	}
	return "https://" + host + "/v1/ops/" + url.PathEscape(operation), nil
}

// ============================================================
// THE NAMED OPERATIONS
// ============================================================

// SystemInfo mirrors the agent's static host report.
type SystemInfo struct {
	Hostname  string  `json:"hostname"`
	OS        string  `json:"os"`
	OSPretty  string  `json:"os_pretty"`
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

// Metrics mirrors the agent's resource usage report.
type Metrics struct {
	CPUPercent float64 `json:"cpu_percent"`
	RAMUsed    int64   `json:"ram_used"`
	RAMTotal   int64   `json:"ram_total"`
	DiskUsed   int64   `json:"disk_used"`
	DiskTotal  int64   `json:"disk_total"`
	NetIn      int64   `json:"net_in"`
	NetOut     int64   `json:"net_out"`
}

// ServiceState is one managed unit and its status.
type ServiceState struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// AgentInfo is what an agent reports about itself.
type AgentInfo struct {
	Version        string    `json:"version"`
	AgentID        string    `json:"agent_id"`
	Hostname       string    `json:"hostname"`
	CertNotAfter   time.Time `json:"cert_not_after"`
	CertSerial     string    `json:"cert_serial"`
	DeniedSerials  int       `json:"denied_serials"`
	RawExecEnabled bool      `json:"raw_exec_enabled"`
}

// SystemInfo asks a host to describe itself.
func (c *Client) SystemInfo(ctx context.Context, target Target) (SystemInfo, error) {
	var out SystemInfo
	err := c.Call(ctx, target, "system.info", nil, &out)
	return out, err
}

// Metrics asks a host for current resource usage.
func (c *Client) Metrics(ctx context.Context, target Target) (Metrics, error) {
	var out Metrics
	err := c.Call(ctx, target, "system.metrics", nil, &out)
	return out, err
}

// ServiceList reports the status of every service the agent manages.
func (c *Client) ServiceList(ctx context.Context, target Target) ([]ServiceState, error) {
	var out []ServiceState
	err := c.Call(ctx, target, "service.list", nil, &out)
	return out, err
}

// ServiceStatus reports one service.
func (c *Client) ServiceStatus(ctx context.Context, target Target, name string) (ServiceState, error) {
	var out ServiceState
	err := c.Call(ctx, target, "service.status", map[string]string{"name": name}, &out)
	return out, err
}

// ServiceControl starts, stops, restarts or reloads one service. The agent
// refuses any name outside its own allow list and any verb outside those four,
// so a bad value here is a refusal rather than an arbitrary command.
func (c *Client) ServiceControl(ctx context.Context, target Target, name, action string) (ServiceState, error) {
	var out ServiceState
	err := c.Call(ctx, target, "service.control", map[string]string{"name": name, "action": action}, &out)
	return out, err
}

// AgentInfo asks an agent about its own identity and certificate.
func (c *Client) AgentInfo(ctx context.Context, target Target) (AgentInfo, error) {
	var out AgentInfo
	err := c.Call(ctx, target, "agent.info", nil, &out)
	return out, err
}

// AgentChannel reports which channel this client can talk to a server over.
// The panel client speaks mutual TLS and nothing else: a server that has not
// enrolled cannot be called at all, which is deliberate - the old channel is a
// path for an agent to reach the panel while it is being migrated, never a path
// for the panel to run something as root on a machine that has no certificate.
func (c *Client) AgentChannel() string { return agentpki.ChannelMutualTLS }

// SyncPKI pushes the current deny list to an agent, so a revoked panel
// certificate stops being accepted there without waiting for it to expire.
func (c *Client) SyncPKI(ctx context.Context, target Target) error {
	serials, err := c.authority.DeniedSerials(ctx)
	if err != nil {
		return err
	}
	return c.Call(ctx, target, "pki.sync", map[string]any{"denied_serials": serials}, nil)
}
