package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/hunterinvariants/HYPERION-DST/storage/uring"
)

func main() {
	entries := flag.Uint("entries", 8, "io_uring submission queue entries")
	flag.Parse()
	if *entries == 0 || *entries > 32768 {
		fmt.Fprintln(os.Stderr, "entries must be between 1 and 32768")
		os.Exit(2)
	}
	fmt.Printf("platform=%s/%s go=%s entries=%d\n",
		runtime.GOOS, runtime.GOARCH, runtime.Version(), *entries)
	if err := uring.Probe(uint32(*entries)); err != nil {
		fmt.Fprintln(os.Stderr, "io_uring: FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("io_uring_setup: PASS")
}
