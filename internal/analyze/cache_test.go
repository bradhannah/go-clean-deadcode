package analyze

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/go-clean-deadcode/internal/config"
)

func TestCacheInvalidatesAdditionsEditsAndPolicy(t *testing.T) {
	root := t.TempDir()
	cache := t.TempDir()
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/cache\n\ngo 1.27.0\n")
	write("main.go", "package main\nfunc main() {}\nfunc dead() {}\n")
	c := config.Default()
	c.Modules[0].EntryPoints = []config.EntryPoint{{Package: ".", Role: "application"}}
	opt := Options{Root: root, Config: c}
	a := CachedRun(context.Background(), opt, cache)
	if !a.Complete || a.Metrics.CacheHit {
		t.Fatalf("cold: %+v", a.Diagnostics)
	}
	b := CachedRun(context.Background(), opt, cache)
	if !b.Metrics.CacheHit || b.InputDigest != a.InputDigest {
		t.Fatal("identical input must reuse report")
	}
	write("main.go", "package main\nfunc main() { dead() }\nfunc dead() {}\n")
	d := CachedRun(context.Background(), opt, cache)
	if d.Metrics.CacheHit || d.Summary.Unresolved != 0 {
		t.Fatalf("changed source accepted stale result: %+v", d.Summary)
	}
	write("extra.go", "package main\nfunc anotherDead() {}\n")
	e := CachedRun(context.Background(), opt, cache)
	if e.Metrics.CacheHit || e.Summary.Unresolved != 1 {
		t.Fatalf("added source not detected: %+v", e.Summary)
	}
	opt.Config.Configurations[0].Tags = []string{"newtag"}
	f := CachedRun(context.Background(), opt, cache)
	if f.Metrics.CacheHit {
		t.Fatal("policy/build change must invalidate")
	}
	if err := os.WriteFile(filepath.Join(cache, f.InputDigest+".json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	g := CachedRun(context.Background(), opt, cache)
	if g.Metrics.CacheHit || !g.Complete {
		t.Fatal("corrupt cache should cause fresh analysis")
	}
}

func TestConfigurationVariants(t *testing.T) {
	root, c := fixture(t)
	c.Configurations = []config.Configuration{{Name: "linux", GOOS: "linux", GOARCH: "amd64"}, {Name: "windows", GOOS: "windows", GOARCH: "amd64"}}
	r := Run(context.Background(), Options{Root: root, Config: c})
	if !r.Complete {
		t.Fatalf("variants incomplete: %+v", r.Diagnostics)
	}
	for _, s := range r.Symbols {
		if s.Name == "PlatformOnly" {
			if len(s.States) != 1 || s.States[0].Scope != ".@windows" {
				t.Fatalf("excluded variant classified: %+v", s.States)
			}
			return
		}
	}
	t.Fatal("Windows declaration missing")
}
