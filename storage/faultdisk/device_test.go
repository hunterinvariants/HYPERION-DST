package faultdisk

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/hunterinvariants/hyperion/storage"
	"github.com/hunterinvariants/hyperion/storage/wal"
)

func durableLog(t *testing.T, config Config) *Device {
	t.Helper()
	device, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	log, _, err := wal.Open(device)
	if err != nil {
		t.Fatal(err)
	}
	entries := []storage.Entry{
		{Index: 1, Term: 1, Command: 10},
		{Index: 2, Term: 1, Command: 20},
		{Index: 3, Term: 2, Command: 30},
	}
	for _, entry := range entries {
		if err := log.Append(context.Background(), []storage.Entry{entry}); err != nil {
			t.Fatal(err)
		}
		if err := log.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	return device
}

func TestBitRotIsDetectedBeforeReplay(t *testing.T) {
	device := durableLog(t, Config{Seed: 1, BitFlipPerMillion: OneMillion})
	rebooted, err := device.Crash()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := wal.Recover(rebooted.DurableBytes()); err == nil {
		t.Fatal("bit-rot image passed WAL validation")
	}
}

func TestMisdirectedWriteIsDetectedBySequence(t *testing.T) {
	device := durableLog(t, Config{Seed: 2, MisdirectPerMillion: OneMillion})
	rebooted, err := device.Crash()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = wal.Recover(rebooted.DurableBytes())
	if !errors.Is(err, wal.ErrSequence) {
		t.Fatalf("misdirected write error = %v, want ErrSequence", err)
	}
}

func TestPhantomPrefixNeedsQuorumEvidence(t *testing.T) {
	device := durableLog(t, Config{Seed: 3, PhantomReadPerMillion: OneMillion})
	rebooted, err := device.Crash()
	if err != nil {
		t.Fatal(err)
	}
	records, _, err := wal.Recover(rebooted.DurableBytes())
	if err != nil {
		t.Fatalf("stale but internally valid prefix should be locally indistinguishable: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("phantom read exposed %d records, want previous durable prefix of 2", len(records))
	}
}

func TestFaultTraceIsSeedReproducible(t *testing.T) {
	run := func() ([]byte, []Fault) {
		device := durableLog(t, Config{
			Seed: 0xBADC0DE, BitFlipPerMillion: 500_000,
			MisdirectPerMillion: 500_000, PhantomReadPerMillion: 500_000,
		})
		rebooted, err := device.Crash()
		if err != nil {
			t.Fatal(err)
		}
		return rebooted.DurableBytes(), rebooted.Faults()
	}
	imageA, faultsA := run()
	imageB, faultsB := run()
	if !reflect.DeepEqual(imageA, imageB) || !reflect.DeepEqual(faultsA, faultsB) {
		t.Fatalf("same seed produced different storage faults")
	}
}
