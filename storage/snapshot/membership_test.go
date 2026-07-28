package snapshot

import "testing"

func TestSnapshotMembershipRoundTrip(t *testing.T) {
	want := Image{
		LastIndex: 11,
		LastTerm:  4,
		State:     []byte("state"),
		OldVoters: 0b111,
		NewVoters: 0b1101,
	}
	got, err := Decode(Encode(want))
	if err != nil {
		t.Fatal(err)
	}
	if got.LastIndex != want.LastIndex || got.LastTerm != want.LastTerm ||
		got.OldVoters != want.OldVoters || got.NewVoters != want.NewVoters ||
		string(got.State) != string(want.State) {
		t.Fatalf("snapshot metadata = %+v", got)
	}
}
