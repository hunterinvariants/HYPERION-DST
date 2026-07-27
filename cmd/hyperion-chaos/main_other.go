//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "hyperion-chaos requires Linux")
	os.Exit(2)
}
