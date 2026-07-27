//go:build !linux || !amd64

package uring

import "errors"

type DurableWriter struct{}

func OpenDurableWriter(string, uint32, int) (*DurableWriter, error) {
	return nil, ErrUnsupported
}

func (*DurableWriter) AppendDurable(uint64, []byte) error { return ErrUnsupported }
func (*DurableWriter) Close() error                       { return errors.New(ErrUnsupported.Error()) }
