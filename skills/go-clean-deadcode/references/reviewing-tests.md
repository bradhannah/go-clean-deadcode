# Reviewing tests without confusing reachability with purpose

Read implementation and test together. Establish the input, assertion, failure
mode, setup, and contract protected before deciding on retirement.

| Evidence | Interpretation |
| --- | --- |
| Non-test function reachable only from tests | Candidate for contract review; may be an external API, fixture, or abandoned implementation. |
| Test-file helper unreachable | Check generated harness, build tags, example documentation and interface requirements before removing. |
| Same functions reached by two tests | Not evidence of redundant tests; inputs/assertions can differ. |
| Mock implementation reached | Not proof the real implementation is exercised. |
| Registered benchmark/fuzz target | Recognized by harness, not necessarily run during ordinary tests. |
| Example without expected output | May be intentionally nonexecuting documentation. |
| TestMain/setup path | Suite infrastructure, not necessarily an assertion about the reached function. |
| Possible dynamic witness | Analyzer's may-call relation; not runtime observation or exact per-test attribution. |
| Lexical test reference | A closure belongs to that registered test in source; the ownership step is not a direct-call assertion. |

For mixed-purpose tests, remove only obsolete assertions or migrate retained checks
to the current public behavior. Preserve historical regressions unless their
contract was explicitly retired. Snapshot setup, callback mocks, serialization
methods and generic constraints can impose requirements beyond runtime calls.

Recursive groups are finite graph components. Their members referencing each other
does not make them live. Group membership also does not authorize deleting the
group: incoming roots and type constraints still matter. Absence from a bounded
witness list cannot establish absence of all tests.

When investigation is inconclusive, report the precise unanswered question. An
AI's semantic judgment supplements the deterministic facts; it does not convert
uncertainty into proof. Runtime coverage, if separately available, is observational
evidence and cannot establish unusedness by itself.
