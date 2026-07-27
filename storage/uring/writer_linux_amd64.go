//go:build linux && amd64

package uring

import (
	"errors"
	"fmt"
	"syscall"
)

type DurableWriter struct {
	ring      *Ring
	fd        int
	memory    []byte
	blockSize int
	sequence  uint64
}

func OpenDurableWriter(path string, depth uint32, blockSize int) (*DurableWriter, error) {
	if depth == 0 || blockSize < DefaultAlignment || blockSize%DefaultAlignment != 0 {
		return nil, errors.New("uring: depth must be positive and block size page-aligned")
	}
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_DIRECT, 0o640)
	if err != nil {
		return nil, fmt.Errorf("uring: open O_DIRECT file: %w", err)
	}
	writer := &DurableWriter{fd: fd, blockSize: blockSize}
	fail := func(err error) (*DurableWriter, error) {
		_ = writer.Close()
		return nil, err
	}
	writer.memory, err = syscall.Mmap(-1, 0, int(depth)*blockSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		return fail(fmt.Errorf("uring: allocate aligned buffers: %w", err))
	}
	writer.ring, err = OpenRing(depth)
	if err != nil {
		return fail(err)
	}
	iovec := syscall.Iovec{Base: &writer.memory[0], Len: uint64(len(writer.memory))}
	if err := writer.ring.RegisterBuffers([]syscall.Iovec{iovec}); err != nil {
		return fail(err)
	}
	if err := writer.ring.RegisterFiles([]int32{int32(fd)}); err != nil {
		return fail(err)
	}
	return writer, nil
}

// AppendDurable writes one aligned block through WRITE_FIXED and waits for a
// separate FSYNC completion before returning.
func (w *DurableWriter) AppendDurable(block uint64, payload []byte) error {
	if len(payload) > w.blockSize {
		return fmt.Errorf("uring: payload %d exceeds block size %d", len(payload), w.blockSize)
	}
	slot := int(w.sequence % uint64(len(w.memory)/w.blockSize))
	buffer := w.memory[slot*w.blockSize : (slot+1)*w.blockSize]
	clear(buffer)
	copy(buffer, payload)
	w.sequence++
	if err := w.ring.WriteFixed(0, block*uint64(w.blockSize), buffer, 0, w.sequence<<1); err != nil {
		return err
	}
	return w.ring.Fsync(0, w.sequence<<1|1)
}

func (w *DurableWriter) Close() error {
	var errs []error
	if w.ring != nil {
		errs = append(errs, w.ring.Close())
		w.ring = nil
	}
	if len(w.memory) != 0 {
		errs = append(errs, syscall.Munmap(w.memory))
		w.memory = nil
	}
	if w.fd >= 0 {
		errs = append(errs, syscall.Close(w.fd))
		w.fd = -1
	}
	return errors.Join(errs...)
}
