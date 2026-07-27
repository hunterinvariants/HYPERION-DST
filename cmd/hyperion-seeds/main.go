package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/hunterinvariants/HYPERION-DST/sim"
)

type result struct {
	seed int64
	err  error
}

func main() {
	first := flag.Int64("from", 1, "first seed, inclusive")
	last := flag.Int64("to", 1000, "last seed, inclusive")
	steps := flag.Uint64("steps", 2000, "virtual steps per seed")
	workers := flag.Int("workers", runtime.GOMAXPROCS(0), "parallel seed runners")
	flag.Parse()
	if *first < 0 || *last < *first || *workers < 1 {
		fmt.Fprintln(os.Stderr, "invalid seed range or worker count")
		os.Exit(2)
	}

	jobs := make(chan int64)
	results := make(chan result, *workers)
	var wg sync.WaitGroup
	for range *workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seed := range jobs {
				results <- result{seed: seed, err: run(seed, *steps)}
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
			os.Exit(1)
		}
		completed.Add(1)
	}
	fmt.Printf("PASS seeds=%d range=%d..%d steps=%d\n", completed.Load(), *first, *last, *steps)
}

func run(seed int64, steps uint64) error {
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
