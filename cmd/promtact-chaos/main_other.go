//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "promtact-chaos requires Linux")
	os.Exit(2)
}
