package dst

import "fmt"

// Injector programs a deterministic network fault.
//
// The engine consults injectors *after* it has drawn a message's random loss
// and delay from the seeded stream. Registering an injector therefore never
// shifts the schedule: the same seed produces the same draws with and without
// it, so a run with a fault and a run without one are directly comparable and
// any difference is attributable to the fault alone.
//
// Allow must be a pure function of its arguments and the injector's own
// configuration. An injector that consults wall-clock time, a map iteration, or
// a random source destroys reproducibility.
type Injector interface {
	// Name identifies the fault in drop accounting.
	Name() string
	// Allow reports whether this message may be delivered. delay is the
	// virtual time the engine drew for it, so an injector can decide based on
	// when the message would arrive rather than when it was sent.
	Allow(now uint64, from, to uint32, delay uint64) bool
}

// InjectorFunc adapts a plain function to Injector.
type InjectorFunc struct {
	Label string
	Fn    func(now uint64, from, to uint32, delay uint64) bool
}

func (f InjectorFunc) Name() string { return f.Label }

func (f InjectorFunc) Allow(now uint64, from, to uint32, delay uint64) bool {
	return f.Fn(now, from, to, delay)
}

// Split drops every message that crosses between the two groups, in both
// directions, modelling a network partition. Messages within a group and
// messages involving a node in neither group are unaffected.
func Split(a, b []uint32) Injector {
	left, right := membership(a), membership(b)
	return InjectorFunc{
		Label: fmt.Sprintf("split%v|%v", a, b),
		Fn: func(_ uint64, from, to uint32, _ uint64) bool {
			return !(left[from] && right[to]) && !(right[from] && left[to])
		},
	}
}

// Isolate drops every message to or from the node, modelling a host that is
// unreachable while still running.
func Isolate(nodes ...uint32) Injector {
	isolated := membership(nodes)
	return InjectorFunc{
		Label: fmt.Sprintf("isolate%v", nodes),
		Fn: func(_ uint64, from, to uint32, _ uint64) bool {
			return !isolated[from] && !isolated[to]
		},
	}
}

// Link drops messages in one direction only, modelling an asymmetric failure.
// These are the failures a symmetric partition model cannot express, and the
// ones consensus protocols most often get wrong.
func Link(from, to uint32) Injector {
	return InjectorFunc{
		Label: fmt.Sprintf("link%d->%d", from, to),
		Fn: func(_ uint64, source, target uint32, _ uint64) bool {
			return source != from || target != to
		},
	}
}

// During restricts an injector to the virtual time window [start, end). It
// makes heal-and-recover scenarios expressible without the caller mutating the
// engine mid-run.
func During(start, end uint64, inner Injector) Injector {
	return InjectorFunc{
		Label: fmt.Sprintf("%s@[%d,%d)", inner.Name(), start, end),
		Fn: func(now uint64, from, to uint32, delay uint64) bool {
			if now < start || now >= end {
				return true
			}
			return inner.Allow(now, from, to, delay)
		},
	}
}

func membership(nodes []uint32) map[uint32]bool {
	set := make(map[uint32]bool, len(nodes))
	for _, node := range nodes {
		set[node] = true
	}
	return set
}
