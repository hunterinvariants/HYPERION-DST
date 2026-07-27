package wal

import (
	"errors"
	"sync"
)

var ErrInvalidTear = errors.New("wal: invalid torn-write length")

// Device is the minimum append-and-sync surface needed by Log.
type Device interface {
	Append([]byte) error
	Sync() error
	DurableBytes() []byte
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
