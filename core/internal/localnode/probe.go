package localnode

import (
	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// Probe is everything the service layer needs from this package: which node
// this machine is, and what it is made of. It is an interface so that the
// registration and verification logic above can be tested against a machine
// that does not exist, rather than against whatever host the test suite
// happens to run on.
type Probe interface {
	// Identity returns the node identity of this machine without creating one.
	// It returns ErrNoIdentity when the machine has never been registered.
	Identity() (*Identity, error)

	// EnsureIdentity returns the node identity, generating and persisting one
	// if this machine has never been registered. Only the registration path
	// calls it: everything on the read path must not be able to invent an
	// identity as a side effect of asking about one.
	EnsureIdentity() (*Identity, error)

	// Witness measures which machine this is, now.
	Witness() MachineWitness

	// Facts measures the host.
	Facts() (Facts, error)

	// SaveIdentity persists a node id as this machine's identity, with the
	// witness measured now. See localnode.SaveIdentity for its two callers.
	SaveIdentity(nodeID uuid.UUID) (*Identity, error)
}

// SystemProbe is the real machine.
type SystemProbe struct {
	// EtcRoot is where node.json lives. Empty means the panel's configured etc
	// directory, which is the only correct answer outside a test.
	EtcRoot string
}

var _ Probe = (*SystemProbe)(nil)

// NewSystemProbe reads the panel host through the panel's own path layout.
func NewSystemProbe() *SystemProbe { return &SystemProbe{} }

func (p *SystemProbe) etcRoot() string {
	if p.EtcRoot != "" {
		return p.EtcRoot
	}
	return config.EtcRoot()
}

func (p *SystemProbe) Identity() (*Identity, error) { return LoadIdentity(p.etcRoot()) }

func (p *SystemProbe) EnsureIdentity() (*Identity, error) { return LoadOrCreateIdentity(p.etcRoot()) }

func (p *SystemProbe) Witness() MachineWitness { return ReadMachineWitness() }

func (p *SystemProbe) Facts() (Facts, error) { return CollectFacts() }

func (p *SystemProbe) SaveIdentity(nodeID uuid.UUID) (*Identity, error) {
	return SaveIdentity(p.etcRoot(), nodeID)
}
