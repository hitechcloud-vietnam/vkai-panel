package upgrade

// Test doubles for everything the upgrader touches outside its own process.
//
// The rule these enforce is the one stated in the package doc: no test opens a
// socket, restarts a service or writes outside its own t.TempDir(). The
// "filesystem" is a real temporary directory rather than an interface, because
// the upgrade's correctness lives in symlink renames, hard links and directory
// permissions - behaviour an in-memory filesystem would have to reimplement,
// and would therefore be able to get wrong in the test's favour.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ------------------------------------------------------------------ clock

// fakeClock never blocks. Sleep moves time forward instead, which is what
// makes a ninety-second health timeout a sub-millisecond test.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d > 0 {
		c.now = c.now.Add(d)
	}
}

// ------------------------------------------------------------------ http

// fakeHTTP serves a fixed set of URLs. Anything else is a 404, so a test that
// makes the upgrader fetch an unexpected URL fails rather than hanging.
type fakeHTTP struct {
	mu     sync.Mutex
	bodies map[string][]byte
	status map[string]int
	errs   map[string]error
	hits   map[string]int
}

func newFakeHTTP() *fakeHTTP {
	return &fakeHTTP{
		bodies: map[string][]byte{},
		status: map[string]int{},
		errs:   map[string]error{},
		hits:   map[string]int{},
	}
}

func (f *fakeHTTP) serve(url string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[url] = body
	f.status[url] = http.StatusOK
}

func (f *fakeHTTP) serveStatus(url string, code int, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[url] = body
	f.status[url] = code
}

func (f *fakeHTTP) hitCount(url string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[url]
}

func (f *fakeHTTP) Do(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	url := req.URL.String()
	f.hits[url]++
	err := f.errs[url]
	body, ok := f.bodies[url]
	code := f.status[url]
	f.mu.Unlock()

	if err != nil {
		return nil, err
	}
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("not found")),
			Request:    req,
		}, nil
	}
	if code == 0 {
		code = http.StatusOK
	}
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}, nil
}

// ------------------------------------------------------------------ runner

// fakeSystemd stands in for systemctl and pg_dump.
//
// It answers "is this service healthy" by looking at where the installation's
// current symlink points, which is the same thing a real systemd would be
// reporting on: the unit runs whatever the symlink resolves to. That makes the
// rollback tests honest - flipping the symlink back really does change what the
// health check sees - instead of driving the fake from a call counter.
type fakeSystemd struct {
	mu   sync.Mutex
	root string

	// unhealthyReleases: is-active fails while current points at one of
	// these. Keyed by the release directory's base name.
	unhealthyReleases map[string]bool
	// restartFailsFor: systemctl restart fails while current points at one
	// of these. This is how a failing rollback is simulated.
	restartFailsFor map[string]bool
	// dumpFails makes the database dump command fail.
	dumpFails bool
	// dumpWritesNothing makes the dump command succeed without producing a
	// file, which the upgrader must catch.
	dumpWritesNothing bool

	calls []string
}

func newFakeSystemd(root string) *fakeSystemd {
	return &fakeSystemd{
		root:              root,
		unhealthyReleases: map[string]bool{},
		restartFailsFor:   map[string]bool{},
	}
}

// currentRelease is the base name of whatever /current points at right now.
func (r *fakeSystemd) currentRelease() string {
	target, err := os.Readlink(filepath.Join(r.root, "current"))
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

func (r *fakeSystemd) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	unhealthy := r.unhealthyReleases
	restartFails := r.restartFailsFor
	dumpFails := r.dumpFails
	dumpWritesNothing := r.dumpWritesNothing
	r.mu.Unlock()

	if name == "systemctl" {
		if len(args) == 0 {
			return nil, fmt.Errorf("systemctl needs a subcommand")
		}
		cur := r.currentRelease()
		switch args[0] {
		case "restart":
			if restartFails[cur] {
				return []byte("Job for unit failed"), fmt.Errorf("systemctl restart failed for release %s", cur)
			}
			return nil, nil
		case "is-active":
			if unhealthy[cur] {
				return []byte("activating"), fmt.Errorf("unit is not active on release %s", cur)
			}
			return []byte("active"), nil
		default:
			return nil, nil
		}
	}

	// Anything else is treated as the database dump command.
	if dumpFails {
		return []byte("connection refused"), fmt.Errorf("%s failed", name)
	}
	if dumpWritesNothing {
		return nil, nil
	}
	for _, a := range args {
		if dest, ok := strings.CutPrefix(a, "--file="); ok {
			if err := os.WriteFile(dest, []byte("PGDMP fake dump\n"), 0o640); err != nil {
				return nil, err
			}
		}
	}
	return nil, nil
}

func (r *fakeSystemd) callsMatching(prefix string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, c := range r.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func (r *fakeSystemd) setUnhealthy(release string, unhealthy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unhealthyReleases[release] = unhealthy
}

func (r *fakeSystemd) setRestartFails(release string, fails bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restartFailsFor[release] = fails
}

// ------------------------------------------------------------------ archives

type tarEntry struct {
	Name     string
	Body     string
	Mode     int64
	Typeflag byte
	Linkname string
}

// buildTarGz produces a gzipped tar in memory.
func buildTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typeflag := e.Typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		mode := e.Mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{
			Name:     e.Name,
			Mode:     mode,
			Size:     int64(len(e.Body)),
			Typeflag: typeflag,
			Linkname: e.Linkname,
			ModTime:  time.Unix(1700000000, 0),
		}
		if typeflag != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %q: %v", e.Name, err)
		}
		if typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.Body)); err != nil {
				t.Fatalf("write tar body %q: %v", e.Name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// releaseTarball is a plausible release: a binary, the built UI and a version
// stamp.
func releaseTarball(t *testing.T, version string) []byte {
	t.Helper()
	return buildTarGz(t, []tarEntry{
		{Name: "VERSION", Body: version + "\n"},
		{Name: "core/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "core/vkai-api", Body: "#!/bin/sh\necho api " + version + "\n", Mode: 0o755},
		{Name: "panel/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "panel/index.html", Body: "<h1>" + version + "</h1>\n"},
	})
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ------------------------------------------------------------------ env

const (
	testFeedURL = "https://releases.example.test/vkai-panel/feed.json"
	testTarBase = "https://releases.example.test/vkai-panel/"
)

// testEnv is one fake installation: a temporary root laid out the way the
// installer lays out a real one, plus the fakes wired into an Upgrader.
type testEnv struct {
	t      *testing.T
	root   string
	http   *fakeHTTP
	runner *fakeSystemd
	clock  *fakeClock
	u      *Upgrader

	mu     sync.Mutex
	events []Event
}

// newTestEnv builds an installation already running currentVersion. cfgFn may
// adjust the configuration before the Upgrader is constructed.
func newTestEnv(t *testing.T, currentVersion string, cfgFn func(*Config)) *testEnv {
	t.Helper()
	root := t.TempDir()

	env := &testEnv{
		t:      t,
		root:   root,
		http:   newFakeHTTP(),
		runner: newFakeSystemd(root),
		clock:  newFakeClock(),
	}

	mustMkdirAll(t, filepath.Join(root, "releases", currentVersion))
	mustMkdirAll(t, filepath.Join(root, "etc"))
	mustMkdirAll(t, filepath.Join(root, "www", "backup", "databases"))
	mustWriteFile(t, filepath.Join(root, "releases", currentVersion, "VERSION"), currentVersion+"\n")
	mustSymlink(t, filepath.Join("releases", currentVersion), filepath.Join(root, "current"))

	cfg := Config{
		Root:           root,
		FeedURL:        testFeedURL,
		CurrentVersion: currentVersion,
		Services:       []string{"vkai-api", "vkai-ui", "vkai-agent"},
		KeepReleases:   5,
		HealthTimeout:  30 * time.Second,
		HealthInterval: 2 * time.Second,
		Database: DatabaseBackupConfig{
			Enabled: true,
			Command: "pg_dump",
			Name:    "vkai_panel",
		},
		Progress: env.record,
	}
	if cfgFn != nil {
		cfgFn(&cfg)
	}

	u, err := New(cfg, Deps{
		HTTP:         env.http,
		Runner:       env.runner,
		Clock:        env.clock,
		DiskFree:     func(string) (uint64, error) { return 100 << 30, nil },
		ProcessAlive: func(int) bool { return false },
		PID:          4242,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	env.u = u
	return env
}

func (e *testEnv) record(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, ev)
}

// publish makes version available in the feed and serves its tarball. Extra
// manifests can be added with publishManifests for feeds that carry history.
func (e *testEnv) publish(version, minUpgradeFrom string) Manifest {
	e.t.Helper()
	tarball := releaseTarball(e.t, version)
	url := testTarBase + "vkai-panel-" + version + ".tar.gz"
	e.http.serve(url, tarball)
	m := Manifest{
		Version:        version,
		ReleasedAt:     time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		MinUpgradeFrom: minUpgradeFrom,
		TarballURL:     url,
		SHA256:         sha256Hex(tarball),
		ChangelogURL:   "https://docs.example.test/changelog/" + version,
	}
	e.publishManifests(m)
	return m
}

func (e *testEnv) publishManifests(ms ...Manifest) {
	e.t.Helper()
	body, err := json.Marshal(ms)
	if err != nil {
		e.t.Fatalf("marshal feed: %v", err)
	}
	e.http.serve(testFeedURL, body)
}

func (e *testEnv) steps() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, 0, len(e.events))
	for _, ev := range e.events {
		out = append(out, ev.Step.String()+":"+string(ev.Status))
	}
	return out
}

func (e *testEnv) hasStep(step Step, status Status) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ev := range e.events {
		if ev.Step == step && ev.Status == status {
			return true
		}
	}
	return false
}

// currentTarget resolves the installation's current symlink.
func (e *testEnv) currentTarget() string {
	e.t.Helper()
	target, err := os.Readlink(filepath.Join(e.root, "current"))
	if err != nil {
		e.t.Fatalf("readlink current: %v", err)
	}
	return filepath.Base(target)
}

// ------------------------------------------------------------------ helpers

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
