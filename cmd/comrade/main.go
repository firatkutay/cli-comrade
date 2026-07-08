// Command comrade is the entry point for cli-comrade, a cross-platform AI
// CLI companion for the terminal.
package main

import (
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

func main() {
	root := cli.NewRootCmd(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
