package wal

import (
	"context"
	"errors"
	"syscall"
	"testing"

	"github.com/hunterinvariants/hyperion/storage"
)

// failingDevice models kernel failures at the durability boundary. It keeps
// acknowledged stable bytes separate so every test can crash and reopen.
type failingDevice struct {
	stable    []byte
	pending   []byte
	limit     int
	appendErr error
	syncErr   error
}

func (d *failingDevice) Append(p []byte) error {
	if d.appendErr != nil {
		return d.appendErr
	}
	if d.limit >= 0 && len(d.stable)+len(d.pending)+len(p) > d.limit {
		return syscall.ENOSPC
	}
	d.pending = append(d.pending, p...)
	return nil
}

func (d *failingDevice) Sync() error {
	if d.syncErr != nil {
		return d.syncErr
	}
	d.stable = append(d.stable, d.pending...)
	d.pending = d.pending[:0]
	return nil
}

func (d *failingDevice) DurableBytes() []byte {
	return append([]byte(nil), d.stable...)
}

func (d *failingDevice) TruncateDurable(size int) error {
	if size < 0 || size > len(d.stable) {
		return ErrInvalidTear
	}
	d.stable = d.stable[:size]
	return nil
}

func TestDiskFullLeavesOnlyAcknowledgedPrefix(t *testing.T) {
	device := &failingDevice{limit: RecordSize}
	log, _, err := Open(device)
	if err != nil {
		t.Fatal(err)
	}
	first := storage.Entry{Index: 1, Term: 1, Command: 10}
	if err := log.Append(context.Background(), []storage.Entry{first}); err != nil {
		t.Fatal(err)
	}
	if err := log.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := storage.Entry{Index: 2, Term: 1, Command: 20}
	if err := log.Append(context.Background(), []storage.Entry{second}); !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("append error = %v, want ENOSPC", err)
	}
	_, records, err := Open(&failingDevice{stable: device.DurableBytes(), limit: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Entry != first {
		t.Fatalf("recovered records = %+v", records)
	}
}

func TestFailedSyncNeverMakesEntryDurable(t *testing.T) {
	device := &failingDevice{limit: -1, syncErr: syscall.EIO}
	log, _, err := Open(device)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), []storage.Entry{{
		Index: 1, Term: 1, Command: 99,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := log.Sync(context.Background()); !errors.Is(err, syscall.EIO) {
		t.Fatalf("sync error = %v, want EIO", err)
	}
	_, records, err := Open(&failingDevice{stable: device.DurableBytes(), limit: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("failed sync exposed records: %+v", records)
	}
}

func TestAppendIOErrorDoesNotAdvanceDurableSequence(t *testing.T) {
	device := &failingDevice{limit: -1, appendErr: syscall.EIO}
	log, _, err := Open(device)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), []storage.Entry{{
		Index: 1, Term: 1, Command: 7,
	}}); !errors.Is(err, syscall.EIO) {
		t.Fatalf("append error = %v, want EIO", err)
	}
	if len(device.DurableBytes()) != 0 {
		t.Fatal("append I/O error changed durable media")
	}
}
