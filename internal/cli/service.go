package cli

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

	"github.com/hunterinvariants/hyperion/protocol"
	"github.com/hunterinvariants/hyperion/server"
)

func serveCommand() Command {
	return Command{
		Name:    "serve",
		Summary: "run one node of a replicated cluster",
		Run:     runServe,
	}
}

func runServe(args []string) int {
	flags := flag.NewFlagSet("hyperiond", flag.ExitOnError)
	var config server.Config
	var peers string
	cluster := flags.String("config", "", "cluster file describing every node; selects this one by -id")
	flags.Uint64Var(&config.ElectionTicks, "election-ticks", 0, "base election timeout in ticks")
	id := flags.Uint("id", 0, "node ID (1..64)")
	flags.StringVar(&config.PeerAddress, "peer-address", "", "peer listen address")
	flags.StringVar(&config.ClientAddress, "client-address", "", "client listen address")
	flags.StringVar(&config.HTTPAddress, "http-address", "", "health and metrics listen address")
	flags.StringVar(&config.DataDir, "data-dir", "", "durable node data directory")
	flags.StringVar(&peers, "peers", "", "comma-separated ID=host:port peers")
	flags.DurationVar(&config.TickInterval, "tick", 50*time.Millisecond, "Raft tick interval")
	flags.IntVar(&config.QueueCapacity, "queue-capacity", 1024, "bounded inbound queue")
	flags.DurationVar(&config.RequestTimeout, "request-timeout", 5*time.Second, "client request timeout")
	flags.Uint64Var(&config.SnapshotEntries, "snapshot-entries", 10000, "committed entries between snapshots")
	_ = flags.Parse(args)

	if *cluster != "" {
		// A cluster file supplies everything except which node this process
		// is. Silently ignoring a flag the operator also passed would start a
		// node with settings they did not intend, so conflicting flags are an
		// error rather than a preference.
		var conflicting []string
		flags.Visit(func(f *flag.Flag) {
			if f.Name != "config" && f.Name != "id" {
				conflicting = append(conflicting, "-"+f.Name)
			}
		})
		if len(conflicting) > 0 {
			return serveFatalf("-config supplies %s; pass them in the file or drop -config",
				strings.Join(conflicting, ", "))
		}
		if *id == 0 {
			return serveFatalf("-config needs -id to say which node this process is")
		}
		spec, err := server.LoadSpec(*cluster)
		if err != nil {
			return serveFatalf("%v", err)
		}
		config, err = spec.ConfigFor(uint32(*id))
		if err != nil {
			return serveFatalf("%v", err)
		}
	} else {
		config.ID = uint32(*id)
		config.Peers = make(map[uint32]string)
		for _, item := range strings.Split(peers, ",") {
			if item == "" {
				continue
			}
			parts := strings.SplitN(item, "=", 2)
			if len(parts) != 2 {
				return serveFatalf("invalid peer %q", item)
			}
			value, err := strconv.ParseUint(parts[0], 10, 32)
			if err != nil || value == 0 {
				return serveFatalf("invalid peer ID %q", parts[0])
			}
			config.Peers[uint32(value)] = parts[1]
		}
	}

	instance, err := server.Open(config)
	if err != nil {
		return serveFatalf("open: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := instance.Run(ctx); err != nil {
		return serveFatalf("run: %v", err)
	}
	return 0
}

func serveFatalf(format string, values ...any) int {
	fmt.Fprintf(os.Stderr, "hyperiond: "+format+"\n", values...)
	return 1
}

func ctlCommand() Command {
	return Command{
		Name:    "ctl",
		Summary: "send one client request to a cluster endpoint",
		Run:     runCtl,
	}
}

func runCtl(args []string) int {
	flags := flag.NewFlagSet("hyperionctl", flag.ExitOnError)
	address := flags.String("address", "127.0.0.1:9201", "client endpoint")
	operation := flags.String("operation", "status", "put, delete, get, or status")
	client := flags.Uint64("client", 1, "stable client ID")
	request := flags.Uint64("request", 1, "monotonic request ID")
	key := flags.Uint64("key", 0, "integer key")
	value := flags.Uint64("value", 0, "integer value")
	timeout := flags.Duration("timeout", 5*time.Second, "request timeout")
	_ = flags.Parse(args)
	ops := map[string]protocol.ClientOp{
		"put": protocol.ClientPut, "delete": protocol.ClientDelete,
		"get": protocol.ClientGet, "status": protocol.ClientStatus,
	}
	op, ok := ops[*operation]
	if !ok {
		fmt.Fprintln(os.Stderr, "hyperionctl: invalid operation")
		return 2
	}
	response, err := protocol.Call(*address, protocol.ClientRequest{
		Operation: op, ClientID: *client, RequestID: *request, Key: *key, Value: *value,
	}, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hyperionctl:", err)
		return 1
	}
	fmt.Printf("status=%d leader=%d request=%d value=%d commit=%d\n",
		response.Status, response.Leader, response.RequestID, response.Value, response.Commit)
	if response.Status != protocol.StatusOK && response.Status != protocol.StatusNotFound {
		return 3
	}
	return 0
}
