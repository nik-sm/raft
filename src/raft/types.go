package raft

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// TODO - could reduce usages of map and use slices instead where possible
// TODO - could avoid future errors using `type hostID int`

type agentState int

type host int

const (
	follower  agentState = iota
	election  agentState = iota
	leader    agentState = iota
	candidate agentState = iota
)

type peerMap map[host]peer

type peerStringMap map[host]string

type vcMap map[host]ViewChange

type viewEvent struct {
	view          host
	votesReceived []int
	// 1 indicates received a vote from this peer
	// 0 indicates received no vote from this peer
	// all -1's indicates we joined this view by VC Proof
}
type viewHistory []viewEvent

type agent struct {
	verbose         bool
	id              host
	lastAttempted   host
	lastInstalled   host
	state           agentState
	recvChan        chan incomingUDPMessage
	quitChan        chan bool
	electionTimeout time.Duration // seconds between elections
	vcpTimeout      time.Duration // seconds between ViewChangeProof messages
	peers           peerMap       // look up table of peer id, ip, port, hostname
	vcMessages      vcMap         // for temporary storage of messages during election
	vh              viewHistory   // print at end to check protocol correctness
	killSwitch      bool          // if true, node should exit upon becoming leader
	electionTicker  time.Ticker   // timeouts start each election
	proofTicker     time.Ticker   // timeouts cause ViewChangeProof message to send
}

func (m peerMap) String() string {
	var sb strings.Builder
	for i, v := range m {
		sb.WriteString(fmt.Sprintf("%d=%s,", i, v.String()))
	}
	return sb.String()
}

func (m vcMap) String() string {
	var sb strings.Builder
	for i, v := range m {
		sb.WriteString(fmt.Sprintf("%d={%s},", i, v.String()))
	}
	return sb.String()
}

func (v viewHistory) String() string {
	var sb strings.Builder
	for _, voteStruct := range v {
		sb.WriteString(fmt.Sprintf("view %d: [", voteStruct.view))
		for _, vote := range voteStruct.votesReceived {
			sb.WriteString(fmt.Sprintf("%d,", vote))
		}
		sb.WriteString("]\n")
	}
	return sb.String()
}

func (a agent) String() string {
	return fmt.Sprintf("Agent: id=%d, lastAttempted=%d, lastInstalled=%d, electionTimeout=%d, vcpTimeout=%d, peers={%s}, vcMessage={%s\n}, vh={%s}",
		a.id, a.lastAttempted, a.lastInstalled, a.electionTimeout, a.vcpTimeout, a.peers.String(), a.vcMessages.String(), a.vh.String())
}

type incomingUDPMessage struct {
	SourcePeerID host
	SourcePeer   peer
	Contents     GenericMessage
}

func (m incomingUDPMessage) String() string {
	return fmt.Sprintf("Incoming message: SourcePeerID: %d. SourcePeer: %s. Contents: %s\n", m.SourcePeerID, m.SourcePeer.String(), m.Contents.String())
}

type peer struct {
	IP       net.IP
	Port     int
	Hostname string
}

func (p peer) String() string {
	return fmt.Sprintf("Peer IP: %s, Port: %d, Hostname: %s", p.IP.String(), p.Port, p.Hostname)
}