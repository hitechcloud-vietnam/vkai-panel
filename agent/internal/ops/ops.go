// Package ops is the agent's list of things the panel is allowed to ask for.
//
// It replaces a single endpoint, /execute, that took a command name and an
// argument list and ran them as root. That endpoint meant the panel and the
// agent were the same trust boundary: anyone who could authenticate to an agent
// had a root shell on the customer's server, and there was no second line of
// defence behind the credential. Compromising the panel compromised every
// managed host, completely, in one step.
//
// What replaces it is a closed set of named operations with typed arguments.
// Every operation validates its arguments before doing anything, no argument
// reaches a shell, and no operation takes a program name from the caller. The
// panel can ask "restart nginx"; it cannot ask "run this".
//
// The escape hatch, exec.raw, exists only because removing a capability
// outright can strand a deployment that depends on it. It is off unless
// VKAI_AGENT_ALLOW_RAW_EXEC is explicitly set, it announces itself at startup,
// and it logs every invocation with the full command line. It should be treated
// as a temporary measure with a date on it, not a setting.
package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/audit"
	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/metrics"
)

// Response is the envelope every operation returns. It is deliberately explicit
// rather than relying on the HTTP status alone, so a caller that reads only the
// body still knows whether the operation ran.
type Response struct {
	OK     bool        `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// Handler runs one operation. It receives the raw argument object so each
// operation can decode into its own typed struct.
type Handler func(ctx context.Context, args json.RawMessage) (interface{}, error)

// Operation is one named entry in the registry.
type Operation struct {
	Name    string
	Summary string
	Handler Handler
}

// ErrUnknownOperation is returned for a name that is not registered. The panel
// asking for something that does not exist is a version mismatch, not an
// attack, so it is reported plainly.
var ErrUnknownOperation = errors.New("ops: unknown operation")

// ErrInvalidArgument covers every argument that fails validation.
var ErrInvalidArgument = errors.New("ops: invalid argument")

// CommandRunner runs one external program. It exists so the tests can drive the
// operations without touching systemd.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// AgentInfo is what agent.info reports.
type AgentInfo struct {
	Version        string    `json:"version"`
	AgentID        string    `json:"agent_id"`
	Hostname       string    `json:"hostname"`
	CertNotAfter   time.Time `json:"cert_not_after"`
	CertSerial     string    `json:"cert_serial"`
	DeniedSerials  int       `json:"denied_serials"`
	RawExecEnabled bool      `json:"raw_exec_enabled"`
}

// Deps is everything the registry needs from the rest of the agent.
type Deps struct {
	Run           CommandRunner
	Info          func() AgentInfo
	ApplyDenyList func([]string)
	AllowRawExec  bool
	Logger        *log.Logger

	// Collector reads the host. Sharing one collector between the periodic
	// report and the panel's on-demand system.metrics call is what lets a CPU
	// percentage be measured against the previous sample instead of against a
	// sub-second sleep taken inside the request.
	Collector *metrics.Collector

	// Audit is the node's own record of what was asked and what was done. It is
	// written here, on the machine the work happened on, so an operator can
	// audit this agent without trusting the panel that drove it.
	Audit *audit.Log

	// LogRoots and DiskRoots bound what log.read and disk.usage may touch.
	// Empty means the defaults.
	LogRoots  []string
	DiskRoots []string
}

// Registry is the closed set of operations this agent will perform.
type Registry struct {
	ops      map[string]Operation
	deps     Deps
	logger   *log.Logger
	logDirs  []string
	dataDirs []string
}

// New builds the registry. exec.raw is registered only when it was explicitly
// enabled, so an operation that is off is not merely refused: it is absent.
func New(deps Deps) *Registry {
	if deps.Run == nil {
		deps.Run = defaultRunner
	}
	if deps.Logger == nil {
		deps.Logger = log.Default()
	}
	if deps.Collector == nil {
		deps.Collector = metrics.NewCollector()
	}
	r := &Registry{
		ops:      make(map[string]Operation),
		deps:     deps,
		logger:   deps.Logger,
		logDirs:  sanitiseRoots(deps.LogRoots, DefaultLogRoots),
		dataDirs: sanitiseRoots(deps.DiskRoots, DefaultDiskRoots),
	}

	r.register("system.info", "static facts about the host", r.systemInfo)
	r.register("system.metrics", "current resource usage, per mount and per core", r.systemMetrics)
	r.register("service.list", "status of the services the panel manages", r.serviceList)
	r.register("service.status", "status of one named service", r.serviceStatus)
	r.register("service.control", "start, stop, restart or reload one named service", r.serviceControl)
	r.register("log.list", "the log files this agent will read", r.logList)
	r.register("log.read", "the last lines of one log file", r.logRead)
	r.register("disk.usage", "filesystem capacity, and optionally the size of one path", r.diskUsage)
	r.register("agent.info", "the agent's own identity and certificate", r.agentInfo)
	r.register("pki.sync", "accept the panel's revoked certificate list", r.pkiSync)

	if deps.AllowRawExec {
		r.register("exec.raw", "DEPRECATED escape hatch: run an arbitrary command", r.execRaw)
		deps.Logger.Printf("WARNING: exec.raw is ENABLED. Any caller holding a valid panel " +
			"certificate can run arbitrary commands as root on this host. " +
			"Unset VKAI_AGENT_ALLOW_RAW_EXEC as soon as the operations that need it are named.")
	}
	return r
}

func (r *Registry) register(name, summary string, h Handler) {
	r.ops[name] = Operation{Name: name, Summary: summary, Handler: h}
}

// Names lists the registered operations, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.ops))
	for name := range r.ops {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Dispatch runs one operation by name.
func (r *Registry) Dispatch(ctx context.Context, name string, args json.RawMessage) (interface{}, error) {
	op, ok := r.ops[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownOperation, name)
	}
	return op.Handler(ctx, args)
}

// ============================================================
// SYSTEM
// ============================================================

func (r *Registry) systemInfo(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	if err := noArguments(raw); err != nil {
		return nil, err
	}
	return CollectSystemInfo(ctx, r.collector()), nil
}

func (r *Registry) systemMetrics(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	if err := noArguments(raw); err != nil {
		return nil, err
	}
	return CollectMetrics(ctx, r.collector()), nil
}

// ============================================================
// SERVICES
// ============================================================

// managedServices is the closed set of unit names the panel may ask about. A
// name outside it is refused, so "restart this service" can never become
// "restart the thing that happens to be named like a path".
var managedServices = map[string]bool{
	"nginx":         true,
	"apache2":       true,
	"httpd":         true,
	"openlitespeed": true,
	"lshttpd":       true,
	"caddy":         true,
	"mariadb":       true,
	"mysql":         true,
	"mysqld":        true,
	"postgresql":    true,
	"redis":         true,
	"redis-server":  true,
	"docker":        true,
	"memcached":     true,
	"vkai-agent":    true,
}

// phpFPMUnit matches the versioned PHP-FPM units, which cannot be listed
// exhaustively because the set depends on what is installed.
var phpFPMUnit = regexp.MustCompile(`^php[0-9]+(\.[0-9]+)?-fpm$`)

// unitNameShape is the first gate: anything that is not a plain systemd unit
// name is rejected before the allow list is even consulted.
var unitNameShape = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9@._-]{0,63}$`)

func validService(name string) error {
	if !unitNameShape.MatchString(name) {
		return fmt.Errorf("%w: %q is not a service name", ErrInvalidArgument, name)
	}
	if managedServices[name] || phpFPMUnit.MatchString(name) {
		return nil
	}
	return fmt.Errorf("%w: %q is not a service this agent manages", ErrInvalidArgument, name)
}

// ServiceArgs is the argument object of service.status.
type ServiceArgs struct {
	Name string `json:"name"`
}

// ServiceControlArgs is the argument object of service.control.
type ServiceControlArgs struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

// ServiceState is what both service operations return.
type ServiceState struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// controlActions is the closed set of verbs. "enable" and "disable" are absent
// deliberately: changing what starts at boot is a change to the host's
// configuration, not an operation, and it belongs behind its own named call
// when there is a reason for one.
var controlActions = map[string]bool{
	"start":   true,
	"stop":    true,
	"restart": true,
	"reload":  true,
}

func (r *Registry) serviceStatus(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	var args ServiceArgs
	if err := decode(raw, &args); err != nil {
		return nil, err
	}
	if err := validService(args.Name); err != nil {
		return nil, err
	}
	return ServiceState{Name: args.Name, Status: serviceStatus(ctx, r.deps.Run, args.Name)}, nil
}

func (r *Registry) serviceControl(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	var args ServiceControlArgs
	if err := decode(raw, &args); err != nil {
		return nil, err
	}
	if err := validService(args.Name); err != nil {
		return nil, err
	}
	action := strings.ToLower(strings.TrimSpace(args.Action))
	if !controlActions[action] {
		return nil, fmt.Errorf("%w: %q is not one of start, stop, restart, reload", ErrInvalidArgument, args.Action)
	}
	// Recorded before the command runs, not after: an operation that hangs, or
	// that takes the machine down with it, must still leave evidence of who
	// asked for it.
	r.record(ctx, "service.control", audit.OutcomeExecuted, audit.Argv("systemctl", action, args.Name))
	r.logger.Printf("service.control: %s %s", action, args.Name)
	if _, err := r.deps.Run(ctx, "systemctl", action, args.Name); err != nil {
		return nil, fmt.Errorf("ops: systemctl %s %s failed: %w", action, args.Name, err)
	}
	return ServiceState{Name: args.Name, Status: serviceStatus(ctx, r.deps.Run, args.Name)}, nil
}

func (r *Registry) serviceList(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	if err := noArguments(raw); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(managedServices))
	for name := range managedServices {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ServiceState, 0, len(names))
	for _, name := range names {
		out = append(out, ServiceState{Name: name, Status: serviceStatus(ctx, r.deps.Run, name)})
	}
	return out, nil
}

func serviceStatus(ctx context.Context, run CommandRunner, name string) string {
	out, err := run(ctx, "systemctl", "is-active", name)
	status := strings.TrimSpace(string(out))
	if status == "" {
		if err != nil {
			return "unknown"
		}
		return "inactive"
	}
	return status
}

// ============================================================
// AGENT AND PKI
// ============================================================

func (r *Registry) agentInfo(_ context.Context, raw json.RawMessage) (interface{}, error) {
	if err := noArguments(raw); err != nil {
		return nil, err
	}
	if r.deps.Info == nil {
		return nil, errors.New("ops: this agent cannot report its identity")
	}
	return r.deps.Info(), nil
}

// PKISyncArgs is the argument object of pki.sync.
type PKISyncArgs struct {
	DeniedSerials []string `json:"denied_serials"`
}

func (r *Registry) pkiSync(_ context.Context, raw json.RawMessage) (interface{}, error) {
	var args PKISyncArgs
	if err := decode(raw, &args); err != nil {
		return nil, err
	}
	if r.deps.ApplyDenyList == nil {
		return nil, errors.New("ops: this agent cannot accept a deny list")
	}
	r.deps.ApplyDenyList(args.DeniedSerials)
	return map[string]int{"denied_serials": len(args.DeniedSerials)}, nil
}

// ============================================================
// THE ESCAPE HATCH
// ============================================================

// RawExecArgs is the argument object of exec.raw.
type RawExecArgs struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout"`
}

// RawExecResult is what exec.raw returns.
type RawExecResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

func (r *Registry) execRaw(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	var args RawExecArgs
	if err := decode(raw, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Command) == "" {
		return nil, fmt.Errorf("%w: command is required", ErrInvalidArgument)
	}
	timeout := 30 * time.Second
	if args.Timeout > 0 && args.Timeout <= 600 {
		timeout = time.Duration(args.Timeout) * time.Second
	}
	// Loud on purpose. If this line is in the journal, someone should be able
	// to say why.
	r.logger.Printf("SECURITY: exec.raw invoked: command=%q args=%q", args.Command, args.Args)
	r.record(ctx, "exec.raw", audit.OutcomeExecuted, audit.Argv(args.Command, args.Args...))

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, args.Command, args.Args...)
	output, err := cmd.CombinedOutput()
	result := RawExecResult{Output: string(output), ExitCode: cmd.ProcessState.ExitCode()}
	if err != nil {
		result.Error = err.Error()
	}
	return result, nil
}

// ============================================================
// HELPERS
// ============================================================

// decode reads an argument object. An absent body is an empty object, so an
// operation that takes no arguments can be called with no body at all.
func decode(raw json.RawMessage, out interface{}) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	// An unknown field is a caller talking to the wrong agent version, and
	// silently ignoring it is how an argument gets dropped without anyone
	// noticing.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return nil
}

// noArguments is the decoder for an operation that takes none. It is not the
// same as ignoring the body: an operation that quietly discarded an argument
// would let a panel believe it had asked for something it had not, and the
// difference only ever shows up as a mystery in production.
func noArguments(raw json.RawMessage) error {
	return decode(raw, &struct{}{})
}

func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// collector returns the shared host collector.
func (r *Registry) collector() *metrics.Collector {
	if r.deps.Collector == nil {
		r.deps.Collector = metrics.NewCollector()
	}
	return r.deps.Collector
}

// logRoots and diskRoots are the directories the path-taking operations are
// confined to. log.read may only read within the first; disk.usage may only
// measure within the second.
func (r *Registry) logRoots() []string  { return r.logDirs }
func (r *Registry) diskRoots() []string { return r.dataDirs }

// record writes one line into the node's own operation record, naming the
// caller the control channel verified. An operation records what it actually
// did - the argument vector it ran, the path it resolved to and read - which is
// the part the request alone does not say.
func (r *Registry) record(ctx context.Context, operation, outcome, detail string) {
	if r.deps.Audit == nil {
		return
	}
	r.deps.Audit.Record(audit.Entry{
		Actor:     audit.ActorFrom(ctx),
		Operation: operation,
		Outcome:   outcome,
		Detail:    detail,
	})
}
