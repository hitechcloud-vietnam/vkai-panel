package ops

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
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
