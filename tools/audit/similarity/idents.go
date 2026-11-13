//go:build ignore

// Idents lists names, for the check that no distinctive name of Crush's
// stands in the new code.
//
//	go run idents.go -decl list.tsv -out decl.tsv
//
// reads the Go files of a corpus list (path on disk, name to show) and writes
// one line per distinct declared name: kind, name, the number of files that
// declare it, and the first of them. Kinds: package, type, func, method,
// const, var, field (a struct's field or an interface's method), and tag:KEY
// for the value a struct tag gives under KEY (json, yaml, toml, env, flag...),
// options after the first comma dropped.
//
//	go run idents.go -use targets.tsv -out use.tsv
//
// reads the Go files of a target list (set, group, kind, path on disk, name
// to show) and writes one line per file and distinct identifier token: set,
// group, name to show, identifier, how often it stands in the file. Every
// identifier token counts, declared or used, in code only (not comments,
// not strings).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
)

type entry struct {
	n     int
	first string
}

func main() {
	decl := flag.String("decl", "", "corpus list: write the declared names")
	use := flag.String("use", "", "target list: write the identifiers of each file")
	outFn := flag.String("out", "", "output file")
	flag.Parse()
	out := bufio.NewWriterSize(os.Stdout, 1<<20)
	if *outFn != "" {
		f, err := os.Create(*outFn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "idents:", err)
			os.Exit(1)
		}
		defer f.Close()
		out = bufio.NewWriterSize(f, 1<<20)
	}
	defer out.Flush()
	switch {
	case *decl != "":
		declared(*decl, out)
	case *use != "":
		used(*use, out)
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// isGo says whether a file is Go or Gno source, by the name it is shown
// under. A corpus file's name may end in @ and the start of its blob id.
func isGo(name string) bool {
	if i := strings.LastIndexByte(name, '@'); i > 0 && !strings.ContainsAny(name[i:], "/.") {
		name = name[:i]
	}
	return strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".gno")
}

func readList(fn string, cols int) [][]string {
	fh, err := os.Open(fn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "idents:", err)
		os.Exit(1)
	}
	defer fh.Close()
	var out [][]string
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		p := strings.Split(sc.Text(), "\t")
		if len(p) >= cols {
			out = append(out, p)
		}
	}
	return out
}

func declared(list string, out *bufio.Writer) {
	seen := map[string]*entry{}
	clean := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	add := func(kind, name, shown string) {
		name = clean.Replace(name)
		if name == "" || name == "_" {
			return
		}
		k := kind + "\t" + name
		e := seen[k]
		if e == nil {
			e = &entry{first: shown}
			seen[k] = e
		}
		e.n++
	}
	tags := func(tag *ast.BasicLit, shown string) {
		if tag == nil || len(tag.Value) < 2 {
			return
		}
		st := reflect.StructTag(strings.Trim(tag.Value, "`"))
		for _, key := range []string{"json", "yaml", "toml", "env", "flag", "mapstructure", "jsonschema", "description", "name", "short", "long"} {
			if v, ok := st.Lookup(key); ok {
				if key != "description" && key != "jsonschema" {
					v, _, _ = strings.Cut(v, ",")
				}
				if v != "" && v != "-" {
					add("tag:"+key, v, shown)
				}
			}
		}
	}
	var fields func(fl *ast.FieldList, shown string)
	fields = func(fl *ast.FieldList, shown string) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				add("field", n.Name, shown)
			}
			tags(f.Tag, shown)
			switch t := f.Type.(type) {
			case *ast.StructType:
				fields(t.Fields, shown)
			case *ast.InterfaceType:
				fields(t.Methods, shown)
			}
		}
	}
	for _, p := range readList(list, 2) {
		if !isGo(p[1]) {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p[0], nil, parser.SkipObjectResolution)
		if f == nil {
			continue
		}
		_ = err // a file that does not parse to the end still gives what was read
		shown := p[1]
		add("package", f.Name.Name, shown)
		ast.Inspect(f, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					add("method", d.Name.Name, shown)
				} else {
					add("func", d.Name.Name, shown)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						add("type", s.Name.Name, shown)
					case *ast.ValueSpec:
						kind := "var"
						if d.Tok == token.CONST {
							kind = "const"
						}
						for _, n := range s.Names {
							add(kind, n.Name, shown)
						}
					}
				}
			case *ast.StructType:
				fields(d.Fields, shown)
			case *ast.InterfaceType:
				fields(d.Methods, shown)
			}
			return true
		})
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(out, "%s\t%d\t%s\n", k, seen[k].n, seen[k].first)
	}
}

func used(list string, out *bufio.Writer) {
	for _, p := range readList(list, 5) {
		if !isGo(p[4]) {
			continue
		}
		src, err := os.ReadFile(p[3])
		if err != nil {
			continue
		}
		fset := token.NewFileSet()
		f := fset.AddFile("", fset.Base(), len(src))
		var s scanner.Scanner
		s.Init(f, src, func(token.Position, string) {}, 0)
		n := map[string]int{}
		for {
			_, tok, lit := s.Scan()
			if tok == token.EOF {
				break
			}
			if tok == token.IDENT {
				n[lit]++
			}
		}
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%d\n", p[0], p[1], p[4], k, n[k])
		}
	}
}
