//go:build linux && amd64

package cli

// platformCommands adds every command that needs Linux kernel facilities.
func platformCommands() []Command { return []Command{chaosCommand(), rawBenchCommand()} }
