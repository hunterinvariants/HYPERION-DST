// Package protocol defines Promtact's versioned peer and client wire format.
package protocol

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

const (
	Version      = uint16(1)
	HeaderSize   = 16
	MaxFrameSize = 1 << 20
)

type Kind uint16

const (
	KindPeer Kind = iota + 1
	KindClientRequest
	KindClientResponse
)

var (
	frameMagic       = [4]byte{'H', 'Y', 'P', 'R'}
	frameCRC         = crc32.MakeTable(crc32.Castagnoli)
	ErrFrame         = errors.New("protocol: invalid frame")
	ErrVersion       = errors.New("protocol: unsupported version")
	ErrFrameTooLarge = errors.New("protocol: frame too large")
)

func WriteFrame(w io.Writer, kind Kind, payload []byte) error {
	if len(payload) > MaxFrameSize {
		return ErrFrameTooLarge
	}
	var header [HeaderSize]byte
	copy(header[:4], frameMagic[:])
	binary.LittleEndian.PutUint16(header[4:6], Version)
	binary.LittleEndian.PutUint16(header[6:8], uint16(kind))
	binary.LittleEndian.PutUint32(header[8:12], uint32(len(payload)))
	binary.LittleEndian.PutUint32(header[12:16], crc32.Checksum(payload, frameCRC))
	if err := writeFull(w, header[:]); err != nil {
		return err
	}
	return writeFull(w, payload)
}

func ReadFrame(r io.Reader) (Kind, []byte, error) {
	var header [HeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	if string(header[:4]) != string(frameMagic[:]) {
		return 0, nil, ErrFrame
	}
	if binary.LittleEndian.Uint16(header[4:6]) != Version {
		return 0, nil, ErrVersion
	}
	length := binary.LittleEndian.Uint32(header[8:12])
	if length > MaxFrameSize {
		return 0, nil, ErrFrameTooLarge
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	if crc32.Checksum(payload, frameCRC) != binary.LittleEndian.Uint32(header[12:16]) {
		return 0, nil, ErrFrame
	}
	return Kind(binary.LittleEndian.Uint16(header[6:8])), payload, nil
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) != 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
