// Package config loads strict, explicit analysis scope and retention policy.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type Config struct {
	SchemaVersion   int             `json:"schema_version"`
	Modules         []Module        `json:"modules"`
	Configurations  []Configuration `json:"configurations"`
	ExcludedModules []Exclusion     `json:"excluded_modules,omitempty"`
	Exceptions      []Exception     `json:"exceptions,omitempty"`
}

type Module struct {
	Path         string       `json:"path"`
	Patterns     []string     `json:"patterns"`
	EntryPoints  []EntryPoint `json:"entry_points"`
	Tests        *bool        `json:"tests,omitempty"`
	Library      bool         `json:"library,omitempty"`
	APIRoots     string       `json:"api_roots,omitempty"`
	TestSupport  []string     `json:"test_support,omitempty"`
	IgnoredMains []Exclusion  `json:"ignored_mains,omitempty"`
}

// APIRootsExported treats every exported function and method outside internal
// packages as callable by importers that this analysis cannot see.
const APIRootsExported = "exported"

type EntryPoint struct {
	Package        string   `json:"package"`
	Role           string   `json:"role"`
	Configurations []string `json:"configurations,omitempty"`
}

type Configuration struct {
	Name       string   `json:"name"`
	GOOS       string   `json:"goos"`
	GOARCH     string   `json:"goarch"`
	Tags       []string `json:"tags"`
	CGOEnabled bool     `json:"cgo_enabled"`
}

type Exclusion struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type Exception struct {
	ID          string `json:"id"`
	Symbol      string `json:"symbol"`
	Category    string `json:"category"`
	Reason      string `json:"reason"`
	ApprovedBy  string `json:"approved_by"`
	ReviewAfter string `json:"review_after"`
	Fingerprint string `json:"fingerprint"`
}

func Default() Config {
	return Config{SchemaVersion: 1,
		Modules:        []Module{{Path: ".", Patterns: []string{"./..."}, EntryPoints: []EntryPoint{}}},
		Configurations: []Configuration{{Name: "host", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Tags: []string{}}}}
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 4<<20))
	d.DisallowUnknownFields()
	var c Config
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("configuration: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("configuration must contain one JSON object")
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if c.SchemaVersion != 1 {
		return fmt.Errorf("unsupported configuration schema %d", c.SchemaVersion)
	}
	if len(c.Modules) == 0 || len(c.Configurations) == 0 {
		return fmt.Errorf("modules and configurations must be nonempty")
	}
	names := map[string]bool{}
	for _, v := range c.Configurations {
		if v.Name == "" || names[v.Name] || v.GOOS == "" || v.GOARCH == "" {
			return fmt.Errorf("configuration names must be unique and GOOS/GOARCH explicit")
		}
		names[v.Name] = true
	}
	paths := map[string]bool{}
	for _, m := range c.Modules {
		if !relative(m.Path) || paths[filepath.Clean(m.Path)] {
			return fmt.Errorf("invalid or duplicate module path %q", m.Path)
		}
		paths[filepath.Clean(m.Path)] = true
		if len(m.Patterns) == 0 {
			return fmt.Errorf("module %s requires package patterns", m.Path)
		}
		for _, p := range m.Patterns {
			if p != "." && !strings.HasPrefix(p, "./") || strings.Contains(p, ".."+string(filepath.Separator)) {
				return fmt.Errorf("package patterns must be local: %q", p)
			}
		}
		seen := map[string]bool{}
		for _, e := range m.EntryPoints {
			if e.Package == "" || seen[e.Package] || (e.Role != "application" && e.Role != "utility") {
				return fmt.Errorf("invalid or duplicate entry point %q", e.Package)
			}
			seen[e.Package] = true
			for _, n := range e.Configurations {
				if !names[n] {
					return fmt.Errorf("unknown entry-point configuration %q", n)
				}
			}
		}
		for _, x := range m.IgnoredMains {
			if x.Path == "" || strings.TrimSpace(x.Reason) == "" {
				return fmt.Errorf("ignored mains require path and reason")
			}
		}
		for _, entry := range m.TestSupport {
			if !strings.HasPrefix(entry, "./") || slices.Contains(strings.Split(entry, "/"), "..") {
				return fmt.Errorf("module %s: test_support entries must be ./ module-relative: %q", m.Path, entry)
			}
		}
		if m.APIRoots != "" && m.APIRoots != APIRootsExported {
			return fmt.Errorf("module %s: api_roots must be %q when set", m.Path, APIRootsExported)
		}
	}
	for _, x := range c.ExcludedModules {
		if !relative(x.Path) || strings.TrimSpace(x.Reason) == "" {
			return fmt.Errorf("excluded modules require relative path and reason")
		}
	}
	seen := map[string]bool{}
	for _, x := range c.Exceptions {
		if x.ID == "" || seen[x.ID] || x.Symbol == "" || strings.TrimSpace(x.Reason) == "" || strings.TrimSpace(x.ApprovedBy) == "" || x.ReviewAfter == "" || len(x.Fingerprint) != 64 {
			return fmt.Errorf("exceptions require unique id, exact symbol, reason, approved_by, review_after and 64-character fingerprint")
		}
		seen[x.ID] = true
		if x.Category != "unreachable" && x.Category != "test_only" && x.Category != "unused_test_helper" {
			return fmt.Errorf("invalid exception category %q", x.Category)
		}
	}
	return nil
}

func relative(p string) bool {
	return p != "" && !filepath.IsAbs(p) && filepath.Clean(p) != ".." && !strings.HasPrefix(filepath.Clean(p), ".."+string(filepath.Separator))
}
func (m Module) IncludeTests() bool { return m.Tests == nil || *m.Tests }

// IsTestSupport reports whether a module-relative file belongs to declared test
// support: an entry ending in .go names a file, one ending in /... a directory
// tree, and any other entry one package directory.
func (m Module) IsTestSupport(file string) bool {
	dir := "./" + filepath.ToSlash(filepath.Dir(file))
	file = "./" + filepath.ToSlash(file)
	for _, entry := range m.TestSupport {
		switch {
		case strings.HasSuffix(entry, ".go"):
			if file == entry {
				return true
			}
		case strings.HasSuffix(entry, "/..."):
			base := strings.TrimSuffix(entry, "/...")
			if dir == base || strings.HasPrefix(dir, base+"/") {
				return true
			}
		case dir == strings.TrimSuffix(entry, "/"):
			return true
		}
	}
	return false
}
func (e EntryPoint) Enabled(name string) bool {
	if len(e.Configurations) == 0 {
		return true
	}
	for _, n := range e.Configurations {
		if n == name {
			return true
		}
	}
	return false
}
