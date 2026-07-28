// Package snapshot defines the portable, checksummed snapshot image.
package snapshot

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

const HeaderSize = 52

var (
	imageMagic  = [8]byte{'H', 'Y', 'P', 'S', 'N', 'A', 'P', '2'}
	imageTable  = crc32.MakeTable(crc32.Castagnoli)
	ErrFormat   = errors.New("snapshot: invalid image")
	ErrChecksum = errors.New("snapshot: checksum mismatch")
)

type Image struct {
	LastIndex uint64
	LastTerm  uint64
	State     []byte
	OldVoters uint64
	NewVoters uint64
}

func Encode(image Image) []byte {
	out := make([]byte, HeaderSize+len(image.State))
	copy(out[:8], imageMagic[:])
	binary.LittleEndian.PutUint64(out[8:16], image.LastIndex)
	binary.LittleEndian.PutUint64(out[16:24], image.LastTerm)
	binary.LittleEndian.PutUint64(out[24:32], image.OldVoters)
	binary.LittleEndian.PutUint64(out[32:40], image.NewVoters)
	binary.LittleEndian.PutUint64(out[40:48], uint64(len(image.State)))
	copy(out[HeaderSize:], image.State)
	checksum := crc32.New(imageTable)
	_, _ = checksum.Write(out[:48])
	_, _ = checksum.Write(out[HeaderSize:])
	binary.LittleEndian.PutUint32(out[48:52], checksum.Sum32())
	return out
}

func Decode(data []byte) (Image, error) {
	if len(data) < HeaderSize || string(data[:8]) != string(imageMagic[:]) {
		return Image{}, ErrFormat
	}
	length := binary.LittleEndian.Uint64(data[40:48])
	if length > uint64(len(data)-HeaderSize) || uint64(len(data)) != uint64(HeaderSize)+length {
		return Image{}, ErrFormat
	}
	checksum := crc32.New(imageTable)
	_, _ = checksum.Write(data[:48])
	_, _ = checksum.Write(data[HeaderSize:])
	if checksum.Sum32() != binary.LittleEndian.Uint32(data[48:52]) {
		return Image{}, ErrChecksum
	}
	return Image{
		LastIndex: binary.LittleEndian.Uint64(data[8:16]),
		LastTerm:  binary.LittleEndian.Uint64(data[16:24]),
		OldVoters: binary.LittleEndian.Uint64(data[24:32]),
		NewVoters: binary.LittleEndian.Uint64(data[32:40]),
		State:     append([]byte(nil), data[HeaderSize:]...),
	}, nil
}
