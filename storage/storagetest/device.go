// Package storagetest provides the conformance suite for storage backends.
//
// A durable backend is only useful if it behaves the way the write-ahead log
// assumes, and those assumptions are easy to satisfy accidentally on a happy
// path and violate under truncation or reopen. Run this suite against a new
// wal.Device implementation before trusting it with consensus state.
package storagetest

import (
	"bytes"
	"errors"
	"testing"

	"github.com/hunterinvariants/HYPERION-DST/storage/wal"
)

// NewDevice constructs a fresh, empty device. The suite calls it once per
// property, so each property starts from a clean backend.
type NewDevice func(t *testing.T) wal.Device

// RunDeviceSuite checks every property wal.Log relies on. A conforming
// implementation passes all of them.
func RunDeviceSuite(t *testing.T, factory NewDevice) {
	t.Helper()
	t.Run("empty device is empty", func(t *testing.T) {
		if got := factory(t).DurableBytes(); len(got) != 0 {
			t.Fatalf("fresh device reports %d durable bytes", len(got))
		}
	})
	t.Run("append then sync is durable", func(t *testing.T) {
		device := factory(t)
		want := []byte("hyperion durable record")
		if err := device.Append(want); err != nil {
			t.Fatalf("append: %v", err)
		}
		if err := device.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if got := device.DurableBytes(); !bytes.Equal(got, want) {
			t.Fatalf("durable bytes = %q, want %q", got, want)
		}
	})
	t.Run("appends concatenate in order", func(t *testing.T) {
		device := factory(t)
		for _, part := range [][]byte{[]byte("aaa"), []byte("bb"), []byte("cccc")} {
			if err := device.Append(part); err != nil {
				t.Fatalf("append: %v", err)
			}
		}
		if err := device.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if got, want := device.DurableBytes(), []byte("aaabbcccc"); !bytes.Equal(got, want) {
			t.Fatalf("durable bytes = %q, want %q", got, want)
		}
	})
	t.Run("sync is idempotent", func(t *testing.T) {
		device := factory(t)
		if err := device.Append([]byte("once")); err != nil {
			t.Fatalf("append: %v", err)
		}
		for i := 0; i < 3; i++ {
			if err := device.Sync(); err != nil {
				t.Fatalf("sync %d: %v", i, err)
			}
		}
		if got, want := device.DurableBytes(), []byte("once"); !bytes.Equal(got, want) {
			t.Fatalf("durable bytes = %q, want %q", got, want)
		}
	})
	t.Run("empty append is harmless", func(t *testing.T) {
		device := factory(t)
		if err := device.Append(nil); err != nil {
			t.Fatalf("append nil: %v", err)
		}
		if err := device.Append([]byte{}); err != nil {
			t.Fatalf("append empty: %v", err)
		}
		if err := device.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if got := device.DurableBytes(); len(got) != 0 {
			t.Fatalf("empty appends produced %d durable bytes", len(got))
		}
	})
	t.Run("durable bytes are a copy", func(t *testing.T) {
		// wal.Recover parses the returned slice. If it aliased device state,
		// a parser bug would silently corrupt the log.
		device := factory(t)
		if err := device.Append([]byte("original")); err != nil {
			t.Fatalf("append: %v", err)
		}
		if err := device.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		first := device.DurableBytes()
		for i := range first {
			first[i] = 'x'
		}
		if got, want := device.DurableBytes(), []byte("original"); !bytes.Equal(got, want) {
			t.Fatalf("mutating the returned slice changed the device: %q, want %q", got, want)
		}
	})
	t.Run("truncate shortens durable state", func(t *testing.T) {
		device := factory(t)
		if err := device.Append([]byte("keep|discard")); err != nil {
			t.Fatalf("append: %v", err)
		}
		if err := device.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if err := device.TruncateDurable(4); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		if got, want := device.DurableBytes(), []byte("keep"); !bytes.Equal(got, want) {
			t.Fatalf("durable bytes = %q, want %q", got, want)
		}
	})
	t.Run("append continues after truncate", func(t *testing.T) {
		// Torn-tail recovery truncates and then keeps writing. A device that
		// resumed at the pre-truncation offset would leave a hole.
		device := factory(t)
		if err := device.Append([]byte("keep|discard")); err != nil {
			t.Fatalf("append: %v", err)
		}
		if err := device.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if err := device.TruncateDurable(4); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		if err := device.Append([]byte("+more")); err != nil {
			t.Fatalf("append after truncate: %v", err)
		}
		if err := device.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if got, want := device.DurableBytes(), []byte("keep+more"); !bytes.Equal(got, want) {
			t.Fatalf("durable bytes = %q, want %q", got, want)
		}
	})
	t.Run("truncate to zero empties the device", func(t *testing.T) {
		device := factory(t)
		if err := device.Append([]byte("everything")); err != nil {
			t.Fatalf("append: %v", err)
		}
		if err := device.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if err := device.TruncateDurable(0); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		if got := device.DurableBytes(); len(got) != 0 {
			t.Fatalf("device reports %d bytes after truncating to zero", len(got))
		}
	})
	t.Run("truncate rejects out of range sizes", func(t *testing.T) {
		device := factory(t)
		if err := device.Append([]byte("12345")); err != nil {
			t.Fatalf("append: %v", err)
		}
		if err := device.Sync(); err != nil {
			t.Fatalf("sync: %v", err)
		}
		if err := device.TruncateDurable(-1); !errors.Is(err, wal.ErrInvalidTear) {
			t.Fatalf("truncate(-1) returned %v, want wal.ErrInvalidTear", err)
		}
		if err := device.TruncateDurable(6); !errors.Is(err, wal.ErrInvalidTear) {
			t.Fatalf("truncate past the end returned %v, want wal.ErrInvalidTear", err)
		}
		if got, want := device.DurableBytes(), []byte("12345"); !bytes.Equal(got, want) {
			t.Fatalf("a rejected truncation changed the device: %q, want %q", got, want)
		}
	})
}
