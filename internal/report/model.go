// Package report defines the versioned, shared text/JSON analysis model.
package report

const SchemaVersion = 1
const ToolVersion = "0.1.0-dev"

type Report struct {
	SchemaVersion int               `json:"schema_version"`
	ToolVersion   string            `json:"tool_version"`
	Mode          string            `json:"mode"`
	Complete      bool              `json:"complete"`
	InputDigest   string            `json:"input_digest,omitempty"`
	Scopes        []Scope           `json:"scopes"`
	Symbols       []Symbol          `json:"symbols"`
	Edges         []Edge            `json:"edges"`
	References    []Reference       `json:"references"`
	Groups        []Group           `json:"groups"`
	Findings      []Finding         `json:"findings"`
	Exceptions    []ExceptionResult `json:"exceptions"`
	// ReflectionReviews are the configured reflection reviews and their status.
	ReflectionReviews []ReflectionReviewResult `json:"reflection_reviews"`
	Diagnostics       []Diagnostic             `json:"diagnostics"`
	Summary           Summary                  `json:"summary"`
	Plan              Plan                     `json:"plan"`
	Notes             []Note                   `json:"notes"`
	Metrics           Metrics                  `json:"metrics"`
	Limitations       []string                 `json:"limitations"`
}

type Scope struct {
	ID            string       `json:"id"`
	Module        string       `json:"module"`
	Patterns      []string     `json:"patterns"`
	Configuration string       `json:"configuration"`
	GOOS          string       `json:"goos"`
	GOARCH        string       `json:"goarch"`
	Tags          []string     `json:"tags"`
	CGOEnabled    bool         `json:"cgo_enabled"`
	Complete      bool         `json:"complete"`
	Packages      int          `json:"packages"`
	EntryPoints   []EntryPoint `json:"entry_points"`
	APIRoots      string       `json:"api_roots,omitempty"`
	TestSupport   []string     `json:"test_support,omitempty"`
	// ReflectiveLookups lists reachable reflective method lookups.
	ReflectiveLookups []ReflectiveLookup `json:"reflective_lookups"`
	Tests             bool               `json:"tests"`
	NestedModules     []string           `json:"nested_modules"`
}

type EntryPoint struct {
	Package    string `json:"package"`
	Directory  string `json:"directory"`
	Role       string `json:"role"`
	Classified bool   `json:"classified"`
}

type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Symbol struct {
	ID             string       `json:"id"`
	Package        string       `json:"package"`
	Name           string       `json:"name"`
	Receiver       string       `json:"receiver,omitempty"`
	Kind           string       `json:"kind"`
	TestFile       bool         `json:"test_file"`
	TestSupport    bool         `json:"test_support,omitempty"`
	Lines          int          `json:"lines"`
	Generated      bool         `json:"generated"`
	Fingerprint    string       `json:"fingerprint"`
	RegisteredTest bool         `json:"registered_test"`
	Locations      []Location   `json:"locations"`
	States         []State      `json:"states"`
	Obligations    []Obligation `json:"obligations"`
}

type State struct {
	Scope              string    `json:"scope"`
	Production         string    `json:"production"`
	Tests              string    `json:"tests"`
	ProductionRoots    []string  `json:"production_roots"`
	TestRoots          []string  `json:"test_roots"`
	Witnesses          []Witness `json:"witnesses"`
	ProvenanceComplete bool      `json:"witness_search_complete"`
	// ReflectionReviewed marks a state made unreachable because every
	// reflective method lookup in the analysis is reviewed.
	ReflectionReviewed bool `json:"reflection_reviewed,omitempty"`
}

// ReflectiveLookup is a reachable call to reflect's method lookup by index or
// name. Via calls the lookup; Entry is the first function outside the standard
// library on a call path to Via, which is what a review considers.
type ReflectiveLookup struct {
	Analysis string `json:"analysis"`
	Entry    string `json:"entry"`
	Via      string `json:"via"`
}

// ReflectionReviewResult is a configured reflection review and whether it
// matched a lookup entry.
type ReflectionReviewResult struct {
	ID          string `json:"id"`
	Entry       string `json:"entry"`
	Reason      string `json:"reason"`
	ApprovedBy  string `json:"approved_by"`
	ReviewAfter string `json:"review_after"`
	Status      string `json:"status"`
}

type Witness struct {
	Root    string   `json:"root"`
	Context string   `json:"context"`
	Kind    string   `json:"kind"`
	Path    []string `json:"path"`
}

type Edge struct {
	Scope    string   `json:"scope"`
	Context  string   `json:"context"`
	Caller   string   `json:"caller"`
	Callee   string   `json:"callee"`
	Kind     string   `json:"kind"`
	Location Location `json:"location"`
}

type Reference struct {
	Scope    string   `json:"scope"`
	Symbol   string   `json:"symbol"`
	From     string   `json:"from,omitempty"`
	Kind     string   `json:"kind"`
	Location Location `json:"location"`
}

type Obligation struct {
	Scope       string   `json:"scope"`
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Location    Location `json:"location"`
}

type Group struct {
	ID        string   `json:"id"`
	Scope     string   `json:"scope"`
	Members   []string `json:"members"`
	Recursive bool     `json:"recursive"`
}

// Plan simulates removing every unresolved finding, then whatever that removal
// exposes, until nothing new becomes unused. It never edits source; the waves
// describe what repeated clean-up passes would each find.
type Plan struct {
	Waves        []Wave `json:"waves"`
	Functions    int    `json:"functions"`
	Declarations int    `json:"declarations"`
	Tests        int    `json:"tests"`
	Lines        int    `json:"lines"`
}

type Wave struct {
	Number    int       `json:"number"`
	Removals  []Removal `json:"removals"`
	Functions int       `json:"functions"`
	// Declarations counts types, fields, constants and variables.
	Declarations int `json:"declarations"`
	TestHelpers  int `json:"test_helpers"`
	Tests        int `json:"tests"`
	Lines        int `json:"lines"`
	// Tests that stay but directly reference something this wave removes; each
	// needs an edit, and may be a live-behaviour test using it as a fixture.
	TestsNeedingEdits []string `json:"tests_needing_edits"`
}

// Removal is one planned declaration removal. Reason is the finding category,
// "dedicated_test" for a test whose only production references are removed, or
// "exposed" for code that only removed code reached.
type Removal struct {
	Symbol        string   `json:"symbol"`
	Reason        string   `json:"reason"`
	Lines         int      `json:"lines"`
	ReferencedBy  []string `json:"referenced_by,omitempty"`
	OtherPackages bool     `json:"other_packages,omitempty"`
}

type Finding struct {
	ID        string   `json:"id"`
	Category  string   `json:"category"`
	Symbol    string   `json:"symbol"`
	Scopes    []string `json:"scopes"`
	Status    string   `json:"status"`
	Exception string   `json:"exception,omitempty"`
	// Reads lists where live code reads a never_assigned field.
	Reads []Location `json:"reads,omitempty"`
}

// Note is review information that is never a finding, such as an unused value
// of an enum that is otherwise in use.
type Note struct {
	Symbol  string `json:"symbol"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type ExceptionResult struct {
	ID          string `json:"id"`
	Symbol      string `json:"symbol"`
	Category    string `json:"category"`
	Reason      string `json:"reason"`
	ApprovedBy  string `json:"approved_by"`
	ReviewAfter string `json:"review_after"`
	Status      string `json:"status"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Scope    string `json:"scope,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type Summary struct {
	Symbols           int `json:"symbols"`
	Unreachable       int `json:"unreachable"`
	TestOnly          int `json:"test_only"`
	UnusedTestHelpers int `json:"unused_test_helpers"`
	Unresolved        int `json:"unresolved"`
	Accepted          int `json:"accepted"`
	StaleExceptions   int `json:"stale_exceptions"`
	StructuralMethods int `json:"structural_methods"`
	RequiredMethods   int `json:"required_methods"`
	NeverAssigned     int `json:"never_assigned"`
	Notes             int `json:"notes"`
}

type Metrics struct {
	DurationMS     int64 `json:"duration_ms"`
	LoadMS         int64 `json:"load_ms"`
	SSAMS          int64 `json:"ssa_ms"`
	ReachabilityMS int64 `json:"reachability_ms"`
	CacheHit       bool  `json:"cache_hit"`
}

func New(mode string) *Report {
	return &Report{SchemaVersion: SchemaVersion, ToolVersion: ToolVersion, Mode: mode,
		Complete: true, Scopes: []Scope{}, Symbols: []Symbol{}, Edges: []Edge{},
		References: []Reference{}, Groups: []Group{}, Findings: []Finding{},
		Exceptions: []ExceptionResult{}, ReflectionReviews: []ReflectionReviewResult{}, Diagnostics: []Diagnostic{}, Plan: Plan{Waves: []Wave{}}, Notes: []Note{},
		Limitations: []string{
			"Reachability is a conservative static approximation, not evidence of execution or assertion coverage.",
			"Test witnesses are may-reach paths in a combined test-executable analysis; dynamic targets can be shared across tests and packages.",
			"External consumers, assembly, go:linkname and native callbacks require explicit review.",
			"Only configured build variants are analyzed; uncalled methods may still be required by interfaces or generic constraints.",
			"Findings concern source function/method declarations, not unused variables, fields, or unreachable branches.",
		}}
}

func (r *Report) Error(code, scope, message string) {
	r.Complete = false
	r.Diagnostics = append(r.Diagnostics, Diagnostic{Code: code, Scope: scope, Message: message, Severity: "error"})
}

func (r *Report) ExitCode() int {
	if !r.Complete {
		return 2
	}
	if r.Summary.Unresolved > 0 || r.Summary.StaleExceptions > 0 {
		return 1
	}
	return 0
}
