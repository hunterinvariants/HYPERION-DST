package main

import (
	"os"

	"github.com/hunterinvariants/promtact/internal/cli"
)

// This binary keeps its historical name and behavior; the implementation lives
// in internal/cli and is shared with the promtact umbrella command.
func main() { os.Exit(cli.Execute("seeds", os.Args[1:])) }
