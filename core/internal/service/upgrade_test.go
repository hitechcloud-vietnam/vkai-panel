package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeEngine struct {
	mu       sync.Mutex
	status   UpgradeStatus
	jobID    string
	startErr error
	progress UpgradeProgress
	progErr  error
	starts   int
}

func (f *fakeEngine) Check(context.Context) (UpgradeStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, nil
}

func (f *fakeEngine) Start(_ context.Context, version string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	return f.jobID, f.startErr
}

func (f *fakeEngine) Progress(context.Context, string) (UpgradeProgress, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.progress, f.progErr
}

func newTestService(t *testing.T, engine UpgradeEngine, current string) *UpgradeService {
	t.Helper()
	t.Setenv("VKAI_ETC_ROOT", t.TempDir())
	s := NewUpgradeService(engine, nil, nil)
	s.build = UpgradeBuildInfo{Version: current, Commit: "abc1234", Date: "2026-01-01T00:00:00Z"}
	return s
}

func TestClassify(t *testing.T) {
	cases := []struct {
		step    string
		percent int
		want    UpgradeJobState
	}{
		{"download", 10, UpgradeJobRunning},
		{"Restarting services", 70, UpgradeJobRunning},
		{"completed", 100, UpgradeJobSucceeded},
		{"health_check", 100, UpgradeJobSucceeded},
		{"failed", 40, UpgradeJobFailed},
		{"rollback complete", 100, UpgradeJobRolledBack},
		{"rolled_back", 100, UpgradeJobRolledBack},
		{"verify", 20, UpgradeJobRunning},
		// The engine may report its step and that step's own outcome together.
		// Only the last word may end the job as failed, and only the first may
		// end it as succeeded.
		{"check succeeded", 20, UpgradeJobRunning},
		{"download succeeded", 30, UpgradeJobRunning},
		{"done succeeded", 100, UpgradeJobSucceeded},
		{"restart failed", 60, UpgradeJobFailed},
		{"rollback succeeded", 100, UpgradeJobRolledBack},
		{"rollback failed", 90, UpgradeJobFailed},
		{"", 40, UpgradeJobRunning},
	}
	for _, c := range cases {
		if got := classifyUpgradeStep(c.step, c.percent); got != c.want {
			t.Errorf("classifyUpgradeStep(%q,%d) = %q, want %q", c.step, c.percent, got, c.want)
		}
	}
}

// Every step identifier the release engine emits must land on a row of the
// step list, otherwise the UI freezes on the previous step for that stretch of
// the upgrade. The literals here are internal/upgrade.Step.String().
func TestEngineStepsAllMap(t *testing.T) {
	engineSteps := map[string]string{
		"lock":            "lock",
		"check":           "check",
		"download":        "download",
		"verify":          "verify",
		"stage":           "stage",
		"preflight":       "preflight",
		"backup_database": "backup",
		"switch":          "switch",
		"restart":         "restart",
		"health_check":    "health",
		"prune":           "cleanup",
		"cleanup":         "cleanup",
	}
	for step, want := range engineSteps {
		if got := upgradeStepKey(step); got != want {
			t.Errorf("upgradeStepKey(%q) = %q, want %q", step, got, want)
		}
		if upgradeStepIndex(step) < 0 {
			t.Errorf("upgradeStepIndex(%q) = -1", step)
		}
	}
}

func TestStepIndex(t *testing.T) {
	if got := upgradeStepKey("Downloading release"); got != "download" {
		t.Fatalf("got %q", got)
	}
	if got := upgradeStepIndex("something else"); got != -1 {
		t.Fatalf("got %d", got)
	}
	// The list must stay in the order the engine walks it.
	if upgradeStepIndex("download") >= upgradeStepIndex("switch") {
		t.Fatal("download must come before switch")
	}
	if upgradeStepIndex("restart") >= upgradeStepIndex("cleanup") {
		t.Fatal("restart must come before cleanup")
	}
}

func TestVersionValidation(t *testing.T) {
	for _, bad := range []string{"1.0.0; rm -rf /", "../../etc", "$(id)", "1.0.0 && x", ""} {
		if err := validateUpgradeVersion(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	for _, good := range []string{"1.0.0", "v1.2.3", "1.2.3-rc1", "0.3.0"} {
		if err := validateUpgradeVersion(good); err != nil {
			t.Errorf("rejected %q: %v", good, err)
		}
	}
}

func TestUnavailableWithoutEngine(t *testing.T) {
	s := newTestService(t, nil, "1.0.0")
	if _, err := s.Start(context.Background(), UpgradeCaller{}, "1.1.0"); !errors.Is(err, ErrUpgradeUnavailable) {
		t.Fatalf("want unavailable, got %v", err)
	}
	view := s.Status()
	if view.Available || view.Current != "1.0.0" {
		t.Fatalf("bad view %+v", view)
	}
}

func TestSecondStartIsRefused(t *testing.T) {
	eng := &fakeEngine{jobID: "job-1", progress: UpgradeProgress{Step: "download", Percent: 5}}
	s := newTestService(t, eng, "1.0.0")

	if _, err := s.Start(context.Background(), UpgradeCaller{}, "1.1.0"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, err := s.Start(context.Background(), UpgradeCaller{}, "1.1.0"); !errors.Is(err, ErrUpgradeInProgress) {
		t.Fatalf("want in-progress, got %v", err)
	}
	if view := s.Status(); view.UpdateAvailable {
		t.Fatal("update should not be offered while a job runs")
	}
}

func TestStartWithoutTarget(t *testing.T) {
	eng := &fakeEngine{jobID: "job-1"}
	s := newTestService(t, eng, "1.0.0")
	if _, err := s.Start(context.Background(), UpgradeCaller{}, ""); !errors.Is(err, ErrUpgradeNoTarget) {
		t.Fatalf("want no-target, got %v", err)
	}

	eng.status = UpgradeStatus{Current: "1.0.0", Latest: "1.1.0", UpdateAvailable: true, Changelog: "notes"}
	if _, err := s.Check(context.Background(), UpgradeCaller{}); err != nil {
		t.Fatalf("check: %v", err)
	}
	res, err := s.Start(context.Background(), UpgradeCaller{}, "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if res.ToVersion != "1.1.0" || res.FromVersion != "1.0.0" {
		t.Fatalf("bad result %+v", res)
	}
}

// The API is replaced by the upgrade it started. A new process must be able to
// answer for the job the old one left behind.
func TestRestartReconciliation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VKAI_ETC_ROOT", dir)

	eng := &fakeEngine{jobID: "job-1", progress: UpgradeProgress{Step: "restart", Percent: 60}}
	old := NewUpgradeService(eng, nil, nil)
	old.build = UpgradeBuildInfo{Version: "1.0.0"}
	if _, err := old.Start(context.Background(), UpgradeCaller{}, "1.1.0"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// New process, new version: the upgrade landed.
	fresh := NewUpgradeService(eng, nil, nil)
	fresh.build = UpgradeBuildInfo{Version: "1.1.0"}
	job, err := fresh.Progress(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("progress: %v", err)
	}
	if job.State != UpgradeJobSucceeded || job.Percent != 100 {
		t.Fatalf("want succeeded, got %+v", job)
	}
	if !job.Detached {
		t.Fatal("job should be marked detached")
	}
}

func TestRestartRollback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VKAI_ETC_ROOT", dir)

	eng := &fakeEngine{jobID: "job-2", progress: UpgradeProgress{Step: "restart", Percent: 60}}
	old := NewUpgradeService(eng, nil, nil)
	old.build = UpgradeBuildInfo{Version: "1.0.0"}
	if _, err := old.Start(context.Background(), UpgradeCaller{}, "1.1.0"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// New process, still the old version, and past the restart grace.
	fresh := NewUpgradeService(eng, nil, nil)
	fresh.build = UpgradeBuildInfo{Version: "1.0.0"}
	fresh.now = func() time.Time { return time.Now().Add(2 * upgradeRestartGrace) }
	fresh.mu.Lock()
	fresh.reconcileLocked()
	fresh.mu.Unlock()

	job, err := fresh.Progress(context.Background(), "job-2")
	if err != nil {
		t.Fatalf("progress: %v", err)
	}
	if job.State != UpgradeJobRolledBack {
		t.Fatalf("want rolled_back, got %+v", job)
	}
}

func TestRestartWithinGraceStaysRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VKAI_ETC_ROOT", dir)

	eng := &fakeEngine{jobID: "job-3", progress: UpgradeProgress{Step: "restart", Percent: 60}}
	old := NewUpgradeService(eng, nil, nil)
	old.build = UpgradeBuildInfo{Version: "1.0.0"}
	if _, err := old.Start(context.Background(), UpgradeCaller{}, "1.1.0"); err != nil {
		t.Fatalf("start: %v", err)
	}

	fresh := NewUpgradeService(eng, nil, nil)
	fresh.build = UpgradeBuildInfo{Version: "1.0.0"}
	job, err := fresh.Progress(context.Background(), "job-3")
	if err != nil {
		t.Fatalf("progress: %v", err)
	}
	if job.State != UpgradeJobRunning {
		t.Fatalf("want running, got %+v", job)
	}
}

func TestUnknownJobID(t *testing.T) {
	s := newTestService(t, &fakeEngine{jobID: "job-1"}, "1.0.0")
	if _, err := s.Progress(context.Background(), "nope"); !errors.Is(err, ErrUpgradeJobNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}
