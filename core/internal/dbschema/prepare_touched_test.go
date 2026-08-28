package dbschema_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/dbschema"
)

// touchedFiles are the files repaired by this change. Every statement in them
// is prepared against the live schema, one by one, with the result printed:
// a statement that prepares is a statement whose columns exist.
var touchedFiles = []string{
	"internal/repository/waf.go",
	"internal/repository/cluster.go",
	"internal/repository/job.go",
	"internal/repository/config.go",
	"internal/repository/daily_report.go",
	"internal/service/job.go",
	"internal/service/waf.go",
	"internal/service/cluster.go",
}

func TestTouchedStatementsPrepare(t *testing.T) {
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

	stmts, err := dbschema.CollectStatements(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatalf("collect SQL statements: %v", err)
	}

	want := make(map[string]bool, len(touchedFiles))
	for _, f := range touchedFiles {
		want[f] = true
	}

	seen := 0
	for _, st := range stmts {
		file := filepath.ToSlash(rel(root, st.File))
		if !want[file] {
			continue
		}
		if st.Skipped != "" && !preparableSkips[st.Skipped] {
			t.Logf("SKIP    %s:%d %s -- %s", file, st.Line, st.Func, st.Skipped)
			continue
		}
		ps, err := db.PrepareContext(context.Background(), st.SQL)
		if err != nil {
			t.Errorf("REFUSED %s:%d %s: %v\n     %s", file, st.Line, st.Func, err, headOf(st.SQL))
			continue
		}
		_ = ps.Close()
		seen++
		t.Logf("PREPARED %s:%d %s\n         %s", file, st.Line, st.Func, oneLine(st.SQL))
	}
	if seen == 0 {
		t.Fatal("no statements were prepared; the file list is wrong")
	}
	t.Logf("%d statement(s) prepared across %d repaired files", seen, len(touchedFiles))
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
