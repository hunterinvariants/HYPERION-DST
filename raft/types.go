package raft

type State uint8

const (
	Follower State = iota
	Candidate
	Leader
)

type Entry struct {
	Term    uint64
	Command uint64
}

type MessageType uint8

const (
	MsgPreVote MessageType = iota
	MsgPreVoteResponse
	MsgRequestVote
	MsgRequestVoteResponse
	MsgAppend
	MsgAppendResponse
)

type Message struct {
	Type     MessageType
	From, To uint32
	Term     uint64
	LogIndex uint64
	LogTerm  uint64
	Entry    Entry
	HasEntry bool
	Commit   uint64
	Reject   bool
	Match    uint64
}
