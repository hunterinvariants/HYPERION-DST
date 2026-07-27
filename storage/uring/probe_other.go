//go:build !linux || !amd64

package uring

import (
	"errors"
	"runtime"
)

var ErrUnsupported = errors.New("uring: io_uring requires linux/amd64")

func Probe(uint32) error {
	return errors.New(ErrUnsupported.Error() + "; current platform is " + runtime.GOOS + "/" + runtime.GOARCH)
}
