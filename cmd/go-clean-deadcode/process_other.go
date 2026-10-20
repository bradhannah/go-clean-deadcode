//go:build !unix

package main

import "os/exec"

// CommandContext stops the worker on these hosts. Full descendant cleanup needs
// the host's external supervisor (for example a Windows job object).
func setupProcessGroup(cmd *exec.Cmd) {}
