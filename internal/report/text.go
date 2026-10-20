package report

import (
	"fmt"
	"io"
	"strings"
)

// Text prints factual findings; it never recommends deleting a test automatically.
func Text(w io.Writer, r *Report, only string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "GO SOURCE REACHABILITY REPORT\nTool: %s  Schema: %d  Complete: %t\n", r.ToolVersion, r.SchemaVersion, r.Complete)
	for _, s := range r.Scopes {
		fmt.Fprintf(&b, "\nScope: %s (%s/%s, tags=%v, cgo=%t)\n  Packages: %d  Tests: %t  Complete: %t\n", s.ID, s.GOOS, s.GOARCH, s.Tags, s.CGOEnabled, s.Packages, s.Tests, s.Complete)
		fmt.Fprintf(&b, "  Package patterns: %v\n", s.Patterns)
		for _, ep := range s.EntryPoints {
			fmt.Fprintf(&b, "  Entry: %-12s %s (%s)\n", ep.Role, ep.Package, ep.Directory)
		}
		for _, entry := range s.TestSupport {
			fmt.Fprintf(&b, "  Test support: %s\n", entry)
		}
		if s.APIRoots != "" {
			fmt.Fprintf(&b, "  API roots: %s (exported functions and methods outside internal packages)\n", s.APIRoots)
		}
		for _, m := range s.NestedModules {
			fmt.Fprintf(&b, "  Nested module: %s\n", m)
		}
	}
	fmt.Fprintf(&b, "\nSummary\n  Unreachable functions: %d\n  Non-test functions reachable only in tests: %d\n  Unreachable test declarations: %d\n  Unresolved findings: %d\n  Accepted exceptions: %d\n  Stale exceptions: %d\n", r.Summary.Unreachable, r.Summary.TestOnly, r.Summary.UnusedTestHelpers, r.Summary.Unresolved, r.Summary.Accepted, r.Summary.StaleExceptions)
	fmt.Fprintf(&b, "  Structural interface-marker methods: %d\n", r.Summary.StructuralMethods)
	fmt.Fprintf(&b, "  Methods required by asserted interfaces: %d\n", r.Summary.RequiredMethods)
	if len(r.Plan.Waves) > 0 {
		fmt.Fprintf(&b, "\nRemoval plan (simulated; nothing is edited)\n")
		for _, w := range r.Plan.Waves {
			fmt.Fprintf(&b, "  Wave %d: %d functions, %d test helpers, %d tests, %d lines; %d surviving tests need edits\n",
				w.Number, w.Functions, w.TestHelpers, w.Tests, w.Lines, len(w.TestsNeedingEdits))
		}
		fmt.Fprintf(&b, "  Total: %d functions and test helpers, %d tests, %d lines\n", r.Plan.Functions, r.Plan.Tests, r.Plan.Lines)
	}
	byID := map[string]Symbol{}
	for _, s := range r.Symbols {
		byID[s.ID] = s
	}
	for _, f := range r.Findings {
		if only != "" && f.Symbol != only && f.ID != only {
			continue
		}
		s := byID[f.Symbol]
		fmt.Fprintf(&b, "\n%s  %s [%s]\n  Symbol: %s\n", f.ID, strings.ToUpper(f.Category), f.Status, s.ID)
		for _, l := range s.Locations {
			fmt.Fprintf(&b, "  Declaration: %s:%d:%d\n", l.File, l.Line, l.Column)
		}
		fmt.Fprintf(&b, "  Test file: %t  Generated: %t  Registered test entry: %t\n  Fingerprint: %s\n", s.TestFile, s.Generated, s.RegisteredTest, s.Fingerprint)
		for _, st := range s.States {
			fmt.Fprintf(&b, "  %s: selected-executables=%s tests=%s\n", st.Scope, st.Production, st.Tests)
			for _, v := range st.Witnesses {
				relation := "path"
				if v.Kind == "lexical_test_reference" {
					relation = "source association"
				}
				fmt.Fprintf(&b, "    %s %s (%s): %s\n", v.Context, relation, v.Kind, strings.Join(v.Path, " -> "))
			}
			if !st.ProvenanceComplete {
				fmt.Fprintln(&b, "    Path examples are bounded; root attribution is not exhaustive.")
			}
		}
		fmt.Fprintln(&b, "  Direct source references (callers may themselves be unreachable):")
		count := 0
		for _, ref := range r.References {
			if ref.Symbol == s.ID {
				from := ref.From
				if from == "" {
					from = "package initialization/declaration"
				}
				fmt.Fprintf(&b, "    %s:%d  %s  %s\n", ref.Location.File, ref.Location.Line, ref.Kind, from)
				count++
			}
		}
		if count == 0 {
			fmt.Fprintln(&b, "    None in the analyzed source.")
		}
		for _, o := range s.Obligations {
			fmt.Fprintf(&b, "  Obligation: %s (%s:%d)\n", o.Description, o.Location.File, o.Location.Line)
		}
		for _, g := range r.Groups {
			for _, id := range g.Members {
				if id == s.ID {
					fmt.Fprintf(&b, "  Recursive group %s: %s\n", g.ID, strings.Join(g.Members, ", "))
					break
				}
			}
		}
		if f.Exception != "" {
			for _, x := range r.Exceptions {
				if x.ID == f.Exception {
					fmt.Fprintf(&b, "  Exception %s: %s (approved by %s; review %s)\n", x.ID, x.Reason, x.ApprovedBy, x.ReviewAfter)
				}
			}
		}
	}
	for _, x := range r.Exceptions {
		if x.Status != "matched" {
			fmt.Fprintf(&b, "\nException %s: %s (%s)\n", x.ID, x.Status, x.Symbol)
		}
	}
	for _, d := range r.Diagnostics {
		fmt.Fprintf(&b, "\n%s %s [%s]: %s\n", strings.ToUpper(d.Severity), d.Code, d.Scope, d.Message)
	}
	fmt.Fprintf(&b, "\nTiming: total=%dms load=%dms ssa=%dms reachability=%dms cache_hit=%t\n", r.Metrics.DurationMS, r.Metrics.LoadMS, r.Metrics.SSAMS, r.Metrics.ReachabilityMS, r.Metrics.CacheHit)
	fmt.Fprintln(&b, "\nInterpretation limits:")
	for _, l := range r.Limitations {
		fmt.Fprintf(&b, "  - %s\n", l)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
