package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readEntries(t *testing.T, path string) []Entry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the record at %s: %v", path, err)
	}
	var entries []Entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("the record holds a line that is not JSON: %q (%v)", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

// The record exists so an operator can answer "who told this machine to do
// that" without asking the panel, which is the party whose compromise is the
// thing being investigated.
func TestAnEntryNamesWhoAskedAndWhatWasRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operations.log")
	record := Open(Options{Path: path, AgentID: "agent-42", Fallback: log.New(os.Stderr, "", 0)})
	defer record.Close()

	record.Record(Entry{
		Actor:      Actor{Name: "vkai-panel", Serial: "0a1b2c", Address: "10.0.0.5:51234"},
		Operation:  "service.control",
		Outcome:    OutcomeExecuted,
		Arguments:  `{"name":"nginx","action":"restart"}`,
		Detail:     Argv("systemctl", "restart", "nginx"),
		DurationMS: 42,
	})

	entries := readEntries(t, path)
	if len(entries) != 1 {
		t.Fatalf("the record holds %d entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Actor.Name != "vkai-panel" || entry.Actor.Serial != "0a1b2c" {
		t.Fatalf("the entry does not name the certificate that asked: %+v", entry.Actor)
	}
	if entry.Detail != "systemctl restart nginx" {
		t.Fatalf("the entry records %q as what was run, want the exact command line", entry.Detail)
	}
	if entry.AgentID != "agent-42" {
		t.Fatalf("the entry is not attributable to this agent: %q", entry.AgentID)
	}
	if entry.Time.IsZero() {
		t.Fatal("the entry has no timestamp")
	}
}

// A refusal is often the more interesting line: it is what an attempt to reach
// outside the allowed paths looks like in the record.
func TestARefusalIsRecordedTheSameWayAsAnExecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operations.log")
	record := Open(Options{Path: path, Fallback: log.New(os.Stderr, "", 0)})
	defer record.Close()

	record.Record(Entry{
		Actor:     Actor{Name: "vkai-panel"},
		Operation: "log.read",
		Outcome:   OutcomeRefused,
		Arguments: `{"path":"/etc/shadow"}`,
		Error:     "ops: invalid argument: /etc/shadow is outside the log directories",
	})
	entries := readEntries(t, path)
	if len(entries) != 1 || entries[0].Outcome != OutcomeRefused {
		t.Fatalf("the refusal was not recorded: %+v", entries)
	}
	if !strings.Contains(entries[0].Arguments, "/etc/shadow") {
		t.Fatalf("the refused arguments were not recorded: %+v", entries[0])
	}
}

// An argument object is capped at a megabyte by the HTTP layer. A megabyte per
// line would fill a small VPS's disk faster than any legitimate use of the
// channel.
func TestAnOversizedArgumentIsTruncatedAndSaysSo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operations.log")
	record := Open(Options{Path: path, Fallback: log.New(os.Stderr, "", 0)})
	defer record.Close()

	record.Record(Entry{Operation: "exec.raw", Arguments: strings.Repeat("A", 100000)})
	entries := readEntries(t, path)
	if len(entries[0].Arguments) > MaxArgumentBytes+64 {
		t.Fatalf("the recorded argument is %d bytes, want it capped near %d",
			len(entries[0].Arguments), MaxArgumentBytes)
	}
	if !strings.Contains(entries[0].Arguments, "truncated") {
		t.Fatal("a truncated argument does not say that it was truncated")
	}
}

// A record that grows without limit is a way to fill the disk of the machine it
// is meant to protect.
func TestTheRecordRotatesAndKeepsAFixedNumberOfFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "operations.log")
	record := Open(Options{Path: path, MaxBytes: 512, Keep: 2, Fallback: log.New(os.Stderr, "", 0)})
	defer record.Close()

	for n := 0; n < 200; n++ {
		record.Record(Entry{Operation: "service.status", Detail: strings.Repeat("x", 100)})
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cannot list %s: %v", dir, err)
	}
	if len(entries) > 3 { // the current file plus Keep rotations
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("the record left %d files behind: %v", len(entries), names)
	}
	for _, entry := range entries {
		info, statErr := entry.Info()
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Size() > 4096 {
			t.Fatalf("%s grew to %d bytes despite a 512 byte rotation limit", entry.Name(), info.Size())
		}
	}
}

// An agent that stops answering the panel because its log directory has the
// wrong permissions is worse than an agent whose record is degraded and says so.
func TestAnUnwritableRecordDegradesToTheProcessLogRatherThanFailing(t *testing.T) {
	// A path whose parent is a regular file cannot be created as a directory.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	var captured bytes.Buffer
	record := Open(Options{
		Path:     filepath.Join(blocker, "sub", "operations.log"),
		Fallback: log.New(&captured, "", 0),
	})
	defer record.Close()

	if record.Path() != "" {
		t.Fatalf("a record that cannot be written reports itself as writable at %s", record.Path())
	}
	if !strings.Contains(captured.String(), "WARNING") {
		t.Fatalf("the failure was not announced: %q", captured.String())
	}

	captured.Reset()
	record.Record(Entry{
		Actor:     Actor{Name: "vkai-panel"},
		Operation: "service.control",
		Outcome:   OutcomeExecuted,
		Detail:    "systemctl restart nginx",
	})
	line := captured.String()
	if !strings.Contains(line, "service.control") || !strings.Contains(line, "systemctl restart nginx") {
		t.Fatalf("the operation was not mirrored into the process log: %q", line)
	}
	if !strings.Contains(line, "vkai-panel") {
		t.Fatalf("the mirrored line does not name who asked: %q", line)
	}
}

// An argument containing a space must not read as two arguments, or the record
// is misleading about the exact thing it exists to prove.
func TestArgvQuotesArgumentsThatWouldOtherwiseSplit(t *testing.T) {
	got := Argv("systemctl", "restart", "my service")
	if got != `systemctl restart "my service"` {
		t.Fatalf("Argv produced %q", got)
	}
}

func TestTheActorTravelsWithTheContext(t *testing.T) {
	actor := Actor{Name: "vkai-panel", Serial: "ff00", Address: "10.0.0.5:1234"}
	ctx := WithActor(context.Background(), actor)
	if got := ActorFrom(ctx); got != actor {
		t.Fatalf("the actor came back as %+v", got)
	}
	// Work the agent starts on its own has no caller, and must not appear in
	// the record as an empty one.
	if got := ActorFrom(context.Background()); got.Name == "" {
		t.Fatal("an operation with no caller produced a nameless actor")
	}
}

func TestConcurrentRecordsAreAllWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operations.log")
	record := Open(Options{Path: path, Fallback: log.New(os.Stderr, "", 0)})
	defer record.Close()

	const writers = 20
	done := make(chan struct{}, writers)
	for n := 0; n < writers; n++ {
		go func() {
			for i := 0; i < 25; i++ {
				record.Record(Entry{Operation: "system.metrics", Time: time.Now()})
			}
			done <- struct{}{}
		}()
	}
	for n := 0; n < writers; n++ {
		<-done
	}
	if entries := readEntries(t, path); len(entries) != writers*25 {
		t.Fatalf("the record holds %d entries, want %d", len(entries), writers*25)
	}
}

// A zero in the options means "unset", not "keep nothing": a caller that passes
// no rotation settings must get history, not a record that discards itself the
// first time it fills up.
func TestTheDefaultsKeepHistoryRatherThanDiscardingIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "operations.log")
	record := Open(Options{Path: path, MaxBytes: 512, Fallback: log.New(os.Stderr, "", 0)})
	defer record.Close()

	for n := 0; n < 100; n++ {
		record.Record(Entry{Operation: "service.status", Detail: strings.Repeat("x", 100)})
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("rotation left only %d file(s); the history was discarded", len(entries))
	}
}
