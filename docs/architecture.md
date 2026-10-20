# Analysis architecture

## Scope and loading

The engine uses `go/packages`, typed ASTs, SSA with generic instantiation, and
Rapid Type Analysis from `golang.org/x/tools`. The implementation is original;
the official `cmd/deadcode` and `cmd/callgraph` tools provide conceptual/API
references, not subprocess dependencies.

For each configured module and GOOS/GOARCH/tag configuration:

1. Inventory `./...` executable packages independently of selected package patterns.
2. Require explicit classification of every discovered executable. Validate that
   configured executables exist and are included in the loaded analysis.
3. Load a **test-free** typed program; root selected executable `main` and `init`.
4. Separately load a **test-enabled** typed program; root generated test-executable
   `main` and `init`. The program models registered tests, benchmarks, fuzz targets,
   examples, and `TestMain`; no target code is executed.
5. Merge source declaration identities only after their reachability is recorded.

Each program uses a combined root set. In v1, multiple production executables share
one RTA, and multiple test executables share another RTA. This is conservative:
runtime types introduced by one root can broaden dynamic targets attributed to
another. Production and test type universes are **never** combined. Isolated
per-test RTA and VTA refinement are future work, not current precision guarantees.

The environment explicitly selects GOOS, GOARCH, CGO and tags. It uses
`GOFLAGS=-mod=readonly`, `GOWORK=off`, `GOTOOLCHAIN=local` and the standard package
driver. Ambient GOFLAGS and workspace/package-driver overrides are not inherited.
Use module replacements declared by the target itself. Provision dependencies
first for offline operation; ordinary Go module/build caches can be populated.
No `go generate`, target test execution, source rewriting or `go mod tidy` occurs.

## Evidence and uncertainty

RTA's reachable set is authoritative for the selected static model. It is not
reconstructed from call-graph edges: reflection may retain a function without an
ordinary explanatory path. `unexplained_retention` means no witness was obtained
within the search bounds, not proof that reflection specifically caused it.

Source references are independently collected from type-resolved AST identifiers,
including dead callers. A reference does not prove runtime reachability. Direct
SSA edges include unreachable bodies so mutually recursive dead code remains
visible. Reported graph edges connect inventoried source declarations; they are
not an exhaustive export of dependency, synthetic-wrapper or closure nodes.
Witness paths can include those intermediate functions by their qualified labels.

Production witnesses select one deterministic path per reachable declaration.
Test witnesses for test-only implementations select at most three upstream seed
paths and visit at most 2,000 nodes per search. Generated harness references
identify registered test entries. Test seeds are explanatory boundaries, not
custom replacement roots. `production_roots` and `test_roots` are **witness roots**,
not exhaustive root-ownership inventories. `witness_search_complete=false` makes
bounded or unexplained attribution explicit. Live test-file declarations and
production-live declarations do not receive additional test witnesses in v1.

`lexical_test_reference` witnesses use a closure's enclosing registered test as a
source-association boundary. The parent-to-closure step is lexical ownership, not
a proven direct call. This avoids presenting unrelated test callbacks joined by
RTA through `testing.tRunner` as the strongest explanation for a subtest body.

Interface obligations include explicit variable declarations and assignments.
The engine does not yet enumerate every return, argument conversion, embedding,
or generic constraint requirement. Lack of an obligation record never establishes
independent deletability. Recognized empty private interface marker methods remain
in the symbol inventory and structural count without creating runtime findings.
A method a runtime type needs to satisfy an interface that reachable code checks
with a type assertion or type switch is required even when nothing calls it:
removing it still compiles but silently changes which branch runs. Such methods
carry an `assertion_satisfaction` obligation, create no finding, and are counted
as `required_methods`. Reachability already keeps exported methods of runtime
types, so this matters most for unexported interface methods, such as a test
fake checked by production code.
Methods with no Go body retain an explicit native/assembly review note. Blank
functions (`func _()`) are not inventoried: nothing can call them, and they exist
only as compile-time assertions, such as `stringer`'s constant checks.

Assembly, `go:linkname`, external consumers and native callbacks are limitations.
The tool reports functions, methods, package-level types, constants and
variables, and struct fields of package-level struct types; it does not report
local declarations or unreachable branches. Test reachability cannot establish assertion coverage,
observed execution, redundancy, or product intent. Generated declarations are
included and explicitly labeled, never silently treated as live.

## Cycles

RTA computes a fixed point. Path searches use visited sets. Iterative Kosaraju
traversal identifies strongly connected components; no recursive graph walk can
loop forever. A disconnected cycle is unreachable even though its members call
one another. Groups use source/static and possible dynamic edges within one build
scope; dynamic cycles are possible call groups, not proof of runtime recursion.
v1 does not group cycles that require unmodeled dead dynamic calls or synthetic
closure edges. This does not change RTA's declaration reachability classification.

## Removal plan

The plan replays clean-up passes on the evidence already in memory, without
reloading or rebuilding. It marks every unresolved finding removed, removes tests
whose direct production references are all removed, then walks test call edges
from every surviving entry point: test-reached declarations that no other
test-reached declaration calls. Test-reached code that is not production-live,
required by an obligation, or reached again is exposed and forms the next wave.
It repeats until a wave is empty. Calls inside closures count for the enclosing
declaration, so removing a test also removes what its subtests call.

Declarations follow uses: a type, field, constant or variable is live when a
reachable function, exported API in an API package, or another live declaration
names it. A struct field uses its own type; a live type keeps the fields that are
never removal candidates, so their types stay live too. Each wave after the first
recomputes this from the surviving functions, which exposes declarations only
removed code used.

The plan does not predict compile fallout such as type switch cases or interface
assertions naming a removed type; the build reports those directly.

## Declarations

Package-level types, constants and variables, and the fields of package-level
struct types, are inventoried with what each uses. Uses are recorded after the
whole package is inventoried, so a use may name a later declaration. Field uses in
generic code resolve to the declared field. A field access is a write when it is
an assignment or increment target (directly, through indexing or a nested field),
a composite literal key, an address, a slice of an array field, a range target,
or the receiver of a pointer-receiver method call; any other access is a read.

Removing a field must not change what its struct means, so these are never
removal findings: exported, tagged, embedded and blank fields; and every field of
a layout-significant struct. A struct is layout-significant when it reaches
`encoding/binary`, `reflect` or `unsafe`, is converted to or from another struct
type, or is built with an unkeyed composite literal; a struct field type of a
significant struct is significant too. Fields of a dead type are removed with it
and not reported separately. Variables whose initializer calls a function are
kept for its effects. Blank `_` declarations are not inventoried and do not keep
what they name live: a `var _ I = (*T)(nil)` assertion on an otherwise unused
interface is removed together with the interface.

A constant block of two or more names, or a constant of a named type, is a value
set. An unused value of a set that is otherwise in use becomes a note: values can
be reserved for future use or mirror an external format. A set with no live value
is reported in full.

`never_assigned` reports an eligible field of a live struct that reachable code
reads and nothing, live or dead, writes. Its reads always see the zero value, so
the code depending on them cannot behave differently. The plan never removes such
a field: the reads need review first, and removing the field means removing the
branches that read it.

## Repeatability and failures

Symbol IDs use package, receiver base name and declaration name. Initializers add
file and initializer ordinal; generic instantiations merge into their declaration.
Line movement alone does not change ordinary finding IDs. Declaration fingerprints
hash sorted printer-normalized declarations across the analyzed variants. They
bind an exception to the declaration, not its entire transitive behavior.

Collections are deterministically ordered. Timing and cache metadata vary by run.
Report/schema versions are explicit. Unsupported configuration, package errors,
panics and interrupted analysis produce incomplete results, never acceptance.
The CLI isolates analysis in a worker to enforce its wall-clock timeout. On Unix,
the worker process group is terminated and waited for. Other hosts stop the worker;
full descendant cleanup requires an external supervisor until native job-object
support is implemented.
