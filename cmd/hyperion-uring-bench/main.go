package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/hunterinvariants/HYPERION-DST/storage/uring"
)

func main() {
	path := flag.String("path", "/tmp/hyperion-uring-bench.dat", "dedicated regular test file")
	operations := flag.Int("operations", 1000, "durable block writes")
	keep := flag.Bool("keep", false, "keep benchmark file")
	flag.Parse()
	if *operations < 1 || *operations > 1_000_000 {
		fatal(fmt.Errorf("operations must be between 1 and 1000000"))
	}
	writer, err := uring.OpenDurableWriter(*path, 32, uring.DefaultAlignment)
	if err != nil {
		fatal(err)
	}
	if !*keep {
		defer os.Remove(*path)
	}
	defer writer.Close()

	payload := make([]byte, 48)
	latencies := make([]time.Duration, 0, *operations)
	started := time.Now()
	for i := 0; i < *operations; i++ {
		start := time.Now()
		if err := writer.AppendDurable(uint64(i), payload); err != nil {
			fatal(fmt.Errorf("operation %d: %w", i, err))
		}
		latencies = append(latencies, time.Since(start))
	}
	total := time.Since(started)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	fmt.Printf("path=%s operations=%d block=%d total=%s ops_per_sec=%.0f p50=%s p99=%s max=%s\n",
		*path, *operations, uring.DefaultAlignment, total,
		float64(*operations)/total.Seconds(),
		percentile(latencies, 50), percentile(latencies, 99), latencies[len(latencies)-1])
}

func percentile(values []time.Duration, percentile int) time.Duration {
	index := (len(values)*percentile + 99) / 100
	if index > 0 {
		index--
	}
	return values[index]
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
