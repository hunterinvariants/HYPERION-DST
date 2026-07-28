//go:build linux && amd64

package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hunterinvariants/HYPERION-DST/storage/uring"
)

const blockSize = uring.DefaultAlignment

func main() {
	deviceFlag := flag.String("device", "", "dedicated disposable block device")
	confirm := flag.String("confirm-destroy", "", "must equal ERASE:<canonical-device>")
	expectedSize := flag.Uint64("expected-size", 0, "required exact device size in bytes")
	operations := flag.Int("operations", 10_000, "durable writes")
	startBlock := flag.Uint64("start-block", 256, "first 4096-byte block")
	flag.Parse()

	device, err := filepath.EvalSymlinks(*deviceFlag)
	if err != nil {
		fatal(fmt.Errorf("canonical device: %w", err))
	}
	if device == "/dev/sda" || !strings.HasPrefix(device, "/dev/") {
		fatal(errors.New("refusing system or non-/dev path"))
	}
	if *confirm != "ERASE:"+device {
		fatal(fmt.Errorf("confirmation must be exactly ERASE:%s", device))
	}
	if *expectedSize == 0 || *operations < 1 || *operations > 1_000_000 {
		fatal(errors.New("expected-size and operations safety bounds are required"))
	}
	isBlock, err := uring.IsBlockDevice(device)
	if err != nil || !isBlock {
		fatal(fmt.Errorf("not a block device: %w", err))
	}
	size, err := uring.BlockDeviceSize(device)
	if err != nil {
		fatal(err)
	}
	if size != *expectedSize {
		fatal(fmt.Errorf("device size changed: got %d, expected %d", size, *expectedSize))
	}
	if err := verifyUnused(device); err != nil {
		fatal(err)
	}
	lastByte := (*startBlock + uint64(*operations)) * blockSize
	if lastByte > size {
		fatal(fmt.Errorf("write range ends at %d beyond device size %d", lastByte, size))
	}

	writer, err := uring.OpenDurableWriter(device, 32, blockSize)
	if err != nil {
		fatal(err)
	}
	defer writer.Close()

	payload := make([]byte, 48)
	copy(payload, "HYPERION-RAW-BENCH")
	latencies := make([]time.Duration, 0, *operations)
	started := time.Now()
	for operation := 0; operation < *operations; operation++ {
		binary.LittleEndian.PutUint64(payload[24:32], uint64(operation))
		start := time.Now()
		if err := writer.AppendDurable(*startBlock+uint64(operation), payload); err != nil {
			fatal(fmt.Errorf("operation %d: %w", operation, err))
		}
		latencies = append(latencies, time.Since(start))
	}
	total := time.Since(started)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	fmt.Printf("device=%s size=%d range=%d..%d operations=%d block=%d total=%s ops_per_sec=%.0f p50=%s p99=%s max=%s\n",
		device, size, *startBlock, *startBlock+uint64(*operations)-1,
		*operations, blockSize, total, float64(*operations)/total.Seconds(),
		percentile(latencies, 50), percentile(latencies, 99), latencies[len(latencies)-1])
}

func verifyUnused(device string) error {
	var stat syscall.Stat_t
	if err := syscall.Stat(device, &stat); err != nil {
		return err
	}
	major, minor := deviceNumbers(uint64(stat.Rdev))
	needle := strconv.FormatUint(uint64(major), 10) + ":" + strconv.FormatUint(uint64(minor), 10)
	mountInfo, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(mountInfo)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 2 && fields[2] == needle {
			return fmt.Errorf("device is mounted according to mountinfo")
		}
	}
	swaps, err := os.ReadFile("/proc/swaps")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(swaps), "\n") {
		if strings.HasPrefix(line, device+" ") || strings.HasPrefix(line, device+"\t") {
			return errors.New("device is active swap")
		}
	}
	base := filepath.Base(device)
	matches, err := filepath.Glob("/sys/class/block/" + base + "*")
	if err != nil {
		return err
	}
	if len(matches) != 1 || filepath.Base(matches[0]) != base {
		return fmt.Errorf("device has partitions or ambiguous sysfs children: %v", matches)
	}
	return scanner.Err()
}

func deviceNumbers(dev uint64) (uint32, uint32) {
	major := uint32((dev>>8)&0xfff | (dev >> 32 & 0xfffff000))
	minor := uint32(dev&0xff | (dev >> 12 & 0xffffff00))
	return major, minor
}

func percentile(values []time.Duration, p int) time.Duration {
	index := (len(values)*p + 99) / 100
	if index > 0 {
		index--
	}
	return values[index]
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "REFUSED/FAILED:", err)
	os.Exit(1)
}
