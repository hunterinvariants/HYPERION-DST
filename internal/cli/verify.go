package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

func verifyCommand() Command {
	return Command{
		Name:    "verify",
		Summary: "run the bounded TLA+ model check and report the result",
		Run:     runVerify,
	}
}

// runVerify drives verification/run-tlc.sh rather than invoking TLC itself.
// That script owns the pinned tla2tools version, its checksum, and the model
// checker's flags, and it is what CI and the recorded evidence invoke. A second
// invocation path here would be a second thing to keep correct.
func runVerify(args []string) int {
	flags := flag.NewFlagSet("promtact verify", flag.ExitOnError)
	script := flags.String("script", "verification/run-tlc.sh", "model check script to run")
	asJSON := flags.Bool("json", false, "print the result as JSON")
	quiet := flags.Bool("quiet", false, "suppress the model checker's own output")
	timeout := flags.Duration("timeout", 30*time.Minute, "abort the run after this long")
	_ = flags.Parse(args)

	if _, err := os.Stat(*script); err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var captured bytes.Buffer
	command := exec.CommandContext(ctx, "bash", *script)
	// TLC's own output goes to stderr as it runs, so a long check shows
	// progress, while stdout stays clean for the machine-readable result.
	var sink io.Writer = &captured
	if !*quiet {
		sink = io.MultiWriter(&captured, os.Stderr)
	}
	command.Stdout, command.Stderr = sink, sink

	runErr := command.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		fmt.Fprintf(os.Stderr, "verify: model check exceeded %s\n", *timeout)
		return 2
	}
	var notFound *exec.Error
	if errors.As(runErr, &notFound) {
		fmt.Fprintf(os.Stderr, "verify: cannot run %s: %v\n", *script, notFound)
		fmt.Fprintln(os.Stderr, "verify: the model check needs bash and a Java runtime")
		return 2
	}

	result, parseErr := ParseTLC(captured.String())
	if parseErr != nil {
		// The run produced output that cannot be read as a result. Reporting
		// zeros here would put invented numbers into an evidence document, so
		// this fails instead.
		fmt.Fprintf(os.Stderr, "verify: %v\n", parseErr)
		if runErr != nil {
			fmt.Fprintf(os.Stderr, "verify: the model check also exited with: %v\n", runErr)
		}
		return 2
	}

	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "verify: %v\n", err)
			return 2
		}
	} else {
		fmt.Printf("states_generated=%d distinct=%d left_on_queue=%d depth=%d violations=%d",
			result.StatesGenerated, result.DistinctStates,
			result.StatesLeftOnQueue, result.Depth, len(result.Violations))
		if result.CollisionOptimistic != "" || result.CollisionActual != "" {
			fmt.Printf(" collision_optimistic=%s collision_actual=%s",
				result.CollisionOptimistic, result.CollisionActual)
		}
		if result.Duration != "" {
			fmt.Printf(" duration=%q", result.Duration)
		}
		fmt.Println()
		for _, violation := range result.Violations {
			fmt.Printf("violated=%s\n", violation)
		}
	}

	switch {
	case len(result.Violations) > 0:
		return 1
	case runErr != nil:
		fmt.Fprintf(os.Stderr, "verify: the model check exited with: %v\n", runErr)
		return 1
	case !result.Exhaustive():
		// Completing with states still queued means the bound was hit rather
		// than the space exhausted, which is not the claim the evidence makes.
		fmt.Fprintln(os.Stderr, "verify: the search did not exhaust the bounded state space")
		return 1
	}
	return 0
}
