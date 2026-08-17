package scenario_test

import (
	"strings"
	"testing"

	"github.com/hunterinvariants/promtact/dst/scenario"
)

const valid = `{
  "name": "leader partition",
  "seed": "0x4A2C",
  "nodes": 5,
  "steps": 1000,
  "dropPermille": 25,
  "maxDelay": 5,
  "proposeEvery": 17,
  "restartEvery": 101,
  "faults": [
    {"type": "split", "a": [1], "b": [2, 3, 4, 5], "start": 200, "end": 700},
    {"type": "isolate", "nodes": [3]},
    {"type": "link", "from": 1, "to": 2}
  ]
}`

func TestReadAcceptsACompleteScenario(t *testing.T) {
	spec, err := scenario.Read(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if spec.Name != "leader partition" || spec.Nodes != 5 || spec.Steps != 1000 {
		t.Fatalf("parsed %+v", spec)
	}
	seed, err := spec.ParsedSeed()
	if err != nil || seed != 0x4A2C {
		t.Fatalf("seed = %#x, %v", seed, err)
	}
	config, err := spec.EngineConfig()
	if err != nil {
		t.Fatalf("engine config: %v", err)
	}
	if config.Seed != 0x4A2C || config.DropPermille != 25 || config.MaxDelay != 5 {
		t.Fatalf("engine config = %+v", config)
	}
	injectors, err := spec.Injectors()
	if err != nil {
		t.Fatalf("injectors: %v", err)
	}
	if len(injectors) != 3 {
		t.Fatalf("built %d injectors, want 3", len(injectors))
	}
	// The windowed fault must carry its bounds into the name, otherwise drop
	// accounting cannot distinguish it from an unbounded one.
	if !strings.Contains(injectors[0].Name(), "[200,700)") {
		t.Fatalf("windowed injector is named %q", injectors[0].Name())
	}
}

func TestReadRejectsMalformedScenarios(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{"unknown field", `{"seed":"1","nodes":3,"steps":10,"noSuchField":1}`, "noSuchField"},
		{"bad seed", `{"seed":"not-a-number","nodes":3,"steps":10}`, "invalid seed"},
		{"zero nodes", `{"seed":"1","nodes":0,"steps":10}`, "nodes must be"},
		{"too many nodes", `{"seed":"1","nodes":65,"steps":10}`, "nodes must be"},
		{"zero steps", `{"seed":"1","nodes":3,"steps":0}`, "steps must be positive"},
		{"loss above 1000", `{"seed":"1","nodes":3,"steps":10,"dropPermille":1001}`, "dropPermille"},
		{"unknown fault", `{"seed":"1","nodes":3,"steps":10,"faults":[{"type":"meteor"}]}`, "unknown type"},
		{"node out of range", `{"seed":"1","nodes":3,"steps":10,"faults":[{"type":"isolate","nodes":[9]}]}`, "outside 1..3"},
		{"empty group", `{"seed":"1","nodes":3,"steps":10,"faults":[{"type":"split","a":[1],"b":[]}]}`, "at least one node"},
		{"overlapping split", `{"seed":"1","nodes":3,"steps":10,"faults":[{"type":"split","a":[1,2],"b":[2,3]}]}`, "both sides"},
		{"duplicate node", `{"seed":"1","nodes":3,"steps":10,"faults":[{"type":"isolate","nodes":[1,1]}]}`, "twice"},
		{"self link", `{"seed":"1","nodes":3,"steps":10,"faults":[{"type":"link","from":2,"to":2}]}`, "distinct"},
		{"backwards window", `{"seed":"1","nodes":3,"steps":10,"faults":[{"type":"isolate","nodes":[1],"start":90,"end":10}]}`, "must be after"},
		{"trailing content", `{"seed":"1","nodes":3,"steps":10} {"seed":"2"}`, "unexpected content"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := scenario.Read(strings.NewReader(test.body))
			if err == nil {
				t.Fatal("accepted a malformed scenario")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q does not mention %q", err, test.want)
			}
		})
	}
}

// TestUnboundedFaultNeedsNoWindow pins that omitting end applies the fault for
// the whole run rather than for zero steps.
func TestUnboundedFaultNeedsNoWindow(t *testing.T) {
	spec, err := scenario.Read(strings.NewReader(
		`{"seed":"1","nodes":3,"steps":10,"faults":[{"type":"isolate","nodes":[2]}]}`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	injectors, err := spec.Injectors()
	if err != nil {
		t.Fatalf("injectors: %v", err)
	}
	if !injectors[0].Allow(0, 1, 3, 0) {
		t.Fatal("unrelated traffic was blocked")
	}
	if injectors[0].Allow(0, 1, 2, 0) {
		t.Fatal("traffic to the isolated node was allowed")
	}
	if injectors[0].Allow(1_000_000, 1, 2, 0) {
		t.Fatal("the fault expired despite having no window")
	}
}
