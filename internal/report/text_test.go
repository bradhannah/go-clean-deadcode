package report

import (
	"bytes"
	"strings"
	"testing"
)

func TestTextRetainsEvidenceAndLimits(t *testing.T) {
	r := New("analyze")
	r.Symbols = []Symbol{{
		ID:        "p.Legacy",
		Locations: []Location{{File: "legacy.go", Line: 12}},
		States: []State{{
			Scope: "host", Production: "unreachable", Tests: "reachable",
			Witnesses: []Witness{{Root: "p.TestLegacy", Context: "tests", Kind: "static", Path: []string{"p.TestLegacy", "p.Legacy"}}},
		}},
	}}
	r.Findings = []Finding{{ID: "F1", Symbol: "p.Legacy", Category: "test_only", Status: "unresolved"}}
	r.Summary.TestOnly, r.Summary.Unresolved = 1, 1
	r.References = []Reference{{Symbol: "p.Legacy", From: "p.TestLegacy", Kind: "call", Location: Location{File: "legacy_test.go", Line: 20}}}
	var b bytes.Buffer
	if err := Text(&b, r, ""); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TEST_ONLY", "legacy.go:12", "legacy_test.go:20", "p.TestLegacy -> p.Legacy", "selected-executables=unreachable tests=reachable", "not evidence of execution"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
}
