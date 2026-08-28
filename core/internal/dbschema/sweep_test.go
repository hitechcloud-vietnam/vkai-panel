package dbschema_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/dbschema"
)

// coreRoot returns the absolute path of core/, derived from this test file's
// own location so the test does not depend on the working directory.
func coreRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine the path of this test file")
	}
	// .../core/internal/dbschema/sweep_test.go -> .../core
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// loadSchema returns the schema to check against. When VKAI_SCHEMA_DSN is set
// the schema is read from that live PostgreSQL database, which is the ground
// truth; otherwise it is replayed from the migration files, which needs no
// database and so runs in a plain `go test ./...`.
func loadSchema(t *testing.T, root string) *dbschema.Schema {
	t.Helper()

	if dsn := os.Getenv("VKAI_SCHEMA_DSN"); dsn != "" {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatalf("open VKAI_SCHEMA_DSN: %v", err)
		}
		defer db.Close()
		if err := db.Ping(); err != nil {
			t.Fatalf("ping VKAI_SCHEMA_DSN: %v", err)
		}
		schema, err := dbschema.LoadSchemaFromDB(db)
		if err != nil {
			t.Fatalf("read schema from database: %v", err)
		}
		return schema
	}

	schema, err := dbschema.LoadSchemaFromMigrations(filepath.Join(root, "migrations"))
	if err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
	return schema
}

// TestMigrationReplayMatchesLiveDatabase proves the no-database path of this
// check tells the truth. It only runs when VKAI_SCHEMA_DSN points at a database
// that has had every migration applied.
func TestMigrationReplayMatchesLiveDatabase(t *testing.T) {
	dsn := os.Getenv("VKAI_SCHEMA_DSN")
	if dsn == "" {
		t.Skip("VKAI_SCHEMA_DSN is not set; skipping the live-database cross-check")
	}
	root := coreRoot(t)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open VKAI_SCHEMA_DSN: %v", err)
	}
	defer db.Close()
	live, err := dbschema.LoadSchemaFromDB(db)
	if err != nil {
		t.Fatalf("read schema from database: %v", err)
	}
	replayed, err := dbschema.LoadSchemaFromMigrations(filepath.Join(root, "migrations"))
	if err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	var diffs []string
	for _, table := range live.TableNames() {
		if !replayed.HasTable(table) {
			diffs = append(diffs, fmt.Sprintf("table %s is in the database but not in the migrations", table))
			continue
		}
		for _, col := range live.ColumnNames(table) {
			if !replayed.HasColumn(table, col) {
				diffs = append(diffs, fmt.Sprintf("%s.%s is in the database but not in the migrations", table, col))
			}
		}
	}
	for _, table := range replayed.TableNames() {
		if !live.HasTable(table) {
			diffs = append(diffs, fmt.Sprintf("table %s is in the migrations but not in the database", table))
			continue
		}
		for _, col := range replayed.ColumnNames(table) {
			if !live.HasColumn(table, col) {
				diffs = append(diffs, fmt.Sprintf("%s.%s is in the migrations but not in the database", table, col))
			}
		}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Fatalf("the migration replay disagrees with the live schema:\n  %s", strings.Join(diffs, "\n  "))
	}
	t.Logf("migration replay matches the live schema: %d tables", len(live.Tables))
}

// knownGaps are column references that are defects, are reported by the sweep,
// and are NOT fixed here because the files that contain them are owned by
// another agent for the duration of this change. They are listed so the sweep
// can stay green for everything else without any of them going quiet.
//
// Every entry is a real bug. The firewall CLI writes source_ip, comment and
// enabled into a table whose columns are source, (nothing) and status, and soft
// deletes against a deleted_at that firewall_rules has never had. The site CLI
// writes document_root into a table whose column is root_dir. Each is an
// immediate runtime error the first time the subcommand is run.
//
// A waiver that no longer matches a finding fails this test on purpose: when
// the owner fixes one of these, the line below has to be deleted, and the
// failure message says so. That is what stops this list becoming a place where
// defects go to be forgotten.
var knownGaps = []struct {
	File   string // path relative to core/
	Table  string
	Column string
	Why    string
}{
	{"internal/cli/firewall.go", "firewall_rules", "source_ip", "schema calls it source; internal/cli is owned by another agent"},
	{"internal/cli/firewall.go", "firewall_rules", "comment", "no such column; internal/cli is owned by another agent"},
	{"internal/cli/firewall.go", "firewall_rules", "enabled", "schema calls it status; internal/cli is owned by another agent"},
	{"internal/cli/firewall.go", "firewall_rules", "deleted_at", "firewall_rules has no soft delete; internal/cli is owned by another agent"},
	{"internal/cli/site.go", "websites", "document_root", "schema calls it root_dir; internal/cli is owned by another agent"},
}

// TestQueryColumnsExistInSchema is the sweep. Every column named by a Go SQL
// query must exist in the schema. A query that names a column the database has
// never had is an endpoint that fails the first time somebody presses the
// button, and nothing else in the build catches it.
func TestQueryColumnsExistInSchema(t *testing.T) {
	root := coreRoot(t)
	schema := loadSchema(t, root)

	// Only the directories that exist. "pkg" is passed because a Go layout often
	// has one; this module does not, and a sweep that fails because a
	// conventional directory is absent is reporting the repository layout as a
	// defect.
	stmts, err := dbschema.CollectStatements(existingDirs(root, "internal", "cmd", "pkg")...)
	if err != nil {
		t.Fatalf("collect SQL statements: %v", err)
	}
	if len(stmts) == 0 {
		t.Fatal("no SQL statements found; the collector is broken")
	}

	findings, skipped := dbschema.Sweep(schema, stmts)

	t.Logf("schema source: %s (%d tables)", schema.Source, len(schema.Tables))
	t.Logf("statements found: %d, analysed: %d, not analysed: %d",
		len(stmts), len(stmts)-len(skipped), len(skipped))

	for _, st := range skipped {
		t.Logf("NOT ANALYSED %s:%d %s: %s | %s",
			rel(root, st.File), st.Line, st.Func, st.Skipped, headOf(st.SQL))
	}

	// Split the findings into the ones a waiver covers and the ones that fail
	// the build.
	used := make([]bool, len(knownGaps))
	var blocking []dbschema.Finding
	for _, f := range findings {
		file := rel(root, f.File)
		waived := false
		for i, gap := range knownGaps {
			if gap.File == filepath.ToSlash(file) && gap.Table == f.Table && gap.Column == f.Column {
				used[i] = true
				waived = true
				break
			}
		}
		if waived {
			t.Logf("WAIVED %s:%d %s.%s -- %s", file, f.Line, f.Table, f.Column, gapReason(f.Table, f.Column))
			continue
		}
		blocking = append(blocking, f)
	}
	for i, gap := range knownGaps {
		if !used[i] {
			t.Errorf("stale waiver: %s / %s.%s is no longer reported by the sweep. "+
				"Delete that entry from knownGaps in %s.",
				gap.File, gap.Table, gap.Column, "internal/dbschema/sweep_test.go")
		}
	}

	findings = blocking
	if len(findings) == 0 {
		return
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%d query column(s) do not exist in the schema:\n", len(findings))
	for _, f := range findings {
		if f.Column == "" {
			fmt.Fprintf(&b, "  %s:%d %s: %s %s -- %s\n     %s\n",
				rel(root, f.File), f.Line, f.Func, f.Kind, f.Table, f.Reason, f.SQLHead)
			continue
		}
		fmt.Fprintf(&b, "  %s:%d %s: %s %s.%s (%s) -- %s\n     %s\n",
			rel(root, f.File), f.Line, f.Func, f.Kind, f.Table, f.Column, f.Clause, f.Reason, f.SQLHead)
	}
	t.Fatal(b.String())
}

func gapReason(table, column string) string {
	for _, gap := range knownGaps {
		if gap.Table == table && gap.Column == column {
			return gap.Why
		}
	}
	return "unknown"
}

func rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return r
	}
	return path
}

func headOf(sqlText string) string {
	s := strings.Join(strings.Fields(sqlText), " ")
	if len(s) > 90 {
		s = s[:90] + " ..."
	}
	return s
}

// preparableSkips are the reasons the reader declines to analyse a statement
// that nevertheless leave a complete, valid SQL string behind. PostgreSQL can
// check those itself, which is how the joins and subqueries the reader refuses
// to guess at still get verified.
var preparableSkips = map[string]bool{
	"contains a join":     true,
	"contains a subquery": true,
}

// TestStatementsPrepareAgainstLiveSchema asks PostgreSQL to prepare every query
// in the repository. A statement that prepares is a statement whose tables,
// columns and types all exist: it is the strongest form of this check, and it
// covers the joins and subqueries the reader in this package deliberately
// refuses to analyse.
//
// It runs only when VKAI_SCHEMA_DSN points at a database with every migration
// applied, because it needs a real server to do the work.
func TestStatementsPrepareAgainstLiveSchema(t *testing.T) {
	dsn := os.Getenv("VKAI_SCHEMA_DSN")
	if dsn == "" {
		t.Skip("VKAI_SCHEMA_DSN is not set; skipping the live PREPARE check")
	}
	root := coreRoot(t)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open VKAI_SCHEMA_DSN: %v", err)
	}
	defer db.Close()

	// Only the directories that exist. "pkg" is passed because a Go layout often
	// has one; this module does not, and a sweep that fails because a
	// conventional directory is absent is reporting the repository layout as a
	// defect.
	stmts, err := dbschema.CollectStatements(existingDirs(root, "internal", "cmd", "pkg")...)
	if err != nil {
		t.Fatalf("collect SQL statements: %v", err)
	}

	prepared, failed := 0, 0
	for _, st := range stmts {
		if st.Skipped != "" && !preparableSkips[st.Skipped] {
			continue
		}
		file := filepath.ToSlash(rel(root, st.File))
		if waivedFile(file) {
			continue
		}
		ps, err := db.PrepareContext(context.Background(), st.SQL)
		if err != nil {
			failed++
			t.Errorf("%s:%d %s: PostgreSQL refused to prepare this statement: %v\n     %s",
				file, st.Line, st.Func, err, headOf(st.SQL))
			continue
		}
		_ = ps.Close()
		prepared++
	}
	t.Logf("prepared %d statement(s) against the live schema, %d refused", prepared, failed)
}

// waivedFile reports whether a file has a knownGaps entry. Those files are
// known to be broken and are owned by another agent, so PostgreSQL is not asked
// to prepare their statements either.
func waivedFile(file string) bool {
	for _, gap := range knownGaps {
		if gap.File == file {
			return true
		}
	}
	return false
}

// existingDirs returns the given subdirectories of root that are actually
// present. A collector that walks a fixed list of conventional directories
// should skip the ones this repository does not have, rather than report their
// absence as a failure of the sweep.
func existingDirs(root string, names ...string) []string {
	dirs := make([]string, 0, len(names))
	for _, name := range names {
		p := filepath.Join(root, name)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			dirs = append(dirs, p)
		}
	}
	return dirs
}
