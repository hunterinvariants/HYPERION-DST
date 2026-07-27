package raft

// Membership represents either one voter set or a joint-consensus transition.
// During a transition, an index is committed only with a majority of both sets.
type Membership struct {
	Old []uint32
	New []uint32
}

func (m Membership) Joint() bool { return len(m.New) != 0 }

func (m Membership) CanCommit(acked map[uint32]bool) bool {
	if !majority(m.Old, acked) {
		return false
	}
	return !m.Joint() || majority(m.New, acked)
}

func (m Membership) Contains(id uint32) bool {
	return contains(m.Old, id) || contains(m.New, id)
}

func majority(voters []uint32, acked map[uint32]bool) bool {
	if len(voters) == 0 {
		return false
	}
	count := 0
	for _, id := range voters {
		if acked[id] {
			count++
		}
	}
	return count >= len(voters)/2+1
}

func contains(ids []uint32, target uint32) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
