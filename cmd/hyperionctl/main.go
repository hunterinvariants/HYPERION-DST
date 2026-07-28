package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/hunterinvariants/HYPERION-DST/protocol"
)

func main() {
	address := flag.String("address", "127.0.0.1:9201", "client endpoint")
	operation := flag.String("operation", "status", "put, delete, get, or status")
	client := flag.Uint64("client", 1, "stable client ID")
	request := flag.Uint64("request", 1, "monotonic request ID")
	key := flag.Uint64("key", 0, "integer key")
	value := flag.Uint64("value", 0, "integer value")
	timeout := flag.Duration("timeout", 5*time.Second, "request timeout")
	flag.Parse()
	ops := map[string]protocol.ClientOp{
		"put": protocol.ClientPut, "delete": protocol.ClientDelete,
		"get": protocol.ClientGet, "status": protocol.ClientStatus,
	}
	op, ok := ops[*operation]
	if !ok {
		fmt.Fprintln(os.Stderr, "hyperionctl: invalid operation")
		os.Exit(2)
	}
	response, err := protocol.Call(*address, protocol.ClientRequest{
		Operation: op, ClientID: *client, RequestID: *request, Key: *key, Value: *value,
	}, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hyperionctl:", err)
		os.Exit(1)
	}
	fmt.Printf("status=%d leader=%d request=%d value=%d commit=%d\n",
		response.Status, response.Leader, response.RequestID, response.Value, response.Commit)
	if response.Status != protocol.StatusOK && response.Status != protocol.StatusNotFound {
		os.Exit(3)
	}
}
