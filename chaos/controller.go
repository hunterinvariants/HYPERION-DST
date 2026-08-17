// Package chaos controls kernel fault injection inside an isolated netns.
package chaos

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

var safeName = regexp.MustCompile(`^promtact-[a-zA-Z0-9_-]{1,32}$`)

type Plan struct {
	Namespace string
	HostVeth  string
	PeerVeth  string
	HostCIDR  string
	PeerCIDR  string
	BPFObject string
	Delay     time.Duration
	LossPct   float64
}

func (p Plan) Validate() error {
	if !safeName.MatchString(p.Namespace) ||
		!safeName.MatchString(p.HostVeth) ||
		!safeName.MatchString(p.PeerVeth) {
		return errors.New("chaos: namespace and interfaces must start with promtact-")
	}
	if p.BPFObject == "" {
		return errors.New("chaos: BPF object is required")
	}
	hostIP, hostNet, err := net.ParseCIDR(p.HostCIDR)
	if err != nil {
		return errors.New("chaos: invalid host CIDR")
	}
	peerIP, peerNet, err := net.ParseCIDR(p.PeerCIDR)
	if err != nil || !hostNet.Contains(peerIP) || !peerNet.Contains(hostIP) || hostIP.Equal(peerIP) {
		return errors.New("chaos: peer CIDR must be distinct and in the same subnet")
	}
	if p.Delay < 0 || p.Delay > time.Minute || p.LossPct < 0 || p.LossPct > 100 {
		return errors.New("chaos: delay/loss outside safety bounds")
	}
	return nil
}

type Runner interface {
	Run(context.Context, string, ...string) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, output)
	}
	return nil
}

type Controller struct {
	plan   Plan
	runner Runner
	active bool
}

func New(plan Plan, runner Runner) (*Controller, error) {
	if plan.HostCIDR == "" {
		plan.HostCIDR = "192.0.2.1/30"
	}
	if plan.PeerCIDR == "" {
		plan.PeerCIDR = "192.0.2.2/30"
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	if runner == nil {
		return nil, errors.New("chaos: runner is required")
	}
	return &Controller{plan: plan, runner: runner}, nil
}

func (c *Controller) Apply(ctx context.Context) error {
	if c.active {
		return errors.New("chaos: plan already active")
	}
	p := c.plan
	steps := [][]string{
		{"ip", "netns", "add", p.Namespace},
		{"ip", "link", "add", p.HostVeth, "type", "veth", "peer", "name", p.PeerVeth},
		{"ip", "link", "set", p.PeerVeth, "netns", p.Namespace},
		{"ip", "addr", "add", p.HostCIDR, "dev", p.HostVeth},
		{"ip", "link", "set", p.HostVeth, "up"},
		{"ip", "netns", "exec", p.Namespace, "ip", "link", "set", "lo", "up"},
		{"ip", "netns", "exec", p.Namespace, "ip", "addr", "add", p.PeerCIDR, "dev", p.PeerVeth},
		{"ip", "netns", "exec", p.Namespace, "ip", "link", "set", p.PeerVeth, "up"},
		{"ip", "netns", "exec", p.Namespace, "ip", "link", "set", p.PeerVeth,
			"xdp", "obj", p.BPFObject, "sec", "xdp"},
		{"ip", "netns", "exec", p.Namespace, "tc", "qdisc", "add", "dev", p.PeerVeth, "clsact"},
		{"ip", "netns", "exec", p.Namespace, "tc", "filter", "add", "dev", p.PeerVeth,
			"egress", "bpf", "direct-action", "obj", p.BPFObject, "sec", "classifier"},
	}
	if p.Delay > 0 || p.LossPct > 0 {
		steps = append(steps, []string{"ip", "netns", "exec", p.Namespace, "tc",
			"qdisc", "add", "dev", p.PeerVeth, "root", "netem",
			"delay", p.Delay.String(), "loss", strconv.FormatFloat(p.LossPct, 'f', 3, 64) + "%"})
	}
	for _, step := range steps {
		if err := c.runner.Run(ctx, step[0], step[1:]...); err != nil {
			_ = c.cleanup(context.Background())
			return err
		}
	}
	c.active = true
	return nil
}

func (c *Controller) Close(ctx context.Context) error {
	err := c.cleanup(ctx)
	c.active = false
	return err
}

func (c *Controller) cleanup(ctx context.Context) error {
	// Deleting the namespace detaches XDP/TC and destroys the peer. Deleting
	// the host veth is idempotent cleanup for partially completed setup.
	errNS := c.runner.Run(ctx, "ip", "netns", "del", c.plan.Namespace)
	errLink := c.runner.Run(ctx, "ip", "link", "del", c.plan.HostVeth)
	if errNS != nil && errLink != nil {
		return errors.Join(errNS, errLink)
	}
	return nil
}
