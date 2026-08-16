package wal

import (
	"context"

	"github.com/hunterinvariants/hyperion/storage"
)

// Log serializes entries into fixed-size records. Append stages records;
// Sync establishes the durability boundary.
type Log struct {
	device  Device
	nextSeq uint64
	scratch [RecordSize]byte
}

func Open(device Device) (*Log, []Record, error) {
	records, validBytes, err := Recover(device.DurableBytes())
	if err != nil {
		return nil, nil, err
	}
	// A partial final record is an expected crash artifact. Remove it before
	// accepting new appends so a later record cannot straddle the torn tail.
	if validBytes != len(device.DurableBytes()) {
		if err := device.TruncateDurable(validBytes); err != nil {
			return nil, records, err
		}
	}
	next := uint64(1)
	if len(records) > 0 {
		next = records[len(records)-1].Sequence + 1
	}
	return &Log{device: device, nextSeq: next}, records, nil
}

func (l *Log) Append(ctx context.Context, entries []storage.Entry) error {
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		Encode(&l.scratch, Record{Sequence: l.nextSeq, Entry: entry})
		if err := l.device.Append(l.scratch[:]); err != nil {
			return err
		}
		l.nextSeq++
	}
	return nil
}

func (l *Log) Sync(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return l.device.Sync()
}

// Recover returns the longest verified record prefix and its byte length.
// An incomplete tail is ignored. Corruption of a complete record is fatal.
func Recover(image []byte) ([]Record, int, error) {
	count := len(image) / RecordSize
	records := make([]Record, 0, count)
	for i := 0; i < count; i++ {
		record, err := Decode(image[i*RecordSize : (i+1)*RecordSize])
		if err != nil {
			return nil, i * RecordSize, err
		}
		want := uint64(i + 1)
		if record.Sequence != want {
			return nil, i * RecordSize, ErrSequence
		}
		records = append(records, record)
	}
	return records, count * RecordSize, nil
}
