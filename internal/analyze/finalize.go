package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/bradhannah/go-clean-deadcode/internal/config"
	"github.com/bradhannah/go-clean-deadcode/internal/report"
)

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:12]) }
func unique[T any](items []T) []T {
	by := map[string]T{}
	for _, v := range items {
		b, _ := json.Marshal(v)
		by[string(b)] = v
	}
	keys := make([]string, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]T, 0, len(keys))
	for _, k := range keys {
		out = append(out, by[k])
	}
	return out
}

func (e *engine) finalize(c config.Config) {
	for ref := range e.refs {
		if s := e.symbols[ref.Symbol]; s != nil {
			if obligationKind(ref.Kind) {
				s.Obligations = append(s.Obligations, report.Obligation{Scope: ref.Scope, Kind: "interface_assignment", Description: "Method required by assignment to " + strings.TrimPrefix(ref.Kind, "interface_obligation:"), Location: ref.Location})
			} else {
				e.report.References = append(e.report.References, ref)
			}
		}
	}
	for edge := range e.edges {
		if e.symbols[edge.Caller] != nil && e.symbols[edge.Callee] != nil {
			e.report.Edges = append(e.report.Edges, edge)
		}
	}
	reflectionNotes := e.applyReflectionReviews(c)
	e.significant = e.declarationStates()
	liveAny := map[string]bool{}
	for id, states := range e.states {
		for _, st := range states {
			liveAny[id] = liveAny[id] || st.Production == "reachable" || st.Tests == "reachable"
		}
	}
	live := func(id string) bool { return liveAny[id] }
	inProduction := func(id string) bool {
		for _, st := range e.states[id] {
			if st.Production == "reachable" {
				return true
			}
		}
		return false
	}
	liveGroups := e.liveEnumGroups(inProduction)
	for id, s := range e.symbols {
		bodies := make([]string, 0, len(e.bodies[id]))
		for b := range e.bodies[id] {
			bodies = append(bodies, b)
		}
		sort.Strings(bodies)
		s.Fingerprint = checksum([]byte(strings.Join(bodies, "\x00")))
		prodLive, testLive, prodSeen, testSeen := false, false, false, false
		var scopes []string
		for _, st := range e.states[id] {
			st.ProductionRoots = unique(st.ProductionRoots)
			st.TestRoots = unique(st.TestRoots)
			st.Witnesses = unique(st.Witnesses)
			s.States = append(s.States, *st)
			scopes = append(scopes, st.Scope)
			prodLive = prodLive || st.Production == "reachable"
			testLive = testLive || st.Tests == "reachable"
			prodSeen = prodSeen || st.Production != "not_analyzed"
			testSeen = testSeen || st.Tests != "not_analyzed"
		}
		sort.Slice(s.States, func(i, j int) bool { return s.States[i].Scope < s.States[j].Scope })
		s.Locations = unique(s.Locations)
		s.Obligations = unique(s.Obligations)
		e.report.Symbols = append(e.report.Symbols, *s)
		category := ""
		testHelper := s.TestFile || s.TestSupport
		if !prodLive {
			switch {
			case testHelper && testSeen && !testLive:
				category = "unused_test_helper"
			case !testHelper && prodSeen && testLive:
				category = "test_only"
			case !testHelper && (prodSeen || testSeen) && !testLive:
				category = "unreachable"
			}
		}
		for _, o := range s.Obligations {
			if o.Kind == "marker_method" && category == "unreachable" {
				category = ""
				e.report.Summary.StructuralMethods++
				break
			}
			if o.Kind == "assertion_satisfaction" && category != "" {
				category = ""
				e.report.Summary.RequiredMethods++
				break
			}
		}
		if category != "" && e.symbols[id].Kind == "method" && e.compileRequired(id) {
			category = ""
			e.report.Notes = append(e.report.Notes, e.conformanceNote(id))
		}
		if category != "" {
			if kept, note := e.keptDeclaration(id, e.significant, liveGroups, live, inProduction); kept {
				category = ""
				if note != "" {
					e.report.Notes = append(e.report.Notes, report.Note{Symbol: id, Kind: "unused_enum_value", Message: note})
				}
			}
		}
		if category != "" {
			sort.Strings(scopes)
			e.report.Findings = append(e.report.Findings, report.Finding{ID: "F-" + digest(category+"\x00"+id), Category: category, Symbol: id, Scopes: scopes, Status: "unresolved"})
		}
	}
	for id, reads := range e.neverAssigned(e.significant, live) {
		var scopes []string
		for _, st := range e.symbols[id].States {
			scopes = append(scopes, st.Scope)
		}
		sort.Strings(scopes)
		e.report.Findings = append(e.report.Findings, report.Finding{ID: "F-" + digest("never_assigned\x00"+id),
			Category: "never_assigned", Symbol: id, Scopes: scopes, Status: "unresolved", Reads: reads})
	}
	e.report.Notes = append(e.report.Notes, e.sizeGuardNotes(e.significant, live)...)
	e.report.Notes = append(e.report.Notes, reflectionNotes...)
	sort.Slice(e.report.Notes, func(i, j int) bool { return e.report.Notes[i].Symbol < e.report.Notes[j].Symbol })
	sort.Slice(e.report.Symbols, func(i, j int) bool { return e.report.Symbols[i].ID < e.report.Symbols[j].ID })
	sort.Slice(e.report.Findings, func(i, j int) bool { return e.report.Findings[i].Symbol < e.report.Findings[j].Symbol })
	e.report.Edges = unique(e.report.Edges)
	e.report.References = unique(e.report.References)
	for _, s := range e.report.Scopes {
		e.report.Groups = append(e.report.Groups, groups(s.ID, e.report.Symbols, e.report.Edges)...)
	}
	e.report.Groups = unique(e.report.Groups)
	for _, ex := range c.Exceptions {
		x := report.ExceptionResult{ID: ex.ID, Symbol: ex.Symbol, Category: ex.Category, Reason: ex.Reason, ApprovedBy: ex.ApprovedBy, ReviewAfter: ex.ReviewAfter, Status: "unmatched"}
		date, err := time.Parse("2006-01-02", ex.ReviewAfter)
		if symbol := e.symbols[ex.Symbol]; symbol != nil && symbol.Fingerprint != ex.Fingerprint {
			x.Status = "changed"
		} else if err != nil || !time.Now().UTC().Before(date.Add(24*time.Hour)) {
			x.Status = "expired"
		} else {
			for i := range e.report.Findings {
				f := &e.report.Findings[i]
				if f.Symbol == ex.Symbol && f.Category == ex.Category {
					if f.Status == "accepted" {
						x.Status = "duplicate"
					} else {
						x.Status = "matched"
						f.Status = "accepted"
						f.Exception = ex.ID
					}
				}
			}
		}
		e.report.Exceptions = append(e.report.Exceptions, x)
		if x.Status != "matched" {
			e.report.Summary.StaleExceptions++
		}
	}
	for _, f := range e.report.Findings {
		switch f.Category {
		case "unreachable":
			e.report.Summary.Unreachable++
		case "test_only":
			e.report.Summary.TestOnly++
		case "unused_test_helper":
			e.report.Summary.UnusedTestHelpers++
		case "never_assigned":
			e.report.Summary.NeverAssigned++
		}
		if f.Status == "accepted" {
			e.report.Summary.Accepted++
		} else {
			e.report.Summary.Unresolved++
		}
	}
	e.report.Summary.Symbols = len(e.report.Symbols)
	e.report.Summary.Notes = len(e.report.Notes)
	e.report.Diagnostics = unique(e.report.Diagnostics)
	e.report.Plan = e.plan()
}

// groups uses iterative Kosaraju traversal. Cycles never trigger recursive walking.
func groups(scope string, symbols []report.Symbol, edges []report.Edge) []report.Group {
	adj, rev := map[string][]string{}, map[string][]string{}
	for _, s := range symbols {
		for _, st := range s.States {
			if st.Scope == scope {
				adj[s.ID] = nil
				rev[s.ID] = nil
			}
		}
	}
	self := map[string]bool{}
	for _, e := range edges {
		if e.Scope != scope {
			continue
		}
		if _, ok := adj[e.Caller]; !ok {
			continue
		}
		if _, ok := adj[e.Callee]; !ok {
			continue
		}
		adj[e.Caller] = append(adj[e.Caller], e.Callee)
		rev[e.Callee] = append(rev[e.Callee], e.Caller)
		if e.Caller == e.Callee {
			self[e.Caller] = true
		}
	}
	keys := make([]string, 0, len(adj))
	for k := range adj {
		keys = append(keys, k)
		sort.Strings(adj[k])
		sort.Strings(rev[k])
	}
	sort.Strings(keys)
	seen := map[string]bool{}
	order := []string{}
	type frame struct {
		id   string
		exit bool
	}
	for _, root := range keys {
		if seen[root] {
			continue
		}
		stack := []frame{{id: root}}
		for len(stack) > 0 {
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if f.exit {
				order = append(order, f.id)
				continue
			}
			if seen[f.id] {
				continue
			}
			seen[f.id] = true
			stack = append(stack, frame{id: f.id, exit: true})
			for _, n := range adj[f.id] {
				if !seen[n] {
					stack = append(stack, frame{id: n})
				}
			}
		}
	}
	seen = map[string]bool{}
	out := []report.Group{}
	for i := len(order) - 1; i >= 0; i-- {
		root := order[i]
		if seen[root] {
			continue
		}
		stack := []string{root}
		members := []string{}
		seen[root] = true
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			members = append(members, n)
			for _, p := range rev[n] {
				if !seen[p] {
					seen[p] = true
					stack = append(stack, p)
				}
			}
		}
		sort.Strings(members)
		if len(members) > 1 || self[root] {
			out = append(out, report.Group{ID: "G-" + digest(scope+strings.Join(members, "\x00")), Scope: scope, Members: members, Recursive: true})
		}
	}
	return out
}
