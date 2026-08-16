package dst

import (
	"math/rand"
	"testing"
)

// drain pops the whole queue and reports the order it produced.
func drain(e *Engine[int]) []event[int] {
	out := make([]event[int], 0, len(e.queue))
	for len(e.queue) > 0 {
		out = append(out, e.pop())
	}
	return out
}

func assertOrdered(t *testing.T, got []event[int]) {
	t.Helper()
	for i := 1; i < len(got); i++ {
		if earlier(got[i], got[i-1]) {
			t.Fatalf("event %d (at=%d seq=%d) came after (at=%d seq=%d)",
				i, got[i].at, got[i].seq, got[i-1].at, got[i-1].seq)
		}
	}
}

// TestHeapPopsInScheduleOrder is the property the whole optimization rests on:
// the heap must yield exactly the order a linear scan for the smallest due
// message produced. Ties on due time are broken by sequence number, and those
// ties are the common case at zero delay.
func TestHeapPopsInScheduleOrder(t *testing.T) {
	for _, maxAt := range []uint64{0, 1, 4, 64} {
		rng := rand.New(rand.NewSource(int64(maxAt) + 1))
		e := &Engine[int]{}
		for i := 0; i < 500; i++ {
			at := uint64(0)
			if maxAt > 0 {
				at = uint64(rng.Int63n(int64(maxAt + 1)))
			}
			e.seq++
			e.push(event[int]{at: at, seq: e.seq, msg: i})
		}
		got := drain(e)
		if len(got) != 500 {
			t.Fatalf("drained %d events, want 500", len(got))
		}
		assertOrdered(t, got)
	}
}

// TestIsolateKeepsTheHeapValid guards the one operation that removes arbitrary
// elements. A queue left unheapified would deliver messages out of order, and
// the failure would look like a consensus bug rather than a container bug.
func TestIsolateKeepsTheHeapValid(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	e := &Engine[int]{}
	for i := 0; i < 400; i++ {
		e.seq++
		e.push(event[int]{
			at:   uint64(rng.Int63n(32)),
			seq:  e.seq,
			from: uint32(rng.Int63n(5) + 1),
			to:   uint32(rng.Int63n(5) + 1),
			msg:  i,
		})
	}
	before := len(e.queue)
	dropped := e.Isolate(3)
	if dropped == 0 {
		t.Fatal("no traffic involved node 3, so this run proves nothing")
	}
	if len(e.queue) != before-dropped {
		t.Fatalf("queue length %d does not match %d - %d", len(e.queue), before, dropped)
	}
	got := drain(e)
	assertOrdered(t, got)
	for _, ev := range got {
		if ev.from == 3 || ev.to == 3 {
			t.Fatalf("event %d->%d survived isolation", ev.from, ev.to)
		}
	}
}

func TestPopReleasesTheMessage(t *testing.T) {
	// The queue slice keeps its capacity, so a popped message would stay
	// reachable and pin whatever it references.
	e := &Engine[int]{}
	e.push(event[int]{at: 1, seq: 1, msg: 42})
	e.pop()
	if got := e.queue[:1][0].msg; got != 0 {
		t.Fatalf("popped slot still holds %v", got)
	}
}
