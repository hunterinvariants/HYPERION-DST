// Package scenario is the declarative description of a deterministic run.
//
// A scenario file names a seed, a cluster size, network conditions, and the
// faults to inject, so that a campaign can be checked in, reviewed, and
// reproduced by anyone with the file. The format is protocol-agnostic: it
// describes the engine and its faults, not Raft. A different protocol reuses
// the format and supplies its own runner.
//
// Parsing is strict. An unknown field, an unknown fault type, or an
// out-of-range node is an error rather than a silently ignored line, because a
// scenario that quietly did less than it claimed would produce evidence for a
// campaign that never ran.
package scenario

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/hunterinvariants/promtact/dst"
)

// MaxNodes is the cluster size ceiling. Node identity is carried in a 64-bit
// voter mask in the consensus core, so this is a hard structural limit.
const MaxNodes = 64

// Scenario is one reproducible run.
type Scenario struct {
	// Name labels the run in reports. Optional.
	Name string `json:"name,omitempty"`
	// Seed fixes the schedule. Decimal, or 0x-prefixed hexadecimal, as a
	// string, because JSON numbers cannot express the hexadecimal seeds this
	// project's commands and evidence use.
	Seed string `json:"seed"`
	// Nodes is the cluster size.
	Nodes int `json:"nodes"`
	// Steps is the number of virtual time units to run.
	Steps uint64 `json:"steps"`
	// DropPermille is baseline random message loss, in tenths of a percent.
	DropPermille int `json:"dropPermille,omitempty"`
	// MaxDelay is the inclusive upper bound on random message delay.
	MaxDelay uint64 `json:"maxDelay,omitempty"`
	// ProposeEvery submits a client command every N steps. Zero disables it.
	ProposeEvery uint64 `json:"proposeEvery,omitempty"`
	// RestartEvery crash-restarts one node every N steps, cycling through the
	// cluster. Zero disables it.
	RestartEvery uint64 `json:"restartEvery,omitempty"`
	// Faults are injected in the order given.
	Faults []Fault `json:"faults,omitempty"`
}

// Fault is one declared network fault.
type Fault struct {
	// Type is "split", "isolate", or "link".
	Type string `json:"type"`
	// A and B are the two sides of a split.
	A []uint32 `json:"a,omitempty"`
	B []uint32 `json:"b,omitempty"`
	// Nodes are the nodes an isolate removes from the network.
	Nodes []uint32 `json:"nodes,omitempty"`
	// From and To are the endpoints of a one-way link failure.
	From uint32 `json:"from,omitempty"`
	To   uint32 `json:"to,omitempty"`
	// Start and End bound the fault to [Start, End) in virtual time. Leaving
	// End at zero means the fault applies for the whole run.
	Start uint64 `json:"start,omitempty"`
	End   uint64 `json:"end,omitempty"`
}

// Load reads and validates a scenario file.
func Load(path string) (Scenario, error) {
	file, err := os.Open(path)
	if err != nil {
		return Scenario{}, err
	}
	defer file.Close()
	return Read(file)
}

// Read parses and validates a scenario from r.
func Read(r io.Reader) (Scenario, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var s Scenario
	if err := decoder.Decode(&s); err != nil {
		return Scenario{}, fmt.Errorf("scenario: %w", err)
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return Scenario{}, fmt.Errorf("scenario: unexpected content after the first object")
	}
	if err := s.Validate(); err != nil {
		return Scenario{}, err
	}
	return s, nil
}

// ParsedSeed returns the seed as an integer.
func (s Scenario) ParsedSeed() (int64, error) {
	seed, err := strconv.ParseInt(strings.TrimSpace(s.Seed), 0, 64)
	if err != nil {
		return 0, fmt.Errorf("scenario: invalid seed %q", s.Seed)
	}
	return seed, nil
}

// EngineConfig is the engine configuration this scenario describes.
func (s Scenario) EngineConfig() (dst.Config, error) {
	seed, err := s.ParsedSeed()
	if err != nil {
		return dst.Config{}, err
	}
	return dst.Config{Seed: seed, DropPermille: s.DropPermille, MaxDelay: s.MaxDelay}, nil
}

// Injectors builds the declared faults in declaration order.
func (s Scenario) Injectors() ([]dst.Injector, error) {
	injectors := make([]dst.Injector, 0, len(s.Faults))
	for i, fault := range s.Faults {
		injector, err := fault.build()
		if err != nil {
			return nil, fmt.Errorf("scenario: fault %d: %w", i, err)
		}
		injectors = append(injectors, injector)
	}
	return injectors, nil
}

func (f Fault) build() (dst.Injector, error) {
	var injector dst.Injector
	switch f.Type {
	case "split":
		injector = dst.Split(f.A, f.B)
	case "isolate":
		injector = dst.Isolate(f.Nodes...)
	case "link":
		injector = dst.Link(f.From, f.To)
	default:
		return nil, fmt.Errorf("unknown type %q", f.Type)
	}
	if f.End != 0 {
		injector = dst.During(f.Start, f.End, injector)
	}
	return injector, nil
}

// Validate reports the first problem that would make the run mean something
// other than what the file says.
func (s Scenario) Validate() error {
	if _, err := s.ParsedSeed(); err != nil {
		return err
	}
	if s.Nodes < 1 || s.Nodes > MaxNodes {
		return fmt.Errorf("scenario: nodes must be between 1 and %d, got %d", MaxNodes, s.Nodes)
	}
	if s.Steps < 1 {
		return fmt.Errorf("scenario: steps must be positive, got %d", s.Steps)
	}
	if s.DropPermille < 0 || s.DropPermille > 1000 {
		return fmt.Errorf("scenario: dropPermille must be between 0 and 1000, got %d", s.DropPermille)
	}
	for i, fault := range s.Faults {
		if err := fault.validate(s.Nodes); err != nil {
			return fmt.Errorf("scenario: fault %d: %w", i, err)
		}
	}
	return nil
}

func (f Fault) validate(nodes int) error {
	if f.End != 0 && f.End <= f.Start {
		return fmt.Errorf("window end %d must be after start %d", f.End, f.Start)
	}
	check := func(field string, ids []uint32) error {
		if len(ids) == 0 {
			return fmt.Errorf("%s must list at least one node", field)
		}
		seen := make(map[uint32]bool, len(ids))
		for _, id := range ids {
			if id < 1 || int(id) > nodes {
				return fmt.Errorf("%s names node %d, outside 1..%d", field, id, nodes)
			}
			if seen[id] {
				return fmt.Errorf("%s names node %d twice", field, id)
			}
			seen[id] = true
		}
		return nil
	}
	switch f.Type {
	case "split":
		if err := check("a", f.A); err != nil {
			return err
		}
		if err := check("b", f.B); err != nil {
			return err
		}
		for _, id := range f.A {
			for _, other := range f.B {
				if id == other {
					// A node on both sides would make the split a no-op for
					// its traffic, which is never what the author meant.
					return fmt.Errorf("node %d appears on both sides of the split", id)
				}
			}
		}
	case "isolate":
		if err := check("nodes", f.Nodes); err != nil {
			return err
		}
	case "link":
		if err := check("from", []uint32{f.From}); err != nil {
			return err
		}
		if err := check("to", []uint32{f.To}); err != nil {
			return err
		}
		if f.From == f.To {
			return fmt.Errorf("a link failure needs two distinct nodes, got %d twice", f.From)
		}
	default:
		return fmt.Errorf("unknown type %q", f.Type)
	}
	return nil
}
