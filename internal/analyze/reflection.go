package analyze

import (
	"go/types"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bradhannah/go-clean-deadcode/internal/config"
	"github.com/bradhannah/go-clean-deadcode/internal/report"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Reachability keeps every exported method of a runtime type, because
// reflection could call it. Only reflect's method lookups can: a method value
// obtained by index or name. The report lists who performs those lookups, and
// an owner can review each entry into them as never seeing this module's types.
// When every lookup in an analysis is reviewed, or there is none, code kept
// only by that rule is unreachable there, as the Go linker would treat it.

// reflectiveLookup names the reflect functions that find a method by index or
// name at run time.
var reflectiveLookup = map[string]bool{
	"(reflect.Value).Method": true, "(reflect.Value).MethodByName": true,
	"(*reflect.rtype).Method": true, "(*reflect.rtype).MethodByName": true,
}

type lookupKey struct{ scope, analysis string }

func standardLibrary(fn *ssa.Function) bool {
	if fn.Pkg == nil {
		return false
	}
	path := fn.Pkg.Pkg.Path()
	return !strings.Contains(strings.Split(path, "/")[0], ".")
}

// recordReflectiveLookup records each reachable lookup: the function outside
// package reflect that calls it (through compiler-generated wrappers), and the
// entry, the first function outside the standard library on a call path to it.
func (e *engine) recordReflectiveLookup(scope, analysis string, g *callgraph.Graph) {
	key := lookupKey{scope, analysis}
	if e.lookups[key] == nil {
		e.lookups[key] = map[report.ReflectiveLookup]bool{}
	}
	callers := map[*callgraph.Node]bool{}
	seen := map[*callgraph.Node]bool{}
	var queue []*callgraph.Node
	for fn, node := range g.Nodes {
		if fn != nil && reflectiveLookup[fn.String()] {
			seen[node] = true
			queue = append(queue, node)
		}
	}
	for len(queue) > 0 {
		node := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, edge := range node.In {
			caller := edge.Caller
			if seen[caller] {
				continue
			}
			seen[caller] = true
			switch fn := caller.Func; {
			case fn.Synthetic != "":
				queue = append(queue, caller)
			case fn.Pkg != nil && fn.Pkg.Pkg.Path() == "reflect":
				// Reflect reaches its own lookups only for a method value that
				// a lookup outside reflect already produced.
			default:
				callers[caller] = true
			}
		}
	}
	for via := range callers {
		for _, entry := range lookupEntries(via) {
			e.lookups[key][report.ReflectiveLookup{Analysis: analysis, Entry: entry, Via: via.Func.String()}] = true
		}
		if standardLibrary(via.Func) {
			if e.lookupPackages[key] == nil {
				e.lookupPackages[key] = map[string]string{}
			}
			e.lookupPackages[key][via.Func.String()] = via.Func.Pkg.Pkg.Path()
		}
	}
}

// lookupEntries walks static callers of a standard-library lookup caller to
// the first functions outside the standard library: the code that passes data
// into it. Dynamic edges are not followed, because reachability links every
// function-value call to every function of the same signature; module code that
// refers to the lookup's package is added as an entry instead, which covers
// function and method values. A lookup no such function reaches is its own
// entry.
func lookupEntries(via *callgraph.Node) []string {
	if !standardLibrary(via.Func) {
		return []string{via.Func.String()}
	}
	entries := map[string]bool{}
	seen := map[*callgraph.Node]bool{via: true}
	queue := []*callgraph.Node{via}
	for len(queue) > 0 {
		node := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, edge := range node.In {
			caller := edge.Caller
			if seen[caller] || edge.Site == nil || edge.Site.Common().StaticCallee() == nil {
				continue
			}
			seen[caller] = true
			if caller.Func.Synthetic != "" || standardLibrary(caller.Func) {
				queue = append(queue, caller)
			} else {
				entries[caller.Func.String()] = true
			}
		}
	}
	if len(entries) == 0 {
		return []string{via.Func.String()}
	}
	out := make([]string, 0, len(entries))
	for entry := range entries {
		out = append(out, entry)
	}
	sort.Strings(out)
	return out
}

// recordConformance records, for every conversion of a concrete type to an
// interface anywhere in the program, the methods that conversion requires. A
// method required this way cannot be removed without changing the interface.
func (e *engine) recordConformance(prog *ssa.Program) {
	for fn := range ssautil.AllFunctions(prog) {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				mi, ok := instr.(*ssa.MakeInterface)
				if !ok {
					continue
				}
				e.conformanceFor(mi.X.Type(), mi.Type())
			}
		}
	}
}

func (e *engine) conformanceFor(concrete, target types.Type) {
	iface, ok := target.Underlying().(*types.Interface)
	if !ok || iface.NumMethods() == 0 {
		return
	}
	key := concrete.String() + "\x00" + target.String()
	if e.conversions[key] {
		return
	}
	e.conversions[key] = true
	set := types.NewMethodSet(concrete)
	name := types.TypeString(target, func(p *types.Package) string { return p.Path() })
	for i := range iface.NumMethods() {
		m := iface.Method(i)
		sel := set.Lookup(m.Pkg(), m.Name())
		if sel == nil {
			continue
		}
		if f, ok := sel.Obj().(*types.Func); ok {
			id := objectID(f)
			if !slices.Contains(e.conformance[id], name) {
				e.conformance[id] = append(e.conformance[id], name)
			}
		}
	}
}

// reflectionReviews matches reviews to the recorded lookups and returns the
// analyses whose every lookup entry has a matched, current review.
func (e *engine) reflectionReviews(c config.Config) map[lookupKey]bool {
	// Module code that refers to a standard-library lookup's package can pass
	// it data through a function value that static callers do not show.
	for key, vias := range e.lookupPackages {
		for ref := range e.refs {
			if ref.Scope != key.scope || ref.From == "" || !e.reachableIn(ref.From, key) {
				continue
			}
			for via, pkg := range vias {
				if strings.HasPrefix(ref.Symbol, pkg+".") {
					e.lookups[key][report.ReflectiveLookup{Analysis: key.analysis, Entry: ref.From, Via: via}] = true
				}
			}
		}
	}
	byScope := map[lookupKey][]report.ReflectiveLookup{}
	for key, rows := range e.lookups {
		for row := range rows {
			byScope[key] = append(byScope[key], row)
		}
	}
	current := map[string]bool{}
	matched := map[string]bool{}
	for _, r := range c.ReflectionReviews {
		result := report.ReflectionReviewResult{ID: r.ID, Entry: r.Entry, Reason: r.Reason, ApprovedBy: r.ApprovedBy,
			ReviewAfter: r.ReviewAfter, Status: "unmatched"}
		date, err := time.Parse("2006-01-02", r.ReviewAfter)
		if err != nil || !time.Now().UTC().Before(date.Add(24*time.Hour)) {
			result.Status = "expired"
		} else {
			for _, rows := range byScope {
				for _, row := range rows {
					if row.Entry == r.Entry {
						result.Status = "matched"
						matched[r.Entry] = true
					}
				}
			}
			current[r.Entry] = true
		}
		if result.Status != "matched" {
			e.report.Summary.StaleExceptions++
		}
		e.report.ReflectionReviews = append(e.report.ReflectionReviews, result)
	}
	reviewed := map[lookupKey]bool{}
	for i := range e.report.Scopes {
		s := &e.report.Scopes[i]
		s.ReflectiveLookups = []report.ReflectiveLookup{}
		for _, analysis := range []string{"production", "tests"} {
			key := lookupKey{s.ID, analysis}
			all := true
			for _, row := range byScope[key] {
				s.ReflectiveLookups = append(s.ReflectiveLookups, row)
				all = all && current[row.Entry] && matched[row.Entry]
			}
			reviewed[key] = all
		}
		sort.Slice(s.ReflectiveLookups, func(a, b int) bool {
			x, y := s.ReflectiveLookups[a], s.ReflectiveLookups[b]
			return x.Analysis+x.Entry+x.Via < y.Analysis+y.Entry+y.Via
		})
	}
	return reviewed
}

// retainedByReflection returns, per analysis, the functions and methods that
// only reflection's exported-method rule keeps: no call path from an entry point
// reaches them, no interface conversion requires them, and nothing reachable
// outside the set refers to them.
func (e *engine) retainedByReflection(key lookupKey) map[string]bool {
	unexplained := e.unexplained[key]
	retained := map[string]bool{}
	for id := range unexplained {
		if x := e.symbols[id]; x != nil && (x.Kind == "function" || x.Kind == "method") && len(e.conformance[id]) == 0 {
			retained[id] = true
		}
	}
	referrers := map[string][]string{}
	for _, ref := range e.report.References {
		if ref.Scope == key.scope && retained[ref.Symbol] {
			referrers[ref.Symbol] = append(referrers[ref.Symbol], ref.From)
		}
	}
	for changed := true; changed; {
		changed = false
		for id := range retained {
			for _, from := range referrers[id] {
				// A package-level reference runs at initialization.
				if from == "" || !retained[from] && e.reachableIn(from, key) {
					delete(retained, id)
					changed = true
					break
				}
			}
		}
	}
	return retained
}

func (e *engine) reachableIn(id string, key lookupKey) bool {
	s := e.states[id][key.scope]
	if s == nil {
		return false
	}
	if key.analysis == "production" {
		return s.Production == "reachable"
	}
	return s.Tests == "reachable"
}

// applyReflectionReviews makes code kept only by reflection unreachable in
// each reviewed analysis, and returns the notes for what stays.
func (e *engine) applyReflectionReviews(c config.Config) []report.Note {
	reviewed := e.reflectionReviews(c)
	keys := make([]lookupKey, 0, len(e.unexplained))
	for key := range e.unexplained {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].scope+keys[i].analysis < keys[j].scope+keys[j].analysis })
	notes := map[string]report.Note{}
	kept := map[string]bool{} // retained in some unreviewed production analysis
	for _, key := range keys {
		retained := e.retainedByReflection(key)
		if !reviewed[key] {
			if key.analysis == "production" {
				for id := range retained {
					kept[id] = true
				}
			}
			continue
		}
		for id := range retained {
			s := e.states[id][key.scope]
			if key.analysis == "production" {
				s.Production = "unreachable"
			} else {
				s.Tests = "unreachable"
			}
			s.ReflectionReviewed = true
			delete(e.decl.reachable[key.scope][key.analysis], id)
		}
		if key.analysis != "production" {
			continue
		}
		for id := range e.unexplained[key] {
			if len(e.conformance[id]) > 0 && !retained[id] && e.symbols[id] != nil && !e.symbols[id].TestFile {
				notes[id] = report.Note{Symbol: id, Kind: "interface_conformance", Message: "No call path reaches it, " +
					"but converting its type to " + strings.Join(e.conformance[id], ", ") + " requires it. Nothing calls " +
					"it through the interface either; removing it means removing that method from the interface, " +
					"which needs a review of type switches and assertions on it."}
			}
		}
	}
	for id := range kept {
		x := e.symbols[id]
		if x.TestFile || x.TestSupport || e.productionReachableOtherwise(id) {
			continue
		}
		var lookups []string
		for _, s := range e.report.Scopes {
			for _, row := range s.ReflectiveLookups {
				if row.Analysis == "production" && !reviewed[lookupKey{s.ID, "production"}] && !slices.Contains(lookups, row.Entry) {
					lookups = append(lookups, row.Entry)
				}
			}
		}
		sort.Strings(lookups)
		notes[id] = report.Note{Symbol: id, Kind: "reflection_retained", Message: "Kept only because reachability " +
			"keeps exported methods of runtime types for reflection; no call path from an entry point reaches it. " +
			"Unreviewed reflective method lookup entries: " + strings.Join(lookups, ", ") + ". Review whether they can " +
			"see this type."}
	}
	out := make([]report.Note, 0, len(notes))
	for _, n := range notes {
		out = append(out, n)
	}
	return out
}

// productionReachableOtherwise reports a symbol that some production analysis
// reaches for a reason other than reflection.
func (e *engine) productionReachableOtherwise(id string) bool {
	for scope, s := range e.states[id] {
		if s.Production == "reachable" && !e.unexplained[lookupKey{scope, "production"}][id] {
			return true
		}
	}
	return false
}
