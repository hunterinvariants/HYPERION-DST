package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hunterinvariants/promtact/protocol"
)

func TestThreeProcessClusterReplicatesAndSurvivesLeaderShutdown(t *testing.T) {
	peerAddresses := map[uint32]string{1: freeAddress(t), 2: freeAddress(t), 3: freeAddress(t)}
	clientAddresses := map[uint32]string{1: freeAddress(t), 2: freeAddress(t), 3: freeAddress(t)}
	type running struct {
		cancel context.CancelFunc
		done   <-chan error
	}
	nodes := make(map[uint32]running)
	for id := uint32(1); id <= 3; id++ {
		instance, err := Open(Config{
			ID: id, Peers: peerAddresses, PeerAddress: peerAddresses[id],
			ClientAddress: clientAddresses[id], DataDir: filepath.Join(t.TempDir(), "node"),
			ElectionTicks: 8 + uint64(id)*3, TickInterval: 5 * time.Millisecond,
			RequestTimeout: time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- instance.Run(ctx) }()
		nodes[id] = running{cancel, done}
	}
	defer func() {
		for _, node := range nodes {
			node.cancel()
		}
		for _, node := range nodes {
			<-node.done
		}
	}()
	var leader uint32
	deadline := time.Now().Add(5 * time.Second)
	for leader == 0 && time.Now().Before(deadline) {
		for id, address := range clientAddresses {
			response, err := protocol.Call(address, protocol.ClientRequest{
				Operation: protocol.ClientPut, ClientID: 91, RequestID: 1, Key: 12, Value: 34,
			}, 250*time.Millisecond)
			if err == nil && response.Status == protocol.StatusOK {
				leader = id
				break
			}
		}
	}
	if leader == 0 {
		t.Fatal("cluster did not elect a leader")
	}
	nodes[leader].cancel()
	if err := <-nodes[leader].done; err != nil {
		t.Fatal(err)
	}
	delete(nodes, leader)
	delete(clientAddresses, leader)

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, address := range clientAddresses {
			response, err := protocol.Call(address, protocol.ClientRequest{
				Operation: protocol.ClientGet, ClientID: 92, RequestID: 1, Key: 12,
			}, 250*time.Millisecond)
			if err == nil && response.Status == protocol.StatusOK && response.Value == 34 {
				return
			}
		}
	}
	t.Fatal("replacement leader did not expose replicated value")
}
