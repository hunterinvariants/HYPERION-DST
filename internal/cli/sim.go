package cli

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/hunterinvariants/hyperion/sim"
)

func simCommand() Command {
	return Command{
		Name:    "sim",
		Summary: "run one deterministic simulation and report its trace",
		Run:     runSim,
	}
}

func runSim(args []string) int {
	flags := flag.NewFlagSet("hyperion-sim", flag.ExitOnError)
	seedText := flags.String("seed", "0x4A2C", "deterministic seed (decimal or 0x-prefixed)")
	steps := flags.Uint64("steps", 10000, "virtual clock steps")
	nodes := flags.Int("nodes", 5, "cluster size")
	drop := flags.Int("drop-permille", 25, "messages dropped per thousand")
	delay := flags.Uint64("max-delay", 5, "maximum virtual message delay")
	_ = flags.Parse(args)
	seed, err := strconv.ParseInt(strings.TrimSpace(*seedText), 0, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid seed:", err)
		return 2
	}
	s := sim.New(sim.Config{Nodes: *nodes, Seed: seed, DropPermille: *drop, MaxDelay: *delay})
	var proposed uint64
	for i := uint64(1); i <= *steps; i++ {
		s.Step()
		if i%31 == 0 && s.Propose(i) {
			proposed++
		}
	}
	var maxCommit uint64
	for _, n := range s.Nodes {
		if n.Commit > maxCommit {
			maxCommit = n.Commit
		}
	}
	fmt.Printf("seed=%#x steps=%d leader=%d proposed=%d max_commit=%d trace=%s\n",
		seed, *steps, s.Leader(), proposed, maxCommit, s.TraceHash())
	return 0
}

func seedsCommand() Command {
	return Command{
		Name:    "seeds",
		Summary: "sweep a seed range for safety violations in parallel",
		Run:     runSeeds,
	}
}

type seedResult struct {
	seed int64
	err  error
}

func runSeeds(args []string) int {
	flags := flag.NewFlagSet("hyperion-seeds", flag.ExitOnError)
	first := flags.Int64("from", 1, "first seed, inclusive")
	last := flags.Int64("to", 1000, "last seed, inclusive")
	steps := flags.Uint64("steps", 2000, "virtual steps per seed")
	workers := flags.Int("workers", runtime.GOMAXPROCS(0), "parallel seed runners")
	_ = flags.Parse(args)
	if *first < 0 || *last < *first || *workers < 1 {
		fmt.Fprintln(os.Stderr, "invalid seed range or worker count")
		return 2
	}

	jobs := make(chan int64)
	results := make(chan seedResult, *workers)
	var wg sync.WaitGroup
	for range *workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seed := range jobs {
				results <- seedResult{seed: seed, err: sweepSeed(seed, *steps)}
			}
		}()
	}
	go func() {
		for seed := *first; seed <= *last; seed++ {
			jobs <- seed
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	var completed atomic.Uint64
	for r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "FAILED seed=%#x: %v\n", r.seed, r.err)
			return 1
		}
		completed.Add(1)
	}
	fmt.Printf("PASS seeds=%d range=%d..%d steps=%d\n", completed.Load(), *first, *last, *steps)
	return 0
}

func sweepSeed(seed int64, steps uint64) error {
	s := sim.New(sim.Config{Nodes: 5, Seed: seed, DropPermille: 50, MaxDelay: 5})
	for tick := uint64(1); tick <= steps; tick++ {
		s.Step()
		if tick%17 == 0 {
			s.Propose(uint64(seed)<<32 | tick)
		}
		if tick%101 == 0 {
			if err := s.CrashRestart(uint32((tick/101)%5 + 1)); err != nil {
				return err
			}
		}
		if err := s.CheckSafety(); err != nil {
			return fmt.Errorf("tick=%d trace=%s: %w", tick, s.TraceHash(), err)
		}
	}
	return nil
}
