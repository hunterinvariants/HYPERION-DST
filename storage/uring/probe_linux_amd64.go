//go:build linux && amd64

package uring

import (
	"fmt"
	"syscall"
	"unsafe"
)

const sysIOUringSetup = 425

// Params matches Linux's 120-byte io_uring_params ABI.
type Params struct {
	SQEntries    uint32
	CQEntries    uint32
	Flags        uint32
	SQThreadCPU  uint32
	SQThreadIdle uint32
	Features     uint32
	WQFD         uint32
	Reserved     [3]uint32
	SQOff        [10]uint32
	CQOff        [10]uint32
}

// Probe performs the real io_uring_setup syscall and immediately closes the
// returned descriptor. It is an availability gate, not the storage backend.
func Probe(entries uint32) error {
	if entries == 0 {
		return fmt.Errorf("uring: entries must be positive")
	}
	var params Params
	fd, _, errno := syscall.RawSyscall(sysIOUringSetup, uintptr(entries),
		uintptr(unsafe.Pointer(&params)), 0)
	if errno != 0 {
		return fmt.Errorf("io_uring_setup: %w", errno)
	}
	return syscall.Close(int(fd))
}
