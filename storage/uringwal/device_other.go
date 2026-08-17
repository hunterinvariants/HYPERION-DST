//go:build !linux || !amd64

package uringwal

import (
	"errors"

	"github.com/hunterinvariants/promtact/storage/uring"
)

const BlockSize = uring.DefaultAlignment

type Device struct{}

func Open(string, uint32) (*Device, error) { return nil, uring.ErrUnsupported }
func (*Device) Append([]byte) error        { return uring.ErrUnsupported }
func (*Device) Sync() error                { return uring.ErrUnsupported }
func (*Device) DurableBytes() []byte       { return nil }
func (*Device) TruncateDurable(int) error  { return uring.ErrUnsupported }
func (*Device) Close() error               { return errors.New(uring.ErrUnsupported.Error()) }
