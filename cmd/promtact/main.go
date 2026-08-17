// Command promtact is the umbrella entry point for every Promtact command.
//
// Each subcommand is the same implementation the historical standalone binary
// runs, so "promtact seeds -from 1 -to 1000" and "promtact-seeds -from 1 -to
// 1000" do the same work with the same flags and the same exit status.
package main

import (
	"fmt"
	"os"

	"github.com/hunterinvariants/promtact/internal/cli"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		cli.Usage(os.Stderr, "promtact")
		os.Exit(2)
	}
	switch args[0] {
	case "help", "-h", "-help", "--help":
		cli.Usage(os.Stdout, "promtact")
		os.Exit(0)
	}
	command, ok := cli.Lookup(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "promtact: unknown command %q\n\n", args[0])
		cli.Usage(os.Stderr, "promtact")
		os.Exit(2)
	}
	os.Exit(command.Run(args[1:]))
}
