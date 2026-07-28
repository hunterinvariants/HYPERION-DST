//go:build !linux || !amd64

package uring

func BlockDeviceSize(string) (uint64, error) { return 0, ErrUnsupported }
func IsBlockDevice(string) (bool, error)     { return false, ErrUnsupported }
