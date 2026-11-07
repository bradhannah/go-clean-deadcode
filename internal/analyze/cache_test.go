package analyze

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestPruneCacheRemovesEarlierDaysAndBoundsToday(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	key := func(i int) string { return fmt.Sprintf("%064x", i) }
	put := func(name string, modified time.Time) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	yesterday := now.Add(-13 * time.Hour) // 23:00 UTC on the previous day
	put(key(100)+".json", yesterday)
	put("notes.txt", yesterday)
	put(strings.Repeat("A", 64)+".json", yesterday)
	for i := 0; i < cacheKeep+2; i++ {
		put(key(i)+".json", now.Add(-time.Duration(i+1)*time.Minute))
	}
	current := key(cacheKeep + 1) // the oldest of today's entries
	if err := pruneCache(dir, current, now); err != nil {
		t.Fatal(err)
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	if exists(key(100) + ".json") {
		t.Fatal("entry from an earlier UTC day was kept")
	}
	if !exists("notes.txt") || !exists(strings.Repeat("A", 64)+".json") {
		t.Fatal("files not named like entries must be left alone")
	}
	if !exists(current + ".json") {
		t.Fatal("current entry was removed")
	}
	for i := 0; i < cacheKeep+1; i++ {
		if want := i < cacheKeep-1; exists(key(i)+".json") != want {
			t.Fatalf("entry %d kept=%v, want %v", i, !want, want)
		}
	}
}

func TestCachedRunPrunesUnreachableEntries(t *testing.T) {
	root := t.TempDir()
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/prune\n\ngo 1.27.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(cache, fmt.Sprintf("%064x.json", 1))
	if err := os.WriteFile(stale, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Modules[0].EntryPoints = []config.EntryPoint{{Package: ".", Role: "application"}}
	r := CachedRun(context.Background(), Options{Root: root, Config: c}, cache)
	if !r.Complete {
		t.Fatalf("incomplete: %+v", r.Diagnostics)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("unreachable entry from an earlier day was kept")
	}
	if _, err := os.Stat(filepath.Join(cache, r.InputDigest+".json")); err != nil {
		t.Fatalf("new entry missing: %v", err)
	}
}
