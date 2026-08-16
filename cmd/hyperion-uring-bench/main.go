package main

import (
	"os"

	"github.com/hunterinvariants/HYPERION-DST/internal/cli"
)

// This binary keeps its historical name and behavior; the implementation lives
// in internal/cli and is shared with the hyperion umbrella command.
func main() { os.Exit(cli.Execute("uring-bench", os.Args[1:])) }
