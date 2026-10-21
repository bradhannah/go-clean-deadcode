package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRejectAmbiguousConfiguration(t *testing.T) {
	for _, text := range []string{
		`{"schema_version":1,"unexpected":true}`,
		`{"schema_version":2}`,
		`{"schema_version":1,"modules":[],"configurations":[]}`,
		`{} {}`,
	} {
		p := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(p, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Fatalf("accepted invalid config %s", text)
		}
	}
	c := Default()
	c.Modules[0].Path = "../escape"
	if c.Validate() == nil {
		t.Fatal("module escape accepted")
	}
	c = Default()
	c.Modules[0].EntryPoints = []EntryPoint{{Package: "./cmd/a", Role: "future"}}
	if c.Validate() == nil {
		t.Fatal("unknown role accepted")
	}
	c = Default()
	c.Configurations = append(c.Configurations, c.Configurations[0])
	if c.Validate() == nil {
		t.Fatal("duplicate config accepted")
	}
	c = Default()
	c.Modules[0].APIRoots = "all"
	if c.Validate() == nil {
		t.Fatal("unknown api_roots mode accepted")
	}
	c.Modules[0].APIRoots = APIRootsExported
	if err := c.Validate(); err != nil {
		t.Fatalf("api_roots exported rejected: %v", err)
	}
	for _, entry := range []string{"test", "/abs", "./../escape", "./a/../../b"} {
		c.Modules[0].TestSupport = []string{entry}
		if c.Validate() == nil {
			t.Fatalf("test_support entry %q accepted", entry)
		}
	}
	c.Modules[0].TestSupport = []string{"./test/...", "./internal/fakes", "./internal/game/mock.go"}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid test_support entries rejected: %v", err)
	}
}

func TestTestSupportMatching(t *testing.T) {
	m := Module{TestSupport: []string{"./test/...", "./internal/fakes", "./internal/game/mock_helpers.go"}}
	for file, want := range map[string]bool{
		"test/helpers/state.go": true, "test/kit.go": true, "testing/x.go": false,
		"internal/fakes/fake.go": true, "internal/fakes/deeper/x.go": false,
		"internal/game/mock_helpers.go": true, "internal/game/state.go": false,
	} {
		if got := m.IsTestSupport(file); got != want {
			t.Errorf("IsTestSupport(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestReflectionReviewsNeedEveryField(t *testing.T) {
	valid := ReflectionReview{ID: "r", Entry: "example.com/x.F", Reason: "Sees only its own values.", ApprovedBy: "owner", ReviewAfter: "2027-01-01"}
	c := Default()
	c.ReflectionReviews = []ReflectionReview{valid}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid review rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ReflectionReview){
		"id": func(r *ReflectionReview) { r.ID = "" }, "entry": func(r *ReflectionReview) { r.Entry = "" },
		"reason": func(r *ReflectionReview) { r.Reason = " " }, "approver": func(r *ReflectionReview) { r.ApprovedBy = "" },
		"date": func(r *ReflectionReview) { r.ReviewAfter = "" },
	} {
		review := valid
		mutate(&review)
		c.ReflectionReviews = []ReflectionReview{review}
		if c.Validate() == nil {
			t.Errorf("review without %s accepted", name)
		}
	}
	c.ReflectionReviews = []ReflectionReview{valid, valid}
	if c.Validate() == nil {
		t.Error("duplicate review id accepted")
	}
}
