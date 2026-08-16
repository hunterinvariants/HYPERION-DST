// Package cli holds one implementation per HYPERION command.
//
// Each command exists once here and is reachable two ways: through its
// historical standalone binary under cmd/, and as a subcommand of the hyperion
// umbrella command. The standalone binaries keep their names, flags, output,
// and exit codes, because the recorded qualification gates invoke them by name.
package cli

import (
	"fmt"
	"io"
	"sort"
)

// Command is one named operation.
//
// Run receives the arguments after the command name and returns the process
// exit status. Commands print their own diagnostics rather than returning an
// error, so that their output stays byte-identical to the standalone binaries
// they were extracted from.
type Command struct {
	Name    string
	Summary string
	Run     func(args []string) int
}

// Commands returns every command available on this platform, sorted by name.
func Commands() []Command {
	all := []Command{
		backupCommand(),
		ctlCommand(),
		probeCommand(),
		seedsCommand(),
		serveCommand(),
		simCommand(),
		simulateCommand(),
		verifyCommand(),
		uringBenchCommand(),
	}
	all = append(all, platformCommands()...)
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all
}

// Lookup finds a command by name.
func Lookup(name string) (Command, bool) {
	for _, command := range Commands() {
		if command.Name == name {
			return command, true
		}
	}
	return Command{}, false
}

// Execute runs the named command. It is the entry point for both the umbrella
// command and the standalone binaries.
func Execute(name string, args []string) int {
	command, ok := Lookup(name)
	if !ok {
		return 2
	}
	return command.Run(args)
}

// Usage writes the command list. Platform-restricted commands are absent
// rather than listed as unavailable, so the output describes what this build
// can actually do.
func Usage(w io.Writer, program string) {
	fmt.Fprintf(w, "usage: %s <command> [flags]\n\ncommands:\n", program)
	for _, command := range Commands() {
		fmt.Fprintf(w, "  %-12s %s\n", command.Name, command.Summary)
	}
	fmt.Fprintf(w, "\nrun \"%s <command> -h\" for the flags of one command.\n", program)
}
