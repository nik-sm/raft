package raft

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// ClientID is the integer ID of a client node
type ClientID int

// HostID is the integer ID of a raft host node
type HostID int

// Term is a monotonically increasing integer identifying the current leader's term
type Term int

func (t Term) String() string {
	return fmt.Sprintf("%d", t)
}

type raftNodeState int

const (
	follower  raftNodeState = iota
	leader    raftNodeState = iota
	candidate raftNodeState = iota
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

// HostMap associates a raft host's ID with their net info
type HostMap map[HostID]peer

// ClientMap associates a raft client's ID with their net info
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

// StateMachine is the core data structure whose updates we want to be resilient
type StateMachine struct {
	clientSerialNums map[ClientID]ClientSerialNum // The most recently executed serial number for each client
	contents         []ClientData
}

// NewStateMachine constructs an empty StateMachine
func NewStateMachine() StateMachine {
	return StateMachine{clientSerialNums: make(map[ClientID]ClientSerialNum), contents: make([]ClientData, 0)}
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

// Log holds LogEntries and
type Log []LogEntry

func (r *RaftNode) haveNewerSerialNum(le LogEntry) (bool, ClientResponse) {
	mostRecent := r.stateMachine.clientSerialNums[le.ClientID]
	if int(mostRecent) >= int(le.ClientSerialNum) {
		// NOTE - TODO - the client should keep sending the item with this serial num until it lands.
		// Therefore, if we have seen a higher serial num, the client MUST have successfully submitted this item,
		// and it will exist in our log
		// NOTE - if we do log compaction, we might have thrown away the previous log entry
		return true, r.log.getPrevResponse(le)
	}
	return false, ClientResponse{}
}

func (l Log) getPrevResponse(le LogEntry) ClientResponse {
	for _, entry := range l {
		if entry.ClientID == le.ClientID && entry.ClientSerialNum == le.ClientSerialNum {
			return entry.ClientResponse
		}
	}
	panic("Should have had previous response!")
}

func (r *RaftNode) append(le LogEntry) {
	r.log = append(r.log, le)
	r.executeLog()
}

func (s *StateMachine) apply(le LogEntry) {
	mostRecentSeen := s.clientSerialNums[le.ClientID]
	if int(mostRecentSeen) >= int(le.ClientSerialNum) {
		log.Printf("Log entry is stale/duplicated, no change to State Machine: %s", le.String())
		return
	}
	s.clientSerialNums[le.ClientID] = le.ClientSerialNum
	// NOTE - if we used a more sophisticated type for ClientData,
	// this is where we would need logic to apply the data to the State Machine
	s.contents = append(s.contents, le.ClientData)
}

func (l Log) String() string {
	var sb strings.Builder
	sb.WriteString("log contents: {")
	for i, entry := range l {
		sb.WriteString(fmt.Sprintf("%d={%s},", i, entry.String()))
	}
	sb.WriteString("}")
	return sb.String()
}

// LogIndex is a position in the log
type LogIndex int

// LogEntry is an entry in the log
// NOTE that all fields must be public for Gob encoding during RPC
type LogEntry struct {
	Term       Term
	ClientData ClientData
	// TODO - the following 3 fields are here for convenience
	// They might belong to the StateMachine instead of the Log
	ClientID        ClientID        // The client who sent this data
	ClientSerialNum ClientSerialNum // The unique number the client used to identify this data
	ClientResponse  ClientResponse  // The response that was given to this client
}

// ClientData represents an object sent by the client for storage in the StateMachine
type ClientData string // NOTE - could make contents a state machine update

// NewLogEntry is a public constructor for a LogEntry
func NewLogEntry(t Term, cd ClientData, cid ClientID, csn ClientSerialNum, cr ClientResponse) LogEntry {
	return LogEntry{
		Term:            t,
		ClientData:      cd,
		ClientID:        cid,
		ClientSerialNum: csn,
		ClientResponse:  cr}
}

func (le LogEntry) String() string {
	return fmt.Sprintf("term: %s, clientData: %s, clientID: %d, clientSerialNum: %d, clientResponse: %s", le.Term.String(), le.ClientData, le.ClientID, le.ClientSerialNum, le.ClientResponse)
}

// LogAppend must be public because this is encoded by Gob during AppendEntriesRPC
// NOTE all fields must be public for Gob encoding during RPC
type LogAppend struct {
	Idx   LogIndex
	Entry LogEntry
}

// LogAppendList describes exactly where to place each of a list of LogEntries.
// Therefore, we must use a slice of entries that specify their destination index.
// TODO - due to the log safety properties, can we skip this and just append the list in order?
type LogAppendList []LogAppend

// RPCResponse carries the reply between raft nodes after an RPC
type RPCResponse struct {
	Term     Term
	Success  bool
	LeaderID HostID
}

func (r RPCResponse) String() string {
	return fmt.Sprintf("ClientResponse Term=%d, Success=%t", r.Term, r.Success)
}

// ClientResponse carries the reply from a raft node to a client after RPC
type ClientResponse struct {
	Success bool // if success == false, then client should retry using 'leader'
	Leader  HostID
}

func (c ClientResponse) String() string {
	return fmt.Sprintf("ClientResponse Success=%t, Leader=%d", c.Success, c.Leader)
}

type msgType int

type incomingMsg struct {
	msgType  msgType     // What type of response did we receive
	hostID   HostID      // who responded
	response RPCResponse // Contents of their response
}

const (
	appendEntries msgType = iota
	vote          msgType = iota
)

// RaftNode describes a participant in the Raft protocol
type RaftNode struct {
	id    HostID        // id of this node
	state raftNodeState // follower, leader, or candidate

	// Persistent State
	currentTerm  Term   // latest term server has seen
	votedFor     HostID // ID of candidate that received vote in current term (or -1 if none)
	log          Log
	stateMachine StateMachine

	// Volatile State
	commitIndex   LogIndex
	lastApplied   LogIndex
	currentLeader HostID
	// notice that a Term can have 0 leaders, so this should not be included inside Term type

	// Volatile State (leader only, reset after each election)
	nextIndex       map[HostID]LogIndex // index of next log entry to send to each server. Starts at leader's lastApplied + 1
	matchIndex      map[HostID]LogIndex // index of highest entry known to be replicated on each server. Starts at 0
	indexIncrements map[HostID]int      // length of the entries list we have sent to each peer

	// Convenience variables
	sync.Mutex                       // control acess from multiple goroutines. Notice we can now just do r.Lock() instead of r.mut.Lock()
	incomingChan    chan incomingMsg // for collecting and reacting to async RPC responses
	hosts           HostMap          // look up table of peer id, ip, port, hostname for other raft nodes
	clients         ClientMap        // look up table of peer id, ip, port, hostname for clients
	electionTicker  time.Ticker      // timeouts start each election
	timeoutUnits    time.Duration    // units to use for election timeout
	heartbeatTicker time.Ticker      // Leader-only ticker for sending heartbeats during idle periods
	giveUpTimeout   time.Duration    // during client data storage interaction, time before giving up and replying failure
	votes           electionResults  // temp storage for election results
	quitChan        chan bool        // for cleaning up spawned goroutines
	verbose         bool
}

func (r *RaftNode) String() string {
	return fmt.Sprintf("Raft Node: id=%d, state=%s, currentTerm=%d, votedFor=%d, hosts={%s}, clients={%s}, votes={%s}, verbose=%t, log={%s}, stateMachine={%s}",
		r.id, r.state, r.currentTerm, r.votedFor, r.hosts.String(), r.clients.String(), r.votes.String(), r.verbose, r.log.String(), r.stateMachine.String())
}

// NewRaftNode is a public constructor for RaftNode
func NewRaftNode(id HostID, hosts HostMap, clients ClientMap, quitChan chan bool) *RaftNode {
	// Initialize Log
	initialLog := []LogEntry{
		LogEntry{
			Term:            Term(-1),
			ClientData:      ClientData("LOG_START"),
			ClientID:        ClientID(-1),
			ClientSerialNum: ClientSerialNum(-1)}}

	// Initialize StateMachine
	initialSM := NewStateMachine()

	r := RaftNode{
		id:    id,
		state: follower,

		currentTerm:  0,
		votedFor:     -1,
		log:          initialLog,
		stateMachine: initialSM,

		commitIndex:   0,
		lastApplied:   0,
		currentLeader: -1, // NOTE - notice that client needs to validate the "currentLeader" response that they get during a redirection

		nextIndex:  make(map[HostID]LogIndex),
		matchIndex: make(map[HostID]LogIndex),

		incomingChan:  make(chan incomingMsg),
		hosts:         hosts,
		clients:       clients,
		timeoutUnits:  timeoutUnits,
		giveUpTimeout: 2 * heartbeatTimeout * timeoutUnits, // TODO - is this a reasonable timeout?
		votes:         make(electionResults),
		quitChan:      quitChan,
		verbose:       verbose}

	// Initialize State Machine
	for clientID := range clients {
		r.stateMachine.clientSerialNums[clientID] = -1
	}
	r.stateMachine.contents = append(r.stateMachine.contents, "STATE_MACHINE_START")
	return &r
}

func (s raftNodeState) String() string {
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
