package raft

import (
	"encoding/gob"
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
var tfrom int
var ttil int

func selectElectionTimeout() time.Duration {
	min := 150
	max := 300
	return time.Duration((rand.Intn(max-min+1) + min)) * time.Millisecond
}

func selectHeartbeatTimeout() time.Duration {
	min := 50
	max := 51
	return time.Duration((rand.Intn(max-min+1) + min)) * time.Millisecond
}

func fakeHeartbeatTimeout() time.Duration {
	return time.Duration(10) * time.Second
}

// returns true when an agent has a majority of votes for the proposed view
func (r RaftNode) wonElection() bool {
	return haveMajority(r.votes)
}

func haveMajority(votes map[Host]bool) bool {
	nVoters := len(votes)
	nReq := int(math.Floor(float64(nVoters)/2)) + 1
	nFound := 0
	for _, votedYes := range votes {
		if votedYes {
			nFound++
		}
	}
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
func (r *RaftNode) shiftToFollower(t Term, leaderID Host) {
	r.state = follower
	log.Fatal("TODO")
	return
}
func (r *RaftNode) shiftToLeader() {
	r.state = leader
	r.resetTickers()
	log.Fatal("TODO")
	return
}

func (r *RaftNode) shiftToCandidate() {
	r.state = candidate
	r.resetTickers()
	r.currentTerm += 1
	go r.MultiRequestVoteRPC()
	return
}

// If the client has contacted the wrong leader,
func (r *RaftNode) StoreClientData(cd ClientDataStruct, reply ClientResponse) error {
	if r.state != leader {
		reply.success = false
		// to make client logic easy, we redirect to ourselves if we do not know the leader
		if r.currentLeader == -1 {
			reply.leader = r.id
		} else {
			reply.leader = r.currentLeader
		}
		return nil
	} else { // We are the leader. Attempt to replicate this to all peer logs
		reply.leader = r.id
		entry := LogEntry{term: r.currentTerm,
			contents:        cd.Data,
			clientID:        cd.ClientID,
			clientSerialNum: cd.ClientSerialNum}

		// Try to short-circuit based on the client serial num
		if haveNewer, prevReply := r.log.haveNewerSerialNum(entry); haveNewer {
			reply.success = prevReply.success
			// reply.leader = prevReply.leader
			// TODO - confirm: we do not want to notify about the previous leader, because if it is not us, the client will
			// just get confused and contact the wrong node next time
			// this situation only arises if the client's previous attempt was partially successful, but leader crashed before replying
			return nil
		}

		r.log.append(entry)
		log.Fatal("TODO - append the entry to leader's log")
		haveMajority := false
		for !haveMajority {
			haveMajority = r.MultiAppendEntriesRPC([]LogEntry{entry})
			if haveMajority {
				// TODO - update commit index
				r.executeLog() // TODO - need to make sure this successfully applied something to the state machine
				reply.success = true
				break
			}
		}
		return nil
	}
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
		r.log.append(logEntry)
	}
	if r.verbose {
		log.Printf("log after: %s", r.log.String())
	}
	return LogIndex(len(r.log.contents) - 1)
}

// Based on our commit index, apply any log entries that are ready for commit
func (r *RaftNode) executeLog() {
	// TODO - need some stateMachine variable that represents a set of applied log entries
	log.Fatal("TODO")
}

// TODO - some amount of duplicated logic in AppendEntries() and Vote()
// Handles an incoming AppendEntriesRPC
// Returns false if entries were rejected, or true if accepted
func (r *RaftNode) AppendEntries(ae AppendEntriesStruct, reply RPCResponse) error {
	defer r.persistState()
	defer r.executeLog()
	if ae.T > r.currentTerm {
		r.shiftToFollower(ae.T, ae.LeaderID)
	}

	reply.term = r.currentTerm // no matter what, we will respond with our current term
	if ae.T < r.currentTerm {
		if r.verbose {
			log.Println("AE from stale term")
		}
		reply.success = false
		return nil
	} else if r.log.contents[ae.PrevLogIndex].term != ae.PrevLogTerm { // TODO - does this work for the very first log entry?
		if r.verbose {
			log.Println("my PrevLogTerm does not match theirs")
		}
		reply.success = false
		return nil
	} else {
		if r.verbose {
			log.Println("Applying entries...")
		}
		reply.success = true
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
	} else if ourLastLogEntry.term == theirLastLogTerm {
		if ourLastLogIdx > int(theirLastLogIdx) {
			return false
		} else {
			return true
		}
	} else { // They have more recent term
		return true
	}
}

// TODO - need to decide where to compare lastApplied to commitIndex, and apply log entries to our local state machine

func (r *RaftNode) Vote(rv RequestVoteStruct, reply RPCResponse) error {
	r.persistState()
	if rv.T < r.currentTerm {
		if r.verbose {
			log.Println("RV from stale term")
		}
		reply.term = r.currentTerm
		reply.success = false
		// TODO - should we also resetElectionTicker() here? We want progress, and a failed vote does not help progress, so probably not...
		return nil
	} else if (r.votedFor == -1 || r.votedFor == rv.CandidateID) && r.theyAreUpToDate(rv.LastLogIdx, rv.LastLogTerm) {
		if r.verbose {
			log.Println("Grant vote")
		}
		reply.term = r.currentTerm
		reply.success = true
		// TODO - should change some local state variables, like term number?
		r.resetTickers()
		return nil
	} else {
		log.Fatal("how did we get here?")
		return errors.New("how did we get here")
	}
}

func containsH(s []Host, i Host) bool {
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
		case <-r.heartbeatTicker.C:
			r.MultiAppendEntriesRPC(make([]LogEntry, 0, 0)) // TODO - should we save and inspect the bool result here?
		case <-r.electionTicker.C:
			// Periodically time out and start a new election
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

func getKillSwitch(nodeID Host, testScenario int) bool {
	var killNodes []Host
	if testScenario == 3 {
		killNodes = []Host{1}
	} else if testScenario == 4 {
		killNodes = []Host{1, 2}
	} else if testScenario == 5 {
		killNodes = []Host{1, 2, 3}
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
		r.heartbeatTicker = *time.NewTicker(selectHeartbeatTimeout())
	} else {
		// TODO - goofy solution, set the heartbeat very long unless we are leader
		r.heartbeatTicker = *time.NewTicker(fakeHeartbeatTimeout())
	}
}

func (r *RaftNode) shutdown() {
	log.Println("RAFT NODE SHUTTING DOWN")
	r.quitChan <- true
}

func init() {
	flag.StringVar(&hostfile, "h", "hostfile.txt", "name of hostfile")
	flag.IntVar(&testScenario, "t", 0, "test scenario to run")
	flag.IntVar(&duration, "d", 30, "time until node shutdown")
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.IntVar(&tfrom, "tfrom", 5, "low end of election timeout range")
	flag.IntVar(&ttil, "ttil", 10, "high end of election timeout range")

	recvPort = 4321
	gob.Register(ViewChange{})
	gob.Register(ViewChangeProof{})
	rand.Seed(time.Now().UTC().UnixNano())
}

func Raft() {
	flag.Parse()
	peers := make(PeerMap)
	if !containsI([]int{1, 2, 3, 4, 5}, testScenario) {
		log.Fatalf("invalid test scenario: %d", testScenario)
	} else {
		log.Printf("TEST SCENARIO: %d", testScenario)
	}
	quitChan := make(chan bool)

	r := RaftNode{
		id:    ResolveAllPeers(peers, hostfile),
		state: follower,

		currentTerm: 0,
		votedFor:    -1,
		log:         Log{make(map[Client]ClientSerialNum), make([]LogEntry, 0, 0)},

		commitIndex:   0,
		lastApplied:   0,
		currentLeader: -1, // TODO - notice that client needs to validate the "currentLeader" response that they get during a redirection
		// Alternatively, a node can redirect to itself until it knows who leader is

		nextIndex:  make(map[Host]LogIndex),
		matchIndex: make(map[Host]LogIndex),

		peers:           peers,
		electionTicker:  *time.NewTicker(selectElectionTimeout()),
		heartbeatTicker: *time.NewTicker(fakeHeartbeatTimeout()),
		votes:           make(electionResults),
		killSwitch:      false,
		quitChan:        quitChan,
		verbose:         verbose}

	log.Printf("RaftNode: %s", r.String())

	go r.recvDaemon(quitChan)
	go r.protocol()
	go r.quitter(duration)
	<-quitChan
	log.Println("Finished...")
}
