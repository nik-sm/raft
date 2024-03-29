package raft

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"strings"
)

// AppendEntriesStruct holds the input arguments for the RPC AppendEntries
type AppendEntriesStruct struct {
	Term         Term
	LeaderID     HostID
	PrevLogIndex LogIndex
	PrevLogTerm  Term
	Entries      []LogEntry
	LeaderCommit LogIndex
}

func (ae AppendEntriesStruct) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("AEStruct. {Term: %d, LeaderID: %d, PrevLogIndex: %d, PrevLogTerm: %d, LeaderCommit: %d, Entries: [",
		ae.Term, ae.LeaderID, ae.PrevLogIndex, ae.PrevLogTerm, ae.LeaderCommit))
	for idx, entry := range ae.Entries {
		sb.WriteString(fmt.Sprintf("{idx: %d, entry: %s}\n", idx, entry.String()))
	}
	sb.WriteString("]}")
	return sb.String()
}

// The leader uses this during heartbeats (or after receiving from client) to slowly bring all other logs up to date, and to maintain leadership.
// For each follower,
//   if they are up to date, we send an empty message
//   if they are trailing behind, we send them the log entry at nextIndex[hostID].
//     if they reject this entry, we decrement their index
func (r *RaftNode) heartbeatAppendEntriesRPC() {
	if r.verbose {
		log.Println("heartbeatAppendEntriesRPC")
	}

	leaderLastLogIdx := r.getLastLogIndex()

	for hostID := range r.hosts {
		if hostID != r.id {

			// By default, we assume the peer is up-to-date and will get an empty list
			var entries []LogEntry = make([]LogEntry, 0)

			// Check their "nextIndex" against our last log index
			theirNextIdx := r.nextIndex[hostID]
			if leaderLastLogIdx >= theirNextIdx {
				for i := theirNextIdx; i <= leaderLastLogIdx; i++ {
					entries = append(entries, r.Log[i])
				}
			}

			go r.appendEntriesRPC(hostID, entries)
		}
	}
}

func (r *RaftNode) appendEntriesRPC(hostID HostID, entries []LogEntry) {
	p := r.hosts[hostID]
	prevLogIdx := max(0, r.nextIndex[hostID]-1)
	prevLogTerm := r.Log[prevLogIdx].Term

	args := AppendEntriesStruct{
		Term:         r.CurrentTerm,
		LeaderID:     r.id,
		PrevLogIndex: prevLogIdx,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: r.commitIndex}

	response := RPCResponse{}
	conn, err := rpc.Dial("tcp", fmt.Sprintf("%s:%d", p.IP, p.Port))
	if err != nil {
		log.Printf("WARNING: problem dialing peer: %s, err: %s", p.String(), err)
		response = RPCResponse{Term: r.CurrentTerm, Success: false, LeaderID: r.currentLeader}
	} else {
		err = conn.Call("RaftNode.AppendEntries", args, &response)
		if err != nil {
			log.Println("WARNING:", err)
			response = RPCResponse{Term: r.CurrentTerm, Success: false, LeaderID: r.currentLeader}
		}
	}

	if r.verbose {
		log.Printf("appendEntriesRPC result. host: %d, success: %t", hostID, response.Success)
	}
	r.incomingChan <- incomingMsg{msgType: appendEntries, hostID: hostID, response: response, aeLength: len(entries)}
}

// RequestVoteStruct holds the parameters used during the Vote() RPC
type RequestVoteStruct struct {
	Term        Term
	CandidateID HostID
	LastLogIdx  LogIndex
	LastLogTerm Term
}

func (rv RequestVoteStruct) String() string {
	return fmt.Sprintf("Term: %d, CandidateID: %d, LastLogIdx: %d, LastLogTerm: %d", rv.Term, rv.CandidateID, rv.LastLogIdx, rv.LastLogTerm)
}

// Send a requestVoteRPC to all peers, storing their responses
func (r *RaftNode) multiRequestVoteRPC() {
	if r.verbose {
		log.Println("MultiRequestVote")
	}
	for hostID := range r.hosts {
		if hostID != r.id {
			go r.requestVoteRPC(hostID)
		}
	}
}

// Send out an RPC to the method "Vote" on the remote host
func (r *RaftNode) requestVoteRPC(hostID HostID) {
	p := r.hosts[hostID]
	args := RequestVoteStruct{
		Term:        r.CurrentTerm,
		CandidateID: r.id,
		LastLogIdx:  r.getLastLogIndex(),
		LastLogTerm: r.getLastLogTerm()}
	// Fill response with default values in case of early return
	response := RPCResponse{}

	conn, err := rpc.Dial("tcp", fmt.Sprintf("%s:%d", p.IP, p.Port))
	if err != nil {
		// We do not crash here, because we don't care if that peer might be down
		log.Printf("WARNING: problem dialing peer: %s. err: %s", p.String(), err)
		response = RPCResponse{Term: r.CurrentTerm, Success: false, LeaderID: r.currentLeader}
	} else {
		err = conn.Call("RaftNode.Vote", args, &response)
		if err != nil {
			// We crash here, because we do not tolerate RPC errors
			panic(fmt.Sprintf("requestVoteRPC: %s", err))
		}
	}
	if r.verbose {
		log.Printf("received vote reply from hostID: %d, response.success=%t", hostID, response.Success)
	}
	r.incomingChan <- incomingMsg{msgType: vote, hostID: hostID, response: response}
}

// ClientSerialNum is a unique, monotonically increasing integer that each client attaches to their requests
// The state machine includes a map of clients and their most recently executed serial num
// If a request is received with a stale ClientSerialNum, the leader can immediately reply "success"
type ClientSerialNum int

// ClientDataStruct holds the inputs that a client sends when they want to store information in the statemachine
type ClientDataStruct struct {
	ClientID        ClientID
	Data            ClientData
	ClientSerialNum ClientSerialNum
}

func (r *RaftNode) recvDaemon() {
	rpcServer := rpc.NewServer()
	err := rpcServer.Register(r)
	if err != nil {
		panic(err)
	}
	l, err := net.Listen("tcp", ":"+fmt.Sprintf("%d", r.recvPort))
	if err != nil {
		panic(fmt.Sprintf("listen error: %s", err))
	}
	for {
		select {
		case <-r.quitChan:
			log.Println("QUIT RECV DAEMON")
			return
		default:
			conn, err := l.Accept()
			if err != nil {
				panic(fmt.Sprintf("accept error: %s", err))
			}
			go rpcServer.ServeConn(conn)
		}
	}
}
