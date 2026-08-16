package wal_test

import (
	"path/filepath"
	"testing"

	"github.com/hunterinvariants/HYPERION-DST/storage/storagetest"
	"github.com/hunterinvariants/HYPERION-DST/storage/wal"
)

func TestMemoryDeviceConformance(t *testing.T) {
	storagetest.RunDeviceSuite(t, func(*testing.T) wal.Device {
		return wal.NewMemoryDevice(nil)
	})
}

func TestFileDeviceConformance(t *testing.T) {
	storagetest.RunDeviceSuite(t, func(t *testing.T) wal.Device {
		device, err := wal.OpenFileDevice(filepath.Join(t.TempDir(), "conformance.wal"))
		if err != nil {
			t.Fatalf("open file device: %v", err)
		}
		t.Cleanup(func() { device.Close() })
		return device
	})
}
