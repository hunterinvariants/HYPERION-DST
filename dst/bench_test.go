package dst_test

import (
	"testing"

	"github.com/hunterinvariants/promtact/dst"
)

// The scheduling cost is dominated by how many messages are in flight at once,
// which is what a wide topology at a high delay bound produces. These
// benchmarks measure the queue, not the protocol: the ring's own work per
// message is a handful of field writes.
func benchmarkSchedule(b *testing.B, nodes int, period, maxDelay uint64) {
	b.ReportAllocs()
	for b.Loop() {
		r := newRing(nodes, period)
		engine := dst.New[ping](dst.Config{Seed: 1, MaxDelay: maxDelay}, r, r)
		engine.Run(500)
	}
}

func BenchmarkScheduleNoDelay(b *testing.B)    { benchmarkSchedule(b, 5, 2, 0) }
func BenchmarkScheduleSmallDelay(b *testing.B) { benchmarkSchedule(b, 5, 2, 5) }
func BenchmarkScheduleWideDelay(b *testing.B)  { benchmarkSchedule(b, 16, 1, 32) }
func BenchmarkScheduleDeepQueue(b *testing.B)  { benchmarkSchedule(b, 32, 1, 128) }
