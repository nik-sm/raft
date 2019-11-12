package raft

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// TODO - could reduce usages of map and use slices instead where possible

type Client int // TODO - if we only have a single client, maybe this can be simplified to "type client peer"
type Host int
type Term Host

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

type PeerMap map[Host]peer

type peerStringMap map[Host]string

// Convenient temp storage during elections
type electionResults map[Host]bool

func (e electionResults) String() string {
	var sb strings.Builder
	for host, vote := range e {
		sb.WriteString(fmt.Sprintf("%d=%t,", host, vote))
	}
	return sb.String()
}

// Log
type Log struct {
	clientSerialNums map[Client]ClientSerialNum
	contents         []LogEntry
}

func (l Log) haveNewerSerialNum(le LogEntry) (bool, ClientResponse) {
	mostRecent := l.clientSerialNums[le.clientID]
	if int(mostRecent) >= int(le.clientSerialNum) {
		return true, l.getPrevResponse(le)
	} else {
		return false, ClientResponse{}
	}

}

func (l Log) getPrevResponse(le LogEntry) ClientResponse {
	for idx, entry := range l.contents {
		if entry.clientID == le.clientID && entry.clientSerialNum == le.clientSerialNum {
			return entry.clientResponse
		}
	}
	log.Fatal("Should have had previous response!")
	return ClientResponse{}
}

func (l *Log) append(le LogEntry) {
	l.contents = append(l.contents, le)

	mostRecentSeen := l.clientSerialNums[le.clientID]
	if int(mostRecentSeen) < int(le.clientSerialNum) {
		l.clientSerialNums[le.clientID] = le.clientSerialNum
	}
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
	term            Term
	contents        ClientData
	clientID        Client          // The client who sent this data
	clientSerialNum ClientSerialNum // The unique number the client used to identify this data
	clientResponse  ClientResponse  // The response that was given to this client
}
type ClientData string // NOTE - could make contents a state machine update

func (le LogEntry) String() string {
	return fmt.Sprintf("term: %s, contents: %s", le.term.String(), le.contents)
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
	term    Term
	success bool
}

// Response after RPC from client
type ClientResponse struct {
	success bool // if success == false, then client should retry using 'leader'
	leader  Host
}

// RaftNode
type RaftNode struct {
	id    Host          // id of this node
	state RaftNodeState // follower, leader, or candidate

	// Persistent State
	currentTerm Term // latest term server has seen
	votedFor    Host // ID of candidate that received vote in current term (or null if none) // TODO - null? -1?
	log         Log

	// Volatile State
	commitIndex   LogIndex
	lastApplied   LogIndex
	currentLeader Host
	// TODO - paper does not mention storing leader, but we need to redirect clients to the leader
	// notice that a Term can have 0 leaders, so this should not be included inside Term type

	// Volatile State (leader only, reset after each election)
	nextIndex  map[Host]LogIndex // index of next log entry to send to each server. Starts at leader's lastApplied + 1
	matchIndex map[Host]LogIndex // index of highest entry known to be replicated on each server. Starts at 0

	// Convenience variables
	peers           PeerMap         // look up table of peer id, ip, port, hostname
	electionTicker  time.Ticker     // timeouts start each election
	heartbeatTicker time.Ticker     // Leader-only ticker for sending heartbeats during idle periods
	votes           electionResults // temp storage for election results
	killSwitch      bool            // if true, node should exit upon becoming leader
	quitChan        chan bool       // for cleaning up spawned goroutines
	verbose         bool
	// proofTicker    time.Ticker     // timeouts cause ViewChangeProof message to send // TODO - should this be a fixed multiple of election ticker?
}

func (r RaftNode) String() string {
	return fmt.Sprintf("Raft Node: id=%d, state=%s, currentTerm=%d, votedFor=%d, peers=%s, votes=%s, killSwitch=%t, verbose=%t, log=%s",
		r.id, r.state, r.currentTerm, r.votedFor, r.peers.String(), r.votes.String(), r.killSwitch, r.verbose, r.log.String())
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

func (m PeerMap) String() string {
	var sb strings.Builder
	for i, v := range m {
		sb.WriteString(fmt.Sprintf("%d=%s,", i, v.String()))
	}
	return sb.String()
}
