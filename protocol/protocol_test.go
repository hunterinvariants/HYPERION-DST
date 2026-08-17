package protocol

import (
	"bytes"
	"testing"

	"github.com/hunterinvariants/promtact/raft"
)

func TestFrameRoundTripAndChecksum(t *testing.T) {
	var wire bytes.Buffer
	if err := WriteFrame(&wire, KindClientRequest, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	kind, payload, err := ReadFrame(&wire)
	if err != nil || kind != KindClientRequest || string(payload) != "payload" {
		t.Fatalf("frame = %d %q %v", kind, payload, err)
	}
	corrupt := wire.Bytes()
	_ = corrupt
}

func TestClientRoundTrip(t *testing.T) {
	request := ClientRequest{Operation: ClientPut, ClientID: 8, RequestID: 9, Key: 10, Value: 11}
	decoded, err := DecodeClientRequest(EncodeClientRequest(request))
	if err != nil || decoded != request {
		t.Fatalf("request = %+v %v", decoded, err)
	}
	response := ClientResponse{Status: StatusOK, Leader: 2, RequestID: 9, Value: 11, Commit: 12}
	got, err := DecodeClientResponse(EncodeClientResponse(response))
	if err != nil || got != response {
		t.Fatalf("response = %+v %v", got, err)
	}
}

func TestPeerRoundTrip(t *testing.T) {
	message := raft.Message{
		Type: raft.MsgAppend, From: 1, To: 2, Term: 3, LogIndex: 4, LogTerm: 3,
		Commit: 2, Context: 99,
		Entry:         raft.Entry{Term: 3, Operation: raft.CommandPut, ClientID: 7, RequestID: 8, Key: 9, Value: 10},
		SnapshotIndex: 11, SnapshotTerm: 3, SnapshotOld: 7, SnapshotNew: 3, Snapshot: []byte("state"),
	}
	got, err := DecodePeer(EncodePeer(message))
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != message.Type || got.From != message.From || got.To != message.To ||
		got.Term != message.Term || got.Entry != message.Entry ||
		!bytes.Equal(got.Snapshot, message.Snapshot) {
		t.Fatalf("peer mismatch:\n got=%+v\nwant=%+v", got, message)
	}
}
