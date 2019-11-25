package raft

import (
	"errors"
	"flag"
	"log"
	"math"
	"math/rand"
	"time"
)

var hostfile string
var testScenario int
var recvPort int
var duration int
var verbose bool

// returns true when an agent has a majority of votes for the proposed view
func (r RaftNode) wonElection() bool {
	return haveMajority(r.votes)
}

func haveMajority(votes map[HostID]bool) bool {
	nVoters := len(votes)
	nReq := int(math.Floor(float64(nVoters)/2)) + 1
	nFound := 0
	log.Printf("Checking majority. nVoters: %d, nReq: %d", nVoters, nReq)
	for _, votedYes := range votes {
		if votedYes {
			nFound++
		}
	}
	log.Printf("Checking majority. nFound: %d", nFound)
	if nFound >= nReq {
		return true
	}
	return false
}

func (r RaftNode) persistState() {
	// TODO - save state using Gob (?) for easy recover
	// Save: currentTerm, votedFor, log
	return
}

func (r RaftNode) recoverFromDisk() {
	// TODO - right at the restart of the node, check the standard place for
	// a persisted state object. If exists, apply it, otherwise, just startup normally
}

// NOTE - important that for all the shiftTo...() functions, we must first set our state variable
func (r *RaftNode) shiftToFollower(t Term, leaderID HostID) {
	r.state = follower
	r.resetTickers()
	r.currentTerm = t
	r.currentLeader = leaderID
	// TODO - should we bother storing a nil entry for these leader-only maps?
	r.nextIndex = nil
	r.matchIndex = nil
	return
}

func (r *RaftNode) shiftToLeader() {
	r.state = leader
	r.currentLeader = r.id
	r.resetTickers()
	// Reset leader volatile state
	r.nextIndex = make(map[HostID]LogIndex)
	r.matchIndex = make(map[HostID]LogIndex)
	for peerID := range r.hosts {
		r.nextIndex[peerID] = r.lastApplied + 1
		r.matchIndex[peerID] = 0
	}

	log.Fatal("TODO")
	// r.currentTerm
	// r.votedFor
	return
}

func (r *RaftNode) shiftToCandidate() {
	r.state = candidate
	r.resetTickers()
	r.currentTerm++
	go r.MultiRequestVoteRPC()
	return
}

// StoreClientData allows a client to send data to the raft cluster via RPC for storage
// We fill the reply struct with "success = true" if we are leader and store the data successfully.
// If we are not leader, we will reply with the id of another node, and the client
// must detect this and retry at that node.
// If we do not know or do not yet have a leader, we will reply with leader = -1 and
// client may choose to retry at us or another random node.
func (r *RaftNode) StoreClientData(cd ClientDataStruct, reply *ClientResponse) error {
	// NOTE - if we do not yet know leader, client will see reply.leader = -1.
	// They should wait before recontact, and may recontact us or another random node
	reply.Leader = r.currentLeader
	reply.success = false // by default, assume we will fail

	if r.state != leader {
		return nil
	}

	// We are the leader. Attempt to replicate this to all peer logs
	entry := LogEntry{term: r.currentTerm,
		clientData:      cd.Data,
		clientID:        cd.ClientID,
		clientSerialNum: cd.ClientSerialNum}

	// Try to short-circuit based on the client serial num
	if haveNewer, prevReply := r.haveNewerSerialNum(entry); haveNewer {
		reply.Success = prevReply.Success
		// reply.leader = prevReply.leader
		// NOTE - we do not want to notify about the previous leader, because if it is not us, the client will
		// just get confused and contact the wrong node next time
		// this situation only arises if the client's previous attempt was partially successful, but leader crashed before replying
		return nil
	}

	r.append(entry)
	majorityStored := false
	for !majorityStored {
		select {
		case <-time.After(r.giveUpTimeout): // leader gives up and reports failure to client
			break
		default:
			majorityStored = r.MultiAppendEntriesRPC([]LogEntry{entry})
			if majorityStored {
				r.executeLog()
				reply.Success = true
				break
			}
		}
	}
	return nil
}

// For each incoming log entry, delete log suffix where there is a term conflict, and apply new entries
// Returns the index of the last entry applied
func (r *RaftNode) applyNewLogEntries(updates LogAppendList) LogIndex {
	if r.verbose {
		log.Printf("log before: %s", r.log.String())
	}
	for _, logAppendStruct := range updates {
		logIndex := logAppendStruct.idx
		logEntry := logAppendStruct.entry
		if r.log.contents[logIndex].term != logEntry.term {
			if r.verbose {
				log.Printf("Found entries with conflict: had %s, want %s. Deleting suffix...", r.log.contents[logIndex].String(), logEntry.String())
			}
			// TODO - when we slice a log suffix, we need to also change our clientSerialNums info somehow???
			r.log.contents = r.log.contents[:logIndex] // delete slice suffix, including item at logIndex. slice len changes, while slice cap does not
		}
		r.append(logEntry)
	}
	if r.verbose {
		log.Printf("log after: %s", r.log.String())
	}
	return LogIndex(len(r.log.contents) - 1)
}

// Based on our commit index, apply any log entries that are ready for commit
func (r *RaftNode) executeLog() {
	// TODO - need some stateMachine variable that represents a set of applied log entries
	// TODO - update commit index
	log.Fatal("TODO")
}

// TODO - some amount of duplicated logic in AppendEntries() and Vote()
// Handles an incoming AppendEntriesRPC
// Returns false if entries were rejected, or true if accepted
func (r *RaftNode) AppendEntries(ae AppendEntriesStruct, reply *RPCResponse) error {
	defer r.persistState()
	defer r.executeLog()
	if ae.T > r.currentTerm {
		r.shiftToFollower(ae.T, ae.LeaderID)
	}

	reply.Term = r.currentTerm // no matter what, we will respond with our current term
	if ae.T < r.currentTerm {
		if r.verbose {
			log.Println("AE from stale term")
		}
		reply.Success = false
		return nil
	} else if r.log.contents[ae.PrevLogIndex].term != ae.PrevLogTerm { // TODO - does this work for the very first log entry?
		if r.verbose {
			log.Println("my PrevLogTerm does not match theirs")
		}
		reply.Success = false
		return nil
	} else {
		if r.verbose {
			log.Println("Applying entries...")
		}
		reply.Success = true
		lastIndex := r.applyNewLogEntries(ae.Entries)

		// Now we need to decide how to set our local commit index
		if ae.LeaderCommit > r.commitIndex {
			if lastIndex < ae.LeaderCommit {
				// we still need more entries from the leader, but we can commit what we have so far
				r.commitIndex = lastIndex
			} else {
				// we may now have some log entries that are not yet ready for commit/still need quorum
				// we can commit as much as the leader has
				r.commitIndex = ae.LeaderCommit
			}
		}
		return nil
	}
}

// Returns true if the incoming RequestVote shows that the peer is at least as up-to-date as we are
// See paper section 5.4
// TODO - can this be simplified?
func (r RaftNode) theyAreUpToDate(theirLastLogIdx LogIndex, theirLastLogTerm Term) bool {
	ourLastLogIdx := len(r.log.contents) - 1
	ourLastLogEntry := r.log.contents[ourLastLogIdx]
	if r.verbose {
		log.Printf("Comparing our lastLogEntry: %s, with theirs: (term %d, idx %d)", ourLastLogEntry.String(), theirLastLogTerm, theirLastLogIdx)
	}

	if int(ourLastLogEntry.term) > int(theirLastLogTerm) {
		return false
	} else if ourLastLogEntry.term == theirLastLogTerm && ourLastLogIdx > int(theirLastLogIdx) {
		return false
	}
	return true
}

// TODO - need to decide where to compare lastApplied to commitIndex, and apply log entries to our local state machine

func (r *RaftNode) Vote(rv RequestVoteStruct, reply *RPCResponse) error {
	r.persistState()
	if rv.T < r.currentTerm {
		if r.verbose {
			log.Println("RV from stale term")
		}
		reply.Term = r.currentTerm
		reply.Success = false
		// TODO - should we also resetElectionTicker() here? We want progress, and a failed vote does not help progress, so probably not...
		return nil
	} else if (r.votedFor == -1 || r.votedFor == rv.CandidateID) && r.theyAreUpToDate(rv.LastLogIdx, rv.LastLogTerm) {
		if r.verbose {
			log.Println("Grant vote")
		}
		reply.Term = r.currentTerm
		reply.Success = true
		// TODO - should change some local state variables, like term number?
		r.resetTickers()
		return nil
	} else {
		log.Fatal("how did we get here?")
		return errors.New("how did we get here")
	}
}

func containsH(s []HostID, i HostID) bool {
	for _, v := range s {
		if v == i {
			return true
		}
	}
	return false
}

func containsI(s []int, i int) bool {
	for _, v := range s {
		if v == i {
			return true
		}
	}
	return false
}

func (r *RaftNode) getPrevLogIndex() LogIndex {
	return LogIndex(len(r.log.contents) - 1)
}

func (r *RaftNode) getPrevLogTerm() Term {
	return Term(r.log.contents[int(r.getPrevLogIndex())].term)
}

// Main leader Election Procedure
func (r *RaftNode) protocol() {
	log.Printf("Begin Protocol. verbose: %t", r.verbose)

	for {
		select {
		case <-r.heartbeatTicker.C: // Send a heartbeat (AppendEntriesRPC without contents)
			// We do not track the success/failure of the heartbeat
			r.MultiAppendEntriesRPC(make([]LogEntry, 0, 0))
		case <-r.electionTicker.C: // Periodically time out and start a new election
			if r.verbose {
				log.Println("TIMED OUT!")
			}
			r.shiftToCandidate()
		}
	}
}

// Quit the protocol on a timer (to be run in separate goroutine)
func (r *RaftNode) quitter(quitTime int) {
	for {
		select {
		case <-r.quitChan: // the node decided it should quit
			return
		case <-time.After(time.Duration(quitTime) * time.Second): // we decide the node should quit
			r.quitChan <- true
		}
	}
}

func getKillSwitch(nodeID HostID, testScenario int) bool {
	var killNodes []HostID
	if testScenario == 3 {
		killNodes = []HostID{1}
	} else if testScenario == 4 {
		killNodes = []HostID{1, 2}
	} else if testScenario == 5 {
		killNodes = []HostID{1, 2, 3}
	}

	if len(killNodes) != 0 && containsH(killNodes, nodeID) {
		log.Println("TEST CASE: This node will die upon becoming leader")
		return true
	}
	return false
}

func (r *RaftNode) resetTickers() {
	r.electionTicker = *time.NewTicker(selectElectionTimeout())
	if r.state == leader {
		r.heartbeatTicker = *time.NewTicker(heartbeatTimeout)
	} else {
		// TODO - goofy solution, set the heartbeat very long unless we are leader
		r.heartbeatTicker = *time.NewTicker(fakeHeartbeatTimeout)
	}
}

func (r *RaftNode) shutdown() {
	log.Println("RAFT NODE SHUTTING DOWN")
	r.quitChan <- true
}

func init() {
	flag.StringVar(&hostfile, "h", "hostfile.json", "name of hostfile")
	flag.IntVar(&testScenario, "t", 0, "test scenario to run")
	flag.IntVar(&duration, "d", 30, "time until node shutdown")
	flag.BoolVar(&verbose, "v", false, "verbose output")

	recvPort = 4321
	rand.Seed(time.Now().UTC().UnixNano())
}

func Raft() {
	flag.Parse()
	hosts := make(HostMap)
	clients := make(ClientMap)
	if !containsI([]int{1, 2, 3, 4, 5}, testScenario) {
		log.Fatalf("invalid test scenario: %d", testScenario)
	} else {
		log.Printf("TEST SCENARIO: %d", testScenario)
	}
	quitChan := make(chan bool)

	// Initialize Log
	initialLog := NewLog()
	initialLog.append(LogEntry{term: Term(-1),
		clientData:      ClientData("LOG_START"),
		clientID:        ClientID(-1),
		clientSerialNum: ClientSerialNum(-1)})

	// Initialize StateMachine
	initialSM := NewStateMachine()

	// TODO - make a NewRaftNode constructor
	r := RaftNode{
		id:    HostID(ResolveAllPeers(hosts, clients, hostfile, true)),
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

		hosts:           hosts,
		clients:         clients,
		electionTicker:  *time.NewTicker(selectElectionTimeout()),
		heartbeatTicker: *time.NewTicker(fakeHeartbeatTimeout),
		votes:           make(electionResults),
		killSwitch:      false,
		quitChan:        quitChan,
		verbose:         verbose}

	// Initialize State Machine
	for clientID, _ := range clients {
		r.stateMachine.clientSerialNums[clientID] = -1
	}
	r.stateMachine.contents = append(r.stateMachine.contents, "STATE_MACHINE_START")

	log.Printf("RaftNode: %s", r.String())

	go r.recvDaemon(quitChan)
	go r.protocol()
	go r.quitter(duration)
	<-quitChan
	log.Println("Finished...")
}
