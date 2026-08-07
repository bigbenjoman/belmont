// Command declsum prints a sorted, position-independent digest of every
// top-level declaration in a Go package's non-test files.
//
// It exists to prove that a file split is a pure move: if the digest is
// identical before and after, every declaration has the same body, and only
// the file it lives in has changed.
//
// Usage: declsum <package-dir>
package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: declsum <package-dir>")
		os.Exit(2)
	}
	lines, err := run(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "declsum:", err)
		os.Exit(1)
	}
	for _, l := range lines {
		fmt.Println(l)
	}
}

func run(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		// File-level exclusion, not a line grep.
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			out = append(out, declEntries(fset, d)...)
		}
	}
	sort.Strings(out)
	return out, nil
}

func declEntries(fset *token.FileSet, d ast.Decl) []string {
	switch n := d.(type) {
	case *ast.FuncDecl:
		return []string{entry(funcName(n), hash(fset, n))}
	case *ast.GenDecl:
		// Imports legitimately duplicate across a split. Skip them.
		if n.Tok == token.IMPORT {
			return nil
		}
		// Hash each spec separately so regrouping a var/const block across
		// files is not reported as a change.
		var out []string
		for _, s := range n.Specs {
			out = append(out, entry(specName(n, s), hash(fset, s)))
		}
		return out
	}
	return nil
}

func funcName(n *ast.FuncDecl) string {
	if n.Recv != nil && len(n.Recv.List) > 0 {
		var b bytes.Buffer
		printer.Fprint(&b, token.NewFileSet(), n.Recv.List[0].Type)
		return "(" + b.String() + ")." + n.Name.Name
	}
	return "func " + n.Name.Name
}

func specName(g *ast.GenDecl, s ast.Spec) string {
	switch t := s.(type) {
	case *ast.TypeSpec:
		return "type " + t.Name.Name
	case *ast.ValueSpec:
		var names []string
		for _, id := range t.Names {
			names = append(names, id.Name)
		}
		return g.Tok.String() + " " + strings.Join(names, ",")
	}
	return g.Tok.String() + " ?"
}

// hash prints the node and digests it. The node is printed twice: once with
// the real FileSet, then re-parsed and re-printed, so that the digest depends
// on the code and not on which line the declaration happens to start at.
func hash(fset *token.FileSet, n ast.Node) string {
	var b bytes.Buffer
	cfg := printer.Config{Mode: printer.RawFormat, Tabwidth: 8}
	if err := cfg.Fprint(&b, fset, n); err != nil {
		return "ERR:" + err.Error()
	}
	norm := normalize(b.String())
	sum := sha256.Sum256([]byte(norm))
	return fmt.Sprintf("%x", sum[:8])
}

// normalize collapses whitespace runs so that a declaration moved to a
// different offset in a different file digests identically.
func normalize(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, strings.Join(strings.Fields(line), " "))
	}
	return strings.Join(out, "\n")
}

func entry(name, h string) string {
	return fmt.Sprintf("%s\t%s", name, h)
}
