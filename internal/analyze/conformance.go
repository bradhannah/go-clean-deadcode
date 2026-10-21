package analyze

import (
	"go/ast"
	"go/types"
	"slices"
	"sort"
	"strings"

	"github.com/bradhannah/go-clean-deadcode/internal/report"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// A method can be required at compile time without any call: converting its
// type to an interface, or instantiating a generic with its type as a type
// argument, needs every method the interface or constraint declares. Removing
// it means changing that interface. A requirement counts when the code making
// it is reachable in an analysis; one in dead code goes with that code.

type conformanceEvidence struct {
	interfaces map[string][]string           // method -> interfaces that require it
	live       map[lookupKey]map[string]bool // analysis -> methods a reachable requirement needs
	pending    map[lookupKey][]instanceUse   // instantiations inside functions, until reachability is known
	seen       map[string]bool
}

type instanceUse struct{ method, from string }

func newConformanceEvidence() *conformanceEvidence {
	return &conformanceEvidence{interfaces: map[string][]string{}, live: map[lookupKey]map[string]bool{},
		pending: map[lookupKey][]instanceUse{}, seen: map[string]bool{}}
}

// required lists the method IDs a conversion of concrete to target needs.
func (e *engine) required(concrete, target types.Type) []string {
	iface, ok := target.Underlying().(*types.Interface)
	if !ok || iface.NumMethods() == 0 {
		return nil
	}
	set := types.NewMethodSet(concrete)
	name := types.TypeString(target, func(p *types.Package) string { return p.Path() })
	var ids []string
	for i := range iface.NumMethods() {
		m := iface.Method(i)
		sel := set.Lookup(m.Pkg(), m.Name())
		if sel == nil {
			continue
		}
		if f, ok := sel.Obj().(*types.Func); ok {
			id := objectID(f)
			ids = append(ids, id)
			if !slices.Contains(e.conform.interfaces[id], name) {
				e.conform.interfaces[id] = append(e.conform.interfaces[id], name)
			}
		}
	}
	return ids
}

func (e *engine) requireLive(key lookupKey, ids []string) {
	if len(ids) == 0 {
		return
	}
	if e.conform.live[key] == nil {
		e.conform.live[key] = map[string]bool{}
	}
	for _, id := range ids {
		e.conform.live[key][id] = true
	}
}

// recordConversions records every conversion to an interface in the program,
// and which of them reachable code makes.
func (e *engine) recordConversions(key lookupKey, prog *ssa.Program, res *rta.Result) {
	for fn := range ssautil.AllFunctions(prog) {
		_, reachable := res.Reachable[fn]
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				mi, ok := instr.(*ssa.MakeInterface)
				if !ok {
					continue
				}
				ids := e.required(mi.X.Type(), mi.Type())
				if reachable {
					e.requireLive(key, ids)
				}
			}
		}
	}
}

// recordInstances records the methods that instantiating a generic function or
// type requires of its type arguments. An instantiation in a type or variable
// declaration counts as live; one in a function counts once that function is
// known to be reachable.
func (e *engine) recordInstances(key lookupKey, p *packages.Package) {
	for _, file := range p.Syntax {
		for _, decl := range file.Decls {
			from := ""
			if fd, ok := decl.(*ast.FuncDecl); ok {
				if obj, ok := p.TypesInfo.Defs[fd.Name].(*types.Func); ok {
					from = objectID(obj)
				}
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				inst, ok := p.TypesInfo.Instances[id]
				if !ok || inst.TypeArgs == nil {
					return true
				}
				var params *types.TypeParamList
				switch t := inst.Type.(type) {
				case *types.Named:
					params = t.Origin().TypeParams()
				case *types.Signature:
					if fn, ok := p.TypesInfo.Uses[id].(*types.Func); ok {
						params = fn.Origin().Type().(*types.Signature).TypeParams()
					}
				}
				if params == nil {
					return true
				}
				for i := range min(params.Len(), inst.TypeArgs.Len()) {
					ids := e.required(inst.TypeArgs.At(i), params.At(i).Constraint())
					if from == "" {
						e.requireLive(key, ids)
						continue
					}
					for _, m := range ids {
						e.conform.pending[key] = append(e.conform.pending[key], instanceUse{m, from})
					}
				}
				return true
			})
		}
	}
}

// resolveInstances marks the requirements of instantiations in reachable
// functions once the analysis knows them.
func (e *engine) resolveInstances(key lookupKey) {
	for _, use := range e.conform.pending[key] {
		if e.decl.reachable[key.scope][key.analysis][use.from] {
			e.requireLive(key, []string{use.method})
		}
	}
	delete(e.conform.pending, key)
}

// compileRequired reports a method that reachable code needs at compile time
// in some analysis.
func (e *engine) compileRequired(id string) bool {
	for _, methods := range e.conform.live {
		if methods[id] {
			return true
		}
	}
	return false
}

func (e *engine) conformanceNote(id string) report.Note {
	names := append([]string{}, e.conform.interfaces[id]...)
	sort.Strings(names)
	return report.Note{Symbol: id, Kind: "interface_conformance", Message: "No call path reaches it, but " +
		strings.Join(names, ", ") + " requires it where reachable code converts its type or instantiates a generic " +
		"with it. Removing it means removing that method from the interface or constraint, which needs a review " +
		"of type switches and assertions on it."}
}
