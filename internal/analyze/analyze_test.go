package analyze

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
	for symbol := range found {
		if strings.HasSuffix(symbol, "._") {
			t.Errorf("blank compile-time assertion reported as %s", found[symbol])
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

func libraryFixture(t *testing.T, apiRoots string) *report.Report {
	t.Helper()
	root, err := filepath.Abs("../../testdata/library")
	if err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Modules[0].Library = apiRoots == ""
	c.Modules[0].APIRoots = apiRoots
	return Run(context.Background(), Options{Root: root, Config: c})
}

func TestExportedAPIRoots(t *testing.T) {
	r := libraryFixture(t, config.APIRootsExported)
	if !r.Complete {
		t.Fatalf("incomplete: %+v", r.Diagnostics)
	}
	if r.Scopes[0].APIRoots != config.APIRootsExported {
		t.Errorf("scope does not record api_roots: %+v", r.Scopes[0])
	}
	found := map[string]string{}
	for _, f := range r.Findings {
		found[f.Symbol] = f.Category
	}
	for name, want := range map[string]string{
		"api.unusedHelper": "unreachable", "internal/core.Unused": "unreachable", "api.onlyTested": "test_only",
	} {
		if got := found["example.com/library/"+name]; got != want {
			t.Errorf("%s: got %q want %q", name, got, want)
		}
	}
	for _, name := range []string{"api.Exported", "api.helper", "internal/core.Used", "api.Engine.Run", "api.run",
		"api.hidden.Value", "api.value", "api.New", "api.Map", "api.generic", "api.register",
		"internal/core.Base.Promoted", "internal/core.promoted"} {
		if got := found["example.com/library/"+name]; got != "" {
			t.Errorf("API-reachable %s reported %s", name, got)
		}
	}
}

func TestLibraryWithoutAPIRootsReportsItsAPI(t *testing.T) {
	r := libraryFixture(t, "")
	if !r.Complete {
		t.Fatalf("incomplete: %+v", r.Diagnostics)
	}
	found := map[string]string{}
	for _, f := range r.Findings {
		found[f.Symbol] = f.Category
	}
	if found["example.com/library/api.Exported"] != "unreachable" {
		t.Errorf("library=true alone must not protect exported API: %v", found)
	}
}

func findingsOf(r *report.Report) map[string]string {
	found := map[string]string{}
	for _, f := range r.Findings {
		found[strings.TrimPrefix(f.Symbol, "example.com/sample/")] = f.Category
	}
	return found
}

func TestAssertedInterfaceMethodsAreRequired(t *testing.T) {
	root, c := fixture(t)
	r := Run(context.Background(), Options{Root: root, Config: c})
	if !r.Complete {
		t.Fatalf("incomplete: %+v", r.Diagnostics)
	}
	found := findingsOf(r)
	for _, name := range []string{"lib.job.label", "lib.fakeJob.label"} {
		if got := found[name]; got != "" {
			t.Errorf("%s satisfies an asserted interface but was reported %s", name, got)
		}
	}
	if found["lib.idleJob.label"] != "unreachable" {
		t.Errorf("idleJob never reaches the assertion; got %q", found["lib.idleJob.label"])
	}
	required := false
	for _, s := range r.Symbols {
		if s.ID == "example.com/sample/lib.job.label" {
			for _, o := range s.Obligations {
				required = required || o.Kind == "assertion_satisfaction"
			}
		}
	}
	if !required || r.Summary.RequiredMethods < 2 {
		t.Errorf("assertion obligation missing: required=%v summary=%+v", required, r.Summary)
	}
}

func TestTestSupportDeclarationsAreTestHelpers(t *testing.T) {
	root, c := fixture(t)
	r := Run(context.Background(), Options{Root: root, Config: c})
	without := findingsOf(r)
	if without["lib.SupportOnly"] != "test_only" || without["testkit.Helper"] != "test_only" ||
		without["testkit.Orphan"] != "unreachable" {
		t.Fatalf("baseline classification changed: %v", without)
	}
	c.Modules[0].TestSupport = []string{"./testkit", "./lib/support.go"}
	r = Run(context.Background(), Options{Root: root, Config: c})
	if !r.Complete {
		t.Fatalf("incomplete: %+v", r.Diagnostics)
	}
	with := findingsOf(r)
	for _, name := range []string{"lib.SupportOnly", "testkit.Helper"} {
		if got := with[name]; got != "" {
			t.Errorf("used test support %s reported %s", name, got)
		}
	}
	if with["testkit.Orphan"] != "unused_test_helper" {
		t.Errorf("unused test support should be an unused test helper; got %q", with["testkit.Orphan"])
	}
	if with["lib.Legacy"] != "test_only" {
		t.Errorf("production code outside test support is unaffected; got %q", with["lib.Legacy"])
	}
	if got := r.Scopes[0].TestSupport; len(got) != 2 {
		t.Errorf("scope does not record test support: %v", got)
	}
}

func TestRemovalPlanFollowsTheCascade(t *testing.T) {
	root, c := fixture(t)
	r := Run(context.Background(), Options{Root: root, Config: c})
	if !r.Complete {
		t.Fatalf("incomplete: %+v", r.Diagnostics)
	}
	planned := map[string]report.Removal{}
	wave := map[string]int{}
	for _, w := range r.Plan.Waves {
		for _, x := range w.Removals {
			name := strings.TrimPrefix(x.Symbol, "example.com/sample/")
			planned[name], wave[name] = x, w.Number
		}
	}
	for name, want := range map[string]int{"lib.Obsolete": 1, "lib.TestObsolete": 1, "lib.obsoleteSetup": 2, "lib.DeadA": 1} {
		if wave[name] != want {
			t.Errorf("%s planned in wave %d, want %d", name, wave[name], want)
		}
	}
	if planned["lib.TestObsolete"].Reason != "dedicated_test" || planned["lib.obsoleteSetup"].Reason != "exposed" {
		t.Errorf("unexpected reasons: %+v %+v", planned["lib.TestObsolete"], planned["lib.obsoleteSetup"])
	}
	for _, kept := range []string{"lib.TestMixed", "lib.Live", "lib.job.label", "lib.Hidden.seal"} {
		if _, ok := planned[kept]; ok {
			t.Errorf("%s must not be planned for removal", kept)
		}
	}
	if !slices.Contains(r.Plan.Waves[0].TestsNeedingEdits, "example.com/sample/lib.TestMixed") {
		t.Errorf("TestMixed references removed code and survives: %v", r.Plan.Waves[0].TestsNeedingEdits)
	}
	retired := planned["lib.Retired"]
	if !retired.OtherPackages || !slices.Contains(retired.ReferencedBy, "example.com/sample/consumer.checkRetired") ||
		!slices.Contains(r.Plan.Waves[0].TestsNeedingEdits, "example.com/sample/consumer.checkRetired") {
		t.Errorf("a helper in another package must count as a referrer: %+v", retired)
	}
	if planned["lib.Obsolete"].Lines < 1 || r.Plan.Lines < r.Plan.Waves[0].Lines {
		t.Errorf("line counts missing: %+v", r.Plan)
	}
}

func TestDeclarationsAndTheirSafetyRules(t *testing.T) {
	root, c := fixture(t)
	r := Run(context.Background(), Options{Root: root, Config: c})
	if !r.Complete {
		t.Fatalf("incomplete: %+v", r.Diagnostics)
	}
	found := findingsOf(r)
	for name, want := range map[string]string{
		"lib.deadConfig": "unreachable", "lib.deadLimit": "unreachable", "lib.deadTable": "unreachable",
		"lib.settings.unused": "unreachable", "lib.settings.enabled": "never_assigned",
		"lib.testOnlyKind": "test_only", "lib.unusedFixture": "unused_test_helper",
		// Nothing in the program looks methods up by reflection, so the rule that
		// keeps exported methods of runtime types does not keep these.
		"lib.Receipt.Total": "unreachable", "lib.double": "unreachable",
		"lib.Preset": "test_only", "lib.PresetMorning": "test_only", "lib.PresetNoon": "test_only",
	} {
		if got := found[name]; got != want {
			t.Errorf("%s: got %q want %q", name, got, want)
		}
	}
	for _, kept := range []string{"lib.Shape", "lib.Circle", "lib.Square", "lib.settings", "lib.settings.name",
		"lib.settings.Exported", "lib.settings.tagged", "lib.header.spare", "lib.header.magic", "lib.registry", "lib.digest.sum", "lib.box.value", "lib.box.spare", "lib.instance.id",
		"lib.deadConfig.size", "lib.finisher"} {
		if got := found[kept]; got != "" {
			t.Errorf("%s must not be a finding; got %q", kept, got)
		}
	}
	notes := map[string]string{}
	for _, n := range r.Notes {
		notes[strings.TrimPrefix(n.Symbol, "example.com/sample/")] = n.Kind
	}
	if notes["lib.Square"] != "unused_enum_value" || notes["lib.instance.id"] != "size_guard_field" ||
		notes["lib.box.spare"] != "size_guard_field" || len(r.Notes) != 3 {
		t.Errorf("enum note missing or extra: %v", r.Notes)
	}
	for _, f := range r.Findings {
		if strings.HasSuffix(f.Symbol, "settings.enabled") && len(f.Reads) != 1 {
			t.Errorf("never-assigned read locations: %+v", f.Reads)
		}
	}
	wave := map[string]int{}
	for _, w := range r.Plan.Waves {
		for _, x := range w.Removals {
			wave[strings.TrimPrefix(x.Symbol, "example.com/sample/")] = w.Number
		}
	}
	if wave["lib.deadConfig"] != 1 || wave["lib.setupState"] != 2 || wave["lib.settings.enabled"] != 0 || wave["lib.Square"] != 0 {
		t.Errorf("declaration waves: deadConfig=%d setupState=%d enabled=%d Square=%d",
			wave["lib.deadConfig"], wave["lib.setupState"], wave["lib.settings.enabled"], wave["lib.Square"])
	}
	if r.Plan.Declarations == 0 {
		t.Errorf("plan counts no declarations: %+v", r.Plan)
	}
}

func reflectionFixture(t *testing.T, reviews ...config.ReflectionReview) *report.Report {
	t.Helper()
	root, err := filepath.Abs("../../testdata/reflection")
	if err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Modules[0].EntryPoints = []config.EntryPoint{{Package: "./cmd/app", Role: "application"}}
	c.ReflectionReviews = reviews
	r := Run(context.Background(), Options{Root: root, Config: c})
	if !r.Complete {
		t.Fatalf("incomplete: %+v", r.Diagnostics)
	}
	return r
}

// reflectionFindings leaves out the fixture's ordinary dead code.
func reflectionFindings(r *report.Report) map[string]string {
	found := findingsOf(r)
	for _, dead := range []string{"Labeler", "tag", "tag.Label", "unusedLabel"} {
		delete(found, "example.com/reflection/shop."+dead)
	}
	return found
}

func reflectionReview(entry string) config.ReflectionReview {
	return config.ReflectionReview{ID: "R-1", Entry: entry, Reason: "Only sees the plugin's own handler.",
		ApprovedBy: "owner", ReviewAfter: time.Now().AddDate(0, 1, 0).Format("2006-01-02")}
}

func TestReflectiveLookupKeepsExportedMethodsUntilReviewed(t *testing.T) {
	r := reflectionFixture(t)
	want := report.ReflectiveLookup{Analysis: "production", Entry: "example.com/reflection/plugin.Call", Via: "example.com/reflection/plugin.Call"}
	if lookups := r.Scopes[0].ReflectiveLookups; len(lookups) != 1 || lookups[0] != want {
		t.Fatalf("lookups: %+v", lookups)
	}
	if found := reflectionFindings(r); len(found) != 0 {
		t.Errorf("unreviewed reflection must not create findings: %v", found)
	}
	notes := map[string]string{}
	for _, n := range r.Notes {
		notes[strings.TrimPrefix(n.Symbol, "example.com/reflection/")] = n.Kind
	}
	if notes["shop.Receipt.Total"] != "reflection_retained" || notes["shop.double"] != "reflection_retained" ||
		notes["shop.square.Area"] != "" {
		t.Errorf("notes: %v", notes)
	}
}

func TestReviewedReflectionFindsCodeOnlyItKept(t *testing.T) {
	r := reflectionFixture(t, reflectionReview("example.com/reflection/plugin.Call"))
	if r.ReflectionReviews[0].Status != "matched" || r.Summary.StaleExceptions != 0 {
		t.Fatalf("review: %+v stale=%d", r.ReflectionReviews, r.Summary.StaleExceptions)
	}
	found := findingsOf(r)
	for name, want := range map[string]string{"shop.Receipt.Total": "test_only", "shop.double": "test_only"} {
		if got := found["example.com/reflection/"+name]; got != want {
			t.Errorf("%s: got %q want %q", name, got, want)
		}
	}
	for _, kept := range []string{"plugin.Start", "shop.square.Area", "shop.Print", "shop.Count", "shop.fakePricer.Price", "shop.counter.Reset"} {
		if got := found["example.com/reflection/"+kept]; got != "" {
			t.Errorf("%s must not be a finding; got %q", kept, got)
		}
	}
	conformance := map[string]bool{}
	for _, n := range r.Notes {
		if n.Kind == "interface_conformance" {
			conformance[strings.TrimPrefix(n.Symbol, "example.com/reflection/")] = true
		}
	}
	if !conformance["shop.square.Area"] || !conformance["shop.counter.Reset"] {
		t.Errorf("Area and Reset need interface_conformance notes: %+v", r.Notes)
	}
	for _, dead := range []string{"shop.tag.Label", "shop.unusedLabel"} {
		if got := found["example.com/reflection/"+dead]; got != "unreachable" {
			t.Errorf("%s: a requirement only dead code makes must not keep it; got %q", dead, got)
		}
	}
}

func TestUnmatchedReflectionReviewIsStale(t *testing.T) {
	r := reflectionFixture(t, reflectionReview("example.com/reflection/plugin.Missing"))
	if r.ReflectionReviews[0].Status != "unmatched" || r.Summary.StaleExceptions != 1 || len(reflectionFindings(r)) != 0 {
		t.Fatalf("review: %+v stale=%d findings=%v", r.ReflectionReviews, r.Summary.StaleExceptions, reflectionFindings(r))
	}
}
