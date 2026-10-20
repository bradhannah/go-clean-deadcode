package analyze

import (
	"go/ast"
	"go/types"
	"sort"
	"strings"

	"github.com/bradhannah/go-clean-deadcode/internal/report"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func (e *engine) references(p *packages.Package, file *ast.File, scope string) {
	// References deliberately include dead callers; they are not liveness evidence.
	for _, decl := range file.Decls {
		from := ""
		if f, ok := decl.(*ast.FuncDecl); ok {
			if obj, ok := p.TypesInfo.Defs[f.Name].(*types.Func); ok {
				from = objectID(obj)
			}
		}
		calls := map[*ast.Ident]bool{}
		ast.Inspect(decl, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				x := call.Fun
				switch v := x.(type) {
				case *ast.IndexExpr:
					x = v.X
				case *ast.IndexListExpr:
					x = v.X
				}
				switch v := x.(type) {
				case *ast.Ident:
					calls[v] = true
				case *ast.SelectorExpr:
					calls[v.Sel] = true
				}
			}
			return true
		})
		ast.Inspect(decl, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				if f, ok := p.TypesInfo.Uses[id].(*types.Func); ok {
					k := "reference"
					if calls[id] {
						k = "call"
					}
					e.refs[report.Reference{Scope: scope, Symbol: objectID(f), From: from, Kind: k, Location: e.location(p.Fset, id.Pos())}] = true
				}
			}
			// Explicit interface assignments provide concrete compile-time evidence.
			if v, ok := n.(*ast.ValueSpec); ok && v.Type != nil {
				for _, rhs := range v.Values {
					e.interfaceAssignment(p, p.TypesInfo.TypeOf(rhs), p.TypesInfo.TypeOf(v.Type), rhs, scope)
				}
			}
			if a, ok := n.(*ast.AssignStmt); ok && len(a.Lhs) == len(a.Rhs) {
				for i, rhs := range a.Rhs {
					e.interfaceAssignment(p, p.TypesInfo.TypeOf(rhs), p.TypesInfo.TypeOf(a.Lhs[i]), rhs, scope)
				}
			}
			return true
		})
	}
}

func (e *engine) interfaceAssignment(p *packages.Package, src, dst types.Type, node ast.Node, scope string) {
	if src == nil || dst == nil {
		return
	}
	iface, ok := dst.Underlying().(*types.Interface)
	if !ok || !types.Implements(src, iface) {
		return
	}
	for i := 0; i < iface.NumMethods(); i++ {
		m := iface.Method(i)
		sel := types.NewMethodSet(src).Lookup(m.Pkg(), m.Name())
		if sel == nil {
			continue
		}
		f, ok := sel.Obj().(*types.Func)
		if !ok {
			continue
		}
		id := objectID(f)
		// The declaration may be visited later. Deferred obligations use a separate pass in finalize.
		key := report.Reference{Scope: scope, Symbol: id, Kind: "interface_obligation:" + types.TypeString(dst, func(p *types.Package) string { return p.Path() }), Location: e.location(p.Fset, node.Pos())}
		e.refs[key] = true
	}
}

// witnesses walks incoming edges breadth-first with finite work and output bounds.
// Reaching a test seed stops that branch, avoiding irrelevant test-harness paths.
type graphIndex struct {
	graph    *callgraph.Graph
	names    map[*ssa.Function]string
	incoming map[*callgraph.Node][]*callgraph.Edge
}

func newGraphIndex(g *callgraph.Graph) *graphIndex {
	return &graphIndex{graph: g, names: map[*ssa.Function]string{}, incoming: map[*callgraph.Node][]*callgraph.Edge{}}
}
func (g *graphIndex) name(f *ssa.Function) string {
	if s, ok := g.names[f]; ok {
		return s
	}
	s := functionID(f)
	g.names[f] = s
	return s
}
func (g *graphIndex) in(n *callgraph.Node) []*callgraph.Edge {
	if edges, ok := g.incoming[n]; ok {
		return edges
	}
	edges := append([]*callgraph.Edge{}, n.In...)
	sort.Slice(edges, func(i, j int) bool {
		a, b := g.name(edges[i].Caller.Func), g.name(edges[j].Caller.Func)
		if a != b {
			return a < b
		}
		return edges[i].Pos() < edges[j].Pos()
	})
	g.incoming[n] = edges
	return edges
}

func witnesses(index *graphIndex, target *ssa.Function, seeds map[*ssa.Function]string, context string) ([]report.Witness, bool) {
	graph := index.graph
	start := graph.Nodes[target]
	if start == nil {
		return nil, false
	}
	next := map[*callgraph.Node]*callgraph.Edge{start: nil}
	queue := []*callgraph.Node{start}
	out := []report.Witness{}
	seenRoots := map[string]bool{}
	const maxVisits = 2000
	const maxWitnesses = 3
	for at := 0; at < len(queue); at++ {
		if at >= maxVisits || len(out) >= maxWitnesses {
			return out, false
		}
		n := queue[at]
		if root, ok := seeds[n.Func]; ok && !seenRoots[root] {
			seenRoots[root] = true
			path := []string{functionID(n.Func)}
			kind := "static"
			for cur := n; next[cur] != nil; {
				edge := next[cur]
				if edge.Site == nil || edge.Site.Common().StaticCallee() == nil {
					kind = "possible_dynamic"
				}
				cur = edge.Callee
				path = append(path, functionID(cur.Func))
			}
			if root != path[0] {
				path = append([]string{root}, path...)
				kind = "lexical_test_reference"
			}
			out = append(out, report.Witness{Root: root, Context: context, Kind: kind, Path: path})
			continue
		}
		for _, edge := range index.in(n) {
			if _, ok := next[edge.Caller]; !ok {
				next[edge.Caller] = edge
				queue = append(queue, edge.Caller)
			}
		}
	}
	return out, true
}

func obligationKind(k string) bool { return strings.HasPrefix(k, "interface_obligation:") }

// forwardWitnesses selects one deterministic root path per reachable node in one
// graph traversal. It avoids a whole-graph search for every live declaration.
func forwardWitnesses(index *graphIndex, seeds map[*ssa.Function]string, context string, targets map[*ssa.Function]bool) map[*ssa.Function]report.Witness {
	g := index.graph
	roots := make([]*ssa.Function, 0, len(seeds))
	for f := range seeds {
		roots = append(roots, f)
	}
	sort.Slice(roots, func(i, j int) bool { return seeds[roots[i]] < seeds[roots[j]] })
	prev := map[*callgraph.Node]*callgraph.Edge{}
	origin := map[*callgraph.Node]string{}
	q := []*callgraph.Node{}
	for _, f := range roots {
		if n := g.Nodes[f]; n != nil {
			prev[n] = nil
			origin[n] = seeds[f]
			q = append(q, n)
		}
	}
	for i := 0; i < len(q); i++ {
		n := q[i]
		edges := append([]*callgraph.Edge{}, n.Out...)
		sort.Slice(edges, func(i, j int) bool {
			a, b := index.name(edges[i].Callee.Func), index.name(edges[j].Callee.Func)
			if a != b {
				return a < b
			}
			return edges[i].Pos() < edges[j].Pos()
		})
		for _, edge := range edges {
			if _, ok := prev[edge.Callee]; !ok {
				prev[edge.Callee] = edge
				origin[edge.Callee] = origin[n]
				q = append(q, edge.Callee)
			}
		}
	}
	out := map[*ssa.Function]report.Witness{}
	for n := range prev {
		// Only source declarations need path output, not every dependency wrapper.
		if !targets[n.Func] {
			continue
		}
		path := []string{functionID(n.Func)}
		kind := "static"
		for cur := n; prev[cur] != nil; {
			edge := prev[cur]
			if edge.Site == nil || edge.Site.Common().StaticCallee() == nil {
				kind = "possible_dynamic"
			}
			cur = edge.Caller
			path = append(path, functionID(cur.Func))
		}
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
		out[n.Func] = report.Witness{Root: origin[n], Context: context, Kind: kind, Path: path}
	}
	return out
}
