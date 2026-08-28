package reload

// Proving the panel is still reachable after a change.
//
// A confirmation dialogue is not protection. When an operator is asked to
// confirm "this may make the panel unreachable", they are being asked about a
// network they cannot see from inside a browser tab: whether the new port is
// free, whether a firewall admits it, whether their own address is really the
// one the allow list will see. They click yes because there is no other button.
//
// So the panel checks instead. After a change is committed it proves, from
// inside this process, that the panel is still answering - and undoes the
// change automatically when it is not. What can be proven from here is
// deliberately bounded, and the bound is stated rather than hidden:
//
//	proven      the new listener is bound, accepting connections and serving
//	            this process's handler;
//	proven      the new access gate admits a request identical to the one that
//	            asked for the change - same address, same host, same entrance;
//	not proven  that a firewall or a NAT in front of this machine still allows
//	            the new port. Nothing running on this host can prove that, and
//	            claiming to would be worse than saying so.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// Probe verifies a configuration that has just been committed.
type Probe interface {
	// Verify returns nil when the panel is still reachable. Any error rolls the
	// change back, so an error must mean "unreachable" and never "unsure".
	Verify(ctx context.Context, next *config.PanelAccessConfig, req Request) error
}

// ProbeTimeout bounds one verification. It is short on purpose: an operator is
// holding an HTTP request open waiting for the answer, and the checks are all
// local.
const ProbeTimeout = 5 * time.Second

// probeAttempts and probeBackoff spread the check over the moment a fresh
// listener needs to become ready. A single-shot check would roll back a
// perfectly good change because it arrived a millisecond early.
const (
	probeAttempts = 4
	probeBackoff  = 150 * time.Millisecond
)

// defaultProbe accepts everything. It is what a supervisor with no listener
// gets - a unit test, or a build that only persists configuration - and it is
// deliberately explicit rather than a nil check scattered through Apply.
type defaultProbe struct{}

func (defaultProbe) Verify(context.Context, *config.PanelAccessConfig, Request) error { return nil }

// Probes runs several checks and fails on the first one that does.
type Probes []Probe

func (p Probes) Verify(ctx context.Context, next *config.PanelAccessConfig, req Request) error {
	for _, probe := range p {
		if probe == nil {
			continue
		}
		if err := probe.Verify(ctx, next, req); err != nil {
			return err
		}
	}
	return nil
}

// SetProbe installs the verification used by Apply. It is separate from New
// because the probes are the appliers themselves: the listener proves it is
// listening and the gate proves it still admits the caller, and both exist only
// after they have been registered.
func (s *Supervisor) SetProbe(p Probe) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p == nil {
		p = defaultProbe{}
	}
	s.probe = p
}

// retry runs a check until it passes or the attempts run out.
func retry(ctx context.Context, check func() error) error {
	deadline, cancel := context.WithTimeout(context.WithoutCancel(ctx), ProbeTimeout)
	defer cancel()

	var err error
	for attempt := 0; attempt < probeAttempts; attempt++ {
		if err = check(); err == nil {
			return nil
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("%w (gave up after %s)", err, ProbeTimeout)
		case <-time.After(probeBackoff):
		}
	}
	return err
}

// errNoListener is what a probe reports when there is nothing bound at all,
// which is the one failure that is never a false alarm.
var errNoListener = errors.New("the panel has no listener bound")
