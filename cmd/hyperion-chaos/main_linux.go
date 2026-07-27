//go:build linux

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hunterinvariants/HYPERION-DST/chaos"
)

func main() {
	object := flag.String("bpf-object", "", "compiled hyperion_chaos.bpf.o")
	delay := flag.Duration("delay", 0, "netem delay")
	loss := flag.Float64("loss", 0, "netem loss percentage")
	confirm := flag.Bool("yes-really", false, "required safety acknowledgement")
	flag.Parse()
	if !*confirm {
		fmt.Fprintln(os.Stderr, "refusing privileged network changes without -yes-really")
		os.Exit(2)
	}
	plan := chaos.Plan{
		Namespace: "hyperion-chaos", HostVeth: "hyperion-host",
		PeerVeth: "hyperion-peer", BPFObject: *object, Delay: *delay, LossPct: *loss,
	}
	controller, err := chaos.New(plan, chaos.ExecRunner{})
	if err != nil {
		fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := controller.Apply(ctx); err != nil {
		fatal(err)
	}
	fmt.Println("chaos namespace active; interrupt to clean up")
	<-ctx.Done()
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := controller.Close(cleanupCtx); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
