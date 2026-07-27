package uring

import "testing"

func TestDirectIOAlignment(t *testing.T) {
	if !IsAligned(0, 4096, 4096) || !IsAligned(8192, 8192, 4096) {
		t.Fatal("aligned I/O rejected")
	}
	if IsAligned(1, 4096, 4096) || IsAligned(0, 4095, 4096) {
		t.Fatal("misaligned I/O accepted")
	}
}
