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
type Peer struct {
	IP       net.IP
	Port     int
	Hostname string
}

func (p Peer) String() string {
	return fmt.Sprintf("Peer IP: %s, Port: %d, Hostname: %s", p.IP.String(), p.Port, p.Hostname)
}

// HostMap associates a raft host's ID with their net info
type HostMap map[HostID]Peer

// ClientMap associates a raft client's ID with their net info
type ClientMap map[ClientID]Peer

type hostStringMap map[HostID]raftNodeJSON
type clientStringMap map[ClientID]clientNodeJSON

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
	ClientSerialNums map[ClientID]ClientSerialNum // The most recently executed serial number for each client
	Contents         []ClientData
}

// NewStateMachine constructs an empty StateMachine
func NewStateMachine() StateMachine {
	return StateMachine{ClientSerialNums: make(map[ClientID]ClientSerialNum), Contents: make([]ClientData, 0)}
}

func (s StateMachine) String() string {
	var sb strings.Builder
	sb.WriteString("clientSerialNums: {")
	for client, serialNum := range s.ClientSerialNums {
		sb.WriteString(fmt.Sprintf("%d: %d,", client, serialNum))
	}
	sb.WriteString("}. contents: {")
	for idx, item := range s.Contents {
		sb.WriteString(fmt.Sprintf("idx: %d, item: %s,", idx, item))
	}
	sb.WriteString("}")
	return sb.String()
}

// Log holds LogEntries and
type Log []LogEntry

func (r *RaftNode) haveNewerSerialNum(cid ClientID, csn ClientSerialNum) (bool, ClientResponse) {
	mostRecent := r.StateMachine.ClientSerialNums[cid]
	if int(mostRecent) >= int(csn) {
		// NOTE - TODO - the client should keep sending the item with this serial num until it lands.
		// Therefore, if we have seen a higher serial num, the client MUST have successfully submitted this item,
		// and it will exist in our log
		// NOTE - if we do log compaction, we might have thrown away the previous log entry
		return true, r.Log.getPrevResponse(cid, csn)
	}
	return false, ClientResponse{}
}

func (l Log) getPrevResponse(cid ClientID, csn ClientSerialNum) ClientResponse {
	for _, entry := range l {
		if entry.ClientID == cid && entry.ClientSerialNum == csn {
			return entry.ClientResponse
		}
	}
	panic("Should have had previous response!")
}

func (r *RaftNode) append(le LogEntry) {
	r.Log = append(r.Log, le)
	r.executeLog()
}

func (s *StateMachine) apply(le LogEntry) {
	mostRecentSeen := s.ClientSerialNums[le.ClientID]
	if int(mostRecentSeen) >= int(le.ClientSerialNum) {
		log.Printf("Log entry is stale/duplicated, no change to State Machine: %s", le.String())
		return
	}
	s.ClientSerialNums[le.ClientID] = le.ClientSerialNum
	// NOTE - if we used a more sophisticated type for ClientData,
	// this is where we would need logic to apply the data to the State Machine
	s.Contents = append(s.Contents, le.ClientData)
}

func (l Log) String() string {
	var sb strings.Builder
	sb.WriteString("log contents: [")
	for i, entry := range l {
		sb.WriteString(fmt.Sprintf("%d={%s}\n", i, entry.String()))
	}
	sb.WriteString("]")
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
	aeLength int         // For appendEntriesRPC, we also note how many entries were sent
}

func (m *incomingMsg) String() string {
	return fmt.Sprintf("incoming msg. msgType=%d, hostID=%d, response={%s}, aeLength=%d", m.msgType, m.hostID, m.response.String(), m.aeLength)
}

const (
	appendEntries msgType = iota
	vote          msgType = iota
)

// RaftNode describes a participant in the Raft protocol
type RaftNode struct {
	id       HostID        // id of this node
	recvPort int           // Designated port for receiving
	state    raftNodeState // follower, leader, or candidate

	// Exported for persistent storage
	CurrentTerm  Term   // latest term server has seen
	VotedFor     HostID // ID of candidate that received vote in current term (or -1 if none)
	Log          Log
	StateMachine StateMachine

	// Volatile State
	commitIndex   LogIndex
	lastApplied   LogIndex
	currentLeader HostID
	// notice that a Term can have 0 leaders, so this should not be included inside Term type

	// Volatile State (leader only, reset after each election)
	nextIndex  map[HostID]LogIndex // index of next log entry to send to each server. Starts at leader's lastApplied + 1
	matchIndex map[HostID]LogIndex // index of highest entry known to be replicated on each server. Starts at 0
	//indexIncrements map[HostID]int      // length of the entries list we have sent to each peer
	// TODO - notice that indexIncrements approach is flawed because we may receive the successful response to a previous heartbeat when the indexIncrement has already been set to a positive number. This would mean we bump that follower's nextIndex value an extra time, and get an out-of-bounds panic.

	// Convenience variables
	sync.Mutex                          // control acess from multiple goroutines. Notice we can now just do r.Lock() instead of r.mut.Lock()
	incomingChan       chan incomingMsg // for collecting and reacting to async RPC responses
	hosts              HostMap          // look up table of peer id, ip, port, hostname for other raft nodes
	clients            ClientMap        // look up table of peer id, ip, port, hostname for clients
	electionTicker     time.Ticker      // timeouts start each election
	timeoutUnits       time.Duration    // units to use for election timeout
	heartbeatTicker    time.Ticker      // Leader-only ticker for sending heartbeats during idle periods
	clientReplyTimeout time.Duration    // during client data storage interaction, time before giving up and replying failure
	votes              electionResults  // temp storage for election results
	quitChan           chan bool        // for cleaning up spawned goroutines. Exported for testing, but ignored for JSON marshalling
	verbose            bool
}

func (r *RaftNode) String() string {
	return fmt.Sprintf("Raft Node: id=%d, state=%s, currentTerm=%d, votedFor=%d, hosts={%s}, clients={%s}, votes={%s}, verbose=%t, log={%s}, stateMachine={%s}",
		r.id, r.state, r.CurrentTerm, r.VotedFor, r.hosts.String(), r.clients.String(), r.votes.String(), r.verbose, r.Log.String(), r.StateMachine.String())
}

func stringOneLog(l []LogEntry) string {
	var sb strings.Builder
	sb.WriteString("Log. \n[")
	for idx, entry := range l {
		sb.WriteString(fmt.Sprintf("{index=%d, entry=%s\n},", idx, entry.String()))
	}
	sb.WriteString("]")
	return sb.String()
}

func (r *RaftNode) printLog() {
	log.Print(stringOneLog(r.Log))
}

func (r *RaftNode) printStateMachine() {
	var sb strings.Builder
	sb.WriteString("StateMachine. \nclientSerialNums: [")
	for cid, csn := range r.StateMachine.ClientSerialNums {
		sb.WriteString(fmt.Sprintf("{client=%d, serialNum=%d},", cid, csn))
	}
	sb.WriteString("].\ncontents: [")
	for idx, entry := range r.StateMachine.Contents {
		sb.WriteString(fmt.Sprintf("{index=%d, entry=%s},\n", idx, entry))
	}
	sb.WriteString("]")
	log.Println(sb.String())
}

func (r *RaftNode) printResults() {
	r.printLog()
	r.printStateMachine()
}

// NewRaftNode is a public constructor for RaftNode
func NewRaftNode(id HostID, recvPort int, hosts HostMap, clients ClientMap, quitChan chan bool) *RaftNode {
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
		id:       id,
		recvPort: recvPort,
		state:    follower,

		// Exported for persistent storage
		CurrentTerm:  0,
		VotedFor:     -1,
		Log:          initialLog,
		StateMachine: initialSM,

		commitIndex:   0,
		lastApplied:   0,
		currentLeader: -1, // NOTE - notice that client needs to validate the "currentLeader" response that they get during a redirection

		nextIndex:  make(map[HostID]LogIndex),
		matchIndex: make(map[HostID]LogIndex),
		//indexIncrements: make(map[HostID]int),

		incomingChan:       make(chan incomingMsg),
		hosts:              hosts,
		clients:            clients,
		timeoutUnits:       timeoutUnits,
		clientReplyTimeout: 2 * heartbeatTimeout * timeoutUnits, // TODO - is this a reasonable timeout?
		votes:              make(electionResults),
		quitChan:           quitChan,
		verbose:            verbose}

	// Initialize State Machine
	for clientID := range clients {
		r.StateMachine.ClientSerialNums[clientID] = -1
	}
	r.StateMachine.Contents = append(r.StateMachine.Contents, "STATE_MACHINE_START")
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
