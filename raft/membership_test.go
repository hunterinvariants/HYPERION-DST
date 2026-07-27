package raft

import "testing"

func TestJointConsensusRequiresBothMajorities(t *testing.T) {
	config := Membership{Old: []uint32{1, 2, 3}, New: []uint32{2, 3, 4}}
	tests := []struct {
		name  string
		acked map[uint32]bool
		want  bool
	}{
		{"old only", map[uint32]bool{1: true, 2: true}, false},
		{"new only", map[uint32]bool{2: true, 4: true}, false},
		{"intersection", map[uint32]bool{2: true, 3: true}, true},
		{"cross majorities", map[uint32]bool{1: true, 2: true, 4: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.CanCommit(tt.acked); got != tt.want {
				t.Fatalf("CanCommit = %v, want %v", got, tt.want)
			}
		})
	}
}
