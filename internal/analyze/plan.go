package analyze

import (
	"sort"

	"github.com/bradhannah/go-clean-deadcode/internal/report"
)

const maxReferencedBy = 20

// planGraph is the combined evidence of every scope: a declaration counts as
// reached when any scope reaches it, which keeps the simulation conservative.
type planGraph struct {
	symbols    map[string]*report.Symbol
	prodLive   map[string]bool
	testLive   map[string]bool
	testEdges  map[string][]string
	references map[string]map[string]bool // test-side code -> declarations it references directly
	referrers  map[string][]string        // declaration -> test-side code that references it directly
}

func newPlanGraph(r *report.Report) *planGraph {
	g := &planGraph{symbols: map[string]*report.Symbol{}, prodLive: map[string]bool{}, testLive: map[string]bool{},
		testEdges: map[string][]string{}, references: map[string]map[string]bool{}, referrers: map[string][]string{}}
	for i := range r.Symbols {
		s := &r.Symbols[i]
		g.symbols[s.ID] = s
		for _, st := range s.States {
			g.prodLive[s.ID] = g.prodLive[s.ID] || st.Production == "reachable"
			g.testLive[s.ID] = g.testLive[s.ID] || st.Tests == "reachable"
		}
	}
	for _, edge := range r.Edges {
		if edge.Context == "tests" && edge.Caller != edge.Callee {
			g.testEdges[edge.Caller] = append(g.testEdges[edge.Caller], edge.Callee)
		}
	}
	// Test-side code is a registered test or a declaration in a test file or test
	// support: a helper's reference matters as much as a test's.
	for _, ref := range r.References {
		from := g.symbols[ref.From]
		if from == nil || !from.RegisteredTest && !from.TestFile && !from.TestSupport || g.symbols[ref.Symbol] == nil {
			continue
		}
		if g.references[ref.From] == nil {
			g.references[ref.From] = map[string]bool{}
		}
		if !g.references[ref.From][ref.Symbol] {
			g.references[ref.From][ref.Symbol] = true
			g.referrers[ref.Symbol] = append(g.referrers[ref.Symbol], ref.From)
		}
	}
	for id := range g.referrers {
		sort.Strings(g.referrers[id])
	}
	return g
}

func (g *planGraph) testHelper(id string) bool {
	s := g.symbols[id]
	return s.TestFile || s.TestSupport
}

// required reports declarations a finding never covers: methods kept for
// interface structure or for an asserted interface.
func (g *planGraph) required(id string) bool {
	for _, o := range g.symbols[id].Obligations {
		if o.Kind == "marker_method" || o.Kind == "assertion_satisfaction" {
			return true
		}
	}
	return false
}

// dedicated reports whether a surviving registered test references production
// code only where that code is removed, so the test exists for removed code
// alone. Helpers are not removed this way: surviving tests may call them.
func (g *planGraph) dedicated(test string, removed map[string]int) bool {
	if !g.symbols[test].RegisteredTest {
		return false
	}
	production := 0
	for id := range g.references[test] {
		if g.testHelper(id) {
			continue
		}
		production++
		if _, gone := removed[id]; !gone {
			return false
		}
	}
	return production > 0
}

// reachedByTests walks test call edges from every surviving entry point: a
// test-reached declaration that no other test-reached declaration calls.
func (g *planGraph) reachedByTests(removed map[string]int) map[string]bool {
	called := map[string]bool{}
	for caller, callees := range g.testEdges {
		if g.testLive[caller] {
			for _, callee := range callees {
				called[callee] = true
			}
		}
	}
	var queue []string
	reached := map[string]bool{}
	for id := range g.testLive {
		if kind := g.symbols[id].Kind; kind != "function" && kind != "method" {
			continue // Declarations are reached through uses, not entry points.
		}
		if _, gone := removed[id]; g.testLive[id] && !called[id] && !gone {
			reached[id] = true
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, next := range g.testEdges[id] {
			if _, gone := removed[next]; !gone && !reached[next] {
				reached[next] = true
				queue = append(queue, next)
			}
		}
	}
	return reached
}

func (e *engine) plan() report.Plan {
	g := newPlanGraph(e.report)
	removed := map[string]int{}
	reasons := map[string]string{}
	var next []string
	for _, f := range e.report.Findings {
		// A never-assigned field is used; its fix is a review, not a removal.
		if f.Status == "unresolved" && f.Category != "never_assigned" {
			next = append(next, f.Symbol)
			reasons[f.Symbol] = f.Category
		}
	}
	plan := report.Plan{Waves: []report.Wave{}}
	for wave := 1; len(next) > 0; wave++ {
		for _, id := range next {
			removed[id] = wave
		}
		// Tests that exist only for this wave's removals go with it.
		for test := range g.references {
			if _, gone := removed[test]; !gone && g.dedicated(test, removed) {
				removed[test] = wave
				reasons[test] = "dedicated_test"
				next = append(next, test)
			}
		}
		plan.Waves = append(plan.Waves, g.wave(wave, next, reasons, removed))
		reached := g.reachedByTests(removed)
		next = nil
		for id := range g.testLive {
			if _, gone := removed[id]; gone || reached[id] || g.prodLive[id] || g.required(id) || e.decl.decls[id] != nil {
				continue
			}
			next = append(next, id)
			reasons[id] = "exposed"
		}
		next = append(next, e.exposedDeclarations(g, reached, removed)...)
		for _, id := range next {
			if reasons[id] == "" {
				reasons[id] = "exposed"
			}
		}
	}
	for _, w := range plan.Waves {
		plan.Functions += w.Functions + w.TestHelpers
		plan.Declarations += w.Declarations
		plan.Tests += w.Tests
		plan.Lines += w.Lines
	}
	return plan
}

// exposedDeclarations lists live types, fields, constants and variables that
// nothing surviving uses any more and that may be removed.
func (e *engine) exposedDeclarations(g *planGraph, reached map[string]bool, removed map[string]int) []string {
	seeds := map[string]bool{}
	for id := range g.prodLive {
		if _, gone := removed[id]; g.prodLive[id] && !gone && e.decl.decls[id] == nil {
			seeds[id] = true
		}
	}
	for id := range reached {
		seeds[id] = true
	}
	for _, roots := range e.decl.roots {
		for id := range roots {
			seeds[id] = true
		}
	}
	live := e.liveDeclarations(seeds, removed, e.significant)
	wasLive := func(id string) bool { return g.prodLive[id] || g.testLive[id] }
	inProduction := func(id string) bool { _, gone := removed[id]; return live[id] && g.prodLive[id] && !gone }
	liveGroups := e.liveEnumGroups(inProduction)
	var exposed []string
	for id := range e.decl.decls {
		if _, gone := removed[id]; gone || live[id] || !wasLive(id) {
			continue
		}
		if kept, _ := e.keptDeclaration(id, e.significant, liveGroups, func(id string) bool { return live[id] }, inProduction); !kept {
			exposed = append(exposed, id)
		}
	}
	return exposed
}

func (g *planGraph) wave(number int, ids []string, reasons map[string]string, removed map[string]int) report.Wave {
	sort.Strings(ids)
	w := report.Wave{Number: number, Removals: []report.Removal{}, TestsNeedingEdits: []string{}}
	edits := map[string]bool{}
	for _, id := range ids {
		s := g.symbols[id]
		item := report.Removal{Symbol: id, Reason: reasons[id], Lines: s.Lines}
		for _, test := range g.referrers[id] {
			if g.symbols[test].Package != s.Package {
				item.OtherPackages = true
			}
			if len(item.ReferencedBy) < maxReferencedBy {
				item.ReferencedBy = append(item.ReferencedBy, test)
			}
			if _, gone := removed[test]; !gone {
				edits[test] = true
			}
		}
		w.Removals = append(w.Removals, item)
		w.Lines += s.Lines
		switch {
		case s.RegisteredTest:
			w.Tests++
		case s.Kind != "function" && s.Kind != "method":
			w.Declarations++
		case g.testHelper(id):
			w.TestHelpers++
		default:
			w.Functions++
		}
	}
	for test := range edits {
		w.TestsNeedingEdits = append(w.TestsNeedingEdits, test)
	}
	sort.Strings(w.TestsNeedingEdits)
	return w
}
