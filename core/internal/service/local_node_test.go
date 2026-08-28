package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/localnode"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// These tests are about the two claims the local node makes: that registering
// the same machine twice describes one node, and that the panel will not run a
// command here on the belief that a row is this machine unless it can prove it.
// Both are checked without a database and without being on any particular
// machine, because both have to hold on machines nobody has run this on.

// ============================================================
// A machine, and a database, that the tests can lie about
// ============================================================

type fakeNodeRow struct {
	server models.Server
	local  *repository.LocalNodeRecord
}

// fakeLocalNodeStore behaves like the upsert in repository/server.go: keyed on
// the node id, which is the server row's primary key.
type fakeLocalNodeStore struct {
	rows          map[uuid.UUID]*fakeNodeRow
	tenant        uuid.UUID
	registrations []repository.LocalNodeRegistration
	mismatches    []string
	touched       int
	missingTable  bool
}

func newFakeStore() *fakeLocalNodeStore {
	return &fakeLocalNodeStore{rows: map[uuid.UUID]*fakeNodeRow{}, tenant: uuid.New()}
}

func (f *fakeLocalNodeStore) RegisterLocalNode(_ context.Context, reg repository.LocalNodeRegistration) (*models.Server, bool, error) {
	f.registrations = append(f.registrations, reg)
	if f.missingTable {
		return nil, false, repository.ErrLocalNodeBookkeepingUnavailable
	}
	now := time.Now().UTC()
	row, existed := f.rows[reg.NodeID]
	if !existed {
		row = &fakeNodeRow{server: models.Server{
			ID:       reg.NodeID,
			TenantID: reg.TenantID,
			Role:     reg.Role,
			Status:   reg.Status,
		}}
		f.rows[reg.NodeID] = row
	}
	row.server.Hostname = reg.Facts.Hostname
	row.server.IPAddress = reg.Facts.IPAddress
	row.server.OS = derefString(reg.Facts.OS)
	row.server.Kernel = derefString(reg.Facts.Kernel)
	row.server.AgentStatus = "online"
	row.server.LastSeenAt = &now
	if row.local == nil {
		row.local = &repository.LocalNodeRecord{ServerID: reg.NodeID, RegisteredAt: now}
	}
	row.local.Hostname = reg.Facts.Hostname
	row.local.Fingerprint = derefString(reg.Fingerprint)
	row.local.FingerprintSource = derefString(reg.FingerprintSource)
	row.local.LastVerifiedAt = &now
	row.local.LastSeenAt = &now
	row.local.AgentStatus = "online"
	row.local.LastMismatchAt = nil
	row.local.LastMismatchReason = ""
	return &row.server, !existed, nil
}

func (f *fakeLocalNodeStore) GetLocalNode(_ context.Context, serverID uuid.UUID) (*repository.LocalNodeRecord, error) {
	if f.missingTable {
		return nil, repository.ErrLocalNodeBookkeepingUnavailable
	}
	row, ok := f.rows[serverID]
	if !ok || row.local == nil {
		return nil, sql.ErrNoRows
	}
	record := *row.local
	return &record, nil
}

func (f *fakeLocalNodeStore) FindLocalNodeByFingerprint(_ context.Context, fingerprint string) (*repository.LocalNodeRecord, error) {
	if f.missingTable {
		return nil, repository.ErrLocalNodeBookkeepingUnavailable
	}
	if fingerprint == "" {
		return nil, sql.ErrNoRows
	}
	for _, row := range f.rows {
		if row.local != nil && row.local.Fingerprint == fingerprint {
			record := *row.local
			return &record, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeLocalNodeStore) TouchLocalNode(_ context.Context, serverID uuid.UUID, at time.Time) error {
	row, ok := f.rows[serverID]
	if !ok || row.local == nil {
		return sql.ErrNoRows
	}
	f.touched++
	row.local.LastVerifiedAt = &at
	row.server.LastSeenAt = &at
	return nil
}

func (f *fakeLocalNodeStore) NoteLocalNodeMismatch(_ context.Context, serverID uuid.UUID, reason string, at time.Time) error {
	f.mismatches = append(f.mismatches, reason)
	if row, ok := f.rows[serverID]; ok && row.local != nil {
		row.local.LastMismatchAt = &at
		row.local.LastMismatchReason = reason
		row.server.AgentStatus = "offline"
	}
	return nil
}

func (f *fakeLocalNodeStore) DefaultTenantID(context.Context) (uuid.UUID, error) {
	return f.tenant, nil
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// fakeProbe is a machine the test controls: its identity file, its machine id,
// and what it can measure about itself.
type fakeProbe struct {
	identity *localnode.Identity
	witness  localnode.MachineWitness
	facts    localnode.Facts
	factsErr error
	saved    []uuid.UUID
}

func (p *fakeProbe) Identity() (*localnode.Identity, error) {
	if p.identity == nil {
		return nil, localnode.ErrNoIdentity
	}
	copied := *p.identity
	copied.Created = false
	return &copied, nil
}

func (p *fakeProbe) EnsureIdentity() (*localnode.Identity, error) {
	if p.identity == nil {
		p.identity = &localnode.Identity{
			NodeID:            uuid.New(),
			Fingerprint:       p.witness.Fingerprint,
			FingerprintSource: p.witness.Source,
			Path:              "/vkai-panel/etc/node.json",
		}
		copied := *p.identity
		copied.Created = true
		return &copied, nil
	}
	return p.Identity()
}

func (p *fakeProbe) Witness() localnode.MachineWitness { return p.witness }

func (p *fakeProbe) Facts() (localnode.Facts, error) { return p.facts, p.factsErr }

func (p *fakeProbe) SaveIdentity(nodeID uuid.UUID) (*localnode.Identity, error) {
	p.saved = append(p.saved, nodeID)
	p.identity = &localnode.Identity{
		NodeID:            nodeID,
		Fingerprint:       p.witness.Fingerprint,
		FingerprintSource: p.witness.Source,
		Path:              "/vkai-panel/etc/node.json",
	}
	return p.Identity()
}

func newLocalNodeService(store localNodeStore, probe localnode.Probe) *ServerService {
	return &ServerService{logger: zap.NewNop(), localNodes: store, probe: probe}
}

func machineFacts(hostname string) localnode.Facts {
	os := "Debian GNU/Linux 12 (bookworm)"
	kernel := "6.8.0-79-generic"
	cores := 4
	ram := int64(8 << 30)
	disk := int64(80 << 30)
	return localnode.Facts{
		Hostname:  hostname,
		IPAddress: "203.0.113.10",
		OS:        &os,
		Kernel:    &kernel,
		CPUCores:  &cores,
		RAMTotal:  &ram,
		DiskTotal: &disk,
	}
}

// ============================================================
// Registration
// ============================================================

func TestRegisteringThePanelHostTwiceProducesOneNode(t *testing.T) {
	store := newFakeStore()
	probe := &fakeProbe{
		witness: localnode.MachineWitness{Fingerprint: "fingerprint-of-this-machine", Source: "machine-id"},
		facts:   machineFacts("panel-host"),
	}
	svc := newLocalNodeService(store, probe)

	first, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{})
	if err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if !first.Created {
		t.Fatal("the first registration did not report creating the node")
	}

	second, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{})
	if err != nil {
		t.Fatalf("second registration: %v", err)
	}
	if second.Created {
		t.Fatal("registering the same machine again reported creating a second node")
	}
	if len(store.rows) != 1 {
		t.Fatalf("the machine was registered twice and produced %d nodes, want 1", len(store.rows))
	}
	if first.NodeID != second.NodeID {
		t.Fatalf("the same machine registered as two different nodes: %s then %s", first.NodeID, second.NodeID)
	}
}

func TestARenamedAndReaddressedMachineUpdatesItsNodeRatherThanDuplicatingIt(t *testing.T) {
	store := newFakeStore()
	probe := &fakeProbe{
		witness: localnode.MachineWitness{Fingerprint: "fingerprint-of-this-machine", Source: "machine-id"},
		facts:   machineFacts("panel-host"),
	}
	svc := newLocalNodeService(store, probe)

	before, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{})
	if err != nil {
		t.Fatalf("first registration: %v", err)
	}

	// The operator renames the host and the provider moves it to a new address.
	// Neither is the node's identity, which is exactly why they can change.
	renamed := machineFacts("panel-host-renamed")
	renamed.IPAddress = "198.51.100.42"
	probe.facts = renamed

	after, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{})
	if err != nil {
		t.Fatalf("second registration: %v", err)
	}
	if len(store.rows) != 1 {
		t.Fatalf("renaming the machine produced %d nodes, want 1", len(store.rows))
	}
	if after.NodeID != before.NodeID {
		t.Fatalf("renaming the machine changed its node id from %s to %s", before.NodeID, after.NodeID)
	}
	if after.Server.Hostname != "panel-host-renamed" {
		t.Fatalf("hostname=%q, want the new name", after.Server.Hostname)
	}
	if after.Server.IPAddress != "198.51.100.42" {
		t.Fatalf("ip_address=%q, want the new address", after.Server.IPAddress)
	}
}

func TestAFactThatCannotBeMeasuredIsStoredAsNullRatherThanAsAPlausibleValue(t *testing.T) {
	store := newFakeStore()
	// A container with no /etc/os-release, an unreadable /proc/meminfo and a
	// statfs that failed: the three facts are simply not known.
	facts := localnode.Facts{Hostname: "minimal", IPAddress: "10.0.0.5"}
	probe := &fakeProbe{
		witness: localnode.MachineWitness{Fingerprint: "fp", Source: "machine-id"},
		facts:   facts,
	}
	svc := newLocalNodeService(store, probe)

	if _, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{}); err != nil {
		t.Fatalf("registration: %v", err)
	}
	if len(store.registrations) != 1 {
		t.Fatalf("registrations=%d, want 1", len(store.registrations))
	}
	written := store.registrations[0].Facts
	if written.OS != nil {
		t.Fatalf("os was written as %q, but nothing could be read; it must stay null", *written.OS)
	}
	if written.Kernel != nil {
		t.Fatalf("kernel was written as %q, but nothing could be read; it must stay null", *written.Kernel)
	}
	if written.CPUCores != nil {
		t.Fatalf("cpu_cores was written as %d, but nothing could be read; it must stay null", *written.CPUCores)
	}
	if written.RAMTotal != nil {
		t.Fatalf("ram_total was written as %d, but nothing could be read; it must stay null", *written.RAMTotal)
	}
	if written.DiskTotal != nil {
		t.Fatalf("disk_total was written as %d, but nothing could be read; it must stay null", *written.DiskTotal)
	}
	if written.IPv6Address != nil {
		t.Fatalf("ipv6_address was written as %q on a host with no IPv6", *written.IPv6Address)
	}
}

func TestAMachineWhoseIdentityFileWasLostAdoptsItsOwnNodeInsteadOfCreatingASecond(t *testing.T) {
	store := newFakeStore()
	witness := localnode.MachineWitness{Fingerprint: "fingerprint-of-this-machine", Source: "machine-id"}
	probe := &fakeProbe{witness: witness, facts: machineFacts("panel-host")}
	svc := newLocalNodeService(store, probe)

	original, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{})
	if err != nil {
		t.Fatalf("first registration: %v", err)
	}

	// The etc directory is rebuilt without node.json - a restore that skipped
	// it, a reinstall over the top - and the machine underneath is the same.
	probe.identity = nil

	again, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{})
	if err != nil {
		t.Fatalf("second registration: %v", err)
	}
	if !again.Adopted {
		t.Fatal("the machine did not adopt the node it had already registered")
	}
	if again.NodeID != original.NodeID {
		t.Fatalf("the machine adopted node %s, want its own node %s", again.NodeID, original.NodeID)
	}
	if len(store.rows) != 1 {
		t.Fatalf("a lost identity file produced %d nodes, want 1", len(store.rows))
	}
}

func TestRegistrationRefusesToRebindANodeOntoADifferentMachineUnlessAsked(t *testing.T) {
	store := newFakeStore()
	probe := &fakeProbe{
		witness: localnode.MachineWitness{Fingerprint: "machine-a", Source: "machine-id"},
		facts:   machineFacts("panel-host"),
	}
	svc := newLocalNodeService(store, probe)
	if _, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{}); err != nil {
		t.Fatalf("first registration: %v", err)
	}

	// The panel's etc directory is copied to another machine, which then starts
	// the panel. node.json says it is the node; the hardware says otherwise.
	probe.witness = localnode.MachineWitness{Fingerprint: "machine-b", Source: "machine-id"}

	if _, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{}); err == nil {
		t.Fatal("a copied identity re-registered itself on different hardware with no operator involved")
	} else if !errors.Is(err, localnode.ErrIdentityMismatch) {
		t.Fatalf("error=%v, want an identity mismatch", err)
	}

	// The operator, standing on the rebuilt machine, says so explicitly.
	rebound, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{Rebind: true})
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if !rebound.Rebound || !rebound.Verified {
		t.Fatalf("rebind produced %+v, want a rebound and verified node", rebound)
	}
	if len(store.rows) != 1 {
		t.Fatalf("the rebind produced %d nodes, want 1", len(store.rows))
	}
}

// ============================================================
// The guard
// ============================================================

func TestTheLocalShortPathIsRefusedWhenTheIdentityDoesNotMatchThisMachine(t *testing.T) {
	store := newFakeStore()
	probe := &fakeProbe{
		witness: localnode.MachineWitness{Fingerprint: "machine-a", Source: "machine-id"},
		facts:   machineFacts("panel-host"),
	}
	svc := newLocalNodeService(store, probe)
	registered, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}

	route := svc.ResolveNodeRoute(context.Background(), registered.NodeID)
	if !route.Local || !route.Verified {
		t.Fatalf("on the machine that registered it, the route is %+v; want a verified local route", route)
	}

	// The database is restored onto a second machine, which has the panel's
	// configuration but is not the machine the node describes.
	probe.witness = localnode.MachineWitness{Fingerprint: "machine-b", Source: "machine-id"}

	route = svc.ResolveNodeRoute(context.Background(), registered.NodeID)
	if route.Local {
		t.Fatal("the panel offered the local short path on a machine that is not the node")
	}
	if !route.Mismatch {
		t.Fatal("the panel did not recognise the row as claiming to be this machine")
	}
	if len(store.mismatches) != 1 {
		t.Fatalf("mismatches recorded=%d, want 1 so an operator can find out", len(store.mismatches))
	}
	if !strings.Contains(store.mismatches[0], "machine-id") {
		t.Fatalf("the recorded reason %q does not say what disagreed", store.mismatches[0])
	}
}

func TestAnotherPanelsHostIsSimplyRemoteAndNotAMismatch(t *testing.T) {
	store := newFakeStore()
	probe := &fakeProbe{
		witness: localnode.MachineWitness{Fingerprint: "machine-a", Source: "machine-id"},
		facts:   machineFacts("panel-a"),
	}
	svc := newLocalNodeService(store, probe)
	if _, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{}); err != nil {
		t.Fatalf("registration: %v", err)
	}

	// A second panel, on its own machine, sharing this database.
	other := uuid.New()
	now := time.Now().UTC()
	store.rows[other] = &fakeNodeRow{
		server: models.Server{ID: other, Hostname: "panel-b"},
		local: &repository.LocalNodeRecord{
			ServerID: other, Hostname: "panel-b",
			Fingerprint: "machine-b", FingerprintSource: "machine-id",
			RegisteredAt: now, LastVerifiedAt: &now,
		},
	}

	route := svc.ResolveNodeRoute(context.Background(), other)
	if route.Local {
		t.Fatal("this panel claimed another panel's host as its own")
	}
	if route.Mismatch {
		t.Fatal("another panel's host was reported as a mismatch; it is simply not this machine")
	}
	if len(store.mismatches) != 0 {
		t.Fatalf("this panel wrote %d mismatch records onto a node that is not its own", len(store.mismatches))
	}
}

func TestARemoteNodeNeverTakesTheShortPath(t *testing.T) {
	store := newFakeStore()
	probe := &fakeProbe{witness: localnode.MachineWitness{Fingerprint: "machine-a", Source: "machine-id"}}
	svc := newLocalNodeService(store, probe)

	route := svc.ResolveNodeRoute(context.Background(), uuid.New())
	if route.Local || route.Mismatch {
		t.Fatalf("route=%+v, want a plain remote route", route)
	}
	if route.Reason == "" {
		t.Fatal("a refused route gave no reason")
	}
}

func TestAnUnmigratedInstallationHasNoLocalNodeRatherThanAFailure(t *testing.T) {
	store := newFakeStore()
	store.missingTable = true
	probe := &fakeProbe{
		identity: &localnode.Identity{NodeID: uuid.New(), Fingerprint: "machine-a", FingerprintSource: "machine-id"},
		witness:  localnode.MachineWitness{Fingerprint: "machine-a", Source: "machine-id"},
	}
	svc := newLocalNodeService(store, probe)

	route := svc.ResolveNodeRoute(context.Background(), probe.identity.NodeID)
	if route.Local {
		t.Fatal("a node was treated as local on an installation that cannot record local nodes")
	}
	if !strings.Contains(route.Reason, "local_node.sql") {
		t.Fatalf("reason=%q, want it to name the migration an operator has to run", route.Reason)
	}

	status, err := svc.LocalNodeStatus(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Registered || status.Healthy {
		t.Fatalf("status=%+v, want an unregistered, unhealthy local node", status)
	}
}

func TestAServiceWithNoLocalNodeWiringRefusesRatherThanPanicking(t *testing.T) {
	svc := &ServerService{logger: zap.NewNop()}
	if _, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{}); !errors.Is(err, ErrLocalNodeUnavailable) {
		t.Fatalf("err=%v, want ErrLocalNodeUnavailable", err)
	}
	if route := svc.ResolveNodeRoute(context.Background(), uuid.New()); route.Local {
		t.Fatal("a service with no wiring offered the local short path")
	}
	if err := svc.HeartbeatLocalNode(context.Background()); !errors.Is(err, ErrLocalNodeUnavailable) {
		t.Fatalf("err=%v, want ErrLocalNodeUnavailable", err)
	}
	status, err := svc.LocalNodeStatus(context.Background())
	if err != nil || status.Registered {
		t.Fatalf("status=%+v err=%v, want an unregistered local node and no error", status, err)
	}
}

// ============================================================
// Health
// ============================================================

func TestHealthFollowsTheHeartbeatRatherThanTheRowWrittenAtInstallTime(t *testing.T) {
	store := newFakeStore()
	probe := &fakeProbe{
		witness: localnode.MachineWitness{Fingerprint: "machine-a", Source: "machine-id"},
		facts:   machineFacts("panel-host"),
	}
	svc := newLocalNodeService(store, probe)
	registered, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}

	status, err := svc.LocalNodeStatus(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Healthy || !status.Local {
		t.Fatalf("status=%+v, want a healthy local node just after registration", status)
	}

	// Time passes with no panel running on the machine. The stored
	// agent_status still says 'online', because nothing was there to change it.
	stale := time.Now().UTC().Add(-10 * localNodeStaleAfter)
	store.rows[registered.NodeID].local.LastVerifiedAt = &stale
	if store.rows[registered.NodeID].server.AgentStatus != "online" {
		t.Fatal("the fixture no longer reproduces the case: the stored status has changed on its own")
	}

	status, err = svc.LocalNodeStatus(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Healthy {
		t.Fatal("a node no panel has stood on for hours was reported healthy because a column said 'online'")
	}
	if !strings.Contains(status.Reason, "proved") {
		t.Fatalf("reason=%q, want it to say that nothing has proved it recently", status.Reason)
	}

	// One beat, and it is current again.
	if err := svc.HeartbeatLocalNode(context.Background()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if store.touched != 1 {
		t.Fatalf("touched=%d, want 1", store.touched)
	}
	status, err = svc.LocalNodeStatus(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Healthy {
		t.Fatalf("status=%+v, want healthy after a heartbeat", status)
	}
}

func TestTheHeartbeatDoesNotLandOnAMachineThatIsNotTheNode(t *testing.T) {
	store := newFakeStore()
	probe := &fakeProbe{
		witness: localnode.MachineWitness{Fingerprint: "machine-a", Source: "machine-id"},
		facts:   machineFacts("panel-host"),
	}
	svc := newLocalNodeService(store, probe)
	if _, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{}); err != nil {
		t.Fatalf("registration: %v", err)
	}

	probe.witness = localnode.MachineWitness{Fingerprint: "machine-b", Source: "machine-id"}
	if err := svc.HeartbeatLocalNode(context.Background()); err == nil {
		t.Fatal("the heartbeat reported having seen a machine this panel is not running on")
	}
	if store.touched != 0 {
		t.Fatalf("touched=%d, want 0: last_seen_at must not advance on a mismatch", store.touched)
	}
}

func TestAHostWithNoMachineIdIsLocalOnWeakerEvidenceAndSaysSo(t *testing.T) {
	store := newFakeStore()
	probe := &fakeProbe{
		witness: localnode.MachineWitness{}, // no /etc/machine-id anywhere
		facts:   machineFacts("container-host"),
	}
	svc := newLocalNodeService(store, probe)
	registered, err := svc.RegisterLocalNode(context.Background(), LocalNodeOptions{})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	if registered.Verified {
		t.Fatal("a host with no machine id reported a witnessed identity")
	}

	route := svc.ResolveNodeRoute(context.Background(), registered.NodeID)
	if !route.Local {
		t.Fatalf("route=%+v, want the node id alone to be enough on a host that cannot be witnessed", route)
	}
	if route.Verified {
		t.Fatal("the route claimed to be verified with no witness on either side")
	}
	if route.Reason == "" {
		t.Fatal("the weaker evidence was not reported to the caller")
	}

	// And a machine id appearing where none was recorded is still a mismatch:
	// the change is exactly what a restore onto other hardware looks like.
	probe.witness = localnode.MachineWitness{Fingerprint: "machine-b", Source: "machine-id"}
	route = svc.ResolveNodeRoute(context.Background(), registered.NodeID)
	if route.Local || !route.Mismatch {
		t.Fatalf("route=%+v, want a refused route once the machine can be witnessed and disagrees", route)
	}
}
