package raft

type State uint8

const (
	Follower State = iota
	Candidate
	Leader
)

type EntryKind uint8

const (
	EntryNormal EntryKind = iota
	EntryJointConfig
	EntryFinalConfig
)

type Entry struct {
	Term      uint64
	Command   uint64
	Kind      EntryKind
	OldVoters uint64
	NewVoters uint64
}

type MessageType uint8

const (
	MsgPreVote MessageType = iota
	MsgPreVoteResponse
	MsgRequestVote
	MsgRequestVoteResponse
	MsgAppend
	MsgAppendResponse
	MsgTimeoutNow
	MsgInstallSnapshot
	MsgInstallSnapshotResponse
)

type Message struct {
	Type          MessageType
	From, To      uint32
	Term          uint64
	LogIndex      uint64
	LogTerm       uint64
	Entry         Entry
	HasEntry      bool
	Commit        uint64
	Reject        bool
	Match         uint64
	Context       uint64
	SnapshotIndex uint64
	SnapshotTerm  uint64
	Snapshot      []byte
	SnapshotOld   uint64
	SnapshotNew   uint64
}
