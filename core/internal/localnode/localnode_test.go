package localnode

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// ============================================================
// The identity
// ============================================================

func TestTheNodeIdentityIsGeneratedOnceAndThenReadBack(t *testing.T) {
	etc := t.TempDir()

	first, err := LoadOrCreateIdentity(etc)
	if err != nil {
		t.Fatalf("creating the identity: %v", err)
	}
	if !first.Created {
		t.Fatal("the first call did not report creating the identity")
	}
	if first.NodeID == uuid.Nil {
		t.Fatal("the generated identity carries no node id")
	}

	second, err := LoadOrCreateIdentity(etc)
	if err != nil {
		t.Fatalf("reading the identity back: %v", err)
	}
	if second.Created {
		t.Fatal("the second call reported creating an identity that already existed")
	}
	if second.NodeID != first.NodeID {
		t.Fatalf("the machine came back as a different node: %s then %s", first.NodeID, second.NodeID)
	}
}

func TestTheIdentityFileIsNotReadableByOtherAccounts(t *testing.T) {
	etc := t.TempDir()
	if _, err := LoadOrCreateIdentity(etc); err != nil {
		t.Fatalf("creating the identity: %v", err)
	}
	info, err := os.Stat(IdentityPath(etc))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode=%o, want 600", mode)
	}
}

func TestAMachineWithNoIdentityIsNotGivenOneJustForAsking(t *testing.T) {
	etc := t.TempDir()
	if _, err := LoadIdentity(etc); !errors.Is(err, ErrNoIdentity) {
		t.Fatalf("err=%v, want ErrNoIdentity", err)
	}
	if _, err := os.Stat(IdentityPath(etc)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reading the identity created one as a side effect")
	}
}

func TestAnUnreadableIdentityFileIsAnErrorRatherThanAFreshNode(t *testing.T) {
	etc := t.TempDir()
	if err := os.WriteFile(IdentityPath(etc), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateIdentity(etc); err == nil {
		t.Fatal("a corrupt identity file was silently replaced with a new node id")
	}
	// An empty object is JSON but names no node, and would otherwise become a
	// node with the nil UUID.
	if err := os.WriteFile(IdentityPath(etc), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateIdentity(etc); err == nil {
		t.Fatal("an identity file with no node id was accepted")
	}
}

func TestSavingAnIdentityKeepsTheNodeAndItsRegistrationTime(t *testing.T) {
	etc := t.TempDir()
	original, err := LoadOrCreateIdentity(etc)
	if err != nil {
		t.Fatalf("creating the identity: %v", err)
	}

	adopted := uuid.New()
	saved, err := SaveIdentity(etc, adopted)
	if err != nil {
		t.Fatalf("saving: %v", err)
	}
	if saved.NodeID != adopted {
		t.Fatalf("node_id=%s, want %s", saved.NodeID, adopted)
	}
	if !saved.CreatedAt.Equal(original.CreatedAt) {
		t.Fatal("saving an identity moved the registration time; the node is not new")
	}

	reread, err := LoadIdentity(etc)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if reread.NodeID != adopted {
		t.Fatalf("the saved identity did not survive: node_id=%s", reread.NodeID)
	}
}

func TestAnEmptyNodeIdIsNeverSaved(t *testing.T) {
	if _, err := SaveIdentity(t.TempDir(), uuid.Nil); err == nil {
		t.Fatal("an empty node id was accepted as this machine's identity")
	}
}

// ============================================================
// The witness
// ============================================================

func TestTheWitnessRefusesEveryWayTheMachineCanHaveChangedUnderneath(t *testing.T) {
	machineA := MachineWitness{Fingerprint: "aaa", Source: "machine-id"}
	machineB := MachineWitness{Fingerprint: "bbb", Source: "machine-id"}
	none := MachineWitness{}

	cases := []struct {
		name         string
		recorded     string
		live         MachineWitness
		wantVerified bool
		wantErr      bool
	}{
		{name: "the same machine", recorded: "aaa", live: machineA, wantVerified: true},
		{name: "a different machine", recorded: "aaa", live: machineB, wantErr: true},
		{name: "a machine id appeared where none was recorded", recorded: "", live: machineA, wantErr: true},
		{name: "the machine id went away", recorded: "aaa", live: none, wantErr: true},
		{name: "no machine id on either side", recorded: "", live: none},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verified, err := CompareWitness(tc.recorded, "machine-id", tc.live)
			if tc.wantErr {
				if !errors.Is(err, ErrIdentityMismatch) {
					t.Fatalf("err=%v, want an identity mismatch", err)
				}
				if verified {
					t.Fatal("a refused comparison also reported itself verified")
				}
				return
			}
			if err != nil {
				t.Fatalf("err=%v, want none", err)
			}
			if verified != tc.wantVerified {
				t.Fatalf("verified=%v, want %v", verified, tc.wantVerified)
			}
		})
	}
}

func TestTheRawMachineIdIsNeverWhatIsStored(t *testing.T) {
	const machineID = "0c8f9a1b2c3d4e5f60718293a4b5c6d7"
	fingerprint := fingerprintOf(machineID)
	if fingerprint == machineID {
		t.Fatal("the machine id is stored as itself")
	}
	if len(fingerprint) != 64 {
		t.Fatalf("fingerprint length=%d, want a 64 character SHA-256", len(fingerprint))
	}
	if fingerprintOf(machineID) != fingerprint {
		t.Fatal("the fingerprint of one machine id is not stable")
	}
	if fingerprintOf(machineID+"x") == fingerprint {
		t.Fatal("two machine ids produced the same fingerprint")
	}
}

// ============================================================
// The facts
// ============================================================

func TestTheDistributionNameIsReadFromOsReleaseAndIsAbsentWhenItCannotBe(t *testing.T) {
	pretty := []byte("NAME=\"Ubuntu\"\nVERSION=\"24.04.1 LTS (Noble Numbat)\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nID=ubuntu\n")
	if got := ParseOSRelease(pretty); got != "Ubuntu 24.04.1 LTS" {
		t.Fatalf("os=%q", got)
	}

	noPretty := []byte("NAME=\"Alpine Linux\"\nVERSION=\"3.20\"\n")
	if got := ParseOSRelease(noPretty); got != "Alpine Linux 3.20" {
		t.Fatalf("os=%q", got)
	}

	// A file with nothing usable must produce nothing, so the column stays
	// null rather than becoming a guess.
	if got := ParseOSRelease([]byte("# a comment\nID=weird\n")); got != "" {
		t.Fatalf("os=%q, want nothing", got)
	}
	if got := ParseOSRelease(nil); got != "" {
		t.Fatalf("os=%q, want nothing", got)
	}
}

func TestTotalMemoryIsReportedInBytesAndIsAbsentWhenItCannotBeRead(t *testing.T) {
	meminfo := []byte("MemTotal:        8039256 kB\nMemFree:          230124 kB\nMemAvailable:    5510164 kB\n")
	total, ok := ParseMemTotal(meminfo)
	if !ok {
		t.Fatal("MemTotal was not found in a file that has it")
	}
	if want := int64(8039256) * 1024; total != want {
		t.Fatalf("ram_total=%d, want %d bytes", total, want)
	}

	for _, broken := range []string{"", "MemFree: 12 kB\n", "MemTotal: not-a-number kB\n", "MemTotal: 0 kB\n"} {
		if _, ok := ParseMemTotal([]byte(broken)); ok {
			t.Fatalf("%q was read as a memory total", broken)
		}
	}
}

func TestTheAddressChosenIsTheOneAHostIsReachedAt(t *testing.T) {
	ips := func(values ...string) []net.IP {
		out := make([]net.IP, 0, len(values))
		for _, v := range values {
			out = append(out, net.ParseIP(v))
		}
		return out
	}

	if got := PickIPv4(ips("127.0.0.1", "10.0.0.4", "203.0.113.9")); got != "203.0.113.9" {
		t.Fatalf("ip=%q, want the routable address", got)
	}
	if got := PickIPv4(ips("127.0.0.1", "10.0.0.4")); got != "10.0.0.4" {
		t.Fatalf("ip=%q, want the private address on a host behind NAT", got)
	}
	// A host with nothing else really is reachable at 127.0.0.1, and the column
	// is NOT NULL, so this is the honest answer rather than a placeholder.
	if got := PickIPv4(ips("127.0.0.1", "169.254.10.1")); got != "127.0.0.1" {
		t.Fatalf("ip=%q, want loopback", got)
	}
	if got := PickIPv4(ips("::1", "fe80::1")); got != "" {
		t.Fatalf("ip=%q, want nothing from a list with no IPv4", got)
	}

	if got := PickIPv6(ips("fe80::1", "fd00::5", "2001:db8::7")); got != "2001:db8::7" {
		t.Fatalf("ipv6=%q, want the globally routable address", got)
	}
	// Link-local and unique-local are not fallbacks: the column is nullable, so
	// an absent global address is recorded as absent.
	if got := PickIPv6(ips("fe80::1", "fd00::5", "::1")); got != "" {
		t.Fatalf("ipv6=%q, want nothing", got)
	}
}

func TestThisMachineCanBeMeasured(t *testing.T) {
	facts, err := CollectFacts()
	if err != nil {
		t.Fatalf("collecting facts about the machine running the tests: %v", err)
	}
	if facts.Hostname == "" || facts.IPAddress == "" {
		t.Fatalf("facts=%+v, want the two columns that cannot be null to be filled", facts)
	}
	if net.ParseIP(facts.IPAddress) == nil {
		t.Fatalf("ip_address=%q is not an address", facts.IPAddress)
	}
	if facts.IPv6Address != "" && net.ParseIP(facts.IPv6Address) == nil {
		t.Fatalf("ipv6_address=%q is not an address", facts.IPv6Address)
	}
	// Anything that could not be read has to be absent rather than zero.
	if facts.CPUCores != nil && *facts.CPUCores <= 0 {
		t.Fatal("cpu_cores was written as a non-positive number instead of left null")
	}
	if facts.RAMTotal != nil && *facts.RAMTotal <= 0 {
		t.Fatal("ram_total was written as a non-positive number instead of left null")
	}
	if facts.DiskTotal != nil && *facts.DiskTotal <= 0 {
		t.Fatal("disk_total was written as a non-positive number instead of left null")
	}
}

func TestTheProbeReadsTheIdentityFromTheDirectoryItWasGiven(t *testing.T) {
	etc := t.TempDir()
	probe := &SystemProbe{EtcRoot: etc}

	if _, err := probe.Identity(); !errors.Is(err, ErrNoIdentity) {
		t.Fatalf("err=%v, want ErrNoIdentity", err)
	}
	created, err := probe.EnsureIdentity()
	if err != nil {
		t.Fatalf("ensuring: %v", err)
	}
	if _, err := os.Stat(filepath.Join(etc, IdentityFileName)); err != nil {
		t.Fatalf("the identity was not written where the probe was pointed: %v", err)
	}
	read, err := probe.Identity()
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if read.NodeID != created.NodeID {
		t.Fatal("the probe read back a different node")
	}
}
