package protocol

import (
	"bytes"
	"hash/crc32"
	"testing"

	"github.com/hunterinvariants/promtact/raft"
)

// These targets cover the only path by which bytes chosen by someone else
// enter a node: a socket. The disk format has had a fuzz target since the WAL
// was written; the wire format had none, which was the wrong way round, since
// a peer address is easier to reach than a data directory.
//
// Each target asserts more than "does not panic". A decoder that accepts a
// malformed frame and returns a plausible-looking message is worse than one
// that crashes, because the damage then happens somewhere else.

func FuzzReadFrame(f *testing.F) {
	var valid bytes.Buffer
	if err := WriteFrame(&valid, KindClientRequest, EncodeClientRequest(
		ClientRequest{Operation: ClientPut, ClientID: 1, RequestID: 2, Key: 3, Value: 4})); err != nil {
		f.Fatal(err)
	}
	f.Add(valid.Bytes())
	f.Add([]byte("HYPR"))
	f.Add(make([]byte, HeaderSize))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		kind, payload, err := ReadFrame(bytes.NewReader(data))
		if err != nil {
			return
		}
		// An accepted frame must satisfy everything the format promises,
		// otherwise the caller acts on bytes that were never validated.
		if len(payload) > MaxFrameSize {
			t.Fatalf("accepted a payload of %d bytes, above the %d limit", len(payload), MaxFrameSize)
		}
		if len(data) < HeaderSize+len(payload) {
			t.Fatalf("accepted a frame claiming %d payload bytes from %d input bytes",
				len(payload), len(data))
		}
		if crc32.Checksum(payload, frameCRC) != crc32.Checksum(data[HeaderSize:HeaderSize+len(payload)], frameCRC) {
			t.Fatal("the returned payload is not the bytes that were checksummed")
		}
		if kind == 0 {
			return
		}
	})
}

// FuzzDecodePeer covers the replication message decoder, which carries a
// variable-length snapshot and is therefore the one wire structure with
// attacker-influenced sizes in it.
func FuzzDecodePeer(f *testing.F) {
	f.Add(EncodePeer(raft.Message{Type: raft.MsgAppend, From: 1, To: 2, Term: 3}))
	f.Add(EncodePeer(raft.Message{
		Type: raft.MsgInstallSnapshot, From: 2, To: 1, Term: 9,
		SnapshotIndex: 4, SnapshotTerm: 3, Snapshot: []byte("state"),
	}))
	f.Add(EncodePeer(raft.Message{Type: raft.MsgAppend, HasEntry: true,
		Entry: raft.Entry{Term: 2, Command: 7}}))
	f.Add([]byte{})
	f.Add(make([]byte, 168))

	f.Fuzz(func(t *testing.T, data []byte) {
		message, err := DecodePeer(data)
		if err != nil {
			return
		}
		assertStable(t, message)
	})
}

func FuzzDecodeClientRequest(f *testing.F) {
	f.Add(EncodeClientRequest(ClientRequest{Operation: ClientGet, Key: 1}))
	f.Add(EncodeClientRequest(ClientRequest{Operation: ClientStatus}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		request, err := DecodeClientRequest(data)
		if err != nil {
			return
		}
		if request.Operation < ClientPut || request.Operation > ClientStatus {
			t.Fatalf("accepted operation %d, outside the defined range", request.Operation)
		}
		again, err := DecodeClientRequest(EncodeClientRequest(request))
		if err != nil {
			t.Fatalf("re-encoding an accepted request produced bytes it rejects: %v", err)
		}
		if again != request {
			t.Fatalf("decode is not stable: %+v became %+v", request, again)
		}
	})
}

func FuzzDecodeClientResponse(f *testing.F) {
	f.Add(EncodeClientResponse(ClientResponse{Status: StatusOK, Leader: 1, RequestID: 2}))
	f.Add(EncodeClientResponse(ClientResponse{Status: StatusNotLeader}))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		response, err := DecodeClientResponse(data)
		if err != nil {
			return
		}
		again, err := DecodeClientResponse(EncodeClientResponse(response))
		if err != nil {
			t.Fatalf("re-encoding an accepted response produced bytes it rejects: %v", err)
		}
		if again != response {
			t.Fatalf("decode is not stable: %+v became %+v", response, again)
		}
	})
}

// assertStable requires decoding to be idempotent: whatever the decoder made of
// the input, encoding that and decoding again must give the same message. A
// parser that reads a field it does not write back, or that tolerates a length
// it then ignores, fails here. Exact round-tripping of the input bytes is not
// required, because the format has padding the decoder legitimately skips.
func assertStable(t *testing.T, message raft.Message) {
	t.Helper()
	again, err := DecodePeer(EncodePeer(message))
	if err != nil {
		t.Fatalf("re-encoding an accepted message produced bytes it rejects: %v\nmessage: %+v", err, message)
	}
	if again.Type != message.Type || again.From != message.From || again.To != message.To ||
		again.Term != message.Term || again.LogIndex != message.LogIndex ||
		again.LogTerm != message.LogTerm || again.Commit != message.Commit ||
		again.Match != message.Match || again.Context != message.Context ||
		again.Reject != message.Reject || again.HasEntry != message.HasEntry ||
		again.Entry != message.Entry ||
		again.SnapshotIndex != message.SnapshotIndex || again.SnapshotTerm != message.SnapshotTerm ||
		again.SnapshotOld != message.SnapshotOld || again.SnapshotNew != message.SnapshotNew ||
		!bytes.Equal(again.Snapshot, message.Snapshot) {
		t.Fatalf("decode is not stable:\n first: %+v\nsecond: %+v", message, again)
	}
}
