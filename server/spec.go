package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// MaxNodeID is the highest usable node identifier. Membership is carried in a
// 64-bit voter mask, so this is a structural limit rather than a policy.
const MaxNodeID = 64

// Spec is a declarative description of one cluster.
//
// It describes the whole deployment once, and each process selects its own
// entry by identifier. That is the point: with the peer list derived from the
// same file every node reads, the five processes of a cluster cannot disagree
// about who the members are, which is the mistake a hand-written -peers flag
// invites.
type Spec struct {
	// Name labels the deployment in diagnostics. Optional.
	Name string `json:"name,omitempty"`
	// Nodes lists every member. Order does not matter; identifiers do.
	Nodes []SpecNode `json:"nodes"`

	// The remaining fields apply to every node. A per-node override would let
	// two members run with different timeouts, which is a way to produce a
	// cluster that misbehaves only under load.
	ElectionTicks   uint64 `json:"electionTicks,omitempty"`
	TickInterval    string `json:"tickInterval,omitempty"`
	QueueCapacity   int    `json:"queueCapacity,omitempty"`
	RequestTimeout  string `json:"requestTimeout,omitempty"`
	SnapshotEntries uint64 `json:"snapshotEntries,omitempty"`
}

// SpecNode is one member of the cluster.
type SpecNode struct {
	ID      uint32 `json:"id"`
	Peer    string `json:"peer"`
	Client  string `json:"client"`
	HTTP    string `json:"http,omitempty"`
	DataDir string `json:"dataDir"`
}

// LoadSpec reads and validates a cluster file.
func LoadSpec(path string) (Spec, error) {
	file, err := os.Open(path)
	if err != nil {
		return Spec{}, err
	}
	defer file.Close()
	return ReadSpec(file)
}

// ReadSpec parses and validates a cluster description. Parsing is strict: an
// unknown field is an error, because a misspelled key that was silently
// ignored would start a node with a default nobody chose.
func ReadSpec(r io.Reader) (Spec, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var spec Spec
	if err := decoder.Decode(&spec); err != nil {
		return Spec{}, fmt.Errorf("cluster: %w", err)
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return Spec{}, fmt.Errorf("cluster: unexpected content after the first object")
	}
	if err := spec.Validate(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

// Validate reports the first problem that would make the cluster behave other
// than the file describes.
func (s Spec) Validate() error {
	if len(s.Nodes) == 0 {
		return fmt.Errorf("cluster: no nodes declared")
	}
	if _, err := s.tickInterval(); err != nil {
		return err
	}
	if _, err := s.requestTimeout(); err != nil {
		return err
	}
	if s.QueueCapacity < 0 {
		return fmt.Errorf("cluster: queueCapacity must not be negative")
	}

	ids := make(map[uint32]bool, len(s.Nodes))
	addresses := make(map[string]uint32, len(s.Nodes)*3)
	dataDirs := make(map[string]uint32, len(s.Nodes))
	for _, node := range s.Nodes {
		if node.ID < 1 || node.ID > MaxNodeID {
			return fmt.Errorf("cluster: node id %d is outside 1..%d", node.ID, MaxNodeID)
		}
		if ids[node.ID] {
			return fmt.Errorf("cluster: node id %d is declared twice", node.ID)
		}
		ids[node.ID] = true

		for label, address := range map[string]string{
			"peer": node.Peer, "client": node.Client, "http": node.HTTP,
		} {
			if address == "" {
				if label == "http" {
					continue // the health and metrics endpoint is optional
				}
				return fmt.Errorf("cluster: node %d has no %s address", node.ID, label)
			}
			if _, _, err := net.SplitHostPort(address); err != nil {
				return fmt.Errorf("cluster: node %d %s address %q is not host:port", node.ID, label, address)
			}
			// Two nodes sharing an address means one of them never starts, and
			// the failure appears as an unrelated bind error at run time.
			if other, taken := addresses[address]; taken {
				return fmt.Errorf("cluster: nodes %d and %d both use address %s", other, node.ID, address)
			}
			addresses[address] = node.ID
		}

		if node.DataDir == "" {
			return fmt.Errorf("cluster: node %d has no dataDir", node.ID)
		}
		clean := filepath.Clean(node.DataDir)
		if other, taken := dataDirs[clean]; taken {
			// Sharing a data directory corrupts both nodes' durable state.
			return fmt.Errorf("cluster: nodes %d and %d share dataDir %s", other, node.ID, clean)
		}
		dataDirs[clean] = node.ID
	}
	return nil
}

// ConfigFor returns the running configuration for one member, with its peer
// list derived from every other node in the file.
func (s Spec) ConfigFor(id uint32) (Config, error) {
	if err := s.Validate(); err != nil {
		return Config{}, err
	}
	var self *SpecNode
	for i := range s.Nodes {
		if s.Nodes[i].ID == id {
			self = &s.Nodes[i]
			break
		}
	}
	if self == nil {
		return Config{}, fmt.Errorf("cluster: node %d is not declared in this cluster", id)
	}

	tick, err := s.tickInterval()
	if err != nil {
		return Config{}, err
	}
	timeout, err := s.requestTimeout()
	if err != nil {
		return Config{}, err
	}

	peers := make(map[uint32]string, len(s.Nodes)-1)
	for _, node := range s.Nodes {
		if node.ID != id {
			peers[node.ID] = node.Peer
		}
	}
	config := Config{
		ID:              self.ID,
		Peers:           peers,
		PeerAddress:     self.Peer,
		ClientAddress:   self.Client,
		HTTPAddress:     self.HTTP,
		DataDir:         self.DataDir,
		ElectionTicks:   s.ElectionTicks,
		TickInterval:    tick,
		QueueCapacity:   s.QueueCapacity,
		RequestTimeout:  timeout,
		SnapshotEntries: s.SnapshotEntries,
	}
	// Defaults match the historical flag defaults of hyperiond, so a file that
	// omits a field starts the node the same way the flags did.
	if config.TickInterval == 0 {
		config.TickInterval = 50 * time.Millisecond
	}
	if config.QueueCapacity == 0 {
		config.QueueCapacity = 1024
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 5 * time.Second
	}
	if config.SnapshotEntries == 0 {
		config.SnapshotEntries = 10000
	}
	return config, nil
}

func (s Spec) tickInterval() (time.Duration, error) {
	return parseOptionalDuration("tickInterval", s.TickInterval)
}

func (s Spec) requestTimeout() (time.Duration, error) {
	return parseOptionalDuration("requestTimeout", s.RequestTimeout)
}

func parseOptionalDuration(field, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("cluster: %s %q is not a duration", field, value)
	}
	if d <= 0 {
		return 0, fmt.Errorf("cluster: %s must be positive, got %s", field, value)
	}
	return d, nil
}
