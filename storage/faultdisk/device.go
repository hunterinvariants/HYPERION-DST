// Package faultdisk models deterministic post-ack storage corruption.
package faultdisk

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"

	"github.com/hunterinvariants/promtact/storage/wal"
)

const OneMillion = 1_000_000

type Config struct {
	Seed                  int64
	BitFlipPerMillion     int
	MisdirectPerMillion   int
	PhantomReadPerMillion int
}

type FaultKind string

const (
	FaultBitFlip   FaultKind = "bit_flip"
	FaultMisdirect FaultKind = "misdirected_write"
	FaultPhantom   FaultKind = "phantom_read"
)

type Fault struct {
	Kind   FaultKind
	Source int
	Target int
	Offset int
}

// Device implements wal.Device. Faults are injected only by Crash, making the
// fault point explicit and reproducible in a deterministic simulator trace.
type Device struct {
	mu       sync.Mutex
	config   Config
	rng      *rand.Rand
	durable  []byte
	previous []byte
	pending  []byte
	faults   []Fault
}

func New(config Config) (*Device, error) {
	for _, rate := range []int{
		config.BitFlipPerMillion,
		config.MisdirectPerMillion,
		config.PhantomReadPerMillion,
	} {
		if rate < 0 || rate > OneMillion {
			return nil, errors.New("faultdisk: probability outside 0..1000000")
		}
	}
	return &Device{config: config, rng: rand.New(rand.NewSource(config.Seed))}, nil
}

func (d *Device) Append(data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = append(d.pending, data...)
	return nil
}

func (d *Device) Sync() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.previous = append(d.previous[:0], d.durable...)
	d.durable = append(d.durable, d.pending...)
	d.pending = d.pending[:0]
	return nil
}

func (d *Device) DurableBytes() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]byte(nil), d.durable...)
}

func (d *Device) TruncateDurable(size int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if size < 0 || size > len(d.durable) {
		return errors.New("faultdisk: invalid truncate")
	}
	d.durable = d.durable[:size]
	return nil
}

func (d *Device) Faults() []Fault {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Fault(nil), d.faults...)
}

// Crash returns a rebooted device containing the deterministic faulty image.
func (d *Device) Crash() (*Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	image := append([]byte(nil), d.durable...)
	faults := make([]Fault, 0, 3)

	if d.hit(d.config.BitFlipPerMillion) && len(image) != 0 {
		offset := d.rng.Intn(len(image))
		image[offset] ^= 1 << uint(d.rng.Intn(8))
		faults = append(faults, Fault{Kind: FaultBitFlip, Offset: offset})
	}
	records := len(image) / wal.RecordSize
	if d.hit(d.config.MisdirectPerMillion) && records >= 2 {
		source := d.rng.Intn(records)
		target := d.rng.Intn(records - 1)
		if target >= source {
			target++
		}
		copy(image[target*wal.RecordSize:(target+1)*wal.RecordSize],
			image[source*wal.RecordSize:(source+1)*wal.RecordSize])
		faults = append(faults, Fault{
			Kind: FaultMisdirect, Source: source, Target: target,
		})
	}
	if d.hit(d.config.PhantomReadPerMillion) && d.previous != nil {
		image = append(image[:0], d.previous...)
		faults = append(faults, Fault{Kind: FaultPhantom})
	}

	rebooted, err := New(d.config)
	if err != nil {
		return nil, err
	}
	rebooted.durable = image
	rebooted.faults = faults
	return rebooted, nil
}

func (d *Device) hit(rate int) bool {
	return rate == OneMillion || (rate != 0 && d.rng.Intn(OneMillion) < rate)
}

func (f Fault) String() string {
	return fmt.Sprintf("%s(source=%d,target=%d,offset=%d)", f.Kind, f.Source, f.Target, f.Offset)
}
