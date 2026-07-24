// Command comrade is the entry point for cli-comrade, a cross-platform AI
// CLI companion for the terminal.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/firatkutay/cli-comrade/internal/cli"
)

// version is injected at build time via:
//
//	-ldflags "-X main.version=<version>"
//
// It defaults to "dev" for local, non-release builds.
var version = "dev"

// exitCoder is the shape a command's returned error can implement to
// request a specific process exit code, instead of the blanket 1 every
// OTHER command error still gets below. `comrade doctor` is this
// mechanism's first (and, as of this writing, only) user — see
// internal/cli/doctor.go's doctorFailedError — but it is deliberately a
// small, unexported structural interface here (not a doctor-specific
// type check) so any future command error can opt into a non-default
// exit code the same way, without main.go ever importing internal/cli's
// own error types.
type exitCoder interface {
	ExitCode() int
}

func main() {
	root := cli.NewRootCmd(version)
	err := root.Execute()
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(exitCodeFor(err))
}

// exitCodeFor resolves the process exit code for a non-nil error
// Execute() returned: err's own ExitCode() int if it (or anything it
// wraps) implements exitCoder — see doctorFailedError,
// internal/cli/doctor.go — otherwise the blanket 1 every other command
// error has always exited with. Extracted as its own pure function
// (rather than inlined in main, which os.Exits and so cannot itself be
// unit-tested) so main_test.go can pin both branches directly.
func exitCodeFor(err error) int {
	var ec exitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 1
}
