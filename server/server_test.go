package server

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/hunterinvariants/HYPERION-DST/protocol"
)

func TestSingleNodeClientDeduplicationAndRestart(t *testing.T) {
	data := filepath.Join(t.TempDir(), "node")
	clientAddress := freeAddress(t)
	peerAddress := freeAddress(t)
	run := func() (context.CancelFunc, <-chan error) {
		instance, err := Open(Config{
			ID: 1, Peers: map[uint32]string{}, PeerAddress: peerAddress,
			ClientAddress: clientAddress, DataDir: data, ElectionTicks: 2,
			TickInterval: 5 * time.Millisecond, RequestTimeout: time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- instance.Run(ctx) }()
		waitForEndpoint(t, clientAddress)
		return cancel, done
	}
	cancel, done := run()
	waitForLeader(t, clientAddress)
	put := protocol.ClientRequest{Operation: protocol.ClientPut, ClientID: 77, RequestID: 1, Key: 4, Value: 99}
	first := call(t, clientAddress, put)
	second := call(t, clientAddress, put)
	if first.Status != protocol.StatusOK || second != first {
		t.Fatalf("dedupe first=%+v second=%+v", first, second)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	cancel, done = run()
	defer func() { cancel(); <-done }()
	waitForLeader(t, clientAddress)
	get := call(t, clientAddress, protocol.ClientRequest{Operation: protocol.ClientGet, ClientID: 77, RequestID: 2, Key: 4})
	if get.Status != protocol.StatusOK || get.Value != 99 {
		t.Fatalf("recovered get = %+v", get)
	}
	duplicate := call(t, clientAddress, put)
	if duplicate.Status != protocol.StatusOK || duplicate.Value != 99 {
		t.Fatalf("recovered duplicate = %+v", duplicate)
	}
}

func call(t *testing.T, address string, request protocol.ClientRequest) protocol.ClientResponse {
	t.Helper()
	response, err := protocol.Call(address, request, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func waitForLeader(t *testing.T, address string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, err := protocol.Call(address, protocol.ClientRequest{
			Operation: protocol.ClientPut, ClientID: 1, RequestID: 1, Key: 1, Value: 1,
		}, time.Second)
		if err == nil && response.Status == protocol.StatusOK {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("leader not elected")
}

func waitForEndpoint(t *testing.T, address string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 20*time.Millisecond)
		if err == nil {
			connection.Close()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("endpoint not listening")
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}
