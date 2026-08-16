//go:build linux

package main

import (
	"os"

	"github.com/hunterinvariants/hyperion/internal/cli"
)

// This binary keeps its historical name and behavior; the implementation lives
// in internal/cli and is shared with the hyperion umbrella command.
func main() { os.Exit(cli.Execute("chaos", os.Args[1:])) }
