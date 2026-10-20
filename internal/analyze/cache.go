package analyze

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/bradhannah/go-clean-deadcode/internal/config"
	"github.com/bradhannah/go-clean-deadcode/internal/report"
	"golang.org/x/tools/go/packages"
)

// Snapshot hashes selected dependency source and source inventory, not Git metadata.
// Package metadata is loaded afresh: a new file or changed build selection cannot
// hide behind an old dependency manifest. CGO caching is deliberately unsupported.
func Snapshot(ctx context.Context, root string, c config.Config) (string, error) {
	h := sha256.New()
	write := func(s string) { fmt.Fprintf(h, "%d:%s", len(s), s) }
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	write(string(b))
	write(report.ToolVersion)
	write(runtime.Version())
	write(time.Now().UTC().Format("2006-01-02"))
	write(root)
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	files := map[string]bool{exe: true}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		return "", err
	}
	goBinary, err = filepath.EvalSymlinks(goBinary)
	if err != nil {
		return "", err
	}
	files[goBinary] = true
	for _, v := range c.Configurations {
		if v.CGOEnabled {
			return "", fmt.Errorf("cache disabled for CGO configurations; fresh analysis required")
		}
		cmd := exec.CommandContext(ctx, "go", "env", "-json", "GOVERSION", "GOROOT", "GOEXPERIMENT", "GOTOOLDIR", "GOENV")
		cmd.Dir = root
		cmd.Env = envFor(v)
		env, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("go environment: %w", err)
		}
		write(string(env))
		for _, m := range c.Modules {
			cfg := &packages.Config{Context: ctx, Dir: filepath.Join(root, m.Path), Env: envFor(v), BuildFlags: []string{"-tags=" + strings.Join(v.Tags, ",")}, Tests: m.IncludeTests(), Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedModule | packages.NeedEmbedFiles}
			pkgs, err := packages.Load(cfg, "./...")
			if err != nil {
				return "", err
			}
			var loadErr error
			packages.Visit(pkgs, func(p *packages.Package) bool {
				if len(p.Errors) > 0 {
					loadErr = fmt.Errorf("package %s: %s", p.ID, p.Errors[0].Msg)
				}
				for _, set := range [][]string{p.GoFiles, p.CompiledGoFiles, p.OtherFiles, p.EmbedFiles, p.IgnoredFiles} {
					for _, path := range set {
						files[path] = true
					}
				}
				if p.Module != nil {
					if p.Module.GoMod != "" {
						files[p.Module.GoMod] = true
					}
					if p.Module.Replace != nil && p.Module.Replace.GoMod != "" {
						files[p.Module.Replace.GoMod] = true
					}
				}
				return true
			}, nil)
			if loadErr != nil {
				return "", loadErr
			}
		}
	}
	// Include build-excluded declarations and nested module membership in the key.
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "vendor") {
			return filepath.SkipDir
		}
		if !d.IsDir() && (strings.HasSuffix(path, ".go") || d.Name() == "go.mod" || d.Name() == "go.sum" || d.Name() == "go.work" || d.Name() == "go.work.sum") {
			files[path] = true
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return "", err
		}
		if !info.Mode().IsRegular() {
			f.Close()
			return "", fmt.Errorf("non-regular analysis input %s", path)
		}
		write(path)
		write(info.Mode().String())
		write(fmt.Sprint(info.Size()))
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type cacheRecord struct {
	Digest   string          `json:"digest"`
	Checksum string          `json:"checksum"`
	Report   json.RawMessage `json:"report"`
}

func CachedRun(ctx context.Context, opt Options, dir string) *report.Report {
	start := time.Now()
	root, err := filepath.Abs(opt.Root)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		r := report.New("analyze")
		r.Error("root", "", err.Error())
		return r
	}
	opt.Root = root
	key, err := Snapshot(ctx, root, opt.Config)
	if err != nil {
		r := Run(ctx, opt)
		r.Diagnostics = append(r.Diagnostics, report.Diagnostic{Code: "cache_unavailable", Message: err.Error(), Severity: "info"})
		return r
	}
	path := filepath.Join(dir, key+".json")
	if data, err := os.ReadFile(path); err == nil {
		var record cacheRecord
		if json.Unmarshal(data, &record) == nil && record.Digest == key && checksum(record.Report) == record.Checksum {
			var r report.Report
			if json.Unmarshal(record.Report, &r) == nil && r.Complete && r.SchemaVersion == report.SchemaVersion && r.ToolVersion == report.ToolVersion && r.InputDigest == key {
				after, err := Snapshot(ctx, root, opt.Config)
				if err == nil && after == key {
					r.Metrics = report.Metrics{CacheHit: true, DurationMS: time.Since(start).Milliseconds()}
					return &r
				}
			}
		}
	}
	r := Run(ctx, opt)
	r.InputDigest = key
	if !r.Complete {
		return r
	}
	after, err := Snapshot(ctx, root, opt.Config)
	if err != nil || after != key {
		r.Error("inputs_changed", "", "analysis inputs changed or could not be verified after analysis")
		return r
	}
	data, err := json.Marshal(r)
	if err == nil {
		record := cacheRecord{Digest: key, Checksum: checksum(data), Report: data}
		data, err = json.Marshal(record)
	}
	if err == nil {
		err = storeCache(dir, path, data)
	}
	if err != nil {
		r.Diagnostics = append(r.Diagnostics, report.Diagnostic{Code: "cache_write", Message: err.Error(), Severity: "warning"})
	}
	r.Metrics.DurationMS = time.Since(start).Milliseconds()
	return r
}

func checksum(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func storeCache(dir, path string, data []byte) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".report-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
