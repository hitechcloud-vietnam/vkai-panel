// Package dbschema compares the columns named by the Go SQL queries in this
// repository against the columns the database actually has.
//
// It exists because a whole class of defect in this project was "the code
// queries a column the schema has never had": the query compiles, the unit
// tests pass because they never touch PostgreSQL, and the endpoint fails the
// first time a customer presses the button. A build-time sweep catches that
// class before it ships.
//
// The authoritative schema can be loaded two ways:
//
//   - LoadSchemaFromMigrations replays the CREATE TABLE / ALTER TABLE
//     statements in core/migrations (and core/migrations/pending) the same way
//     deploy/install.sh applies them. This needs no database, so it runs in a
//     plain `go test ./...`.
//   - LoadSchemaFromDB reads information_schema from a live PostgreSQL. This is
//     the ground truth and is used when VKAI_SCHEMA_DSN points at a throwaway
//     database.
//
// Both loaders produce the same Schema type, so the sweep is identical either
// way.
package dbschema

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Schema is the set of tables and their columns.
type Schema struct {
	// Tables maps a lower-cased table name to its lower-cased column set.
	Tables map[string]map[string]bool
	// Views records relations the loader saw but cannot resolve columns for.
	// References into these are reported as unverifiable, never as defects.
	Views map[string]bool
	// Source describes where the schema came from, for reporting.
	Source string
}

func newSchema(source string) *Schema {
	return &Schema{
		Tables: make(map[string]map[string]bool),
		Views:  make(map[string]bool),
		Source: source,
	}
}

// HasTable reports whether the schema defines the named table.
func (s *Schema) HasTable(table string) bool {
	_, ok := s.Tables[strings.ToLower(table)]
	return ok
}

// HasColumn reports whether the named table defines the named column.
func (s *Schema) HasColumn(table, column string) bool {
	cols, ok := s.Tables[strings.ToLower(table)]
	if !ok {
		return false
	}
	return cols[strings.ToLower(column)]
}

// TableNames returns the table names in sorted order.
func (s *Schema) TableNames() []string {
	names := make([]string, 0, len(s.Tables))
	for name := range s.Tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ColumnNames returns the columns of a table in sorted order.
func (s *Schema) ColumnNames(table string) []string {
	cols := s.Tables[strings.ToLower(table)]
	names := make([]string, 0, len(cols))
	for name := range cols {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// LoadSchemaFromDB reads the public schema of a live PostgreSQL database.
// The caller supplies an already-open *sql.DB so this package does not have to
// choose a driver.
func LoadSchemaFromDB(db *sql.DB) (*Schema, error) {
	schema := newSchema("live database")

	rows, err := db.Query(`
		SELECT c.table_name, c.column_name
		FROM information_schema.columns c
		JOIN information_schema.tables t
		  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = 'public' AND t.table_type = 'BASE TABLE'
	`)
	if err != nil {
		return nil, fmt.Errorf("query information_schema.columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return nil, err
		}
		schema.addColumn(table, column)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	viewRows, err := db.Query(`
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type <> 'BASE TABLE'
	`)
	if err != nil {
		return nil, fmt.Errorf("query information_schema.tables: %w", err)
	}
	defer viewRows.Close()
	for viewRows.Next() {
		var name string
		if err := viewRows.Scan(&name); err != nil {
			return nil, err
		}
		schema.Views[strings.ToLower(name)] = true
	}
	return schema, viewRows.Err()
}

func (s *Schema) addColumn(table, column string) {
	table = strings.ToLower(table)
	cols, ok := s.Tables[table]
	if !ok {
		cols = make(map[string]bool)
		s.Tables[table] = cols
	}
	cols[strings.ToLower(column)] = true
}

// MigrationFiles returns the migration files in the order deploy/install.sh
// applies them: the numbered files in migrationsDir first, then the files in
// migrationsDir/pending, each group sorted by name.
func MigrationFiles(migrationsDir string) ([]string, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, err
	}
	var numbered []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		numbered = append(numbered, filepath.Join(migrationsDir, e.Name()))
	}
	sort.Strings(numbered)

	pendingDir := filepath.Join(migrationsDir, "pending")
	var pending []string
	if pendingEntries, err := os.ReadDir(pendingDir); err == nil {
		for _, e := range pendingEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
				continue
			}
			pending = append(pending, filepath.Join(pendingDir, e.Name()))
		}
		sort.Strings(pending)
	}
	return append(numbered, pending...), nil
}

var (
	reCreateTable = regexp.MustCompile(`(?is)\bCREATE\s+(?:UNLOGGED\s+|TEMP\s+|TEMPORARY\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?("?[A-Za-z_][A-Za-z0-9_]*"?)\s*\(`)
	reCreateView  = regexp.MustCompile(`(?is)\bCREATE\s+(?:OR\s+REPLACE\s+)?(?:MATERIALIZED\s+)?VIEW\s+(?:IF\s+NOT\s+EXISTS\s+)?("?[A-Za-z_][A-Za-z0-9_]*"?)`)
	reAlterTable  = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?("?[A-Za-z_][A-Za-z0-9_]*"?)\s+(.*?);`)
	reDropTable   = regexp.MustCompile(`(?is)\bDROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?("?[A-Za-z_][A-Za-z0-9_]*"?)`)
	reAddColumn   = regexp.MustCompile(`(?is)\bADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?("?[A-Za-z_][A-Za-z0-9_]*"?)`)
	reDropColumn  = regexp.MustCompile(`(?is)\bDROP\s+COLUMN\s+(?:IF\s+EXISTS\s+)?("?[A-Za-z_][A-Za-z0-9_]*"?)`)
	reRenameCol   = regexp.MustCompile(`(?is)\bRENAME\s+COLUMN\s+("?[A-Za-z_][A-Za-z0-9_]*"?)\s+TO\s+("?[A-Za-z_][A-Za-z0-9_]*"?)`)
)

// tableConstraintKeywords are the words that start a table-level constraint in
// a CREATE TABLE body, as opposed to a column definition.
var tableConstraintKeywords = map[string]bool{
	"PRIMARY": true, "FOREIGN": true, "UNIQUE": true, "CHECK": true,
	"CONSTRAINT": true, "EXCLUDE": true, "LIKE": true, "DEFERRABLE": true,
}

// LoadSchemaFromMigrations replays the DDL in the migration files and returns
// the resulting schema.
func LoadSchemaFromMigrations(migrationsDir string) (*Schema, error) {
	files, err := MigrationFiles(migrationsDir)
	if err != nil {
		return nil, err
	}
	schema := newSchema("core/migrations")
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if err := schema.applyDDL(stripSQLComments(string(raw))); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
	}
	if len(schema.Tables) == 0 {
		return nil, fmt.Errorf("no tables found in %s", migrationsDir)
	}
	return schema, nil
}

func (s *Schema) applyDDL(sqlText string) error {
	for _, m := range reCreateView.FindAllStringSubmatch(sqlText, -1) {
		s.Views[strings.ToLower(unquoteIdent(m[1]))] = true
	}

	for _, loc := range reCreateTable.FindAllStringSubmatchIndex(sqlText, -1) {
		table := unquoteIdent(sqlText[loc[2]:loc[3]])
		bodyStart := loc[1] // just after the opening paren
		body, ok := balancedParenBody(sqlText, bodyStart)
		if !ok {
			return fmt.Errorf("unbalanced parentheses in CREATE TABLE %s", table)
		}
		if _, exists := s.Tables[strings.ToLower(table)]; !exists {
			s.Tables[strings.ToLower(table)] = make(map[string]bool)
		}
		for _, item := range splitTopLevel(body) {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			first := firstWord(item)
			if tableConstraintKeywords[strings.ToUpper(first)] {
				continue
			}
			s.addColumn(table, unquoteIdent(first))
		}
	}

	for _, m := range reDropTable.FindAllStringSubmatch(sqlText, -1) {
		delete(s.Tables, strings.ToLower(unquoteIdent(m[1])))
	}

	for _, m := range reAlterTable.FindAllStringSubmatch(sqlText, -1) {
		table := strings.ToLower(unquoteIdent(m[1]))
		actions := m[2]
		for _, a := range reAddColumn.FindAllStringSubmatch(actions, -1) {
			s.addColumn(table, unquoteIdent(a[1]))
		}
		for _, d := range reDropColumn.FindAllStringSubmatch(actions, -1) {
			if cols, ok := s.Tables[table]; ok {
				delete(cols, strings.ToLower(unquoteIdent(d[1])))
			}
		}
		for _, r := range reRenameCol.FindAllStringSubmatch(actions, -1) {
			if cols, ok := s.Tables[table]; ok {
				delete(cols, strings.ToLower(unquoteIdent(r[1])))
				cols[strings.ToLower(unquoteIdent(r[2]))] = true
			}
		}
	}
	return nil
}

// stripSQLComments removes -- line comments and /* */ block comments without
// disturbing the contents of string literals.
func stripSQLComments(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		switch {
		case s[i] == '\'':
			j := i + 1
			for j < len(s) {
				if s[j] == '\'' {
					if j+1 < len(s) && s[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			out.WriteString(s[i:min(j, len(s))])
			i = j
		case s[i] == '-' && i+1 < len(s) && s[i+1] == '-':
			j := i
			for j < len(s) && s[j] != '\n' {
				j++
			}
			out.WriteByte(' ')
			i = j
		case s[i] == '/' && i+1 < len(s) && s[i+1] == '*':
			j := i + 2
			for j+1 < len(s) && !(s[j] == '*' && s[j+1] == '/') {
				j++
			}
			out.WriteByte(' ')
			i = min(j+2, len(s))
		default:
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

// balancedParenBody returns the text between start (just after an opening
// paren) and its matching closing paren.
func balancedParenBody(s string, start int) (string, bool) {
	depth := 1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '\'':
			for i++; i < len(s); i++ {
				if s[i] == '\'' {
					if i+1 < len(s) && s[i+1] == '\'' {
						i++
						continue
					}
					break
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[start:i], true
			}
		}
	}
	return "", false
}

// splitTopLevel splits on commas that are not inside parentheses or strings.
func splitTopLevel(s string) []string {
	var parts []string
	depth := 0
	last := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			for i++; i < len(s); i++ {
				if s[i] == '\'' {
					if i+1 < len(s) && s[i+1] == '\'' {
						i++
						continue
					}
					break
				}
			}
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[last:i])
				last = i + 1
			}
		}
	}
	return append(parts, s[last:])
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || c == '"' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		return s[:i]
	}
	return s
}

func unquoteIdent(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"`)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
