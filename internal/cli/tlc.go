package cli

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// TLCResult is the structured form of one TLC model-checking run.
//
// The numbers here are the ones the evidence documents in benchmarks/ quote,
// which is the reason this exists: reading them off a screen and retyping them
// into a report is how a digit goes missing.
type TLCResult struct {
	// Completed reports that TLC finished its search without finding an error.
	Completed bool `json:"completed"`
	// Violations names each invariant TLC reported as violated, plus
	// "deadlock" if it reported one.
	Violations []string `json:"violations"`

	StatesGenerated   uint64 `json:"statesGenerated"`
	DistinctStates    uint64 `json:"distinctStates"`
	StatesLeftOnQueue uint64 `json:"statesLeftOnQueue"`

	// Depth is the number of levels TLC reports for the complete state graph
	// search. Treat it as informational, not as a stable measurement: under
	// parallel search it can overshoot the true graph depth by one, because a
	// worker may reach the next level before the current one closes and TLC
	// records the maximum seen. Measured on this repository's model, four runs
	// gave 26, 25, 25 with -workers auto and 25 with -workers 1. The state
	// counts were identical across all four.
	//
	// For an evidence-grade depth, run the model check serially.
	Depth uint64 `json:"depth"`

	// CollisionOptimistic and CollisionActual are TLC's own estimates of the
	// probability that two distinct states shared a fingerprint and part of
	// the state space therefore went unchecked. They are why a TLC result is
	// bounded model checking and not a proof, so they belong in any report.
	//
	// CollisionActual varies between runs because TLC picks a fresh
	// fingerprint seed each time. Record it as a property of the run, never of
	// the model.
	CollisionOptimistic string `json:"collisionOptimistic,omitempty"`
	CollisionActual     string `json:"collisionActual,omitempty"`

	Duration string `json:"duration,omitempty"`
}

// ErrUnparsedTLC reports output that did not contain a recognizable result.
// Failing here is deliberate: reporting zeros for a run whose output could not
// be read would put fabricated numbers into an evidence document.
var ErrUnparsedTLC = errors.New("tlc: no state count found in the output")

var (
	tlcStates    = regexp.MustCompile(`([\d,]+) states generated, ([\d,]+) distinct states found, ([\d,]+) states left on queue`)
	tlcDepth     = regexp.MustCompile(`depth of the complete state graph search is (\d+)`)
	tlcComplete  = regexp.MustCompile(`Model checking completed\. No error has been found`)
	tlcInvariant = regexp.MustCompile(`Invariant (\S+) is violated`)
	tlcDeadlock  = regexp.MustCompile(`Deadlock reached`)
	tlcOptimist  = regexp.MustCompile(`calculated \(optimistic\):\s*val\s*=\s*(\S+)`)
	tlcActual    = regexp.MustCompile(`based on the actual fingerprints:\s*val\s*=\s*(\S+)`)
	tlcFinished  = regexp.MustCompile(`Finished in (.+?) at `)
)

// ParseTLC extracts the result from TLC's console output. It tolerates
// unrelated lines, because TLC interleaves progress reports and, on a
// violation, a full counterexample trace.
func ParseTLC(output string) (TLCResult, error) {
	var result TLCResult

	if m := tlcStates.FindStringSubmatch(output); m != nil {
		result.StatesGenerated = parseCount(m[1])
		result.DistinctStates = parseCount(m[2])
		result.StatesLeftOnQueue = parseCount(m[3])
	} else {
		return TLCResult{}, ErrUnparsedTLC
	}
	if m := tlcDepth.FindStringSubmatch(output); m != nil {
		result.Depth = parseCount(m[1])
	}
	if m := tlcOptimist.FindStringSubmatch(output); m != nil {
		result.CollisionOptimistic = m[1]
	}
	if m := tlcActual.FindStringSubmatch(output); m != nil {
		result.CollisionActual = m[1]
	}
	if m := tlcFinished.FindStringSubmatch(output); m != nil {
		result.Duration = strings.TrimSpace(m[1])
	}

	seen := make(map[string]bool)
	for _, m := range tlcInvariant.FindAllStringSubmatch(output, -1) {
		name := strings.TrimSuffix(m[1], ".")
		if !seen[name] {
			seen[name] = true
			result.Violations = append(result.Violations, name)
		}
	}
	if tlcDeadlock.MatchString(output) && !seen["deadlock"] {
		result.Violations = append(result.Violations, "deadlock")
	}

	// A run only counts as complete when TLC says so *and* nothing was
	// reported violated. Trusting the completion banner alone would let a
	// violated run be recorded as a pass if TLC ever printed both.
	result.Completed = tlcComplete.MatchString(output) && len(result.Violations) == 0
	return result, nil
}

// Exhaustive reports whether the search covered the whole bounded state space:
// TLC completed, nothing was violated, and no state was left unexplored.
func (r TLCResult) Exhaustive() bool {
	return r.Completed && len(r.Violations) == 0 && r.StatesLeftOnQueue == 0
}

func parseCount(s string) uint64 {
	n, err := strconv.ParseUint(strings.ReplaceAll(s, ",", ""), 10, 64)
	if err != nil {
		return 0
	}
	return n
}
