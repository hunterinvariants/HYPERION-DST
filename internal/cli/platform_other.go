//go:build !linux

package cli

// platformCommands reports the commands that require kernel facilities this
// build cannot reach. Absent rather than present-and-failing: the umbrella
// command should describe what this binary can actually do.
func platformCommands() []Command { return nil }
