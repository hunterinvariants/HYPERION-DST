package wal

import (
	"fmt"
	"os"
	"sync"
)

// FileDevice is the portable durable WAL backend used by multi-process nodes.
// Append writes into the kernel-visible file; Sync is the durability boundary.
type FileDevice struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func OpenFileDevice(path string) (*FileDevice, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, 2); err != nil {
		file.Close()
		return nil, err
	}
	return &FileDevice{file: file, path: path}, nil
}

func (d *FileDevice) Append(p []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.file.Write(p)
	return err
}

func (d *FileDevice) Sync() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.file.Sync()
}

func (d *FileDevice) DurableBytes() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.file.Sync(); err != nil {
		return nil
	}
	data, err := os.ReadFile(d.path)
	if err != nil {
		return nil
	}
	return data
}

func (d *FileDevice) TruncateDurable(size int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if size < 0 {
		return ErrInvalidTear
	}
	info, err := d.file.Stat()
	if err != nil {
		return err
	}
	if int64(size) > info.Size() {
		return ErrInvalidTear
	}
	if err := d.file.Truncate(int64(size)); err != nil {
		return err
	}
	if _, err := d.file.Seek(0, 2); err != nil {
		return err
	}
	return d.file.Sync()
}

func (d *FileDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.file == nil {
		return nil
	}
	err := d.file.Close()
	d.file = nil
	return err
}

func (d *FileDevice) String() string { return fmt.Sprintf("wal.FileDevice(%q)", d.path) }
