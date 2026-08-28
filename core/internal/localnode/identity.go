package localnode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// IdentityFileName is the file under the panel's etc directory that holds the
// node identity. It sits beside .env and config.yaml because it is
// installation state, not a certificate and not a secret worth rotating.
const IdentityFileName = "node.json"

// fingerprintLabel is the domain separation prefix of the machine witness. It
// is deliberately a constant rather than the node id: a fingerprint that
// depended on the node id could not be used to find the node's own row again
// after node.json was lost, which is the one recovery this file supports.
const fingerprintLabel = "vkai-panel:node-fingerprint:v1:"

// Where the machine id is read from, in order. The dbus copy is the fallback
// for a system where systemd did not populate /etc/machine-id.
var machineIDSources = []struct {
	Path string
	Name string
}{
	{Path: "/etc/machine-id", Name: "machine-id"},
	{Path: "/var/lib/dbus/machine-id", Name: "dbus-machine-id"},
}

var (
	// ErrNoIdentity means this machine has never been registered: there is no
	// node.json. It is not a failure on the read path - a panel that has not
	// been registered simply has no local node - so callers test for it.
	ErrNoIdentity = errors.New("localnode: this machine has no node identity yet")

	// ErrIdentityMismatch means a node identity exists but does not describe
	// the machine this process is running on. It is the refusal that keeps a
	// local command from running on the belief that a restored row is local.
	ErrIdentityMismatch = errors.New("localnode: the stored node identity does not belong to this machine")
)

// Identity is what node.json holds: which node this machine is, and the
// witness that it was this machine when the identity was written.
type Identity struct {
	// NodeID is the stable key. It is also servers.id for this node's row.
	NodeID uuid.UUID `json:"node_id"`

	// Fingerprint is the salted hash of the machine id as it was at the moment
	// the identity was written or last re-bound. Empty when no machine id could
	// be read, which is a system the witness cannot cover.
	Fingerprint string `json:"machine_fingerprint,omitempty"`

	// FingerprintSource names which file the witness came from, so an operator
	// reading a mismatch knows what changed.
	FingerprintSource string `json:"fingerprint_source,omitempty"`

	CreatedAt time.Time `json:"created_at"`

	// Path is where this identity was read from or written to. It is not part
	// of the file.
	Path string `json:"-"`

	// Created reports that this call wrote the file, rather than reading one
	// that already existed. Registration uses it to decide whether it may adopt
	// an existing row.
	Created bool `json:"-"`
}

// MachineWitness is the live answer, measured now, to "which machine is this".
type MachineWitness struct {
	Fingerprint string
	Source      string
}

// Available reports whether a witness could be formed at all on this system.
func (w MachineWitness) Available() bool { return w.Fingerprint != "" }

// ReadMachineWitness hashes this machine's machine id. It returns an empty
// witness rather than an error when no machine id file can be read: that is a
// system the witness cannot cover, and the callers have to distinguish it from
// a witness that disagrees.
func ReadMachineWitness() MachineWitness {
	for _, source := range machineIDSources {
		data, err := os.ReadFile(source.Path)
		if err != nil {
			continue
		}
		id := strings.TrimSpace(string(data))
		if id == "" {
			continue
		}
		return MachineWitness{Fingerprint: fingerprintOf(id), Source: source.Name}
	}
	return MachineWitness{}
}

// fingerprintOf hashes one machine id. The raw value never leaves this
// function: it is not written to node.json, not written to the database and not
// logged, because systemd documents the machine id as a value that should not
// be exposed and a database dump should not be where it leaks.
func fingerprintOf(machineID string) string {
	sum := sha256.Sum256([]byte(fingerprintLabel + machineID))
	return hex.EncodeToString(sum[:])
}

// IdentityPath is where the identity file lives under a given etc directory.
func IdentityPath(etcRoot string) string {
	return filepath.Join(etcRoot, IdentityFileName)
}

// LoadIdentity reads the identity of this machine without creating one. It
// returns ErrNoIdentity when the machine has never been registered.
func LoadIdentity(etcRoot string) (*Identity, error) {
	path := IdentityPath(etcRoot)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoIdentity
	}
	if err != nil {
		return nil, fmt.Errorf("localnode: cannot read %s: %w", path, err)
	}
	var identity Identity
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, fmt.Errorf("localnode: %s is not readable as a node identity: %w", path, err)
	}
	if identity.NodeID == uuid.Nil {
		return nil, fmt.Errorf("localnode: %s carries no node id", path)
	}
	identity.Path = path
	return &identity, nil
}

// LoadOrCreateIdentity is the registration path: it returns the existing
// identity, or generates one and persists it. The generated file is the only
// record of which node this machine is, so it is written through a temporary
// file and a rename - a crash during the write leaves either the old identity
// or none, never half of one.
func LoadOrCreateIdentity(etcRoot string) (*Identity, error) {
	identity, err := LoadIdentity(etcRoot)
	switch {
	case err == nil:
		return identity, nil
	case !errors.Is(err, ErrNoIdentity):
		return nil, err
	}

	witness := ReadMachineWitness()
	fresh := &Identity{
		NodeID:            uuid.New(),
		Fingerprint:       witness.Fingerprint,
		FingerprintSource: witness.Source,
		CreatedAt:         time.Now().UTC(),
		Path:              IdentityPath(etcRoot),
		Created:           true,
	}
	if err := fresh.write(etcRoot); err != nil {
		return nil, err
	}
	return fresh, nil
}

// SaveIdentity persists nodeID as this machine's identity, recording the
// machine witness measured now. It is how a node id decided elsewhere becomes
// this machine's, and there are exactly two callers.
//
// The first is adoption: node.json was lost - a rebuilt etc directory, a
// restore that skipped it - and the database still holds a node this same
// machine registered. Generating a fresh identity there would fork one machine
// into two rows, so registration looks the node up by witness and saves the id
// it finds.
//
// The second is a rebind: the panel's etc directory was restored onto a rebuilt
// machine, so the node is the same node and the hardware underneath it is new.
// That one is deliberately never automatic. The runtime verifies and refuses;
// only an operator re-running registration on the machine, and asking for it,
// re-binds - because a rebind performed on a guess is exactly the mistake the
// witness exists to prevent.
func SaveIdentity(etcRoot string, nodeID uuid.UUID) (*Identity, error) {
	if nodeID == uuid.Nil {
		return nil, errors.New("localnode: refusing to save an empty node id")
	}
	witness := ReadMachineWitness()
	identity := &Identity{
		NodeID:            nodeID,
		Fingerprint:       witness.Fingerprint,
		FingerprintSource: witness.Source,
		CreatedAt:         time.Now().UTC(),
	}
	if existing, err := LoadIdentity(etcRoot); err == nil {
		// Keep the original registration time: the node is not new, only the
		// machine underneath it or the file describing it.
		identity.CreatedAt = existing.CreatedAt
	}
	if err := identity.write(etcRoot); err != nil {
		return nil, err
	}
	return identity, nil
}

func (i *Identity) write(etcRoot string) error {
	if err := os.MkdirAll(etcRoot, 0o700); err != nil {
		return fmt.Errorf("localnode: cannot create %s: %w", etcRoot, err)
	}
	data, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := IdentityPath(etcRoot)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("localnode: cannot write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("localnode: cannot install %s: %w", path, err)
	}
	i.Path = path
	return nil
}

// Verify decides whether this identity describes the machine the caller is
// running on, given the witness measured now.
func (i *Identity) Verify(witness MachineWitness) (verified bool, err error) {
	verified, err = CompareWitness(i.Fingerprint, i.FingerprintSource, witness)
	if err != nil {
		return false, fmt.Errorf("%w (node %s, recorded in %s)", err, i.NodeID, i.Path)
	}
	return verified, nil
}

// CompareWitness weighs a recorded machine witness against the one measured
// now. It is the whole of the guard, kept as one function because the same
// comparison is made against node.json and against the database row, and the
// two must not be able to drift apart.
//
// The three outcomes are deliberate:
//
//   - both sides carry a witness and they agree: verified;
//   - they disagree, or one side has a witness and the other does not:
//     ErrIdentityMismatch, because something underneath the identity changed;
//   - neither side can produce a witness: no error, and verified is false. The
//     node id alone still stands, and the caller is told the evidence is weaker
//     rather than being handed a silent yes.
func CompareWitness(recorded, recordedSource string, live MachineWitness) (verified bool, err error) {
	switch {
	case recorded == "" && !live.Available():
		return false, nil
	case recorded == "":
		return false, fmt.Errorf("%w: this machine reports a %s identity, but none was recorded for the node",
			ErrIdentityMismatch, live.Source)
	case !live.Available():
		return false, fmt.Errorf("%w: a %s witness was recorded for the node, but this machine has none now",
			ErrIdentityMismatch, recordedSource)
	case recorded != live.Fingerprint:
		return false, fmt.Errorf("%w: the %s of this machine is not the one recorded for the node",
			ErrIdentityMismatch, live.Source)
	default:
		return true, nil
	}
}
