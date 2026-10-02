// astgraph loads a Go module with full type information and writes a graph of its packages,
// declarations and static calls: nodes (package, func, method, type) and edges (imports, calls,
// uses-type, implements). Test files are left out.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Node struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // package | func | method | type
	Pkg      string `json:"pkg"`
	Name     string `json:"name"`
	Exported bool   `json:"exported"`
	LOC      int    `json:"loc"`
	File     string `json:"file,omitempty"`
	Doc      string `json:"doc,omitempty"`
	Iface    bool   `json:"iface,omitempty"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // imports | calls | uses | implements | external
}

type Graph struct {
	Module string `json:"module"`
	Nodes  []Node `json:"nodes"`
	Edges  []Edge `json:"edges"`
}

func main() {
	dir, module := os.Args[1], os.Args[2]
	cfg := &packages.Config{Dir: dir, Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes |
		packages.NeedTypesInfo | packages.NeedImports | packages.NeedModule, Env: append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=readonly")}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		panic(err)
	}
	g := Graph{Module: module}
	seen := map[string]bool{}
	edge := map[Edge]bool{}
	add := func(n Node) {
		if !seen[n.ID] {
			seen[n.ID] = true
			g.Nodes = append(g.Nodes, n)
		}
	}
	link := func(from, to, kind string) {
		if from != to {
			edge[Edge{from, to, kind}] = true
		}
	}
	internal := func(p *types.Package) bool { return p != nil && strings.HasPrefix(p.Path(), module) }
	objID := func(o types.Object) string {
		if f, ok := o.(*types.Func); ok {
			if sig, ok := f.Type().(*types.Signature); ok && sig.Recv() != nil {
				t := sig.Recv().Type()
				if p, ok := t.(*types.Pointer); ok {
					t = p.Elem()
				}
				if n, ok := t.(*types.Named); ok {
					return o.Pkg().Path() + "." + n.Obj().Name() + "." + o.Name()
				}
			}
		}
		return o.Pkg().Path() + "." + o.Name()
	}
	var named []*types.TypeName
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			fmt.Fprintln(os.Stderr, "load errors in", p.PkgPath, p.Errors[0])
		}
		loc := 0
		for _, f := range p.Syntax {
			name := p.Fset.Position(f.Pos()).Filename
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			loc += p.Fset.Position(f.End()).Line
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					obj, ok := p.TypesInfo.Defs[d.Name].(*types.Func)
					if !ok {
						continue
					}
					kind := "func"
					if d.Recv != nil {
						kind = "method"
					}
					id := objID(obj)
					doc := ""
					if d.Doc != nil {
						doc = strings.SplitN(strings.TrimSpace(d.Doc.Text()), "\n", 2)[0]
					}
					add(Node{ID: id, Kind: kind, Pkg: p.PkgPath, Name: strings.TrimPrefix(id, p.PkgPath+"."), Exported: obj.Exported(),
						LOC: lines(p.Fset, d), File: rel(p.Fset.Position(d.Pos()).Filename, dir), Doc: doc})
					link(p.PkgPath, id, "declares")
					ast.Inspect(d, func(n ast.Node) bool {
						switch x := n.(type) {
						case *ast.CallExpr:
							var ident *ast.Ident
							switch f := x.Fun.(type) {
							case *ast.Ident:
								ident = f
							case *ast.SelectorExpr:
								ident = f.Sel
							}
							if ident != nil {
								if fn, ok := p.TypesInfo.Uses[ident].(*types.Func); ok && fn.Pkg() != nil {
									if internal(fn.Pkg()) {
										link(id, objID(fn), "calls")
									} else {
										link(id, "ext:"+fn.Pkg().Path(), "external")
									}
								}
							}
						case *ast.Ident:
							if tn, ok := p.TypesInfo.Uses[x].(*types.TypeName); ok && internal(tn.Pkg()) {
								link(id, objID(tn), "uses")
							}
						}
						return true
					})
				case *ast.GenDecl:
					for _, s := range d.Specs {
						ts, ok := s.(*ast.TypeSpec)
						if !ok {
							continue
						}
						tn, ok := p.TypesInfo.Defs[ts.Name].(*types.TypeName)
						if !ok {
							continue
						}
						_, iface := tn.Type().Underlying().(*types.Interface)
						add(Node{ID: objID(tn), Kind: "type", Pkg: p.PkgPath, Name: tn.Name(), Exported: tn.Exported(), LOC: lines(p.Fset, ts),
							File: rel(p.Fset.Position(ts.Pos()).Filename, dir), Iface: iface})
						link(p.PkgPath, objID(tn), "declares")
						named = append(named, tn)
						ast.Inspect(ts, func(n ast.Node) bool {
							if x, ok := n.(*ast.Ident); ok {
								if u, ok := p.TypesInfo.Uses[x].(*types.TypeName); ok && internal(u.Pkg()) {
									link(objID(tn), objID(u), "uses")
								}
							}
							return true
						})
					}
				}
			}
		}
		add(Node{ID: p.PkgPath, Kind: "package", Pkg: p.PkgPath, Name: strings.TrimPrefix(p.PkgPath, module+"/"), LOC: loc})
		for path, imp := range p.Imports {
			if strings.HasPrefix(path, module) {
				link(p.PkgPath, path, "imports")
			} else if imp.Module != nil {
				link(p.PkgPath, "mod:"+imp.Module.Path, "imports-module")
			}
		}
	}
	// implements: concrete named types that satisfy an internal interface.
	for _, a := range named {
		ai, ok := a.Type().Underlying().(*types.Interface)
		if !ok || ai.NumMethods() == 0 {
			continue
		}
		for _, b := range named {
			if _, isI := b.Type().Underlying().(*types.Interface); isI || a == b {
				continue
			}
			if types.Implements(b.Type(), ai) || types.Implements(types.NewPointer(b.Type()), ai) {
				link(objID(b), objID(a), "implements")
			}
		}
	}
	for e := range edge {
		g.Edges = append(g.Edges, e)
	}
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].From+g.Edges[i].To+g.Edges[i].Kind < g.Edges[j].From+g.Edges[j].To+g.Edges[j].Kind })
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	b, _ := json.MarshalIndent(g, "", " ")
	os.Stdout.Write(b)
}

func lines(f *token.FileSet, n ast.Node) int {
	return f.Position(n.End()).Line - f.Position(n.Pos()).Line + 1
}

func rel(path, dir string) string { return strings.TrimPrefix(strings.TrimPrefix(path, dir), "/") }
