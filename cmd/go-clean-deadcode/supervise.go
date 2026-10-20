package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/bradhannah/go-clean-deadcode/internal/report"
)

// RTA and SSA construction have no cancellation API. Isolating the worker makes
// the CLI deadline effective even while those library calls are running.
func supervise(args []string, out, errout io.Writer) int {
	if len(args) == 0 || (args[0] != "analyze" && args[0] != "discover") {
		return run(args, out, errout)
	}
	value := func(name, def string) string {
		v := def
		for i := 1; i < len(args); i++ {
			s := strings.TrimLeft(args[i], "-")
			k, x, eq := strings.Cut(s, "=")
			if k != name {
				continue
			}
			if eq {
				v = x
			} else if i+1 < len(args) {
				i++
				v = args[i]
			}
		}
		return v
	}
	duration, err := time.ParseDuration(value("timeout", "5m"))
	if err != nil || duration <= 0 {
		return run(args, out, errout)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(errout, err)
		return 2
	}
	spool, err := os.CreateTemp("", "go-clean-deadcode-output-*")
	if err != nil {
		fmt.Fprintln(errout, err)
		return 2
	}
	defer os.Remove(spool.Name())
	defer spool.Close()
	cmd := exec.CommandContext(ctx, exe, append([]string{"_worker"}, args...)...)
	cmd.Stdout = spool
	cmd.Stderr = errout
	setupProcessGroup(cmd)
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	if ctx.Err() != nil {
		r := report.New(args[0])
		r.Error("cancelled", "", ctx.Err().Error())
		var b strings.Builder
		if value("format", "text") == "json" {
			enc := json.NewEncoder(&b)
			enc.SetIndent("", "  ")
			_ = enc.Encode(r)
		} else {
			_ = report.Text(&b, r, "")
		}
		if output := value("output", ""); output != "" {
			if e := writeOutside(value("root", "."), output, []byte(b.String())); e != nil {
				fmt.Fprintln(errout, e)
			}
		} else {
			_, _ = io.WriteString(out, b.String())
		}
		return 2
	}
	if _, e := spool.Seek(0, 0); e != nil {
		fmt.Fprintln(errout, e)
		return 2
	}
	if _, e := io.Copy(out, spool); e != nil {
		fmt.Fprintln(errout, e)
		return 2
	}
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	fmt.Fprintln(errout, err)
	return 2
}
