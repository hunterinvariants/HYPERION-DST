package wal

import (
	"testing"

	"github.com/hunterinvariants/hyperion/storage"
)

func BenchmarkEncode(b *testing.B) {
	record := Record{Sequence: 1, Entry: storage.Entry{Index: 1, Term: 1, Command: 42}}
	var dst [RecordSize]byte
	b.ReportAllocs()
	for b.Loop() {
		Encode(&dst, record)
	}
}

func FuzzDecodeNeverPanics(f *testing.F) {
	var valid [RecordSize]byte
	Encode(&valid, Record{Sequence: 1, Entry: storage.Entry{Index: 1, Term: 1, Command: 42}})
	f.Add(valid[:])
	f.Add([]byte("torn"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Decode(data)
	})
}
