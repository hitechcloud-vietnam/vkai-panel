package dbschema

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// reSQLStart matches a string that begins a SQL statement. Fragments that are
// concatenated onto a query later (" AND status = $2") deliberately do not
// match on their own: the reader cannot tell which table they belong to, and
// guessing is how false confidence gets built. Concatenations of constants are
// folded into one string before this test is applied, so a query assembled from
// a shared column-list constant is still seen whole.
var reSQLStart = regexp.MustCompile(`(?is)^\s*(SELECT|INSERT|UPDATE|DELETE)\s`)

// unresolved marks the place in a folded string where a non-constant operand
// stood. Its presence makes the statement unanalysable rather than analysable
// with a hole in it.
const unresolved = "\x00?\x00"

// CollectStatements parses every Go file under the given roots and returns the
// SQL statements found in string expressions, annotated with file, line and
// enclosing function.
func CollectStatements(roots ...string) ([]Statement, error) {
	dirs, err := goDirs(roots)
	if err != nil {
		return nil, err
	}

	var stmts []Statement
	for _, dir := range dirs {
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, nil, 0)
		if err != nil {
			return nil, err
		}
		// String constants are resolved package-wide, so collect them from
		// every file in the directory before reading any query.
		consts := map[string]string{}
		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				collectStringConsts(file, consts)
			}
		}
		var paths []string
		for _, pkg := range pkgs {
			for path := range pkg.Files {
				paths = append(paths, path)
			}
		}
		sort.Strings(paths)
		for _, path := range paths {
			for _, pkg := range pkgs {
				if file, ok := pkg.Files[path]; ok {
					stmts = append(stmts, statementsInFile(fset, file, path, consts)...)
					break
				}
			}
		}
	}
	return stmts, nil
}

func goDirs(roots []string) ([]string, error) {
	seen := map[string]bool{}
	var dirs []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// dbschema is the checker itself: its string literals are
				// error messages and information_schema probes, not
				// application queries, so scanning it only produces noise.
				name := d.Name()
				if name == "vendor" || name == "testdata" || name == "dbschema" || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			dir := filepath.Dir(path)
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

// collectStringConsts records package-level string constants and variables
// whose initialiser is itself a foldable string expression.
func collectStringConsts(file *ast.File, out map[string]string) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != len(vs.Values) {
				continue
			}
			for i, name := range vs.Names {
				if value, ok := foldString(vs.Values[i], out); ok && !strings.Contains(value, unresolved) {
					out[name.Name] = value
				}
			}
		}
	}
}

// foldString evaluates a string expression made of literals, previously
// resolved constants and + concatenation. It returns the text with unresolved
// marking every operand it could not evaluate.
func foldString(expr ast.Expr, consts map[string]string) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(e.Value)
		if err != nil {
			return "", false
		}
		return value, true
	case *ast.Ident:
		if value, ok := consts[e.Name]; ok {
			return value, true
		}
		return unresolved, true
	case *ast.ParenExpr:
		return foldString(e.X, consts)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, lok := foldString(e.X, consts)
		right, rok := foldString(e.Y, consts)
		if !lok || !rok {
			return "", false
		}
		return left + right, true
	}
	return "", false
}

func statementsInFile(fset *token.FileSet, file *ast.File, path string, consts map[string]string) []Statement {
	var out []Statement

	type span struct {
		lo, hi token.Pos
		name   string
	}
	var funcs []span
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			name := fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				name = receiverName(fn.Recv.List[0].Type) + "." + name
			}
			funcs = append(funcs, span{fn.Pos(), fn.End(), name})
		}
	}
	enclosing := func(p token.Pos) string {
		for _, f := range funcs {
			if p >= f.lo && p < f.hi {
				return f.name
			}
		}
		return ""
	}

	// Positions already consumed as part of a larger concatenation, so the
	// same query is not reported twice.
	consumed := map[token.Pos]bool{}

	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		var expr ast.Expr
		switch e := n.(type) {
		case *ast.BinaryExpr:
			if e.Op != token.ADD {
				return true
			}
			expr = e
		case *ast.BasicLit:
			if e.Kind != token.STRING {
				return true
			}
			expr = e
		default:
			return true
		}
		if consumed[expr.Pos()] {
			return true
		}

		value, ok := foldString(expr, consts)
		if !ok {
			return true
		}
		// Mark every sub-expression as consumed, whether or not this one
		// turned out to be SQL, so nested literals are not re-reported.
		ast.Inspect(expr, func(sub ast.Node) bool {
			if sub != nil {
				consumed[sub.Pos()] = true
			}
			return true
		})

		if !reSQLStart.MatchString(strings.ReplaceAll(value, unresolved, "")) {
			return true
		}
		var st Statement
		if strings.Contains(value, unresolved) {
			st = Statement{
				SQL:     strings.ReplaceAll(value, unresolved, "<non-constant>"),
				Skipped: "assembled from a non-constant expression",
			}
		} else {
			st = AnalyzeSQL(value)
		}
		st.File = path
		st.Line = fset.Position(expr.Pos()).Line
		st.Func = enclosing(expr.Pos())
		out = append(out, st)
		return true
	}

	ast.Inspect(file, visit)
	return out
}

func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return receiverName(t.X)
	}
	return "?"
}

// Finding is one column reference the sweep could not resolve.
type Finding struct {
	File    string
	Line    int
	Func    string
	Kind    string
	Table   string
	Column  string
	Clause  string
	Reason  string
	SQLHead string
}

// Sweep checks every statement against the schema and returns the findings and
// the statements the reader declined to analyse.
func Sweep(schema *Schema, stmts []Statement) (findings []Finding, skipped []Statement) {
	for _, st := range stmts {
		if st.Skipped != "" || st.Table == "" {
			skipped = append(skipped, st)
			continue
		}
		lower := strings.ToLower(st.Table)
		if schema.Views[lower] {
			skipped = append(skipped, st)
			continue
		}
		if !schema.HasTable(lower) {
			findings = append(findings, Finding{
				File: st.File, Line: st.Line, Func: st.Func, Kind: st.Kind,
				Table: st.Table, Reason: "table does not exist in the schema",
				SQLHead: head(st.SQL),
			})
			continue
		}
		seen := make(map[string]bool)
		for _, ref := range st.Refs {
			key := strings.ToLower(ref.Column)
			if seen[key] {
				continue
			}
			seen[key] = true
			if schema.HasColumn(lower, key) {
				continue
			}
			findings = append(findings, Finding{
				File: st.File, Line: st.Line, Func: st.Func, Kind: st.Kind,
				Table: st.Table, Column: ref.Column, Clause: ref.Clause,
				Reason: "column does not exist in the schema", SQLHead: head(st.SQL),
			})
		}
	}
	return findings, skipped
}

func head(sqlText string) string {
	s := strings.Join(strings.Fields(sqlText), " ")
	if len(s) > 110 {
		s = s[:110] + " ..."
	}
	return s
}
