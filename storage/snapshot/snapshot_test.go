package snapshot

import (
	"errors"
	"testing"
)

func TestRoundTripAndCorruption(t *testing.T) {
	want := Image{LastIndex: 42, LastTerm: 7, State: []byte("deterministic-state")}
	data := Encode(want)
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastIndex != want.LastIndex || got.LastTerm != want.LastTerm ||
		string(got.State) != string(want.State) {
		t.Fatalf("decoded image = %+v", got)
	}
	data[len(data)-1] ^= 1
	if _, err := Decode(data); !errors.Is(err, ErrChecksum) {
		t.Fatalf("corrupt image error = %v", err)
	}
}

func TestTornSnapshotRejected(t *testing.T) {
	data := Encode(Image{LastIndex: 1, State: []byte("state")})
	for cut := 0; cut < len(data); cut++ {
		if _, err := Decode(data[:cut]); err == nil {
			t.Fatalf("accepted torn image at byte %d", cut)
		}
	}
}
