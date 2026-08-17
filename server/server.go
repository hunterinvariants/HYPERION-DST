package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hunterinvariants/promtact/protocol"
	"github.com/hunterinvariants/promtact/raft"
	"github.com/hunterinvariants/promtact/statemachine"
	"github.com/hunterinvariants/promtact/storage/raftstore"
	"github.com/hunterinvariants/promtact/storage/wal"
)

type Config struct {
	ID              uint32
	Peers           map[uint32]string
	PeerAddress     string
	ClientAddress   string
	HTTPAddress     string
	DataDir         string
	ElectionTicks   uint64
	TickInterval    time.Duration
	QueueCapacity   int
	RequestTimeout  time.Duration
	SnapshotEntries uint64
}

type request struct {
	value protocol.ClientRequest
	reply chan protocol.ClientResponse
}

type pendingKey struct{ client, request uint64 }

type pendingMutation struct {
	waiters []chan protocol.ClientResponse
}

type pendingRead struct {
	context uint64
	request request
}

type Metrics struct {
	Requests   atomic.Uint64
	Committed  atomic.Uint64
	Duplicates atomic.Uint64
	Busy       atomic.Uint64
	PeerErrors atomic.Uint64
}

type Server struct {
	config   Config
	node     *raft.Node
	machine  *statemachine.Machine
	device   *wal.FileDevice
	inbound  chan raft.Message
	requests chan request
	pending  map[pendingKey]*pendingMutation
	reads    []pendingRead
	metrics  Metrics
	ready    atomic.Bool
	commit   atomic.Uint64
	listener net.Listener
	peer     net.Listener
	http     *http.Server
	wg       sync.WaitGroup
}

func Open(config Config) (*Server, error) {
	if config.ID == 0 || config.PeerAddress == "" || config.ClientAddress == "" || config.DataDir == "" {
		return nil, errors.New("server: incomplete configuration")
	}
	if config.ElectionTicks == 0 {
		config.ElectionTicks = 10 + uint64(config.ID)
	}
	if config.TickInterval == 0 {
		config.TickInterval = 50 * time.Millisecond
	}
	if config.QueueCapacity == 0 {
		config.QueueCapacity = 1024
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 5 * time.Second
	}
	if err := os.MkdirAll(config.DataDir, 0o700); err != nil {
		return nil, err
	}
	device, err := wal.OpenFileDevice(filepath.Join(config.DataDir, "raft.wal"))
	if err != nil {
		return nil, err
	}
	store, recovery, err := raftstore.Open(device, filepath.Join(config.DataDir, "snapshot.bin"))
	if err != nil {
		device.Close()
		return nil, err
	}
	peerIDs := make([]uint32, 0, len(config.Peers))
	for id := range config.Peers {
		if id != config.ID {
			peerIDs = append(peerIDs, id)
		}
	}
	var node *raft.Node
	machine := statemachine.New()
	if recovery.Snapshot.LastIndex != 0 {
		node = raft.NewRecoveredNodeWithSnapshot(config.ID, peerIDs, config.ElectionTicks, store, recovery.Hard, recovery.Snapshot, recovery.Suffix)
		if len(recovery.Snapshot.State) != 0 {
			machine, err = statemachine.Restore(recovery.Snapshot.State)
			if err != nil {
				device.Close()
				return nil, err
			}
		}
	} else {
		log := append([]raft.Entry{{}}, recovery.Suffix...)
		node = raft.NewRecoveredNode(config.ID, peerIDs, config.ElectionTicks, store, recovery.Hard, log)
	}
	for _, entry := range node.ApplyEntries(nil) {
		if _, err := machine.Apply(entry); err != nil {
			device.Close()
			return nil, err
		}
	}
	return &Server{
		config: config, node: node, machine: machine, device: device,
		inbound:  make(chan raft.Message, config.QueueCapacity),
		requests: make(chan request, config.QueueCapacity),
		pending:  make(map[pendingKey]*pendingMutation),
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	var err error
	s.peer, err = net.Listen("tcp", s.config.PeerAddress)
	if err != nil {
		return err
	}
	s.listener, err = net.Listen("tcp", s.config.ClientAddress)
	if err != nil {
		s.peer.Close()
		return err
	}
	if s.config.HTTPAddress != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", s.health)
		mux.HandleFunc("/metrics", s.serveMetrics)
		s.http = &http.Server{Addr: s.config.HTTPAddress, Handler: mux, ReadHeaderTimeout: 2 * time.Second}
		s.wg.Add(1)
		go func() { defer s.wg.Done(); _ = s.http.ListenAndServe() }()
	}
	s.ready.Store(true)
	s.wg.Add(2)
	go s.acceptPeers(ctx)
	go s.acceptClients(ctx)
	err = s.loop(ctx)
	s.ready.Store(false)
	s.peer.Close()
	s.listener.Close()
	if s.http != nil {
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = s.http.Shutdown(shutdown)
		cancel()
	}
	s.wg.Wait()
	closeErr := s.device.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (s *Server) loop(ctx context.Context) error {
	ticker := time.NewTicker(s.config.TickInterval)
	defer ticker.Stop()
	outbound := make([]raft.Message, 0, 128)
	for {
		select {
		case <-ctx.Done():
			return nil
		case message := <-s.inbound:
			s.node.Step(message)
		case incoming := <-s.requests:
			s.handleRequest(incoming)
		case <-ticker.C:
			s.node.Tick()
		}
		if err := s.node.StorageError(); err != nil {
			return fmt.Errorf("server: raft storage: %w", err)
		}
		outbound = s.node.Drain(outbound[:0])
		for _, message := range outbound {
			go s.sendPeer(message)
		}
		s.apply()
		s.completeReads()
		s.commit.Store(s.node.Commit)
	}
}

func (s *Server) handleRequest(incoming request) {
	s.metrics.Requests.Add(1)
	req := incoming.value
	if req.Operation == protocol.ClientStatus {
		incoming.reply <- s.response(protocol.StatusOK, req, 0)
		return
	}
	if s.node.State != raft.Leader {
		incoming.reply <- s.response(protocol.StatusNotLeader, req, 0)
		return
	}
	if req.Operation == protocol.ClientGet {
		if len(s.reads) != 0 {
			s.metrics.Busy.Add(1)
			incoming.reply <- s.response(protocol.StatusBusy, req, 0)
			return
		}
		contextID := (req.ClientID << 32) ^ req.RequestID
		if contextID == 0 || !s.node.StartReadIndex(contextID) {
			incoming.reply <- s.response(protocol.StatusBusy, req, 0)
			return
		}
		s.reads = append(s.reads, pendingRead{contextID, incoming})
		return
	}
	if req.ClientID == 0 || req.RequestID == 0 {
		incoming.reply <- s.response(protocol.StatusInvalid, req, 0)
		return
	}
	if result, ok, err := s.machine.Cached(req.ClientID, req.RequestID); err != nil {
		incoming.reply <- s.response(protocol.StatusInvalid, req, 0)
		return
	} else if ok {
		s.metrics.Duplicates.Add(1)
		incoming.reply <- s.response(protocol.StatusOK, req, result.Value)
		return
	}
	key := pendingKey{req.ClientID, req.RequestID}
	if pending := s.pending[key]; pending != nil {
		pending.waiters = append(pending.waiters, incoming.reply)
		return
	}
	var operation raft.CommandOp
	switch req.Operation {
	case protocol.ClientPut:
		operation = raft.CommandPut
	case protocol.ClientDelete:
		operation = raft.CommandDelete
	default:
		incoming.reply <- s.response(protocol.StatusInvalid, req, 0)
		return
	}
	if !s.node.ProposeRequest(operation, req.ClientID, req.RequestID, req.Key, req.Value) {
		incoming.reply <- s.response(protocol.StatusBusy, req, 0)
		return
	}
	s.pending[key] = &pendingMutation{waiters: []chan protocol.ClientResponse{incoming.reply}}
}

func (s *Server) apply() {
	for _, entry := range s.node.ApplyEntries(nil) {
		result, err := s.machine.Apply(entry)
		status := protocol.StatusOK
		if err != nil {
			status = protocol.StatusInvalid
		}
		key := pendingKey{entry.ClientID, entry.RequestID}
		if pending := s.pending[key]; pending != nil {
			response := protocol.ClientResponse{Status: status, Leader: s.node.Leader, RequestID: entry.RequestID, Value: result.Value, Commit: s.node.Commit}
			for _, waiter := range pending.waiters {
				waiter <- response
			}
			delete(s.pending, key)
		}
		s.metrics.Committed.Add(1)
	}
	if s.config.SnapshotEntries != 0 && s.node.Applied-s.node.BaseIndex >= s.config.SnapshotEntries {
		_ = s.node.Compact(s.node.Applied, s.machine.Snapshot())
	}
}

func (s *Server) completeReads() {
	remaining := s.reads[:0]
	for _, read := range s.reads {
		index, ok := s.node.ReadIndex(read.context)
		if !ok || s.node.Applied < index {
			remaining = append(remaining, read)
			continue
		}
		value, found := s.machine.Get(read.request.value.Key)
		status := protocol.StatusOK
		if !found {
			status = protocol.StatusNotFound
		}
		read.request.reply <- s.response(status, read.request.value, value)
	}
	s.reads = remaining
}

func (s *Server) response(status protocol.Status, request protocol.ClientRequest, value uint64) protocol.ClientResponse {
	return protocol.ClientResponse{Status: status, Leader: s.node.Leader, RequestID: request.RequestID, Value: value, Commit: s.node.Commit}
}

func (s *Server) acceptPeers(ctx context.Context) {
	defer s.wg.Done()
	for {
		connection, err := s.peer.Accept()
		if err != nil {
			return
		}
		go func() {
			defer connection.Close()
			kind, payload, err := protocol.ReadFrame(connection)
			if err != nil || kind != protocol.KindPeer {
				return
			}
			message, err := protocol.DecodePeer(payload)
			if err != nil || message.To != s.config.ID {
				return
			}
			select {
			case s.inbound <- message:
			case <-ctx.Done():
			default:
				s.metrics.Busy.Add(1)
			}
		}()
	}
}

func (s *Server) acceptClients(ctx context.Context) {
	defer s.wg.Done()
	for {
		connection, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleClient(ctx, connection)
	}
}

func (s *Server) handleClient(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(s.config.RequestTimeout))
	kind, payload, err := protocol.ReadFrame(connection)
	if err != nil || kind != protocol.KindClientRequest {
		return
	}
	value, err := protocol.DecodeClientRequest(payload)
	if err != nil {
		_ = protocol.WriteFrame(connection, protocol.KindClientResponse, protocol.EncodeClientResponse(protocol.ClientResponse{Status: protocol.StatusInvalid}))
		return
	}
	reply := make(chan protocol.ClientResponse, 1)
	select {
	case s.requests <- request{value, reply}:
	default:
		s.metrics.Busy.Add(1)
		_ = protocol.WriteFrame(connection, protocol.KindClientResponse, protocol.EncodeClientResponse(protocol.ClientResponse{Status: protocol.StatusBusy, RequestID: value.RequestID}))
		return
	}
	select {
	case response := <-reply:
		_ = protocol.WriteFrame(connection, protocol.KindClientResponse, protocol.EncodeClientResponse(response))
	case <-ctx.Done():
	case <-time.After(s.config.RequestTimeout):
		_ = protocol.WriteFrame(connection, protocol.KindClientResponse, protocol.EncodeClientResponse(protocol.ClientResponse{Status: protocol.StatusTimeout, RequestID: value.RequestID}))
	}
}

func (s *Server) sendPeer(message raft.Message) {
	address, ok := s.config.Peers[message.To]
	if !ok {
		return
	}
	connection, err := net.DialTimeout("tcp", address, s.config.TickInterval)
	if err != nil {
		s.metrics.PeerErrors.Add(1)
		return
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(s.config.TickInterval))
	if err := protocol.WriteFrame(connection, protocol.KindPeer, protocol.EncodePeer(message)); err != nil {
		s.metrics.PeerErrors.Add(1)
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) serveMetrics(w http.ResponseWriter, _ *http.Request) {
	values := []struct {
		name  string
		value uint64
	}{
		{"promtact_requests_total", s.metrics.Requests.Load()},
		{"promtact_committed_total", s.metrics.Committed.Load()},
		{"promtact_duplicates_total", s.metrics.Duplicates.Load()},
		{"promtact_busy_total", s.metrics.Busy.Load()},
		{"promtact_peer_errors_total", s.metrics.PeerErrors.Load()},
		{"promtact_commit_index", s.commit.Load()},
	}
	for _, metric := range values {
		_, _ = fmt.Fprintln(w, metric.name+" "+strconv.FormatUint(metric.value, 10))
	}
}
