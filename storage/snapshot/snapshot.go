// Package snapshot defines the portable, checksummed snapshot image.
package snapshot

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

const HeaderSize = 36

var (
	imageMagic  = [8]byte{'H', 'Y', 'P', 'S', 'N', 'A', 'P', '1'}
	imageTable  = crc32.MakeTable(crc32.Castagnoli)
	ErrFormat   = errors.New("snapshot: invalid image")
	ErrChecksum = errors.New("snapshot: checksum mismatch")
)

type Image struct {
	LastIndex uint64
	LastTerm  uint64
	State     []byte
}

func Encode(image Image) []byte {
	out := make([]byte, HeaderSize+len(image.State))
	copy(out[:8], imageMagic[:])
	binary.LittleEndian.PutUint64(out[8:16], image.LastIndex)
	binary.LittleEndian.PutUint64(out[16:24], image.LastTerm)
	binary.LittleEndian.PutUint64(out[24:32], uint64(len(image.State)))
	copy(out[HeaderSize:], image.State)
	checksum := crc32.New(imageTable)
	_, _ = checksum.Write(out[:32])
	_, _ = checksum.Write(out[HeaderSize:])
	binary.LittleEndian.PutUint32(out[32:36], checksum.Sum32())
	return out
}

func Decode(data []byte) (Image, error) {
	if len(data) < HeaderSize || string(data[:8]) != string(imageMagic[:]) {
		return Image{}, ErrFormat
	}
	length := binary.LittleEndian.Uint64(data[24:32])
	if length > uint64(len(data)-HeaderSize) || uint64(len(data)) != uint64(HeaderSize)+length {
		return Image{}, ErrFormat
	}
	checksum := crc32.New(imageTable)
	_, _ = checksum.Write(data[:32])
	_, _ = checksum.Write(data[HeaderSize:])
	if checksum.Sum32() != binary.LittleEndian.Uint32(data[32:36]) {
		return Image{}, ErrChecksum
	}
	return Image{
		LastIndex: binary.LittleEndian.Uint64(data[8:16]),
		LastTerm:  binary.LittleEndian.Uint64(data[16:24]),
		State:     append([]byte(nil), data[HeaderSize:]...),
	}, nil
}
