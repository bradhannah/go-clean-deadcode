package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/bradhannah/go-clean-deadcode/internal/analyze"
	"github.com/bradhannah/go-clean-deadcode/internal/config"
	"github.com/bradhannah/go-clean-deadcode/internal/contract"
	"github.com/bradhannah/go-clean-deadcode/internal/report"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "_worker" {
		os.Exit(run(args[1:], os.Stdout, os.Stderr))
	}
	os.Exit(supervise(args, os.Stdout, os.Stderr))
}

func run(args []string, out, errout io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errout, "usage: go-clean-deadcode analyze|discover|version [--root DIR] [--config FILE] [--format text|json] [--output FILE]")
		return 2
	}
	if args[0] == "version" {
		fmt.Fprintln(out, report.ToolVersion)
		return 0
	}
	if args[0] == "schema" {
		kind := "report"
		if len(args) > 1 {
			kind = args[1]
		}
		data, err := contract.Schema(kind)
		if err != nil {
			fmt.Fprintln(errout, err)
			return 2
		}
		if _, err = out.Write(data); err != nil {
			return 2
		}
		return 0
	}
	command := args[0]
	if command != "analyze" && command != "discover" {
		fmt.Fprintln(errout, "unknown command:", command)
		return 2
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(errout)
	root := fs.String("root", ".", "read-only source root")
	conf := fs.String("config", "", "external JSON configuration (required for analyze)")
	format := fs.String("format", "text", "text or json")
	output := fs.String("output", "", "write report outside the source root instead of stdout")
	only := fs.String("symbol", "", "focus text findings on an exact symbol or finding ID; JSON stays complete")
	timeout := fs.Duration("timeout", 5*time.Minute, "analysis deadline")
	cache := fs.String("cache-dir", "", "optional report cache outside the source root")
	verbose := fs.Bool("verbose", false, "phase progress on stderr")
	profile := fs.String("cpuprofile", "", "write CPU profile outside the source root")
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(errout, "format must be text or json")
		return 2
	}
	r := report.New(command)
	c := config.Default()
	var err error
	if *conf != "" {
		c, err = config.Load(*conf)
	} else if command == "analyze" {
		err = fmt.Errorf("analyze requires --config; use discover to inventory executable packages")
	}
	if fs.NArg() != 0 {
		err = fmt.Errorf("package patterns belong in the configuration; unexpected arguments: %v", fs.Args())
	}
	abs, e := filepath.Abs(*root)
	if e != nil {
		err = e
	} else {
		abs, e = filepath.EvalSymlinks(abs)
		if e != nil {
			err = e
		}
	}
	if *timeout <= 0 {
		err = fmt.Errorf("timeout must be positive")
	}
	if err != nil {
		r.Error("configuration", "", err.Error())
	} else {
		if *profile != "" {
			p, e := outsidePath(abs, *profile)
			if e != nil {
				fmt.Fprintln(errout, e)
				return 2
			}
			f, e := os.Create(p)
			if e != nil {
				fmt.Fprintln(errout, e)
				return 2
			}
			if e = pprof.StartCPUProfile(f); e != nil {
				f.Close()
				fmt.Fprintln(errout, e)
				return 2
			}
			defer f.Close()
			defer pprof.StopCPUProfile()
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		opt := analyze.Options{Root: abs, Config: c, Discover: command == "discover"}
		if *verbose {
			opt.Progress = func(s string) { fmt.Fprintln(errout, time.Now().Format("15:04:05"), s) }
		}
		if *cache != "" && command == "analyze" {
			cachePath, e := outsidePath(abs, *cache)
			if e != nil {
				r.Error("cache_path", "", e.Error())
			} else {
				r = analyze.CachedRun(ctx, opt, cachePath)
			}
		} else {
			r = analyze.Run(ctx, opt)
		}
	}
	var buf strings.Builder
	if *format == "json" {
		encoder := json.NewEncoder(&buf)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(r)
	} else {
		err = report.Text(&buf, r, *only)
	}
	if err == nil && *output != "" {
		err = writeOutside(abs, *output, []byte(buf.String()))
	} else if err == nil {
		_, err = io.WriteString(out, buf.String())
	}
	if err != nil {
		fmt.Fprintln(errout, err)
		return 2
	}
	return r.ExitCode()
}

func outsidePath(root, path string) (string, error) {
	canonical, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	canonical, err = filepath.EvalSymlinks(canonical)
	if err != nil {
		return "", err
	}
	root = canonical
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	probe := abs
	var suffix []string
	for {
		resolved, e := filepath.EvalSymlinks(probe)
		if e == nil {
			abs = filepath.Join(append([]string{resolved}, suffix...)...)
			break
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", e
		}
		suffix = append([]string{filepath.Base(probe)}, suffix...)
		probe = parent
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return "", fmt.Errorf("cache must be outside the read-only source root")
	}
	return abs, nil
}

func writeOutside(root, path string, data []byte) error {
	canonical, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	canonical, err = filepath.EvalSymlinks(canonical)
	if err != nil {
		return err
	}
	root = canonical
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return err
	}
	resolved := filepath.Join(parent, filepath.Base(abs))
	rel, err := filepath.Rel(root, resolved)
	if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return fmt.Errorf("report output must be outside the read-only source root")
	}
	if info, err := os.Lstat(resolved); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("report output cannot be a symlink")
	}
	f, err := os.CreateTemp(parent, ".go-clean-deadcode-*")
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
	return os.Rename(f.Name(), resolved)
}
