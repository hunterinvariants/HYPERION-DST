//go:build !linux || !amd64

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "promtact-raw-bench requires linux/amd64")
	os.Exit(2)
}
