package analyze

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bradhannah/go-clean-deadcode/internal/config"
	"github.com/bradhannah/go-clean-deadcode/internal/report"
)

func fixture(t *testing.T) (string, config.Config) {
	t.Helper()
	root, err := filepath.Abs("../../testdata/sample")
	if err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Modules[0].EntryPoints = []config.EntryPoint{{Package: "./cmd/app", Role: "application"}, {Package: "./cmd/tool", Role: "utility"}}
	return root, c
}

func TestReachabilityEvidence(t *testing.T) {
	root, c := fixture(t)
	r := Run(context.Background(), Options{Root: root, Config: c})
	if !r.Complete {
		t.Fatalf("incomplete: %+v", r.Diagnostics)
	}
	found := map[string]string{}
	for _, f := range r.Findings {
		found[f.Symbol] = f.Category
	}
	for name, want := range map[string]string{"DeadA": "unreachable", "DeadB": "unreachable", "DeadGeneric": "unreachable", "Legacy": "test_only", "legacyHelper": "test_only", "unusedTestHelper": "unused_test_helper", "ExampleLegacy": "unused_test_helper"} {
		if got := found["example.com/sample/lib."+name]; got != want {
			t.Errorf("%s: got %q want %q", name, got, want)
		}
	}
	for _, name := range []string{"Live", "Utility", "Real.Work", "Dispatch", "Generic", "RecursiveLive", "callback", "CallbackUsed", "TestLegacy", "BenchmarkLegacy", "FuzzLegacy"} {
		if got := found["example.com/sample/lib."+name]; got != "" {
			t.Errorf("live %s reported %s", name, got)
		}
	}
	if found["example.com/sample/lib.Real.unrelatedEmpty"] != "unreachable" {
		t.Error("unrelated empty private method mistaken for an interface marker")
	}
	if found["example.com/sample/lib.Hidden.seal"] != "" || r.Summary.StructuralMethods != 1 {
		t.Error("verified marker method not retained as structural evidence")
	}
	cycle := false
	for _, g := range r.Groups {
		if slices.Contains(g.Members, "example.com/sample/lib.DeadA") && slices.Contains(g.Members, "example.com/sample/lib.DeadB") {
			cycle = true
		}
	}
	if !cycle {
		t.Error("dead recursive group missing")
	}
	obligation := false
	witness := false
	for _, s := range r.Symbols {
		if s.ID == "example.com/sample/lib.Unused.Work" && len(s.Obligations) > 0 {
			obligation = true
		}
		if s.ID == "example.com/sample/lib.Legacy" {
			for _, st := range s.States {
				for _, w := range st.Witnesses {
					if strings.Contains(w.Root, "TestLegacy") {
						witness = true
					}
				}
			}
		}
	}
	if !obligation {
		t.Error("interface obligation missing")
	}
	if !witness {
		t.Error("test witness missing")
	}
	for _, s := range r.Symbols {
		if s.Name == "SubtestOnly" {
			for _, st := range s.States {
				for _, w := range st.Witnesses {
					if w.Root != "example.com/sample/lib.TestNested" || w.Kind != "lexical_test_reference" {
						t.Errorf("subtest attribution leaked through harness: %+v", w)
					}
				}
			}
		}
	}
	if r.ExitCode() != 1 {
		t.Errorf("unresolved findings must exit 1: %d", r.ExitCode())
	}
}

func TestDiscoveryAndMissingRoot(t *testing.T) {
	root, c := fixture(t)
	c.Modules[0].EntryPoints = c.Modules[0].EntryPoints[:1]
	r := Run(context.Background(), Options{Root: root, Config: c, Discover: true})
	if !r.Complete || len(r.Scopes[0].EntryPoints) != 2 {
		t.Fatalf("discovery: %+v", r)
	}
	r = Run(context.Background(), Options{Root: root, Config: c})
	if r.Complete || r.ExitCode() != 2 {
		t.Fatal("missing entry must fail incomplete")
	}
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == "unclassified_main" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing executable diagnostic")
	}
}

func TestExceptionsAndDeterminism(t *testing.T) {
	root, c := fixture(t)
	base := Run(context.Background(), Options{Root: root, Config: c})
	fp := ""
	for _, s := range base.Symbols {
		if s.ID == "example.com/sample/lib.Legacy" {
			fp = s.Fingerprint
		}
	}
	c.Exceptions = []config.Exception{{ID: "compat", Symbol: "example.com/sample/lib.Legacy", Category: "test_only", Reason: "compatibility fixture", ApprovedBy: "fixture owner", ReviewAfter: "2099-01-01", Fingerprint: fp}, {ID: "stale", Symbol: "example.com/sample/lib.Missing", Category: "unreachable", Reason: "fixture", ApprovedBy: "owner", ReviewAfter: "2099-01-01", Fingerprint: strings.Repeat("0", 64)}}
	a := Run(context.Background(), Options{Root: root, Config: c})
	b := Run(context.Background(), Options{Root: root, Config: c})
	if a.Summary.Accepted != 1 || a.Summary.StaleExceptions != 1 {
		t.Fatalf("exceptions: %+v", a.Summary)
	}
	a.Metrics = report.Metrics{}
	b.Metrics = report.Metrics{}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if string(x) != string(y) {
		t.Error("equivalent input produced nondeterministic report")
	}
}

func TestNestedModulesAndInvalidLoad(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{"go.mod": "module example.com/root\n\ngo 1.27.0\n", "main.go": "package main\nfunc main() {}\n", "nested/go.mod": "module example.com/nested\n\ngo 1.27.0\n"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c := config.Default()
	c.Modules[0].EntryPoints = []config.EntryPoint{{Package: ".", Role: "application"}}
	r := Run(context.Background(), Options{Root: root, Config: c})
	if r.Complete {
		t.Error("unclassified nested module must be incomplete")
	}
	c.ExcludedModules = []config.Exclusion{{Path: "nested", Reason: "explicit fixture exclusion"}}
	r = Run(context.Background(), Options{Root: root, Config: c})
	if !r.Complete {
		t.Fatalf("exclusion: %+v", r.Diagnostics)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.go"), []byte("package main\nnot valid Go"), 0600); err != nil {
		t.Fatal(err)
	}
	r = Run(context.Background(), Options{Root: root, Config: c})
	if r.Complete || r.ExitCode() != 2 {
		t.Fatal("load failure must never be clean")
	}
}

func TestCycleTraversalLarge(t *testing.T) {
	const size = 10000
	syms := make([]report.Symbol, size)
	edges := make([]report.Edge, size)
	for i := range size {
		id := string(rune(i + 100))
		next := string(rune((i+1)%size + 100))
		syms[i] = report.Symbol{ID: id, States: []report.State{{Scope: "s"}}}
		edges[i] = report.Edge{Scope: "s", Caller: id, Callee: next}
	}
	g := groups("s", syms, edges)
	if len(g) != 1 || len(g[0].Members) != size {
		t.Fatalf("cycle grouping: %d groups", len(g))
	}
}
