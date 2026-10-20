// Package analyze computes conservative source reachability without modifying targets.
package analyze

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bradhannah/go-clean-deadcode/internal/config"
	"github.com/bradhannah/go-clean-deadcode/internal/report"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type Options struct {
	Root     string
	Config   config.Config
	Discover bool
	Progress func(string)
}

type engine struct {
	root     string
	report   *report.Report
	symbols  map[string]*report.Symbol
	states   map[string]map[string]*report.State
	edges    map[report.Edge]bool
	refs     map[report.Reference]bool
	progress func(string)
	bodies   map[string]map[string]bool
}

func Run(ctx context.Context, opt Options) (result *report.Report) {
	start := time.Now()
	mode := "analyze"
	if opt.Discover {
		mode = "discover"
	}
	e := &engine{root: opt.Root, report: report.New(mode), symbols: map[string]*report.Symbol{}, states: map[string]map[string]*report.State{}, edges: map[report.Edge]bool{}, refs: map[report.Reference]bool{}}
	e.progress = opt.Progress
	e.bodies = map[string]map[string]bool{}
	defer func() {
		if p := recover(); p != nil {
			e.report.Error("analysis_panic", "", fmt.Sprintf("analysis failed: %v", p))
			result = e.report
		}
		e.report.Metrics.DurationMS = time.Since(start).Milliseconds()
	}()
	if err := opt.Config.Validate(); err != nil {
		e.report.Error("configuration", "", err.Error())
		return e.report
	}
	root, err := filepath.Abs(opt.Root)
	if err != nil {
		e.report.Error("root", "", err.Error())
		return e.report
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		e.report.Error("root", "", err.Error())
		return e.report
	}
	e.root = root
	nested, err := nestedModules(root)
	if err != nil {
		e.report.Error("module_discovery", "", err.Error())
		return e.report
	}
	configured := map[string]bool{}
	for _, m := range opt.Config.Modules {
		configured[filepath.ToSlash(filepath.Clean(m.Path))] = true
	}
	for _, x := range opt.Config.ExcludedModules {
		configured[filepath.ToSlash(filepath.Clean(x.Path))] = true
		e.report.Diagnostics = append(e.report.Diagnostics, report.Diagnostic{Code: "excluded_module", Message: x.Path + ": " + x.Reason, Severity: "info"})
	}
	for _, p := range nested {
		if !configured[p] && !opt.Discover {
			e.report.Error("unclassified_module", "", p+" requires a module scope or explicit exclusion")
		}
	}
	for _, m := range opt.Config.Modules {
		for _, v := range opt.Config.Configurations {
			if ctx.Err() != nil {
				e.report.Error("cancelled", "", ctx.Err().Error())
				return e.report
			}
			e.scope(ctx, m, v, nested, opt.Discover)
		}
	}
	e.finalize(opt.Config)
	if ctx.Err() != nil {
		e.report.Error("cancelled", "", ctx.Err().Error())
	}
	return e.report
}

func (e *engine) log(s string) {
	if e.progress != nil {
		e.progress(s)
	}
}

func nestedModules(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root && (d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "go.mod" && filepath.Dir(path) != root {
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(found)
	return found, err
}

func envFor(v config.Configuration) []string {
	env := []string{}
	// Explicitly neutralize ambient package-selection and write-enabling flags.
	for _, s := range os.Environ() {
		k, _, _ := strings.Cut(s, "=")
		if !slices.Contains([]string{"GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS", "GOWORK", "GOTOOLCHAIN", "GOPACKAGESDRIVER"}, k) {
			env = append(env, s)
		}
	}
	cgo := "0"
	if v.CGOEnabled {
		cgo = "1"
	}
	return append(env, "GOOS="+v.GOOS, "GOARCH="+v.GOARCH, "CGO_ENABLED="+cgo, "GOFLAGS=-mod=readonly", "GOWORK=off", "GOTOOLCHAIN=local", "GOPACKAGESDRIVER=off")
}

func (e *engine) load(ctx context.Context, m config.Module, v config.Configuration, tests bool, patterns []string, full bool) ([]*packages.Package, error) {
	mode := packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedModule
	if full {
		mode = packages.LoadAllSyntax | packages.NeedModule
	}
	t := time.Now()
	e.log(fmt.Sprintf("load %s tests=%t (%s/%s)", m.Path, tests, v.GOOS, v.GOARCH))
	pkgs, err := packages.Load(&packages.Config{Context: ctx, Dir: filepath.Join(e.root, m.Path), Mode: mode, Tests: tests, Env: envFor(v), BuildFlags: []string{"-tags=" + strings.Join(v.Tags, ",")}}, patterns...)
	e.report.Metrics.LoadMS += time.Since(t).Milliseconds()
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages match %v", patterns)
	}
	var errors []string
	packages.Visit(pkgs, func(p *packages.Package) bool {
		for _, x := range p.Errors {
			errors = append(errors, x.Error())
		}
		return true
	}, nil)
	if len(errors) > 0 {
		sort.Strings(errors)
		return nil, fmt.Errorf("%s", strings.Join(slices.Compact(errors), "\n"))
	}
	return pkgs, nil
}

func (e *engine) scope(ctx context.Context, m config.Module, v config.Configuration, nested []string, discover bool) {
	id := filepath.ToSlash(m.Path) + "@" + v.Name
	s := report.Scope{ID: id, Module: m.Path, Configuration: v.Name, GOOS: v.GOOS, GOARCH: v.GOARCH, Tags: append([]string{}, v.Tags...), CGOEnabled: v.CGOEnabled, Tests: m.IncludeTests(), EntryPoints: []report.EntryPoint{}, NestedModules: append([]string{}, nested...)}
	s.Patterns = append([]string{}, m.Patterns...)
	defer func() {
		for _, d := range e.report.Diagnostics {
			if d.Scope == id && d.Severity == "error" {
				s.Complete = false
			}
		}
		e.report.Scopes = append(e.report.Scopes, s)
	}()
	moduleDir := filepath.Join(e.root, m.Path)
	real, err := filepath.EvalSymlinks(moduleDir)
	if err != nil || !within(e.root, real) {
		e.report.Error("module_path", id, "module path must resolve within source root")
		return
	}
	if _, err := os.Stat(filepath.Join(moduleDir, "go.mod")); err != nil {
		e.report.Error("module_path", id, "module has no go.mod")
		return
	}
	moduleDir = real
	all, err := e.load(ctx, m, v, false, []string{"./..."}, false)
	if err != nil {
		e.report.Error("load", id, err.Error())
		return
	}
	selected := map[string]string{}
	matched := map[int]bool{}
	ignored := map[int]bool{}
	for _, p := range all {
		if p.Name != "main" {
			continue
		}
		dir := ""
		if len(p.GoFiles) > 0 {
			dir = filepath.Dir(p.GoFiles[0])
		}
		rel, _ := filepath.Rel(moduleDir, dir)
		local := "./" + filepath.ToSlash(rel)
		if rel == "." {
			local = "."
		}
		ep := report.EntryPoint{Package: p.PkgPath, Directory: local, Role: "unclassified"}
		for i, x := range m.EntryPoints {
			if x.Enabled(v.Name) && (x.Package == p.PkgPath || x.Package == local) {
				ep.Role = x.Role
				ep.Classified = true
				selected[p.PkgPath] = x.Role
				matched[i] = true
			}
		}
		for i, x := range m.IgnoredMains {
			if x.Path == p.PkgPath || x.Path == local {
				if ep.Classified {
					e.report.Error("entry_conflict", id, p.PkgPath+" is both selected and excluded")
				}
				ep.Role = "excluded"
				ep.Classified = true
				ignored[i] = true
				e.report.Diagnostics = append(e.report.Diagnostics, report.Diagnostic{Code: "excluded_main", Scope: id, Message: p.PkgPath + ": " + x.Reason, Severity: "info"})
			}
		}
		s.EntryPoints = append(s.EntryPoints, ep)
		if !ep.Classified && !discover {
			e.report.Error("unclassified_main", id, p.PkgPath+" ("+local+") requires an entry point or explicit exclusion")
		}
	}
	sort.Slice(s.EntryPoints, func(i, j int) bool { return s.EntryPoints[i].Package < s.EntryPoints[j].Package })
	for i, x := range m.EntryPoints {
		if x.Enabled(v.Name) && !matched[i] && !discover {
			e.report.Error("missing_main", id, x.Package+" is not an executable in this configuration")
		}
	}
	for i, x := range m.IgnoredMains {
		if !ignored[i] && !discover {
			e.report.Error("stale_main_exclusion", id, x.Path+" does not match an executable")
		}
	}
	if discover {
		s.Packages = len(all)
		s.Complete = true
		return
	}
	if len(selected) == 0 && !m.Library {
		e.report.Error("no_roots", id, "no selected executable; library scopes must explicitly set library=true")
		return
	}
	prod, err := e.load(ctx, m, v, false, m.Patterns, true)
	if err != nil {
		e.report.Error("load", id, err.Error())
		return
	}
	s.Packages = len(prod)
	if err = e.program(ctx, prod, moduleDir, id, "production", selected); err != nil {
		e.report.Error("analysis", id, err.Error())
		return
	}
	prod = nil
	if m.IncludeTests() {
		tests, err := e.load(ctx, m, v, true, m.Patterns, true)
		if err != nil {
			e.report.Error("test_load", id, err.Error())
			return
		}
		if err = e.program(ctx, tests, moduleDir, id, "tests", selected); err != nil {
			e.report.Error("test_analysis", id, err.Error())
			return
		}
	}
	s.Complete = true
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func (e *engine) location(fs *token.FileSet, pos token.Pos) report.Location {
	p := fs.Position(pos)
	name := p.Filename
	if within(e.root, name) {
		name, _ = filepath.Rel(e.root, name)
	} else if name != "" {
		name = "<dependency>/" + filepath.Base(name)
	}
	return report.Location{File: filepath.ToSlash(name), Line: p.Line, Column: p.Column}
}

func objectID(f *types.Func) string {
	if f == nil || f.Pkg() == nil {
		return ""
	}
	f = f.Origin()
	sig := f.Type().(*types.Signature)
	recv := ""
	if r := sig.Recv(); r != nil {
		t := r.Type()
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		if n, ok := types.Unalias(t).(*types.Named); ok {
			recv = n.Obj().Name() + "."
		}
	}
	return f.Pkg().Path() + "." + recv + f.Name()
}

func functionID(fn *ssa.Function) string {
	if fn == nil {
		return ""
	}
	if o, ok := fn.Object().(*types.Func); ok {
		return objectID(o)
	}
	if p := fn.Parent(); p != nil {
		if i := slices.Index(p.AnonFuncs, fn); i >= 0 {
			return fmt.Sprintf("%s$%d", functionID(p), i+1)
		}
		return functionID(p) + "$" + fn.Name()
	}
	if fn.Pkg != nil {
		return fn.Pkg.Pkg.Path() + "." + fn.Name()
	}
	return fn.String()
}

func (e *engine) state(id, scope string) *report.State {
	if e.states[id] == nil {
		e.states[id] = map[string]*report.State{}
	}
	s := e.states[id][scope]
	if s == nil {
		s = &report.State{Scope: scope, Production: "not_analyzed", Tests: "not_analyzed", ProductionRoots: []string{}, TestRoots: []string{}, Witnesses: []report.Witness{}, ProvenanceComplete: true}
		e.states[id][scope] = s
	}
	return s
}

func (e *engine) program(ctx context.Context, pkgs []*packages.Package, dir, scope, kind string, selected map[string]string) error {
	t := time.Now()
	prog, ssapkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	e.log(scope + " " + kind + ": inventory")
	source := map[*ssa.Function]string{}
	positions := map[token.Position]string{}
	var roots []*ssa.Function
	rootLabels := map[*ssa.Function]string{}
	testSeeds := map[*ssa.Function]string{}
	for _, p := range ssautil.MainPackages(ssapkgs) {
		test := strings.HasSuffix(p.Pkg.Path(), ".test")
		if (kind == "production" && selected[p.Pkg.Path()] != "") || (kind == "tests" && test) {
			for _, n := range []string{"main", "init"} {
				if f := p.Func(n); f != nil {
					roots = append(roots, f)
					rootLabels[f] = functionID(f)
				}
			}
		}
	}
	if kind == "production" {
		for name := range selected {
			found := false
			for _, f := range roots {
				if f.Pkg.Pkg.Path() == name {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("selected executable %s is outside loaded package patterns", name)
			}
		}
	}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.Types == nil || p.TypesInfo == nil {
			return
		}
		if kind == "tests" && p.Name == "main" && strings.HasSuffix(p.PkgPath, ".test") {
			for _, obj := range p.TypesInfo.Uses {
				if f, ok := obj.(*types.Func); ok && f.Pkg() != nil && f.Pkg().Path() != "testing" {
					if fn := prog.FuncValue(f); fn != nil {
						testSeeds[fn] = functionID(fn)
					}
				}
			}
		}
		for _, file := range p.Syntax {
			path := p.Fset.Position(file.Pos()).Filename
			if !within(dir, path) || (p.Module != nil && filepath.Clean(p.Module.Dir) != filepath.Clean(dir)) {
				continue
			}
			initOrdinal := 0
			for _, decl := range file.Decls {
				d, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				obj, ok := p.TypesInfo.Defs[d.Name].(*types.Func)
				if !ok {
					continue
				}
				id := objectID(obj)
				if d.Name.Name == "init" {
					initOrdinal++
					id += "@" + e.location(p.Fset, d.Pos()).File + fmt.Sprintf("#%d", initOrdinal)
				}
				fn := prog.FuncValue(obj)
				if fn == nil {
					continue
				}
				source[fn] = id
				positions[p.Fset.Position(fn.Pos())] = id
				x := e.symbols[id]
				if x == nil {
					x = &report.Symbol{ID: id, Package: p.PkgPath, Name: obj.Name(), Kind: "function", TestFile: strings.HasSuffix(path, "_test.go"), Generated: ast.IsGenerated(file), Locations: []report.Location{}, States: []report.State{}, Obligations: []report.Obligation{}}
					if r := obj.Type().(*types.Signature).Recv(); r != nil {
						x.Kind = "method"
						x.Receiver = types.TypeString(r.Type(), func(*types.Package) string { return "" })
					}
					e.symbols[id] = x
				}
				var body bytes.Buffer
				if err := printer.Fprint(&body, p.Fset, d); err != nil {
					e.report.Error("declaration", scope, err.Error())
				}
				if e.bodies[id] == nil {
					e.bodies[id] = map[string]bool{}
				}
				e.bodies[id][body.String()] = true
				loc := e.location(p.Fset, d.Name.Pos())
				if !slices.Contains(x.Locations, loc) {
					x.Locations = append(x.Locations, loc)
				}
				s := e.state(id, scope)
				if kind == "production" {
					s.Production = "unreachable"
				} else {
					s.Tests = "unreachable"
				}
				if d.Body == nil {
					x.Obligations = append(x.Obligations, report.Obligation{Scope: scope, Kind: "external_body", Description: "Declaration has no Go body; native or assembly implementation requires review.", Location: loc})
				}
				if obj.Type().(*types.Signature).Recv() != nil && !obj.Exported() && d.Body != nil && len(d.Body.List) == 0 && obj.Type().(*types.Signature).Params().Len() == 0 && obj.Type().(*types.Signature).Results().Len() == 0 {
					for _, name := range p.Types.Scope().Names() {
						tn, ok := p.Types.Scope().Lookup(name).(*types.TypeName)
						if !ok {
							continue
						}
						iface, ok := tn.Type().Underlying().(*types.Interface)
						if ok && types.NewMethodSet(iface).Lookup(obj.Pkg(), obj.Name()) != nil && types.Implements(obj.Type().(*types.Signature).Recv().Type(), iface) {
							x.Obligations = append(x.Obligations, report.Obligation{Scope: scope, Kind: "marker_method", Description: "Empty private method satisfies " + p.PkgPath + "." + name + "; retained for compile-time type structure.", Location: loc})
						}
					}
				}
			}
			e.references(p, file, scope)
		}
	})
	e.log(scope + " " + kind + ": build SSA")
	prog.Build()
	e.report.Metrics.SSAMS += time.Since(t).Milliseconds()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Direct SSA edges also cover unreachable bodies: needed to expose dead cycles.
	e.log(scope + " " + kind + ": direct source edges")
	for fn := range source {
		caller := functionID(fn)
		if id, ok := positions[prog.Fset.Position(fn.Pos())]; ok {
			caller = id
		}
		if _, ok := e.symbols[caller]; !ok {
			continue
		}
		for _, b := range fn.Blocks {
			for _, in := range b.Instrs {
				if call, ok := in.(ssa.CallInstruction); ok {
					if callee := call.Common().StaticCallee(); callee != nil {
						target := functionID(callee)
						if id, ok := positions[prog.Fset.Position(callee.Pos())]; ok {
							target = id
						}
						e.edges[report.Edge{Scope: scope, Context: "source", Caller: caller, Callee: target, Kind: "static", Location: e.location(prog.Fset, call.Pos())}] = true
					}
				}
			}
		}
	}
	if len(roots) == 0 {
		return nil
	}
	t = time.Now()
	e.log(scope + " " + kind + ": RTA")
	res := rta.Analyze(roots, true)
	e.report.Metrics.ReachabilityMS += time.Since(t).Milliseconds()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	labels := rootLabels
	if kind == "tests" {
		labels = map[*ssa.Function]string{}
		for fn, id := range testSeeds {
			if s := e.symbols[id]; s != nil {
				s.RegisteredTest = true
			}
			if _, ok := res.Reachable[fn]; ok {
				labels[fn] = id
			}
		}
		for fn, id := range rootLabels {
			labels[fn] = id
		}
		// A closure's lexical test parent is stronger source evidence than an RTA
		// path through testing.tRunner, whose signature joins unrelated callbacks.
		for fn := range res.Reachable {
			for p := fn.Parent(); p != nil; p = p.Parent() {
				if id, ok := testSeeds[p]; ok {
					labels[fn] = id
					break
				}
			}
		}
	}
	e.log(scope + " " + kind + ": witnesses")
	index := newGraphIndex(res.CallGraph)
	targets := map[*ssa.Function]bool{}
	for fn := range res.Reachable {
		id, ok := positions[prog.Fset.Position(fn.Pos())]
		if !ok {
			continue
		}
		targets[fn] = true
		s := e.state(id, scope)
		if kind == "production" {
			s.Production = "reachable"
		} else {
			s.Tests = "reachable"
		}
	}
	productionPaths := map[*ssa.Function]report.Witness{}
	if kind == "production" {
		productionPaths = forwardWitnesses(index, rootLabels, kind, targets)
	}
	for fn := range res.Reachable {
		id, ok := positions[prog.Fset.Position(fn.Pos())]
		if !ok {
			continue
		}
		s := e.state(id, scope)
		// Paths are bounded examples, never an exhaustive attribution claim.
		var w []report.Witness
		complete := false
		if kind == "production" {
			if p, ok := productionPaths[fn]; ok {
				w = []report.Witness{p}
			}
		} else {
			if s.Production == "reachable" || e.symbols[id].TestFile {
				continue
			}
			w, complete = witnesses(index, fn, labels, kind)
		}
		if !complete {
			s.ProvenanceComplete = false
		}
		if len(w) == 0 {
			w = []report.Witness{{Root: "", Context: kind, Kind: "unexplained_retention", Path: []string{id}}}
			s.ProvenanceComplete = false
		}
		for _, v := range w {
			s.Witnesses = append(s.Witnesses, v)
			if v.Root != "" {
				if kind == "production" {
					s.ProductionRoots = append(s.ProductionRoots, v.Root)
				} else {
					s.TestRoots = append(s.TestRoots, v.Root)
				}
			}
		}
	}
	e.log(scope + " " + kind + ": graph edges")
	for _, node := range res.CallGraph.Nodes {
		for _, edge := range node.Out {
			caller, callee := index.name(edge.Caller.Func), index.name(edge.Callee.Func)
			if _, ok := e.symbols[caller]; !ok {
				continue
			}
			if _, ok := e.symbols[callee]; !ok {
				continue
			}
			ek := "dynamic"
			if edge.Site != nil && edge.Site.Common().StaticCallee() != nil {
				ek = "static"
			}
			e.edges[report.Edge{Scope: scope, Context: kind, Caller: caller, Callee: callee, Kind: ek, Location: e.location(prog.Fset, edge.Pos())}] = true
		}
	}
	return nil
}
