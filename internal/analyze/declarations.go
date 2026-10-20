package analyze

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bradhannah/go-clean-deadcode/internal/config"
	"github.com/bradhannah/go-clean-deadcode/internal/report"
	"golang.org/x/tools/go/packages"
)

// Package-level types, constants, variables and struct fields are live when a
// reachable function, or another live declaration, uses them. A struct's
// layout-significant fields and an enum's unused values are never removal
// findings: removing them would change a struct's meaning or a value set that
// is there on purpose.
type declaration struct {
	kind      string // "type", "field", "const" or "var"
	owner     string // the struct type that declares a field
	exported  bool
	tagged    bool
	embedded  bool
	effects   bool   // a variable whose initializer calls something
	enumGroup string // a grouped constant block: a value set such as an enum
	enumType  string // the named type of a typed constant
}

// layoutPackages see a struct's fields through reflection or memory layout, so
// removing even an unused field changes what they do.
var layoutPackages = map[string]bool{"encoding/binary": true, "reflect": true, "unsafe": true}

type fieldRead struct {
	from     string
	location report.Location
}

type declarationEvidence struct {
	decls       map[string]*declaration
	fields      map[*types.Var]string
	uses        map[string]map[string]bool            // user -> declaration
	writes      map[string]bool                       // fields written anywhere, including dead code
	reads       map[string][]fieldRead                // fields read, with the reading function
	significant map[string]bool                       // struct types whose layout matters
	roots       map[string]map[string]bool            // scope -> declarations that exported API keeps
	reachable   map[string]map[string]map[string]bool // scope -> kind -> reachable functions
}

func newDeclarationEvidence() *declarationEvidence {
	return &declarationEvidence{decls: map[string]*declaration{}, fields: map[*types.Var]string{},
		uses: map[string]map[string]bool{}, writes: map[string]bool{}, reads: map[string][]fieldRead{},
		significant: map[string]bool{}, roots: map[string]map[string]bool{}, reachable: map[string]map[string]map[string]bool{}}
}

func (e *engine) reachableFunction(scope, analysis, id string) {
	if e.decl.reachable[scope] == nil {
		e.decl.reachable[scope] = map[string]map[string]bool{}
	}
	if e.decl.reachable[scope][analysis] == nil {
		e.decl.reachable[scope][analysis] = map[string]bool{}
	}
	e.decl.reachable[scope][analysis][id] = true
}

func packageObjectID(obj types.Object) string {
	return obj.Pkg().Path() + "." + obj.Name()
}

func (e *engine) use(from, to string) {
	if from == "" || to == "" || from == to {
		return
	}
	if e.decl.uses[from] == nil {
		e.decl.uses[from] = map[string]bool{}
	}
	e.decl.uses[from][to] = true
}

// declareSymbol creates or extends the symbol for a declaration seen in one scope.
func (e *engine) declareSymbol(p *packages.Package, id, name, kind, path, scope, analysis string, node ast.Node,
	pos token.Pos, module config.Module, dir string, file *ast.File) *report.Symbol {
	x := e.symbols[id]
	if x == nil {
		x = &report.Symbol{ID: id, Package: p.PkgPath, Name: name, Kind: kind, TestFile: strings.HasSuffix(path, "_test.go"),
			Generated: ast.IsGenerated(file), Locations: []report.Location{}, States: []report.State{}, Obligations: []report.Obligation{}}
		if rel, err := filepath.Rel(dir, path); err == nil && !x.TestFile {
			x.TestSupport = module.IsTestSupport(rel)
		}
		e.symbols[id] = x
	}
	var body bytes.Buffer
	if err := printer.Fprint(&body, p.Fset, node); err == nil {
		if e.bodies[id] == nil {
			e.bodies[id] = map[string]bool{}
		}
		e.bodies[id][body.String()] = true
	}
	x.Lines = max(x.Lines, p.Fset.Position(node.End()).Line-p.Fset.Position(node.Pos()).Line+1)
	if loc := e.location(p.Fset, pos); !containsLocation(x.Locations, loc) {
		x.Locations = append(x.Locations, loc)
	}
	s := e.state(id, scope)
	if analysis == "production" {
		s.Production = "unreachable"
	} else {
		s.Tests = "unreachable"
	}
	return x
}

func containsLocation(locations []report.Location, loc report.Location) bool {
	for _, l := range locations {
		if l == loc {
			return true
		}
	}
	return false
}

// declare inventories one package-level declaration group. Uses are recorded by
// declarationUses once the whole package is inventoried, so that a use can name
// a field declared later in the package.
func (e *engine) declare(p *packages.Package, file *ast.File, g *ast.GenDecl, path, scope, analysis string,
	module config.Module, dir string, api bool) {
	group := ""
	if g.Tok == token.CONST && g.Lparen.IsValid() && constNames(g) > 1 {
		loc := e.location(p.Fset, g.Pos())
		group = fmt.Sprintf("%s#%d", loc.File, loc.Line)
	}
	for _, spec := range g.Specs {
		switch spec := spec.(type) {
		case *ast.TypeSpec:
			obj, ok := p.TypesInfo.Defs[spec.Name].(*types.TypeName)
			if !ok || spec.Name.Name == "_" {
				continue
			}
			id := packageObjectID(obj)
			e.declareSymbol(p, id, obj.Name(), "type", path, scope, analysis, spec, spec.Name.Pos(), module, dir, file)
			e.decl.decls[id] = &declaration{kind: "type", exported: obj.Exported()}
			e.markRoot(api, scope, id, obj.Exported())
			structAST, isStruct := spec.Type.(*ast.StructType)
			structType, ok := obj.Type().Underlying().(*types.Struct)
			if isStruct && ok && !obj.IsAlias() {
				e.declareFields(p, file, structAST, structType, id, path, scope, analysis, module, dir, api && obj.Exported())
			}
		case *ast.ValueSpec:
			kind := "var"
			if g.Tok == token.CONST {
				kind = "const"
			}
			for i, name := range spec.Names {
				obj := p.TypesInfo.Defs[name]
				if obj == nil || name.Name == "_" || obj.Parent() != p.Types.Scope() {
					continue
				}
				id := packageObjectID(obj)
				e.declareSymbol(p, id, name.Name, kind, path, scope, analysis, spec, name.Pos(), module, dir, file)
				d := &declaration{kind: kind, exported: obj.Exported(), enumGroup: group}
				if named, ok := obj.Type().(*types.Named); ok && kind == "const" && named.Obj().Pkg() != nil {
					d.enumType = packageObjectID(named.Obj())
				}
				if kind == "var" && i < len(spec.Values) {
					d.effects = callsSomething(p, spec.Values[i])
				} else if kind == "var" && len(spec.Values) == 1 {
					d.effects = callsSomething(p, spec.Values[0])
				}
				e.decl.decls[id] = d
				e.markRoot(api, scope, id, obj.Exported())
			}
		}
	}
}

// declareFields inventories a struct's fields; each field uses its own type, so
// removing an unused field can free a type only it used.
func (e *engine) declareFields(p *packages.Package, file *ast.File, structAST *ast.StructType, st *types.Struct, owner,
	path, scope, analysis string, module config.Module, dir string, api bool) {
	for i := range st.NumFields() {
		field := st.Field(i)
		node := fieldNode(structAST, field)
		if field.Name() == "_" || node == nil {
			continue
		}
		id := owner + "." + field.Name()
		e.decl.fields[field] = id
		e.declareSymbol(p, id, field.Name(), "field", path, scope, analysis, node, field.Pos(), module, dir, file)
		e.decl.decls[id] = &declaration{kind: "field", owner: owner, exported: field.Exported(),
			tagged: st.Tag(i) != "", embedded: field.Embedded()}
		e.markRoot(api, scope, id, field.Exported())
	}
}

// declarationUses records what one inventoried declaration group uses: a
// struct's fields use their own types, other types use their definition, and
// constants and variables use their type and initializer.
func (e *engine) declarationUses(p *packages.Package, g *ast.GenDecl) {
	for _, spec := range g.Specs {
		switch spec := spec.(type) {
		case *ast.TypeSpec:
			obj, ok := p.TypesInfo.Defs[spec.Name].(*types.TypeName)
			if !ok || spec.Name.Name == "_" {
				continue
			}
			id := packageObjectID(obj)
			if spec.TypeParams != nil {
				e.recordUses(p, spec.TypeParams, id)
			}
			structAST, isStruct := spec.Type.(*ast.StructType)
			structType, ok := obj.Type().Underlying().(*types.Struct)
			if !isStruct || !ok || obj.IsAlias() {
				e.recordUses(p, spec.Type, id)
				continue
			}
			for i := range structType.NumFields() {
				field := structType.Field(i)
				if node := fieldNode(structAST, field); node != nil && field.Name() != "_" {
					e.recordUses(p, node.Type, id+"."+field.Name())
				}
			}
		case *ast.ValueSpec:
			for _, name := range spec.Names {
				obj := p.TypesInfo.Defs[name]
				if obj == nil || name.Name == "_" || obj.Parent() != p.Types.Scope() {
					continue
				}
				id := packageObjectID(obj)
				if spec.Type != nil {
					e.recordUses(p, spec.Type, id)
				}
				for _, value := range spec.Values {
					e.recordUses(p, value, id)
				}
			}
		}
	}
}

// fieldNode finds the syntax declaring a field: by name, or for an embedded
// field by the type expression that contains its position.
func fieldNode(structAST *ast.StructType, field *types.Var) *ast.Field {
	for _, f := range structAST.Fields.List {
		for _, name := range f.Names {
			if name.Pos() == field.Pos() {
				return f
			}
		}
		if len(f.Names) == 0 && f.Type.Pos() <= field.Pos() && field.Pos() < f.Type.End() {
			return f
		}
	}
	return nil
}

func (e *engine) markRoot(api bool, scope, id string, exported bool) {
	if !api || !exported {
		return
	}
	if e.decl.roots[scope] == nil {
		e.decl.roots[scope] = map[string]bool{}
	}
	e.decl.roots[scope][id] = true
}

// constNames counts the names a constant block declares. A block of several
// constants is treated as one value set, like an enum or a catalog of indexes.
func constNames(g *ast.GenDecl) int {
	names := 0
	for _, spec := range g.Specs {
		if v, ok := spec.(*ast.ValueSpec); ok {
			names += len(v.Names)
		}
	}
	return names
}

// callsSomething reports an initializer with possible side effects: any call
// that is not a type conversion or a builtin.
func callsSomething(p *packages.Package, expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || found {
			return !found
		}
		if tv, ok := p.TypesInfo.Types[call.Fun]; ok && (tv.IsType() || tv.IsBuiltin()) {
			return true
		}
		found = true
		return false
	})
	return found
}

// declarationTarget names the inventoried declaration an identifier uses.
func (e *engine) declarationTarget(p *packages.Package, id *ast.Ident) string {
	switch obj := p.TypesInfo.Uses[id].(type) {
	case *types.Var:
		if obj.IsField() {
			// A generic type's methods use the instantiated field; Origin is the declared one.
			return e.decl.fields[obj.Origin()]
		}
		if obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope() {
			return packageObjectID(obj)
		}
	case *types.Const:
		if obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope() {
			return packageObjectID(obj)
		}
	case *types.TypeName:
		if obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope() {
			return packageObjectID(obj)
		}
	}
	return ""
}

// recordUses records declarations used under node by from, classifies field
// accesses as reads or writes, and notes struct types whose layout matters.
func (e *engine) recordUses(p *packages.Package, node ast.Node, from string) {
	var stack []ast.Node
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		switch x := n.(type) {
		case *ast.Ident:
			target := e.declarationTarget(p, x)
			if target == "" {
				return true
			}
			e.use(from, target)
			if d := e.decl.decls[target]; d != nil && d.kind == "field" {
				e.use(from, d.owner)
				if fieldWritten(p, stack) {
					e.decl.writes[target] = true
				} else {
					e.decl.reads[target] = append(e.decl.reads[target], fieldRead{from: from, location: e.location(p.Fset, x.Pos())})
				}
			}
		case *ast.CompositeLit:
			if st := namedStruct(p.TypesInfo.TypeOf(x)); st != "" && len(x.Elts) > 0 {
				if _, keyed := x.Elts[0].(*ast.KeyValueExpr); !keyed {
					e.decl.significant[st] = true // Unkeyed literals depend on field order.
				}
			}
		case *ast.CallExpr:
			e.layoutCall(p, x)
		}
		return true
	})
}

// fieldWritten reports whether the field identifier on top of stack is a write
// target: an assignment or increment target (directly or through indexing or a
// nested field), a composite literal key, an address, a slice of it (which
// aliases an array field's memory, as in copy(x.f[:], src)), or the receiver of a
// pointer-receiver method call.
func fieldWritten(p *packages.Package, stack []ast.Node) bool {
	node := stack[len(stack)-1]
	for i := len(stack) - 2; i >= 0; i-- {
		switch parent := stack[i].(type) {
		case *ast.KeyValueExpr:
			_, literal := stack[max(i-1, 0)].(*ast.CompositeLit)
			return parent.Key == node && literal
		case *ast.SelectorExpr:
			if parent.X == node {
				if method, ok := p.TypesInfo.Uses[parent.Sel].(*types.Func); ok {
					sig := method.Type().(*types.Signature)
					_, pointer := sig.Recv().Type().(*types.Pointer)
					return pointer
				}
			}
		case *ast.IndexExpr:
			if parent.X != node {
				return false
			}
		case *ast.SliceExpr:
			return parent.X == node
		case *ast.IndexListExpr, *ast.StarExpr, *ast.ParenExpr:
		case *ast.AssignStmt:
			for _, lhs := range parent.Lhs {
				if lhs == node {
					return true
				}
			}
			return false
		case *ast.IncDecStmt:
			return parent.X == node
		case *ast.UnaryExpr:
			return parent.Op == token.AND
		case *ast.RangeStmt:
			return parent.Key == node || parent.Value == node
		default:
			return false
		}
		node = stack[i]
	}
	return false
}

func namedStruct(t types.Type) string {
	for {
		switch u := t.(type) {
		case *types.Pointer:
			t = u.Elem()
		case *types.Slice:
			t = u.Elem()
		case *types.Array:
			t = u.Elem()
		case *types.Alias:
			t = types.Unalias(u)
		case *types.Named:
			if _, ok := u.Underlying().(*types.Struct); ok && u.Obj().Pkg() != nil {
				return packageObjectID(u.Obj())
			}
			return ""
		default:
			return ""
		}
	}
}

// layoutCall marks struct types that reach reflection, unsafe or binary
// encoding, and both sides of a struct-to-struct conversion.
func (e *engine) layoutCall(p *packages.Package, call *ast.CallExpr) {
	if tv, ok := p.TypesInfo.Types[call.Fun]; ok && tv.IsType() && len(call.Args) == 1 {
		to, from := namedStruct(tv.Type), namedStruct(p.TypesInfo.TypeOf(call.Args[0]))
		if to != "" && from != "" {
			e.decl.significant[to], e.decl.significant[from] = true, true
		}
		if basic, ok := tv.Type.(*types.Basic); ok && basic.Kind() == types.UnsafePointer {
			if st := namedStruct(p.TypesInfo.TypeOf(call.Args[0])); st != "" {
				e.decl.significant[st] = true
			}
		}
		return
	}
	var pkg string
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		if obj := p.TypesInfo.Uses[fun.Sel]; obj != nil && obj.Pkg() != nil {
			pkg = obj.Pkg().Path()
		} else if _, builtin := p.TypesInfo.Uses[fun.Sel].(*types.Builtin); builtin {
			pkg = "unsafe"
		}
	case *ast.Ident:
		if obj := p.TypesInfo.Uses[fun]; obj != nil && obj.Pkg() != nil {
			pkg = obj.Pkg().Path()
		}
	}
	if !layoutPackages[pkg] {
		return
	}
	for _, arg := range call.Args {
		if st := namedStruct(p.TypesInfo.TypeOf(arg)); st != "" {
			e.decl.significant[st] = true
		}
	}
}

// layoutSignificant closes significance over field types: a struct inside a
// layout-significant struct is laid out with it.
func (e *engine) layoutSignificant() map[string]bool {
	significant := map[string]bool{}
	var queue []string
	for id := range e.decl.significant {
		significant[id] = true
		queue = append(queue, id)
	}
	fieldTypes := map[string][]string{}
	for id, d := range e.decl.decls {
		if d.kind == "field" {
			for to := range e.decl.uses[id] {
				fieldTypes[d.owner] = append(fieldTypes[d.owner], to)
			}
		}
	}
	for owner := range e.decl.uses {
		if d := e.decl.decls[owner]; d != nil && d.kind == "type" {
			for to := range e.decl.uses[owner] {
				fieldTypes[owner] = append(fieldTypes[owner], to)
			}
		}
	}
	for len(queue) > 0 {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, to := range fieldTypes[id] {
			if d := e.decl.decls[to]; d != nil && d.kind == "type" && !significant[to] {
				significant[to] = true
				queue = append(queue, to)
			}
		}
	}
	return significant
}

// removableField reports whether a field may ever be planned for removal.
func (e *engine) removableField(d *declaration, significant map[string]bool) bool {
	return !d.exported && !d.tagged && !d.embedded && !significant[d.owner]
}

// liveDeclarations walks uses from the given live functions and roots. A live
// type keeps the fields that may not be removed, so their types stay too.
func (e *engine) liveDeclarations(seeds map[string]bool, removed map[string]int, significant map[string]bool) map[string]bool {
	live := map[string]bool{}
	var queue []string
	visit := func(id string) {
		if _, gone := removed[id]; !gone && !live[id] {
			live[id] = true
			queue = append(queue, id)
		}
	}
	for id := range seeds {
		visit(id)
	}
	keeps := map[string][]string{}
	for id, d := range e.decl.decls {
		if d.kind == "field" && !e.removableField(d, significant) {
			keeps[d.owner] = append(keeps[d.owner], id)
		}
	}
	for len(queue) > 0 {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for to := range e.decl.uses[id] {
			visit(to)
		}
		for _, field := range keeps[id] {
			visit(field)
		}
	}
	return live
}

// declarationStates marks each inventoried declaration reachable in the scopes
// and analyses whose functions or exported API use it.
func (e *engine) declarationStates() map[string]bool {
	significant := e.layoutSignificant()
	scopes := make([]string, 0, len(e.decl.reachable))
	for scope := range e.decl.reachable {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	for _, scope := range scopes {
		for analysis, functions := range e.decl.reachable[scope] {
			seeds := map[string]bool{}
			for id := range functions {
				seeds[id] = true
			}
			if analysis == "production" {
				for id := range e.decl.roots[scope] {
					seeds[id] = true
				}
			}
			for id := range e.liveDeclarations(seeds, nil, significant) {
				if e.decl.decls[id] == nil {
					continue
				}
				s := e.state(id, scope)
				if analysis == "production" && s.Production != "not_analyzed" {
					s.Production = "reachable"
				} else if analysis == "tests" && s.Tests != "not_analyzed" {
					s.Tests = "reachable"
				}
			}
		}
	}
	return significant
}

// liveEnumGroups lists constant blocks with at least one live member.
func (e *engine) liveEnumGroups(live func(string) bool) map[string]bool {
	groups := map[string]bool{}
	for id, d := range e.decl.decls {
		if d.kind == "const" && d.enumGroup != "" && live(id) {
			groups[d.enumGroup] = true
		}
	}
	return groups
}

// keptDeclaration reports a declaration that is never a removal finding, with
// the note to record instead, if any.
func (e *engine) keptDeclaration(id string, significant, liveGroups map[string]bool, live func(string) bool) (bool, string) {
	d := e.decl.decls[id]
	switch {
	case d == nil:
		return false, ""
	case d.kind == "field" && !live(d.owner):
		return true, "" // Removed with its type, which is reported on its own.
	case d.kind == "field" && !e.removableField(d, significant):
		return true, ""
	case d.kind == "var" && d.effects:
		return true, ""
	case d.kind == "const" && d.enumType != "" && live(d.enumType):
		return true, "Unused value of the enum " + d.enumType + ", which is in use; review it, but it is not removed by default."
	case d.kind == "const" && liveGroups[d.enumGroup]:
		return true, "Unused value of a constant block that is in use; review it, but it is not removed by default."
	}
	return false, ""
}

// neverAssigned lists eligible fields that live code reads but nothing writes:
// every read sees the zero value, so code that depends on it cannot run.
func (e *engine) neverAssigned(significant map[string]bool, live func(string) bool) map[string][]report.Location {
	out := map[string][]report.Location{}
	for id, reads := range e.decl.reads {
		d := e.decl.decls[id]
		if d == nil || e.decl.writes[id] || !e.removableField(d, significant) || !live(d.owner) {
			continue
		}
		var locations []report.Location
		for _, r := range reads {
			if live(r.from) && !containsLocation(locations, r.location) {
				locations = append(locations, r.location)
			}
		}
		if len(locations) > 0 {
			sort.Slice(locations, func(i, j int) bool {
				a, b := locations[i], locations[j]
				return a.File < b.File || a.File == b.File && a.Line < b.Line
			})
			out[id] = locations
		}
	}
	return out
}
