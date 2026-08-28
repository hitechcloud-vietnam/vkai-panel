package dbschema

import (
	"fmt"
	"strings"
)

// This file contains a deliberately small SQL reader. It is not a general
// parser: it recognises the shapes this codebase actually writes
// (single-table SELECT / INSERT / UPDATE / DELETE) and refuses to guess at
// anything else. A statement it cannot read with confidence is reported as
// skipped, never as a pass and never as a defect, so the sweep's blind spots
// stay visible instead of turning into false clean bills of health.

type tokenKind int

const (
	tokIdent tokenKind = iota
	tokNumber
	tokString
	tokParam // $1, $2
	tokPunct
)

type sqlToken struct {
	kind  tokenKind
	text  string
	upper string
}

func (t sqlToken) is(punct string) bool { return t.kind == tokPunct && t.text == punct }
func (t sqlToken) kw(word string) bool  { return t.kind == tokIdent && t.upper == word }

func tokenizeSQL(s string) []sqlToken {
	var toks []sqlToken
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i = min(i+2, len(s))
		case c == '\'':
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
			toks = append(toks, sqlToken{kind: tokString, text: s[i:min(j, len(s))]})
			i = j
		case c == '"':
			j := i + 1
			for j < len(s) && s[j] != '"' {
				j++
			}
			text := s[i+1 : min(j, len(s))]
			toks = append(toks, sqlToken{kind: tokIdent, text: text, upper: strings.ToUpper(text)})
			i = min(j+1, len(s))
		case c == '$':
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			toks = append(toks, sqlToken{kind: tokParam, text: s[i:j]})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(s) && isIdentPart(s[j]) {
				j++
			}
			text := s[i:j]
			toks = append(toks, sqlToken{kind: tokIdent, text: text, upper: strings.ToUpper(text)})
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == '.') {
				j++
			}
			toks = append(toks, sqlToken{kind: tokNumber, text: s[i:j]})
			i = j
		case c == ':' && i+1 < len(s) && s[i+1] == ':':
			toks = append(toks, sqlToken{kind: tokPunct, text: "::"})
			i += 2
		default:
			toks = append(toks, sqlToken{kind: tokPunct, text: string(c)})
			i++
		}
	}
	return toks
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// sqlWords are words that appear in identifier position but never name a
// column: reserved words, clause keywords, literals, type names reachable
// without a cast operator, and the unit names EXTRACT and INTERVAL take.
var sqlWords = map[string]bool{
	"ALL": true, "AND": true, "ANY": true, "AS": true, "ASC": true, "BETWEEN": true,
	"BY": true, "CASE": true, "CAST": true, "CONFLICT": true, "CROSS": true,
	"CURRENT_DATE": true, "CURRENT_TIME": true, "CURRENT_TIMESTAMP": true,
	"CURRENT_USER": true, "DEFAULT": true, "DELETE": true, "DESC": true,
	"DISTINCT": true, "DO": true, "ELSE": true, "END": true, "ESCAPE": true,
	"EXCEPT": true, "EXISTS": true, "FALSE": true, "FETCH": true, "FILTER": true,
	"FIRST": true, "FOR": true, "FROM": true, "FULL": true, "GROUP": true,
	"HAVING": true, "ILIKE": true, "IN": true, "INNER": true, "INSERT": true,
	"INTERSECT": true, "INTO": true, "IS": true, "ISNULL": true, "JOIN": true,
	"KEY": true, "LAST": true, "LATERAL": true, "LEFT": true, "LIKE": true,
	"LIMIT": true, "LOCALTIMESTAMP": true, "NATURAL": true, "NEXT": true,
	"NOT": true, "NOTHING": true, "NOTNULL": true, "NULL": true, "NULLS": true,
	"OFFSET": true, "ON": true, "ONLY": true, "OR": true, "ORDER": true,
	"OUTER": true, "OVER": true, "PARTITION": true, "RETURNING": true,
	"RIGHT": true, "ROW": true, "ROWS": true, "SELECT": true, "SET": true,
	"SHARE": true, "SIMILAR": true, "SOME": true, "SYMMETRIC": true, "THEN": true,
	"TRUE": true, "UNION": true, "UNKNOWN": true, "UPDATE": true, "USING": true,
	"VALUES": true, "WHEN": true, "WHERE": true, "WITH": true, "WITHIN": true,

	// EXTRACT / INTERVAL unit names and AT TIME ZONE.
	"AT": true, "CENTURY": true, "DAY": true, "DAYS": true, "DECADE": true,
	"DOW": true, "DOY": true, "EPOCH": true, "HOUR": true, "HOURS": true,
	"INTERVAL": true, "ISODOW": true, "ISOYEAR": true, "MICROSECONDS": true,
	"MILLENNIUM": true, "MILLISECONDS": true, "MINUTE": true, "MINUTES": true,
	"MONTH": true, "MONTHS": true, "QUARTER": true, "SECOND": true,
	"SECONDS": true, "TIME": true, "TIMEZONE": true, "WEEK": true, "YEAR": true,
	"YEARS": true, "ZONE": true,

	// Type names that appear bare after CAST(... AS x) or in ARRAY[]::x.
	"BIGINT": true, "BOOLEAN": true, "BYTEA": true, "DATE": true, "DOUBLE": true,
	"FLOAT": true, "INET": true, "INT": true, "INTEGER": true, "JSON": true,
	"JSONB": true, "NUMERIC": true, "PRECISION": true, "REAL": true,
	"SMALLINT": true, "TEXT": true, "TIMESTAMP": true, "TIMESTAMPTZ": true,
	"UUID": true, "VARCHAR": true,
}

// ColumnRef is one column named by a query.
type ColumnRef struct {
	Table  string
	Column string
	Clause string // where in the statement it appeared, for the report
}

// Statement is one SQL string literal found in the Go sources, together with
// what the reader could work out about it.
type Statement struct {
	File    string
	Line    int
	Func    string
	SQL     string
	Kind    string // SELECT, INSERT, UPDATE, DELETE
	Table   string
	Refs    []ColumnRef
	Star    bool   // the statement selects *
	Skipped string // non-empty when the reader declined to analyse it

	// alias is the FROM/UPDATE alias, used to resolve qualified references.
	alias string
}

// AnalyzeSQL reads one statement and returns the columns it names.
func AnalyzeSQL(sqlText string) Statement {
	st := Statement{SQL: sqlText}

	if strings.Contains(sqlText, "%") {
		st.Skipped = "query is assembled with fmt formatting verbs"
		return st
	}

	toks := tokenizeSQL(sqlText)
	if len(toks) == 0 {
		st.Skipped = "empty statement"
		return st
	}

	// A subquery makes column ownership ambiguous for a reader this small.
	for i := 0; i < len(toks)-1; i++ {
		if toks[i].is("(") && toks[i+1].kw("SELECT") {
			st.Skipped = "contains a subquery"
			return st
		}
	}
	// More than one top-level statement in one literal.
	for _, t := range toks {
		if t.is(";") {
			st.Skipped = "contains multiple statements"
			return st
		}
	}

	st.Kind = toks[0].upper
	switch st.Kind {
	case "SELECT":
		analyzeSelect(&st, toks)
	case "INSERT":
		analyzeInsert(&st, toks)
	case "UPDATE":
		analyzeUpdate(&st, toks)
	case "DELETE":
		analyzeDelete(&st, toks)
	default:
		st.Skipped = fmt.Sprintf("unsupported statement kind %q", st.Kind)
	}
	return st
}

// findTopLevel returns the index of the first token at paren depth 0 matching
// any of the given keywords, or -1.
func findTopLevel(toks []sqlToken, from int, words ...string) int {
	depth := 0
	for i := from; i < len(toks); i++ {
		if toks[i].is("(") {
			depth++
			continue
		}
		if toks[i].is(")") {
			depth--
			continue
		}
		if depth != 0 || toks[i].kind != tokIdent {
			continue
		}
		for _, w := range words {
			if toks[i].upper == w {
				return i
			}
		}
	}
	return -1
}

// readTable reads a table name and optional alias starting at index i. When
// the name is schema-qualified the qualifier is returned separately: a query
// against information_schema or pg_catalog is not a query against this
// application's schema and must not be checked as one.
func readTable(toks []sqlToken, i int) (name, alias, qualifier string, next int, ok bool) {
	if i >= len(toks) || toks[i].kind != tokIdent || sqlWords[toks[i].upper] {
		return "", "", "", i, false
	}
	name = toks[i].text
	i++
	// schema-qualified name
	if i+1 < len(toks) && toks[i].is(".") && toks[i+1].kind == tokIdent {
		qualifier = name
		name = toks[i+1].text
		i += 2
	}
	if i < len(toks) && toks[i].kw("AS") {
		i++
	}
	if i < len(toks) && toks[i].kind == tokIdent && !sqlWords[toks[i].upper] {
		alias = toks[i].text
		i++
	}
	return name, alias, qualifier, i, true
}

// foreignSchema reports whether a qualifier names a catalog outside this
// application's schema.
func foreignSchema(qualifier string) bool {
	switch strings.ToLower(qualifier) {
	case "", "public":
		return false
	}
	return true
}

// collectIdents walks a token range and records every identifier that must be
// a column of the statement's single table.
func collectIdents(st *Statement, toks []sqlToken, lo, hi int, clause string) {
	for i := lo; i < hi && i < len(toks); i++ {
		t := toks[i]
		if t.kind != tokIdent {
			continue
		}
		if sqlWords[t.upper] {
			continue
		}
		// Function call: name immediately followed by "(".
		if i+1 < len(toks) && toks[i+1].is("(") {
			continue
		}
		// Alias introduced by AS.
		if i > 0 && toks[i-1].kw("AS") {
			continue
		}
		// Type name after a :: cast.
		if i > 0 && toks[i-1].is("::") {
			continue
		}
		// Qualified reference table.column / alias.column.
		if i+2 < len(toks) && toks[i+1].is(".") && toks[i+2].kind == tokIdent {
			qualifier := t.text
			column := toks[i+2].text
			if strings.EqualFold(qualifier, st.Table) || (st.alias != "" && strings.EqualFold(qualifier, st.alias)) {
				st.Refs = append(st.Refs, ColumnRef{Table: st.Table, Column: column, Clause: clause})
			}
			i += 2
			continue
		}
		// Right-hand side of a qualified reference, already consumed above
		// unless the qualifier was unknown; skip it either way.
		if i > 0 && toks[i-1].is(".") {
			continue
		}
		st.Refs = append(st.Refs, ColumnRef{Table: st.Table, Column: t.text, Clause: clause})
	}
}

func analyzeSelect(st *Statement, toks []sqlToken) {
	from := findTopLevel(toks, 1, "FROM")
	if from < 0 {
		st.Skipped = "SELECT without a FROM clause"
		return
	}
	name, alias, qualifier, next, ok := readTable(toks, from+1)
	if !ok {
		st.Skipped = "could not read the FROM table"
		return
	}
	if foreignSchema(qualifier) {
		st.Skipped = "queries the " + qualifier + " catalog, not the application schema"
		return
	}
	// A name followed by an open paren in FROM position is a set-returning
	// function, not a table. Its result columns are defined by the function,
	// which this reader cannot see, so there is nothing here to check.
	if next < len(toks) && toks[next].is("(") {
		st.Skipped = "selects from the function " + name + "(), not a table"
		return
	}
	// Reject joins and comma-separated table lists.
	if next < len(toks) {
		t := toks[next]
		if t.is(",") {
			st.Skipped = "selects from more than one table"
			return
		}
		if t.kind == tokIdent {
			switch t.upper {
			case "JOIN", "INNER", "LEFT", "RIGHT", "FULL", "CROSS", "NATURAL", "LATERAL":
				st.Skipped = "contains a join"
				return
			}
		}
	}
	st.Table = name
	st.alias = alias

	// Projection list.
	for i := 1; i < from; i++ {
		if toks[i].is("*") {
			st.Star = true
		}
	}
	collectIdents(st, toks, 1, from, "select list")

	// GROUP BY and ORDER BY may name an output alias from the select list
	// rather than a column, so those names must not be checked as columns.
	before := len(st.Refs)
	collectIdents(st, toks, next, len(toks), "where/group/order")
	aliases := outputAliases(toks, 1, from)
	if len(aliases) > 0 {
		kept := st.Refs[:before]
		for _, ref := range st.Refs[before:] {
			if !aliases[strings.ToLower(ref.Column)] {
				kept = append(kept, ref)
			}
		}
		st.Refs = kept
	}
}

// outputAliases returns the names introduced by AS in a select list.
func outputAliases(toks []sqlToken, lo, hi int) map[string]bool {
	aliases := make(map[string]bool)
	for i := lo; i < hi && i < len(toks); i++ {
		if toks[i].kw("AS") && i+1 < hi && toks[i+1].kind == tokIdent {
			aliases[strings.ToLower(toks[i+1].text)] = true
		}
	}
	return aliases
}

func analyzeInsert(st *Statement, toks []sqlToken) {
	into := findTopLevel(toks, 1, "INTO")
	if into < 0 {
		st.Skipped = "INSERT without INTO"
		return
	}
	name, _, qualifier, next, ok := readTable(toks, into+1)
	if !ok {
		st.Skipped = "could not read the INSERT target table"
		return
	}
	if foreignSchema(qualifier) {
		st.Skipped = "writes to the " + qualifier + " catalog, not the application schema"
		return
	}
	st.Table = name

	if next >= len(toks) || !toks[next].is("(") {
		st.Skipped = "INSERT without an explicit column list"
		return
	}
	depth := 0
	end := next
	for i := next; i < len(toks); i++ {
		if toks[i].is("(") {
			depth++
		} else if toks[i].is(")") {
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
	}
	for i := next + 1; i < end; i++ {
		if toks[i].kind == tokIdent && !sqlWords[toks[i].upper] {
			st.Refs = append(st.Refs, ColumnRef{Table: st.Table, Column: toks[i].text, Clause: "insert columns"})
		}
	}

	// ON CONFLICT (...) DO UPDATE SET ... and RETURNING ...
	if conflict := findTopLevel(toks, end, "CONFLICT"); conflict >= 0 {
		collectIdents(st, toks, conflict+1, len(toks), "on conflict")
	} else if ret := findTopLevel(toks, end, "RETURNING"); ret >= 0 {
		collectIdents(st, toks, ret+1, len(toks), "returning")
	}
}

func analyzeUpdate(st *Statement, toks []sqlToken) {
	name, alias, qualifier, next, ok := readTable(toks, 1)
	if !ok {
		st.Skipped = "could not read the UPDATE target table"
		return
	}
	if foreignSchema(qualifier) {
		st.Skipped = "writes to the " + qualifier + " catalog, not the application schema"
		return
	}
	if findTopLevel(toks, next, "FROM") >= 0 {
		st.Skipped = "UPDATE ... FROM references more than one table"
		return
	}
	st.Table = name
	st.alias = alias
	set := findTopLevel(toks, next, "SET")
	if set < 0 {
		st.Skipped = "UPDATE without SET"
		return
	}
	collectIdents(st, toks, set+1, len(toks), "set/where")
}

func analyzeDelete(st *Statement, toks []sqlToken) {
	from := findTopLevel(toks, 1, "FROM")
	if from < 0 {
		st.Skipped = "DELETE without FROM"
		return
	}
	if findTopLevel(toks, from, "USING") >= 0 {
		st.Skipped = "DELETE ... USING references more than one table"
		return
	}
	name, alias, qualifier, next, ok := readTable(toks, from+1)
	if !ok {
		st.Skipped = "could not read the DELETE target table"
		return
	}
	if foreignSchema(qualifier) {
		st.Skipped = "writes to the " + qualifier + " catalog, not the application schema"
		return
	}
	if next < len(toks) && toks[next].is("(") {
		st.Skipped = "deletes from the function " + name + "(), not a table"
		return
	}
	st.Table = name
	st.alias = alias
	collectIdents(st, toks, next, len(toks), "where")
}
