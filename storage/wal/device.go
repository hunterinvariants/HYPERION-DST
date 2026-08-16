package wal

import (
	"errors"
	"sync"
)

var ErrInvalidTear = errors.New("wal: invalid torn-write length")

// Device is the durable byte surface Log is built on. It is the plug point for
// an alternative storage backend: implement these four methods and Log's
// checksummed records, sequence validation, and torn-tail recovery come with
// it. Verify a new implementation with storagetest.RunDeviceSuite before
// trusting it with consensus state.
type Device interface {
	// Append adds bytes to the end of the log. It need not make them durable.
	Append([]byte) error
	// Sync is the durability boundary: after it returns nil, everything
	// appended so far must survive a crash.
	Sync() error
	// DurableBytes returns a copy of the bytes that would survive a crash.
	//
	// Implementations are not required to be side-effect free here: FileDevice
	// syncs before reading, so for it every appended byte is reported. A device
	// that models crash behavior, such as MemoryDevice, must instead report
	// only what Sync has already made durable, since that difference is the
	// whole point of the simulation. Log calls this once, in Open, when nothing
	// is pending, so the two readings agree there.
	DurableBytes() []byte
	// TruncateDurable shortens durable state to size, which is how a torn tail
	// is discarded. It must reject a negative size or one past the current end
	// with ErrInvalidTear and leave the device unchanged. A subsequent Append
	// must continue from the truncated end, not from the previous one.
	TruncateDurable(int) error
}

// MemoryDevice models volatile controller state separately from stable media.
// It is deterministic and intended for crash/replay simulation.
type MemoryDevice struct {
	mu      sync.Mutex
	durable []byte
	pending []byte
}

func NewMemoryDevice(durable []byte) *MemoryDevice {
	return &MemoryDevice{durable: append([]byte(nil), durable...)}
}

func (d *MemoryDevice) Append(p []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = append(d.pending, p...)
	return nil
}

func (d *MemoryDevice) Sync() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.durable = append(d.durable, d.pending...)
	d.pending = d.pending[:0]
	return nil
}

func (d *MemoryDevice) DurableBytes() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]byte(nil), d.durable...)
}

func (d *MemoryDevice) TruncateDurable(size int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if size < 0 || size > len(d.durable) {
		return ErrInvalidTear
	}
	d.durable = d.durable[:size]
	return nil
}

// Crash drops volatile bytes except for the first tornBytes, which models a
// prefix reaching the medium before power loss. The returned device is the
// rebooted medium; the old device must no longer be used.
func (d *MemoryDevice) Crash(tornBytes int) (*MemoryDevice, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if tornBytes < 0 || tornBytes > len(d.pending) {
		return nil, ErrInvalidTear
	}
	image := make([]byte, 0, len(d.durable)+tornBytes)
	image = append(image, d.durable...)
	image = append(image, d.pending[:tornBytes]...)
	return NewMemoryDevice(image), nil
}
