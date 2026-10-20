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
}
