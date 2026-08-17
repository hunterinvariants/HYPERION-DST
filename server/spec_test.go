package server

import (
	"strings"
	"testing"
	"time"
)

const fiveNodes = `{
  "name": "sentinel five node",
  "tickInterval": "50ms",
  "requestTimeout": "5s",
  "queueCapacity": 1024,
  "snapshotEntries": 1000,
  "nodes": [
    {"id": 1, "peer": "10.77.0.11:9100", "client": "10.77.0.11:9200", "http": "10.77.0.11:9300", "dataDir": "/run/n1"},
    {"id": 2, "peer": "10.77.0.12:9100", "client": "10.77.0.12:9200", "http": "10.77.0.12:9300", "dataDir": "/run/n2"},
    {"id": 3, "peer": "10.77.0.13:9100", "client": "10.77.0.13:9200", "http": "10.77.0.13:9300", "dataDir": "/run/n3"},
    {"id": 4, "peer": "10.77.0.14:9100", "client": "10.77.0.14:9200", "http": "10.77.0.14:9300", "dataDir": "/run/n4"},
    {"id": 5, "peer": "10.77.0.15:9100", "client": "10.77.0.15:9200", "http": "10.77.0.15:9300", "dataDir": "/run/n5"}
  ]
}`

func TestConfigForDerivesThePeerList(t *testing.T) {
	spec, err := ReadSpec(strings.NewReader(fiveNodes))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	config, err := spec.ConfigFor(3)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if config.ID != 3 || config.PeerAddress != "10.77.0.13:9100" {
		t.Fatalf("config = %+v", config)
	}
	if len(config.Peers) != 4 {
		t.Fatalf("peers = %v, want the other four nodes", config.Peers)
	}
	if _, present := config.Peers[3]; present {
		t.Error("a node listed itself as its own peer")
	}
	if got := config.Peers[5]; got != "10.77.0.15:9100" {
		t.Errorf("peer 5 = %q", got)
	}
	if config.TickInterval != 50*time.Millisecond || config.RequestTimeout != 5*time.Second {
		t.Errorf("durations = %s and %s", config.TickInterval, config.RequestTimeout)
	}
}

// TestEveryNodeSeesTheSameMembership is the property the file format exists
// for: five processes reading one file cannot disagree about who the members
// are, which is the failure a hand-written per-node peer list invites.
func TestEveryNodeSeesTheSameMembership(t *testing.T) {
	spec, err := ReadSpec(strings.NewReader(fiveNodes))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	membership := make(map[uint32]map[uint32]string)
	for _, node := range spec.Nodes {
		config, err := spec.ConfigFor(node.ID)
		if err != nil {
			t.Fatalf("config for %d: %v", node.ID, err)
		}
		full := map[uint32]string{node.ID: config.PeerAddress}
		for id, address := range config.Peers {
			full[id] = address
		}
		membership[node.ID] = full
	}
	reference := membership[1]
	for id, view := range membership {
		if len(view) != len(reference) {
			t.Fatalf("node %d sees %d members, node 1 sees %d", id, len(view), len(reference))
		}
		for member, address := range reference {
			if view[member] != address {
				t.Fatalf("node %d has member %d at %q, node 1 has it at %q",
					id, member, view[member], address)
			}
		}
	}
}

func TestConfigForAppliesHistoricalDefaults(t *testing.T) {
	spec, err := ReadSpec(strings.NewReader(`{
      "nodes": [{"id": 1, "peer": "127.0.0.1:9100", "client": "127.0.0.1:9200", "dataDir": "/run/n1"}]
    }`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	config, err := spec.ConfigFor(1)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	// These match the flag defaults promtactd has always had, so a file that
	// omits a field starts the node the way the flags did.
	if config.TickInterval != 50*time.Millisecond {
		t.Errorf("tick interval = %s", config.TickInterval)
	}
	if config.QueueCapacity != 1024 {
		t.Errorf("queue capacity = %d", config.QueueCapacity)
	}
	if config.RequestTimeout != 5*time.Second {
		t.Errorf("request timeout = %s", config.RequestTimeout)
	}
	if config.SnapshotEntries != 10000 {
		t.Errorf("snapshot entries = %d", config.SnapshotEntries)
	}
	if config.HTTPAddress != "" {
		t.Errorf("http address = %q, want empty when omitted", config.HTTPAddress)
	}
}

func TestReadSpecRejectsMalformedClusters(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{"unknown field", `{"nodes":[{"id":1,"peer":"a:1","client":"a:2","dataDir":"/d"}],"nodez":1}`, "nodez"},
		{"no nodes", `{"nodes":[]}`, "no nodes declared"},
		{"id zero", `{"nodes":[{"id":0,"peer":"a:1","client":"a:2","dataDir":"/d"}]}`, "outside 1..64"},
		{"id too high", `{"nodes":[{"id":65,"peer":"a:1","client":"a:2","dataDir":"/d"}]}`, "outside 1..64"},
		{"duplicate id", `{"nodes":[
            {"id":1,"peer":"a:1","client":"a:2","dataDir":"/d1"},
            {"id":1,"peer":"b:1","client":"b:2","dataDir":"/d2"}]}`, "declared twice"},
		{"missing peer", `{"nodes":[{"id":1,"peer":"","client":"a:2","dataDir":"/d"}]}`, "no peer address"},
		{"bad address", `{"nodes":[{"id":1,"peer":"not-a-port","client":"a:2","dataDir":"/d"}]}`, "not host:port"},
		{"shared address", `{"nodes":[
            {"id":1,"peer":"a:1","client":"a:2","dataDir":"/d1"},
            {"id":2,"peer":"a:1","client":"b:2","dataDir":"/d2"}]}`, "both use address"},
		{"shared data dir", `{"nodes":[
            {"id":1,"peer":"a:1","client":"a:2","dataDir":"/same"},
            {"id":2,"peer":"b:1","client":"b:2","dataDir":"/same"}]}`, "share dataDir"},
		{"missing data dir", `{"nodes":[{"id":1,"peer":"a:1","client":"a:2","dataDir":""}]}`, "no dataDir"},
		{"bad duration", `{"tickInterval":"soon","nodes":[{"id":1,"peer":"a:1","client":"a:2","dataDir":"/d"}]}`, "is not a duration"},
		{"zero duration", `{"requestTimeout":"0s","nodes":[{"id":1,"peer":"a:1","client":"a:2","dataDir":"/d"}]}`, "must be positive"},
		{"trailing content", `{"nodes":[{"id":1,"peer":"a:1","client":"a:2","dataDir":"/d"}]} {}`, "unexpected content"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReadSpec(strings.NewReader(test.body))
			if err == nil {
				t.Fatal("accepted a malformed cluster")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q does not mention %q", err, test.want)
			}
		})
	}
}

func TestConfigForRejectsAnUnknownNode(t *testing.T) {
	spec, err := ReadSpec(strings.NewReader(fiveNodes))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := spec.ConfigFor(9); err == nil {
		t.Fatal("a node absent from the cluster was accepted")
	}
}
