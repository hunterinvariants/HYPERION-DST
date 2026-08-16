//go:build linux

package cli

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

func chaosCommand() Command {
	return Command{
		Name:    "chaos",
		Summary: "apply kernel fault injection inside a dedicated namespace",
		Run:     runChaos,
	}
}

func runChaos(args []string) int {
	flags := flag.NewFlagSet("hyperion-chaos", flag.ExitOnError)
	object := flags.String("bpf-object", "", "compiled hyperion_chaos.bpf.o")
	delay := flags.Duration("delay", 0, "netem delay")
	loss := flags.Float64("loss", 0, "netem loss percentage")
	confirm := flags.Bool("yes-really", false, "required safety acknowledgement")
	_ = flags.Parse(args)
	if !*confirm {
		fmt.Fprintln(os.Stderr, "refusing privileged network changes without -yes-really")
		return 2
	}
	plan := chaos.Plan{
		Namespace: "hyperion-chaos", HostVeth: "hyperion-host",
		PeerVeth: "hyperion-peer", HostCIDR: "192.0.2.1/30",
		PeerCIDR: "192.0.2.2/30", BPFObject: *object, Delay: *delay, LossPct: *loss,
	}
	controller, err := chaos.New(plan, chaos.ExecRunner{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := controller.Apply(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("chaos namespace active; test with: ping 192.0.2.2")
	fmt.Println("interrupt to detach programs and clean up")
	<-ctx.Done()
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := controller.Close(cleanupCtx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
