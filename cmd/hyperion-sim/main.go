package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hunterinvariants/HYPERION-DST/sim"
)

func main() {
	seedText := flag.String("seed", "0x4A2C", "deterministic seed (decimal or 0x-prefixed)")
	steps := flag.Uint64("steps", 10000, "virtual clock steps")
	nodes := flag.Int("nodes", 5, "cluster size")
	drop := flag.Int("drop-permille", 25, "messages dropped per thousand")
	delay := flag.Uint64("max-delay", 5, "maximum virtual message delay")
	flag.Parse()
	seed, err := strconv.ParseInt(strings.TrimSpace(*seedText), 0, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid seed:", err)
		os.Exit(2)
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
}
