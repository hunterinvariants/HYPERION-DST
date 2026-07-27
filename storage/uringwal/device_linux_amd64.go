//go:build linux && amd64

// Package uringwal adapts the registered io_uring data path to WAL's Device.
package uringwal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/hunterinvariants/HYPERION-DST/storage/uring"
	"github.com/hunterinvariants/HYPERION-DST/storage/wal"
)

const BlockSize = uring.DefaultAlignment

type Device struct {
	mu      sync.Mutex
	path    string
	writer  *uring.DurableWriter
	durable []byte
	pending []byte
}

func Open(path string, depth uint32) (*Device, error) {
	if path == "" {
		return nil, errors.New("uringwal: path is required")
	}
	durable, err := readRecords(path)
	if err != nil {
		return nil, err
	}
	writer, err := uring.OpenDurableWriter(path, depth, BlockSize)
	if err != nil {
		return nil, err
	}
	return &Device{path: path, writer: writer, durable: durable}, nil
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
	if len(d.pending)%wal.RecordSize != 0 {
		return fmt.Errorf("uringwal: pending length %d is not record-aligned", len(d.pending))
	}
	for len(d.pending) != 0 {
		record := d.pending[:wal.RecordSize]
		block := uint64(len(d.durable) / wal.RecordSize)
		if err := d.writer.AppendDurable(block, record); err != nil {
			return err
		}
		d.durable = append(d.durable, record...)
		d.pending = d.pending[wal.RecordSize:]
	}
	return nil
}

func (d *Device) DurableBytes() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]byte(nil), d.durable...)
}

func (d *Device) TruncateDurable(logicalSize int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if logicalSize < 0 || logicalSize > len(d.durable) ||
		logicalSize%wal.RecordSize != 0 {
		return errors.New("uringwal: invalid truncate boundary")
	}
	physicalSize := int64(logicalSize/wal.RecordSize) * BlockSize
	if err := os.Truncate(d.path, physicalSize); err != nil {
		return fmt.Errorf("uringwal: truncate: %w", err)
	}
	d.durable = d.durable[:logicalSize]
	return nil
}

func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.writer == nil {
		return nil
	}
	err := d.writer.Close()
	d.writer = nil
	return err
}

func readRecords(path string) ([]byte, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("uringwal: open existing image: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size()%BlockSize != 0 {
		return nil, fmt.Errorf("uringwal: physical file has torn block size %d", info.Size())
	}
	count := int(info.Size() / BlockSize)
	records := make([]byte, 0, count*wal.RecordSize)
	block := make([]byte, BlockSize)
	zeros := make([]byte, BlockSize-wal.RecordSize)
	for index := 0; index < count; index++ {
		if _, err := file.ReadAt(block, int64(index*BlockSize)); err != nil && err != io.EOF {
			return nil, fmt.Errorf("uringwal: read block %d: %w", index, err)
		}
		if !bytes.Equal(block[wal.RecordSize:], zeros) {
			return nil, fmt.Errorf("uringwal: non-zero padding in block %d", index)
		}
		records = append(records, block[:wal.RecordSize]...)
	}
	return records, nil
}
