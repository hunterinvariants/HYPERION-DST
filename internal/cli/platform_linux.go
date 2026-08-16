//go:build linux && !amd64

package cli

// platformCommands adds the Linux-only chaos controller. The raw block-device
// benchmark additionally needs amd64 and is absent here.
func platformCommands() []Command { return []Command{chaosCommand()} }
