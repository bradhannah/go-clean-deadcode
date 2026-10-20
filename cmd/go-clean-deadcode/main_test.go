package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bradhannah/go-clean-deadcode/internal/config"
	"github.com/bradhannah/go-clean-deadcode/internal/report"
)

func TestJSONErrorsAndReadOnlyOutput(t *testing.T) {
	var out, errout bytes.Buffer
	code := run([]string{"analyze", "--format", "json"}, &out, &errout)
	var r report.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("invalid failure JSON: %v\n%s", err, out.String())
	}
	if code != 2 || r.Complete {
		t.Fatal("missing configuration must be incomplete")
	}
	root := t.TempDir()
	if err := writeOutside(root, filepath.Join(root, "report.json"), []byte("x")); err == nil {
		t.Fatal("must not write into source root")
	}
	if _, err := outsidePath(root, filepath.Join(root, "cache")); err == nil {
		t.Fatal("cache inside source root")
	}
	other := t.TempDir()
	link := filepath.Join(other, "linked")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := outsidePath(root, filepath.Join(link, "new-cache")); err == nil {
		t.Fatal("symlink must not bypass read-only root")
	}
}

func TestCLIProducesReportAndPreservesFixture(t *testing.T) {
	root, err := filepath.Abs("../../testdata/sample")
	if err != nil {
		t.Fatal(err)
	}
	before := fixtureDigest(t, root)
	c := config.Default()
	c.Modules[0].EntryPoints = []config.EntryPoint{{Package: "./cmd/app", Role: "application"}, {Package: "./cmd/tool", Role: "utility"}}
	data, _ := json.Marshal(c)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	code := run([]string{"analyze", "--root", root, "--config", path, "--format", "json"}, &out, &errout)
	var r report.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if code != 1 || !r.Complete || r.Summary.TestOnly == 0 {
		t.Fatalf("code=%d report=%+v stderr=%s", code, r.Summary, errout.String())
	}
	if after := fixtureDigest(t, root); before != after {
		t.Fatal("analyzer changed source fixture")
	}
}

func fixtureDigest(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprint(h, path)
		_, err = h.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestProcessDeadlineProducesOneIncompleteJSONReport(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess integration")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "gcd")
	build := exec.Command("go", "build", "-o", binary, ".")
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, data)
	}
	root, err := filepath.Abs("../../testdata/sample")
	if err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Modules[0].EntryPoints = []config.EntryPoint{{Package: "./cmd/app", Role: "application"}, {Package: "./cmd/tool", Role: "utility"}}
	b, _ := json.Marshal(c)
	conf := filepath.Join(dir, "config.json")
	if err := os.WriteFile(conf, b, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "analyze", "--root", root, "--config", conf, "--format", "json", "--timeout", "1ms")
	data, err := cmd.Output()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 2 {
		t.Fatalf("deadline exit: %v", err)
	}
	var r report.Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("deadline JSON: %v: %s", err, data)
	}
	if r.Complete || len(r.Diagnostics) == 0 || r.Diagnostics[0].Code != "cancelled" {
		t.Fatalf("deadline reported acceptance: %+v", r.Diagnostics)
	}
}
