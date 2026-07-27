// Package storage defines the durability boundary used by the consensus core.
package storage

import "context"

// Entry is the stable representation of one replicated command.
type Entry struct {
	Index   uint64
	Term    uint64
	Command uint64
}

// StableStore implementations must make Append durable before returning nil.
// A Linux io_uring implementation can register buffers behind this interface.
type StableStore interface {
	Append(context.Context, []Entry) error
	Sync(context.Context) error
}
