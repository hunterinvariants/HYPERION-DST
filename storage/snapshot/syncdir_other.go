//go:build !unix

package snapshot

// Rename is atomic on supported non-Unix development hosts. Production Linux
// builds additionally fsync the containing directory.
func syncDirectory(string) error { return nil }
