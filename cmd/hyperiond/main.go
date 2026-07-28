package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hunterinvariants/HYPERION-DST/server"
)

func main() {
	var config server.Config
	var peers string
	flag.Uint64Var(&config.ElectionTicks, "election-ticks", 0, "base election timeout in ticks")
	id := flag.Uint("id", 0, "node ID (1..64)")
	flag.StringVar(&config.PeerAddress, "peer-address", "", "peer listen address")
	flag.StringVar(&config.ClientAddress, "client-address", "", "client listen address")
	flag.StringVar(&config.HTTPAddress, "http-address", "", "health and metrics listen address")
	flag.StringVar(&config.DataDir, "data-dir", "", "durable node data directory")
	flag.StringVar(&peers, "peers", "", "comma-separated ID=host:port peers")
	flag.DurationVar(&config.TickInterval, "tick", 50*time.Millisecond, "Raft tick interval")
	flag.IntVar(&config.QueueCapacity, "queue-capacity", 1024, "bounded inbound queue")
	flag.DurationVar(&config.RequestTimeout, "request-timeout", 5*time.Second, "client request timeout")
	flag.Uint64Var(&config.SnapshotEntries, "snapshot-entries", 10000, "committed entries between snapshots")
	flag.Parse()
	config.ID = uint32(*id)
	config.Peers = make(map[uint32]string)
	for _, item := range strings.Split(peers, ",") {
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			fatalf("invalid peer %q", item)
		}
		value, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil || value == 0 {
			fatalf("invalid peer ID %q", parts[0])
		}
		config.Peers[uint32(value)] = parts[1]
	}
	instance, err := server.Open(config)
	if err != nil {
		fatalf("open: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := instance.Run(ctx); err != nil {
		fatalf("run: %v", err)
	}
}

func fatalf(format string, values ...any) {
	fmt.Fprintf(os.Stderr, "hyperiond: "+format+"\n", values...)
	os.Exit(1)
}
