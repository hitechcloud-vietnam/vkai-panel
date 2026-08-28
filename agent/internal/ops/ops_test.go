package ops

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/metrics"
)

// fakeRunner records what was executed instead of touching systemd.
type fakeRunner struct {
	calls  [][]string
	output string
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.output == "" {
		return []byte("active"), nil
	}
	return []byte(f.output), nil
}

func newRegistry(t *testing.T, allowRawExec bool) (*Registry, *fakeRunner) {
	t.Helper()
	runner := &fakeRunner{}
	return New(Deps{
		Run:           runner.run,
		AllowRawExec:  allowRawExec,
		Logger:        log.New(os.Stderr, "", 0),
		ApplyDenyList: func([]string) {},
		Info:          func() AgentInfo { return AgentInfo{Version: "test"} },
	}), runner
}

func TestArbitraryCommandExecutionIsAbsentByDefault(t *testing.T) {
	registry, _ := newRegistry(t, false)
	for _, name := range registry.Names() {
		if name == "exec.raw" {
			t.Fatal("exec.raw is registered without being enabled")
		}
	}
	_, err := registry.Dispatch(context.Background(), "exec.raw",
		json.RawMessage(`{"command":"id"}`))
	if !errors.Is(err, ErrUnknownOperation) {
		t.Fatalf("exec.raw returned %v, want ErrUnknownOperation", err)
	}
}

func TestArbitraryCommandExecutionAppearsOnlyWhenEnabled(t *testing.T) {
	registry, _ := newRegistry(t, true)
	found := false
	for _, name := range registry.Names() {
		if name == "exec.raw" {
			found = true
		}
	}
	if !found {
		t.Fatal("exec.raw was enabled but is not registered")
	}
}

func TestServiceControlRefusesAServiceItDoesNotManage(t *testing.T) {
	registry, runner := newRegistry(t, false)
	for _, name := range []string{
		"sshd",             // real unit, but not one the panel manages
		"nginx; rm -rf /",  // an injection attempt
		"../../etc/passwd", // a path, not a unit
		"nginx nginx",      // two arguments smuggled as one
		"",                 // nothing at all
	} {
		args, _ := json.Marshal(ServiceControlArgs{Name: name, Action: "restart"})
		if _, err := registry.Dispatch(context.Background(), "service.control", args); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("service.control accepted %q (error was %v)", name, err)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("a refused service.control still ran something: %v", runner.calls)
	}
}

func TestServiceControlRefusesAnActionItDoesNotOffer(t *testing.T) {
	registry, runner := newRegistry(t, false)
	for _, action := range []string{"mask", "disable", "enable", "restart; id", ""} {
		args, _ := json.Marshal(ServiceControlArgs{Name: "nginx", Action: action})
		if _, err := registry.Dispatch(context.Background(), "service.control", args); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("service.control accepted the action %q (error was %v)", action, err)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("a refused action still ran something: %v", runner.calls)
	}
}

func TestServiceControlRunsExactlyOneFixedCommand(t *testing.T) {
	registry, runner := newRegistry(t, false)
	args, _ := json.Marshal(ServiceControlArgs{Name: "nginx", Action: "restart"})
	if _, err := registry.Dispatch(context.Background(), "service.control", args); err != nil {
		t.Fatalf("service.control failed: %v", err)
	}
	if len(runner.calls) == 0 {
		t.Fatal("service.control ran nothing")
	}
	first := runner.calls[0]
	if strings.Join(first, " ") != "systemctl restart nginx" {
		t.Fatalf("service.control ran %v, want systemctl restart nginx", first)
	}
}

func TestVersionedPHPUnitsAreAccepted(t *testing.T) {
	registry, _ := newRegistry(t, false)
	args, _ := json.Marshal(ServiceArgs{Name: "php8.3-fpm"})
	if _, err := registry.Dispatch(context.Background(), "service.status", args); err != nil {
		t.Fatalf("service.status refused a versioned PHP-FPM unit: %v", err)
	}
}

func TestUnknownArgumentsAreRefusedRatherThanIgnored(t *testing.T) {
	registry, _ := newRegistry(t, false)
	_, err := registry.Dispatch(context.Background(), "service.status",
		json.RawMessage(`{"name":"nginx","sudo":true}`))
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("an unknown argument returned %v, want ErrInvalidArgument", err)
	}
}

func TestUnknownOperationIsRefused(t *testing.T) {
	registry, _ := newRegistry(t, false)
	if _, err := registry.Dispatch(context.Background(), "execute", nil); !errors.Is(err, ErrUnknownOperation) {
		t.Fatalf("an unknown operation returned %v, want ErrUnknownOperation", err)
	}
}

func TestOperationsWithNoArgumentsAcceptAnEmptyBody(t *testing.T) {
	registry, _ := newRegistry(t, false)
	if _, err := registry.Dispatch(context.Background(), "agent.info", nil); err != nil {
		t.Fatalf("agent.info with no body failed: %v", err)
	}
}

func TestPKISyncPassesTheDenyListOn(t *testing.T) {
	var received []string
	registry := New(Deps{
		Run:           (&fakeRunner{}).run,
		Logger:        log.New(os.Stderr, "", 0),
		ApplyDenyList: func(serials []string) { received = serials },
		Info:          func() AgentInfo { return AgentInfo{} },
	})
	if _, err := registry.Dispatch(context.Background(), "pki.sync",
		json.RawMessage(`{"denied_serials":["aa","bb"]}`)); err != nil {
		t.Fatalf("pki.sync failed: %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("pki.sync passed on %v, want two serials", received)
	}
}

// ============================================================
// METRICS THROUGH THE OPERATION SURFACE
// ============================================================

// A dashboard showing 0% CPU because collection failed is worse than one
// showing a gap: nobody investigates a quiet machine. The panel must be able to
// tell the two apart from the operation's result alone.
func TestSystemMetricsReportsAnUncollectableHostAsUnavailableAndNotAsZero(t *testing.T) {
	// A /proc that is not there. Every group fails to collect.
	broken := &metrics.Collector{
		ProcRoot:    filepath.Join(t.TempDir(), "no-proc"),
		CPUInterval: time.Millisecond,
	}
	registry := New(Deps{
		Run:           (&fakeRunner{}).run,
		Logger:        log.New(io.Discard, "", 0),
		Collector:     broken,
		ApplyDenyList: func([]string) {},
		Info:          func() AgentInfo { return AgentInfo{} },
	})

	result, err := registry.Dispatch(context.Background(), "system.metrics", nil)
	if err != nil {
		t.Fatalf("system.metrics failed outright instead of degrading: %v", err)
	}
	reported := result.(Metrics)

	if len(reported.Unavailable) == 0 {
		t.Fatal("a host whose /proc cannot be read reported no unavailable metrics")
	}
	if reported.CPUPercent != nil {
		t.Fatalf("an uncollectable CPU was reported as %.2f%%", *reported.CPUPercent)
	}
	if reported.RAMTotal != nil || reported.RAMUsed != nil || reported.DiskTotal != nil {
		t.Fatalf("uncollectable metrics were reported as numbers: %+v", reported)
	}

	// What the panel actually receives.
	encoded, err := json.Marshal(reported)
	if err != nil {
		t.Fatalf("cannot encode the result: %v", err)
	}
	for _, forbidden := range []string{`"cpu_percent"`, `"ram_used"`, `"disk_used"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("the encoded result carries %s for a metric that was never collected: %s",
				forbidden, encoded)
		}
	}
	if !strings.Contains(string(encoded), `"unavailable"`) {
		t.Fatalf("the encoded result does not name what could not be collected: %s", encoded)
	}
}

// The named operations the panel drives a node with. This is the list, and it
// is asserted rather than described so that removing one is a test failure
// rather than a silent capability loss.
func TestTheOperationSurfaceIsExactlyTheNamedOperations(t *testing.T) {
	registry, _ := newRegistry(t, false)
	want := []string{
		"agent.info",
		"disk.usage",
		"log.list",
		"log.read",
		"pki.sync",
		"service.control",
		"service.list",
		"service.status",
		"system.info",
		"system.metrics",
	}
	got := registry.Names()
	if len(got) != len(want) {
		t.Fatalf("the agent offers %v, want %v", got, want)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("the agent offers %v, want %v", got, want)
		}
	}
}

// Every operation must refuse an argument object it does not understand, so a
// panel talking to the wrong agent version is told rather than having an
// argument silently ignored.
func TestEveryOperationRefusesAnArgumentItDoesNotImplement(t *testing.T) {
	registry, _ := newRegistry(t, false)
	for _, name := range registry.Names() {
		_, err := registry.Dispatch(context.Background(), name,
			json.RawMessage(`{"definitely_not_an_argument":true}`))
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("%s accepted an argument it does not implement (error was %v)", name, err)
		}
	}
}
