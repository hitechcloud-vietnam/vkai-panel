package quota_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/quota"
)

// ---------------------------------------------------------------------------
// A store that can be made to behave badly on purpose. The point of these
// tests is the failure modes: a quota check that cannot reach its data, or that
// finds nothing, must never answer "go ahead" by accident.
// ---------------------------------------------------------------------------

type fakeStore struct {
	assignment *quota.Assignment
	assignErr  error
	counts     map[quota.Resource]int64
	countsErr  error
	measured   quota.Measured
	measErr    error

	events    []quota.Event
	suspended *bool
	suspErr   error
}

func (f *fakeStore) Assignment(context.Context, uuid.UUID) (*quota.Assignment, error) {
	return f.assignment, f.assignErr
}

func (f *fakeStore) Counts(context.Context, uuid.UUID) (map[quota.Resource]int64, error) {
	if f.counts == nil {
		return map[quota.Resource]int64{}, f.countsErr
	}
	return f.counts, f.countsErr
}

func (f *fakeStore) MeasuredUsage(context.Context, uuid.UUID) (quota.Measured, error) {
	return f.measured, f.measErr
}

func (f *fakeStore) RecordEvent(_ context.Context, ev quota.Event) error {
	f.events = append(f.events, ev)
	return nil
}

func (f *fakeStore) RecordEventThrottled(_ context.Context, ev quota.Event, _ time.Duration) (bool, error) {
	f.events = append(f.events, ev)
	return true, nil
}

func (f *fakeStore) SetSuspended(_ context.Context, _ uuid.UUID, s bool, _ string, _ bool) error {
	f.suspended = &s
	return f.suspErr
}

func (f *fakeStore) ManagedTenants(context.Context) ([]uuid.UUID, error) { return nil, nil }
func (f *fakeStore) SiteRoots(context.Context, uuid.UUID) ([]string, error) {
	return nil, nil
}
func (f *fakeStore) DatabaseBytes(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (f *fakeStore) BandwidthBytes(context.Context, uuid.UUID, time.Time, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeStore) SaveDiskUsage(context.Context, uuid.UUID, quota.DiskSample) error { return nil }
func (f *fakeStore) SaveBandwidthUsage(context.Context, uuid.UUID, int64, time.Time) error {
	return nil
}
func (f *fakeStore) ListPackages(context.Context) ([]quota.Package, error) { return nil, nil }
func (f *fakeStore) GetPackage(context.Context, uuid.UUID) (*quota.Package, error) {
	return nil, quota.ErrPackageNotFound
}
func (f *fakeStore) GetPackageBySlug(context.Context, string) (*quota.Package, error) {
	return nil, quota.ErrPackageNotFound
}
func (f *fakeStore) CreatePackage(context.Context, *quota.Package) error { return nil }
func (f *fakeStore) UpdatePackage(context.Context, *quota.Package) error { return nil }
func (f *fakeStore) DeletePackage(context.Context, uuid.UUID) error      { return nil }
func (f *fakeStore) AssignPackage(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error {
	return nil
}
func (f *fakeStore) SetOverride(context.Context, uuid.UUID, quota.Resource, *int64, string, *time.Time, *uuid.UUID) error {
	return nil
}
func (f *fakeStore) DeleteOverride(context.Context, uuid.UUID, quota.Resource) error { return nil }
func (f *fakeStore) ListOverrides(context.Context, uuid.UUID) ([]quota.Override, error) {
	return nil, nil
}
func (f *fakeStore) SetFeatureOverride(context.Context, uuid.UUID, string, bool, string, *time.Time) error {
	return nil
}
func (f *fakeStore) DeleteFeatureOverride(context.Context, uuid.UUID, string) error { return nil }
func (f *fakeStore) ListEvents(context.Context, uuid.UUID, int) ([]quota.Event, error) {
	return f.events, nil
}

func ptr(v int64) *int64 { return &v }

func assignmentWith(limits quota.Limits, action quota.Action) *quota.Assignment {
	return &quota.Assignment{
		TenantID: uuid.New(),
		Package: quota.Package{
			Name:            "Starter",
			Slug:            "starter",
			Limits:          limits,
			OverQuotaAction: action,
			WarnPercent:     90,
			GracePercent:    2,
			GraceFloorMB:    16,
			Features:        map[string]bool{},
		},
		Overrides:        map[quota.Resource]*int64{},
		FeatureOverrides: map[string]bool{},
	}
}

// ---------------------------------------------------------------------------
// FAILING CLOSED
//
// Each of these is a way the enforcer could be disconnected. None of them may
// answer nil.
// ---------------------------------------------------------------------------

func TestNilEnforcerRefuses(t *testing.T) {
	var e *quota.Enforcer
	err := e.Check(context.Background(), uuid.New(), quota.ResourceWebsites)
	if err == nil {
		t.Fatal("a nil enforcer allowed a creation; a wiring mistake must never read as 'no limits'")
	}
	if !quota.IsUnavailable(err) || !errors.Is(err, quota.ErrNotWired) {
		t.Fatalf("expected the not-wired error, got %v", err)
	}
}

func TestEnforcerWithNoStoreRefuses(t *testing.T) {
	e := quota.New(nil, nil)
	if err := e.Check(context.Background(), uuid.New(), quota.ResourceWebsites); !quota.IsUnavailable(err) {
		t.Fatalf("an enforcer with no store allowed a creation: %v", err)
	}
}

func TestStoreFailureRefuses(t *testing.T) {
	e := quota.New(&fakeStore{assignErr: errors.New(`relation "tenant_packages" does not exist`)}, nil)
	err := e.Check(context.Background(), uuid.New(), quota.ResourceWebsites)
	if !quota.IsUnavailable(err) {
		t.Fatalf("a database failure allowed a creation: %v", err)
	}
	if !strings.Contains(err.Error(), "tenant_packages") {
		t.Fatalf("the refusal does not name the cause: %v", err)
	}
}

func TestMissingTenantRefuses(t *testing.T) {
	e := quota.New(&fakeStore{}, nil)
	if err := e.Check(context.Background(), uuid.Nil, quota.ResourceWebsites); err == nil {
		t.Fatal("a request with no tenant was allowed to create a website")
	}
}

// An account with no package is unmanaged and is allowed. This is the one
// permissive case, and it is deliberate; see doc.go.
func TestAccountWithNoPackageIsUnmanaged(t *testing.T) {
	e := quota.New(&fakeStore{assignment: nil}, nil)
	if err := e.Check(context.Background(), uuid.New(), quota.ResourceWebsites); err != nil {
		t.Fatalf("an account with no package was refused: %v", err)
	}
}

// ---------------------------------------------------------------------------
// THE LIMIT ITSELF
// ---------------------------------------------------------------------------

func TestCountedLimitRefusesAtTheLimitAndAllowsBelow(t *testing.T) {
	a := assignmentWith(quota.Limits{Websites: ptr(5)}, quota.ActionRefuse)

	for _, tc := range []struct {
		used    int64
		refused bool
	}{{0, false}, {4, false}, {5, true}, {9, true}} {
		store := &fakeStore{assignment: a, counts: map[quota.Resource]int64{quota.ResourceWebsites: tc.used}}
		err := quota.New(store, nil).Check(context.Background(), a.TenantID, quota.ResourceWebsites)
		if tc.refused && err == nil {
			t.Fatalf("%d of 5 websites: creation was allowed past the limit", tc.used)
		}
		if !tc.refused && err != nil {
			t.Fatalf("%d of 5 websites: creation was refused below the limit: %v", tc.used, err)
		}
		if !tc.refused {
			continue
		}
		// The message has to name the limit and the usage, or the customer
		// cannot tell what to do about it.
		msg := err.Error()
		for _, want := range []string{"websites", "5"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("the refusal does not mention %q: %s", want, msg)
			}
		}
	}
}

func TestZeroIsARealLimitNotUnlimited(t *testing.T) {
	a := assignmentWith(quota.Limits{Mailboxes: ptr(0)}, quota.ActionRefuse)
	store := &fakeStore{assignment: a, counts: map[quota.Resource]int64{}}
	if err := quota.New(store, nil).Check(context.Background(), a.TenantID, quota.ResourceMailboxes); err == nil {
		t.Fatal("a package that includes no mailboxes allowed the first mailbox")
	}
}

func TestNilLimitIsUnlimited(t *testing.T) {
	a := assignmentWith(quota.Limits{}, quota.ActionRefuse)
	store := &fakeStore{assignment: a, counts: map[quota.Resource]int64{quota.ResourceWebsites: 10_000}}
	if err := quota.New(store, nil).Check(context.Background(), a.TenantID, quota.ResourceWebsites); err != nil {
		t.Fatalf("an unlimited package refused a creation: %v", err)
	}
}

func TestOverrideBeatsThePackageBothWays(t *testing.T) {
	base := quota.Limits{Websites: ptr(2)}

	raised := assignmentWith(base, quota.ActionRefuse)
	raised.Overrides[quota.ResourceWebsites] = ptr(10)
	store := &fakeStore{assignment: raised, counts: map[quota.Resource]int64{quota.ResourceWebsites: 5}}
	if err := quota.New(store, nil).Check(context.Background(), raised.TenantID, quota.ResourceWebsites); err != nil {
		t.Fatalf("an override raising the limit did not take effect: %v", err)
	}

	lowered := assignmentWith(quota.Limits{Websites: ptr(50)}, quota.ActionRefuse)
	lowered.Overrides[quota.ResourceWebsites] = ptr(1)
	store = &fakeStore{assignment: lowered, counts: map[quota.Resource]int64{quota.ResourceWebsites: 1}}
	err := quota.New(store, nil).Check(context.Background(), lowered.TenantID, quota.ResourceWebsites)
	if err == nil {
		t.Fatal("an override lowering the limit did not take effect")
	}
	if !strings.Contains(err.Error(), "account override") {
		t.Fatalf("the refusal does not say the limit came from an override: %v", err)
	}

	unlimited := assignmentWith(base, quota.ActionRefuse)
	unlimited.Overrides[quota.ResourceWebsites] = nil // unlimited for this account
	store = &fakeStore{assignment: unlimited, counts: map[quota.Resource]int64{quota.ResourceWebsites: 999}}
	if err := quota.New(store, nil).Check(context.Background(), unlimited.TenantID, quota.ResourceWebsites); err != nil {
		t.Fatalf("an override to unlimited did not take effect: %v", err)
	}
}

// ---------------------------------------------------------------------------
// OVER-QUOTA POLICY, GRACE AND ROUNDING
// ---------------------------------------------------------------------------

func TestDiskGraceBandAndPolicy(t *testing.T) {
	const limitMB = 10 * 1024 // a 10GB package
	measuredAt := time.Now().Add(-10 * time.Minute)

	cases := []struct {
		name    string
		usedMB  int64
		action  quota.Action
		partial bool
		refused bool
	}{
		{"inside the limit", 9000, quota.ActionRefuse, false, false},
		{"a hair over: 10.001GB of 10GB", limitMB + 1, quota.ActionRefuse, false, false},
		{"at the edge of the 2% band", limitMB + 204, quota.ActionRefuse, false, false},
		{"past the band", limitMB + 400, quota.ActionRefuse, false, true},
		{"past the band but the policy only warns", limitMB + 4000, quota.ActionWarn, false, false},
		{"past the band on a partial walk", limitMB + 4000, quota.ActionRefuse, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := assignmentWith(quota.Limits{DiskMB: ptr(limitMB), Websites: ptr(100)}, tc.action)
			store := &fakeStore{
				assignment: a,
				counts:     map[quota.Resource]int64{quota.ResourceWebsites: 1},
				measured: quota.Measured{
					Present: true, DiskUsedMB: tc.usedMB,
					DiskMeasuredAt: &measuredAt, DiskPartial: tc.partial,
				},
			}
			err := quota.New(store, nil).Check(context.Background(), a.TenantID, quota.ResourceWebsites)
			if tc.refused && err == nil {
				t.Fatalf("%d MB of %d MB: the creation was allowed", tc.usedMB, limitMB)
			}
			if !tc.refused && err != nil {
				t.Fatalf("%d MB of %d MB: the creation was refused: %v", tc.usedMB, limitMB, err)
			}
			if !tc.refused {
				return
			}
			// Being refused a website because of disk has to say so.
			if !strings.Contains(err.Error(), "disk space") || !strings.Contains(err.Error(), "website") {
				t.Fatalf("the refusal names neither the limit hit nor what was requested: %v", err)
			}
		})
	}
}

func TestMBFromBytesRoundsUp(t *testing.T) {
	for _, tc := range []struct {
		bytes int64
		want  int64
	}{{0, 0}, {1, 1}, {quota.BytesPerMB, 1}, {quota.BytesPerMB + 1, 2}, {10 * quota.BytesPerMB, 10}} {
		if got := quota.MBFromBytes(tc.bytes); got != tc.want {
			t.Fatalf("MBFromBytes(%d) = %d, want %d", tc.bytes, got, tc.want)
		}
	}
}

func TestGraceFloorProtectsSmallPackages(t *testing.T) {
	p := quota.Package{GracePercent: 2, GraceFloorMB: 16}
	if got := p.GraceMB(512); got != 16 {
		t.Fatalf("a 512MB package got a %d MB grace band; the floor is what keeps it meaningful", got)
	}
	if got := p.GraceMB(10240); got != 204 {
		t.Fatalf("a 10GB package got a %d MB grace band, want 204", got)
	}
}

func TestSuspendedAccountRefusesEverythingAndSaysWhy(t *testing.T) {
	at := time.Now().Add(-time.Hour)
	a := assignmentWith(quota.Limits{Websites: ptr(100)}, quota.ActionRefuse)
	a.Suspended = true
	a.SuspendedAt = &at
	a.SuspendedReason = "unpaid invoice"

	e := quota.New(&fakeStore{assignment: a}, nil)
	for _, r := range quota.CountedResources {
		err := e.Check(context.Background(), a.TenantID, r)
		if !quota.IsSuspended(err) {
			t.Fatalf("%s: a suspended account was allowed to create: %v", r, err)
		}
		if !strings.Contains(err.Error(), "unpaid invoice") {
			t.Fatalf("the suspension refusal does not carry the reason: %v", err)
		}
		if !strings.Contains(err.Error(), "untouched") {
			t.Fatalf("the suspension refusal does not say existing data is safe: %v", err)
		}
	}
}

func TestCheckRejectsMeasuredResources(t *testing.T) {
	a := assignmentWith(quota.Limits{}, quota.ActionRefuse)
	e := quota.New(&fakeStore{assignment: a}, nil)
	if err := e.Check(context.Background(), a.TenantID, quota.ResourceDisk); err == nil {
		t.Fatal("Check accepted a measured resource; disk is consumed, not created")
	}
}

func TestWarningIsRecordedBeforeAnythingIsRefused(t *testing.T) {
	a := assignmentWith(quota.Limits{Websites: ptr(10)}, quota.ActionRefuse)
	store := &fakeStore{assignment: a, counts: map[quota.Resource]int64{quota.ResourceWebsites: 8}}
	if err := quota.New(store, nil).Check(context.Background(), a.TenantID, quota.ResourceWebsites); err != nil {
		t.Fatalf("9 of 10 websites was refused: %v", err)
	}
	if len(store.events) != 1 || store.events[0].Kind != quota.EventWarn {
		t.Fatalf("no warning was recorded at 9 of 10; events: %+v", store.events)
	}
}

// ---------------------------------------------------------------------------
// SUSPENSION IS REVERSIBLE AND DELETES NOTHING
// ---------------------------------------------------------------------------

type recordingSites struct{ suspended, resumed []uuid.UUID }

func (r *recordingSites) SuspendTenantSites(_ context.Context, id uuid.UUID) error {
	r.suspended = append(r.suspended, id)
	return nil
}

func (r *recordingSites) ResumeTenantSites(_ context.Context, id uuid.UUID) error {
	r.resumed = append(r.resumed, id)
	return nil
}

func TestSuspendAndResumeDriveTheSiteController(t *testing.T) {
	store := &fakeStore{}
	sites := &recordingSites{}
	e := quota.New(store, nil)
	e.SetSiteController(sites)

	id := uuid.New()
	if err := e.Suspend(context.Background(), id, "unpaid invoice", false); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if store.suspended == nil || !*store.suspended {
		t.Fatal("the suspension flag was not written")
	}
	if len(sites.suspended) != 1 || sites.suspended[0] != id {
		t.Fatal("the websites were not taken offline")
	}

	if err := e.Resume(context.Background(), id, "paid"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if store.suspended == nil || *store.suspended {
		t.Fatal("the suspension flag was not cleared")
	}
	if len(sites.resumed) != 1 || sites.resumed[0] != id {
		t.Fatal("the websites were not put back")
	}
}

// ---------------------------------------------------------------------------
// THE DISK WALK
// ---------------------------------------------------------------------------

func TestMeasureTreesCountsOnceAndStaysBounded(t *testing.T) {
	root := t.TempDir()
	payload := make([]byte, 8192)

	for _, name := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(root, name), payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A hard link must not be counted twice: a hardlinked backup tree would
	// otherwise double a customer's usage and push them over a quota they never
	// exceeded.
	if err := os.Link(filepath.Join(root, "a"), filepath.Join(root, "a-link")); err != nil {
		t.Skipf("this filesystem does not support hard links: %v", err)
	}

	sample := quota.MeasureTrees(context.Background(), []string{root}, quota.DefaultWalkBudget())
	if sample.Partial {
		t.Fatal("a four-file tree was reported as a partial walk")
	}
	if sample.UsedBytes < 3*8192 {
		t.Fatalf("the walk reported %d bytes for three 8KB files", sample.UsedBytes)
	}
	if sample.UsedBytes > 6*8192 {
		t.Fatalf("the walk reported %d bytes, so the hard link was counted twice", sample.UsedBytes)
	}

	// The budget has to actually stop the walk, or a machine with millions of
	// small files turns every sampling interval into an outage.
	tight := quota.MeasureTrees(context.Background(), []string{root}, quota.WalkBudget{MaxFiles: 2, MaxDuration: time.Minute})
	if !tight.Partial {
		t.Fatal("the inode budget did not stop the walk")
	}
}

func TestMeasureTreesSurvivesAMissingRootAndASymlinkLoop(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("this filesystem does not support symlinks: %v", err)
	}
	done := make(chan quota.DiskSample, 1)
	go func() {
		done <- quota.MeasureTrees(context.Background(),
			[]string{root, filepath.Join(root, "does-not-exist")}, quota.DefaultWalkBudget())
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the walk followed a symlink loop and never finished")
	}
}

func TestMonthWindowIsACalendarMonthInUTC(t *testing.T) {
	from, to := quota.MonthWindow(time.Date(2026, 2, 17, 13, 45, 0, 0, time.UTC))
	if from.Format("2006-01-02") != "2026-02-01" || to.Format("2006-01-02") != "2026-03-01" {
		t.Fatalf("month window is %s..%s", from, to)
	}
}
