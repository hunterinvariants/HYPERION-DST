package paxos

import (
	"fmt"

	"github.com/hunterinvariants/hyperion/dst"
)

// SafetyInvariants returns the properties the engine checks after every step.
//
// The first is the reason Paxos exists. The second and third are local
// consistency properties: they cannot catch an agreement failure, but they
// catch a broken acceptor much closer to the mistake, which is worth more
// during development than a violation reported a thousand steps later.
func (c *Cluster) SafetyInvariants() []dst.Invariant {
	return []dst.Invariant{
		dst.InvariantFunc{Label: "at most one value chosen", Fn: c.checkSingleChoice},
		dst.InvariantFunc{Label: "acceptor never accepts below its promise", Fn: c.checkPromiseRespected},
		dst.InvariantFunc{Label: "acceptance history is monotonic", Fn: c.checkHistoryMonotonic},
	}
}

// checkSingleChoice is the agreement property: across the whole run, at most
// one distinct value may reach a quorum of acceptances.
func (c *Cluster) checkSingleChoice() error {
	if values := c.chosenValues(); len(values) > 1 {
		return fmt.Errorf("values %v were both chosen", values)
	}
	return nil
}

// checkPromiseRespected requires an acceptor's accepted number never to sit
// above the number it has promised on.
func (c *Cluster) checkPromiseRespected() error {
	for _, id := range c.ids {
		node := c.nodes[id]
		num, _, ok := node.Accepted()
		if ok && num > node.Promised() {
			return fmt.Errorf("node %d accepted at %d but promised only %d", id, num, node.Promised())
		}
	}
	return nil
}

// checkHistoryMonotonic requires each acceptor's acceptances to be
// non-decreasing in proposal number. Going backwards would mean an acceptor
// honoured a proposal it had already promised past.
func (c *Cluster) checkHistoryMonotonic() error {
	for _, id := range c.ids {
		history := c.nodes[id].Acceptances()
		for i := 1; i < len(history); i++ {
			if history[i].Num < history[i-1].Num {
				return fmt.Errorf("node %d accepted %d after %d", id, history[i].Num, history[i-1].Num)
			}
		}
	}
	return nil
}
