//go:build linux && amd64

package uring

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const blkGetSize64 = 0x80081272

func BlockDeviceSize(path string) (uint64, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return 0, fmt.Errorf("uring: open block device: %w", err)
	}
	defer syscall.Close(fd)
	var size uint64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), blkGetSize64,
		uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return 0, fmt.Errorf("uring: BLKGETSIZE64: %w", errno)
	}
	return size, nil
}

func IsBlockDevice(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeDevice != 0 && info.Mode()&os.ModeCharDevice == 0, nil
}
