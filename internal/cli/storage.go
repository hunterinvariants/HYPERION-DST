package cli

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/hunterinvariants/hyperion/backup"
	"github.com/hunterinvariants/hyperion/storage/uring"
)

func probeCommand() Command {
	return Command{
		Name:    "probe",
		Summary: "check that this host supports the io_uring data path",
		Run:     runProbe,
	}
}

func runProbe(args []string) int {
	flags := flag.NewFlagSet("hyperion-probe", flag.ExitOnError)
	entries := flags.Uint("entries", 8, "io_uring submission queue entries")
	_ = flags.Parse(args)
	if *entries == 0 || *entries > 32768 {
		fmt.Fprintln(os.Stderr, "entries must be between 1 and 32768")
		return 2
	}
	fmt.Printf("platform=%s/%s go=%s entries=%d\n",
		runtime.GOOS, runtime.GOARCH, runtime.Version(), *entries)
	if err := uring.Probe(uint32(*entries)); err != nil {
		fmt.Fprintln(os.Stderr, "io_uring: FAIL:", err)
		return 1
	}
	fmt.Println("io_uring_setup: PASS")
	return 0
}

func backupCommand() Command {
	return Command{
		Name:    "backup",
		Summary: "create or restore an offline node data directory",
		Run:     runBackup,
	}
}

func runBackup(args []string) int {
	flags := flag.NewFlagSet("hyperion-backup", flag.ExitOnError)
	mode := flags.String("mode", "create", "create or restore")
	data := flags.String("data-dir", "", "offline node data directory")
	image := flags.String("backup-dir", "", "new backup directory")
	_ = flags.Parse(args)
	var err error
	switch *mode {
	case "create":
		err = backup.Create(*data, *image)
	case "restore":
		err = backup.Restore(*image, *data)
	default:
		err = fmt.Errorf("invalid mode %q", *mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hyperion-backup:", err)
		return 1
	}
	return 0
}

func uringBenchCommand() Command {
	return Command{
		Name:    "uring-bench",
		Summary: "measure durable io_uring writes against a regular file",
		Run:     runUringBench,
	}
}

func runUringBench(args []string) int {
	flags := flag.NewFlagSet("hyperion-uring-bench", flag.ExitOnError)
	path := flags.String("path", "/tmp/hyperion-uring-bench.dat", "dedicated regular test file")
	operations := flags.Int("operations", 1000, "durable block writes")
	keep := flags.Bool("keep", false, "keep benchmark file")
	_ = flags.Parse(args)
	if *operations < 1 || *operations > 1_000_000 {
		fmt.Fprintln(os.Stderr, "operations must be between 1 and 1000000")
		return 1
	}
	writer, err := uring.OpenDurableWriter(*path, 32, uring.DefaultAlignment)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
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
			fmt.Fprintln(os.Stderr, fmt.Errorf("operation %d: %w", i, err))
			return 1
		}
		latencies = append(latencies, time.Since(start))
	}
	total := time.Since(started)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	fmt.Printf("path=%s operations=%d block=%d total=%s ops_per_sec=%.0f p50=%s p99=%s max=%s\n",
		*path, *operations, uring.DefaultAlignment, total,
		float64(*operations)/total.Seconds(),
		percentile(latencies, 50), percentile(latencies, 99), latencies[len(latencies)-1])
	return 0
}

// percentile reports the nearest-rank latency at the requested percentile.
func percentile(values []time.Duration, p int) time.Duration {
	index := (len(values)*p + 99) / 100
	if index > 0 {
		index--
	}
	return values[index]
}
