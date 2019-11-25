package raft

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// TODO - could reduce usages of map and use slices instead where possible

type ClientID int // TODO - if we only have a single client, maybe this can be simplified to "type client peer"
type HostID int
type Term HostID

func (t Term) String() string {
	return fmt.Sprintf("%d", t)
}

type RaftNodeState int

const (
	follower  RaftNodeState = iota
	leader    RaftNodeState = iota
	candidate RaftNodeState = iota
)

// Peer represents the network information for a RaftNode
type peer struct {
	IP       net.IP
	Port     int
	Hostname string
}

func (p peer) String() string {
	return fmt.Sprintf("Peer IP: %s, Port: %d, Hostname: %s", p.IP.String(), p.Port, p.Hostname)
}

type HostMap map[HostID]peer
type ClientMap map[ClientID]peer

type hostStringMap map[HostID]string
type clientStringMap map[ClientID]string

// Convenient temp storage during elections
type electionResults map[HostID]bool

func (e electionResults) String() string {
	var sb strings.Builder
	for host, vote := range e {
		sb.WriteString(fmt.Sprintf("%d=%t,", host, vote))
	}
	return sb.String()
}

// State Machine
type StateMachine struct {
	clientSerialNums map[ClientID]ClientSerialNum // The most recently executed serial number for each client
	contents         []ClientData
}

func NewStateMachine() StateMachine {
	return StateMachine{clientSerialNums: make(map[ClientID]ClientSerialNum), contents: make([]ClientData, 0, 0)}
}

func (s StateMachine) String() string {
	var sb strings.Builder
	sb.WriteString("clientSerialNums: {")
	for client, serialNum := range s.clientSerialNums {
		sb.WriteString(fmt.Sprintf("%d: %d,", client, serialNum))
	}
	sb.WriteString("}. contents: {")
	for idx, item := range s.contents {
		sb.WriteString(fmt.Sprintf("idx: %d, item: %s,", idx, item))
	}
	sb.WriteString("}")
	return sb.String()
}

// Log
type Log struct {
	clientSerialNums map[ClientID]ClientSerialNum
	contents         []LogEntry
}

func NewLog() Log {
	return Log{make(map[ClientID]ClientSerialNum), make([]LogEntry, 0, 0)}
}

func (r RaftNode) haveNewerSerialNum(le LogEntry) (bool, ClientResponse) {
	mostRecent := r.stateMachine.clientSerialNums[le.clientID]
	if int(mostRecent) >= int(le.clientSerialNum) {
		// NOTE - TODO - the client should keep sending the item with this serial num until it lands.
		// Therefore, if we have seen a higher serial num, the client MUST have successfully submitted this item,
		// and it will exist in our log
		// NOTE - if we do log compaction, we might have thrown away the previous log entry
		return true, r.log.getPrevResponse(le)
	} else {
		return false, ClientResponse{}
	}
}

func (l Log) getPrevResponse(le LogEntry) ClientResponse {
	for _, entry := range l.contents {
		if entry.clientID == le.clientID && entry.clientSerialNum == le.clientSerialNum {
			return entry.clientResponse
		}
	}
	log.Fatal("Should have had previous response!")
	return ClientResponse{}
}

// TODO - only this append function should be called
// Therefore we should split the package, and only this one should be exported
func (r *RaftNode) append(le LogEntry) {
	r.log.append(le)
	r.stateMachine.append(le)
}

func (l *Log) append(le LogEntry) {
	l.contents = append(l.contents, le)
}

func (s *StateMachine) append(le LogEntry) {
	mostRecentSeen := s.clientSerialNums[le.clientID]
	if int(mostRecentSeen) < int(le.clientSerialNum) {
		s.clientSerialNums[le.clientID] = le.clientSerialNum
	}
	// NOTE - if we used a more sophisticated type for ClientData,
	// this is where we would need logic to apply the data to the State Machine
	s.contents = append(s.contents, le.clientData)
}

func (l Log) String() string {
	var sb strings.Builder
	sb.WriteString("log.clientSerialNums:\n")
	for client, num := range l.clientSerialNums {
		sb.WriteString(fmt.Sprintf("%d=%d", client, num))
	}

	sb.WriteString("\nlog.contents:\n")
	for logIdx, logEntry := range l.contents {
		sb.WriteString(fmt.Sprintf("%d={%s},", logIdx, logEntry.String()))
	}
	return sb.String()
}

type LogIndex int
type LogEntry struct {
	term       Term
	clientData ClientData
	// TODO - the following 3 fields are here for convenience
	// They might belong to the StateMachine instead of the Log
	clientID        ClientID        // The client who sent this data
	clientSerialNum ClientSerialNum // The unique number the client used to identify this data
	clientResponse  ClientResponse  // The response that was given to this client
}
type ClientData string // NOTE - could make contents a state machine update

func (le LogEntry) String() string {
	return fmt.Sprintf("term: %s, clientData: %s, clientID: %d, clientSerialNum: %d, clientResponse: %s", le.term.String(), le.clientData, le.clientID, le.clientSerialNum, le.clientResponse)
}

// During AppendEntries, we need to traverse in a set order
// Therefore, we must use a slice of entries that specify their destination index
type LogAppend struct {
	idx   LogIndex
	entry LogEntry
}
type LogAppendList []LogAppend

// RPC
// Response after RPC from another RaftNode
type RPCResponse struct {
	Term    Term
	Success bool
}

func (r RPCResponse) String() string {
	return fmt.Sprintf("ClientResponse Term=%d, Success=%t", r.Term, r.Success)
}

// Response after RPC from client
type ClientResponse struct {
	Success bool // if success == false, then client should retry using 'leader'
	Leader  HostID
}

func (c ClientResponse) String() string {
	return fmt.Sprintf("ClientResponse Success=%t, Leader=%d", c.Success, c.Leader)
}

// RaftNode
type RaftNode struct {
	id    HostID        // id of this node
	state RaftNodeState // follower, leader, or candidate

	// Persistent State
	currentTerm  Term   // latest term server has seen
	votedFor     HostID // ID of candidate that received vote in current term (or null if none) // TODO - null? -1?
	log          Log
	stateMachine StateMachine

	// Volatile State
	commitIndex   LogIndex
	lastApplied   LogIndex
	currentLeader HostID
	// notice that a Term can have 0 leaders, so this should not be included inside Term type

	// Volatile State (leader only, reset after each election)
	nextIndex  map[HostID]LogIndex // index of next log entry to send to each server. Starts at leader's lastApplied + 1
	matchIndex map[HostID]LogIndex // index of highest entry known to be replicated on each server. Starts at 0

	// Convenience variables
	hosts           HostMap         // look up table of peer id, ip, port, hostname for other raft nodes
	clients         ClientMap       // look up table of peer id, ip, port, hostname for clients
	electionTicker  time.Ticker     // timeouts start each election
	heartbeatTicker time.Ticker     // Leader-only ticker for sending heartbeats during idle periods
	votes           electionResults // temp storage for election results
	killSwitch      bool            // if true, node should exit upon becoming leader
	quitChan        chan bool       // for cleaning up spawned goroutines
	verbose         bool
}

func (r RaftNode) String() string {
	return fmt.Sprintf("Raft Node: id=%d, state=%s, currentTerm=%d, votedFor=%d, hosts={%s}, clients={%s}, votes={%s}, killSwitch=%t, verbose=%t, log=%s, stateMachine={%s}",
		r.id, r.state, r.currentTerm, r.votedFor, r.hosts.String(), r.clients.String(), r.votes.String(), r.killSwitch, r.verbose, r.log.String(), r.stateMachine.String())
}

func (s RaftNodeState) String() string {
	switch s {
	case follower:
		return "follower"
	case leader:
		return "leader"
	case candidate:
		return "candidate"
	default:
		return "UNKNOWN"
	}
}

func (m HostMap) String() string {
	var sb strings.Builder
	for i, v := range m {
		sb.WriteString(fmt.Sprintf("%d={%s},", i, v.String()))
	}
	return sb.String()
}

func (m ClientMap) String() string {
	var sb strings.Builder
	for i, v := range m {
		sb.WriteString(fmt.Sprintf("%d={%s},", i, v.String()))
	}
	return sb.String()
}
