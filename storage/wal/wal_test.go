package wal

import (
	"context"
	"errors"
	"testing"

	"github.com/hunterinvariants/HYPERION-DST/storage"
)

func TestCrashReplayStateEquivalence(t *testing.T) {
	ctx := context.Background()
	dev := NewMemoryDevice(nil)
	log, _, err := Open(dev)
	if err != nil {
		t.Fatal(err)
	}
	durable := []storage.Entry{
		{Index: 11, Term: 3, Command: 7},
		{Index: 12, Term: 3, Command: 9},
	}
	if err := log.Append(ctx, durable); err != nil {
		t.Fatal(err)
	}
	if err := log.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(ctx, []storage.Entry{{Index: 13, Term: 3, Command: 1000}}); err != nil {
		t.Fatal(err)
	}

	rebooted, err := dev.Crash(0)
	if err != nil {
		t.Fatal(err)
	}
	_, records, err := Open(rebooted)
	if err != nil {
		t.Fatal(err)
	}
	state := uint64(5) // snapshot state at index 10
	for _, record := range records {
		state += record.Entry.Command
	}
	if state != 21 {
		t.Fatalf("replayed state = %d, want snapshot(5)+7+9 = 21", state)
	}
}

func TestEveryTornRecordCutRecoversOnlyVerifiedPrefix(t *testing.T) {
	ctx := context.Background()
	for cut := 0; cut < RecordSize; cut++ {
		dev := NewMemoryDevice(nil)
		log, _, err := Open(dev)
		if err != nil {
			t.Fatal(err)
		}
		first := storage.Entry{Index: 1, Term: 1, Command: 41}
		if err := log.Append(ctx, []storage.Entry{first}); err != nil {
			t.Fatal(err)
		}
		if err := log.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		if err := log.Append(ctx, []storage.Entry{{Index: 2, Term: 1, Command: 99}}); err != nil {
			t.Fatal(err)
		}
		rebooted, err := dev.Crash(cut)
		if err != nil {
			t.Fatal(err)
		}
		records, valid, err := Recover(rebooted.DurableBytes())
		if err != nil {
			t.Fatalf("cut %d: %v", cut, err)
		}
		if valid != RecordSize || len(records) != 1 || records[0].Entry != first {
			t.Fatalf("cut %d exposed unverified data: records=%v valid=%d", cut, records, valid)
		}
	}
}

func TestOpenTruncatesTornTailBeforeNextAppend(t *testing.T) {
	ctx := context.Background()
	dev := NewMemoryDevice(nil)
	log, _, err := Open(dev)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(ctx, []storage.Entry{{Index: 1, Term: 1, Command: 10}}); err != nil {
		t.Fatal(err)
	}
	if err := log.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(ctx, []storage.Entry{{Index: 2, Term: 1, Command: 20}}); err != nil {
		t.Fatal(err)
	}
	rebooted, err := dev.Crash(17)
	if err != nil {
		t.Fatal(err)
	}
	reopened, records, err := Open(rebooted)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(rebooted.DurableBytes()) != RecordSize {
		t.Fatalf("recovery did not truncate torn tail: records=%d bytes=%d", len(records), len(rebooted.DurableBytes()))
	}
	if err := reopened.Append(ctx, []storage.Entry{{Index: 2, Term: 2, Command: 30}}); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	_, replayed, err := Open(rebooted)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != 2 || replayed[1].Sequence != 2 || replayed[1].Entry.Command != 30 {
		t.Fatalf("append after recovery produced invalid log: %+v", replayed)
	}
}
func TestChecksumRejectsCompleteCorruption(t *testing.T) {
	dev := NewMemoryDevice(nil)
	log, _, err := Open(dev)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(context.Background(), []storage.Entry{{Index: 1, Term: 2, Command: 3}}); err != nil {
		t.Fatal(err)
	}
	if err := log.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	image := dev.DurableBytes()
	image[36] ^= 0x80
	_, _, err = Recover(image)
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("got %v, want ErrChecksum", err)
	}
}

func TestSequenceGapIsRejected(t *testing.T) {
	var a, b [RecordSize]byte
	Encode(&a, Record{Sequence: 1, Entry: storage.Entry{Index: 1}})
	Encode(&b, Record{Sequence: 3, Entry: storage.Entry{Index: 2}})
	image := append(a[:], b[:]...)
	_, _, err := Recover(image)
	if !errors.Is(err, ErrSequence) {
		t.Fatalf("got %v, want ErrSequence", err)
	}
}
