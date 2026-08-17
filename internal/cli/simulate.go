package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/hunterinvariants/promtact/dst"
	"github.com/hunterinvariants/promtact/dst/raftcluster"
	"github.com/hunterinvariants/promtact/dst/scenario"
	"github.com/hunterinvariants/promtact/raft"
)

func simulateCommand() Command {
	return Command{
		Name:    "simulate",
		Summary: "run a declared scenario file against the Raft cluster",
		Run:     runSimulate,
	}
}

func runSimulate(args []string) int {
	flags := flag.NewFlagSet("promtact simulate", flag.ExitOnError)
	path := flags.String("config", "", "scenario file")
	_ = flags.Parse(args)
	if *path == "" {
		fmt.Fprintln(os.Stderr, "simulate: -config is required")
		return 2
	}
	spec, err := scenario.Load(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simulate:", err)
		return 2
	}
	config, err := spec.EngineConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "simulate:", err)
		return 2
	}
	injectors, err := spec.Injectors()
	if err != nil {
		fmt.Fprintln(os.Stderr, "simulate:", err)
		return 2
	}

	cluster := raftcluster.New(spec.Nodes)
	engine := dst.New[raft.Message](config, cluster, cluster)
	engine.Watch(cluster.SafetyInvariants()...)
	engine.Inject(injectors...)

	var proposed uint64
	for tick := uint64(1); tick <= spec.Steps; tick++ {
		if err := engine.StepChecked(); err != nil {
			fmt.Fprintln(os.Stderr, "simulate:", err)
			return 1
		}
		if spec.ProposeEvery != 0 && tick%spec.ProposeEvery == 0 {
			if cluster.Propose(tick) {
				engine.Collect()
				proposed++
			}
		}
		if spec.RestartEvery != 0 && tick%spec.RestartEvery == 0 {
			id := uint32((tick/spec.RestartEvery)%uint64(spec.Nodes) + 1)
			if err := cluster.Restart(id); err != nil {
				fmt.Fprintf(os.Stderr, "simulate: restart node %d: %v\n", id, err)
				return 1
			}
			engine.Isolate(id)
		}
		if err := engine.CheckInvariants(); err != nil {
			fmt.Fprintln(os.Stderr, "simulate:", err)
			return 1
		}
	}

	var maxCommit uint64
	for _, id := range cluster.Nodes() {
		if commit := cluster.Node(id).Commit; commit > maxCommit {
			maxCommit = commit
		}
	}
	name := spec.Name
	if name == "" {
		name = *path
	}
	fmt.Printf("scenario=%q seed=%s nodes=%d steps=%d leader=%d proposed=%d max_commit=%d trace=%s\n",
		name, spec.Seed, spec.Nodes, spec.Steps, cluster.Leader(), proposed, maxCommit, engine.TraceHash())
	// A fault that never fired makes the run prove less than the file claims,
	// so report the counts rather than leaving the reader to assume.
	for _, injector := range injectors {
		fmt.Printf("fault=%q dropped=%d\n", injector.Name(), engine.InjectedDrops()[injector.Name()])
	}
	return 0
}
