package raft

import (
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strings"
	"time"
)

var hostfile string
var recvPort int
var duration int
var verbose bool

// returns true when an agent has a majority of votes for the proposed view
func (r *RaftNode) wonElection() bool {
	return haveMajority(r.votes)
}

func haveMajority(votes map[HostID]bool) bool {
	var sb strings.Builder
	nVoters := len(votes)
	nReq := int(math.Floor(float64(nVoters)/2)) + 1
	nFound := 0
	sb.WriteString("[")
	for hostID, votedYes := range votes {
		if votedYes {
			nFound++
		}
		sb.WriteString(fmt.Sprintf("|host %d, votedYes %t|", hostID, votedYes))
	}
	sb.WriteString("]")
	log.Printf("Checking majority. nVoters: %d, nReq: %d, nFound: %d, Votes: %s", nVoters, nReq, nFound, sb.String())
	return nFound >= nReq
}

func (r *RaftNode) persistState() {
	r.Lock()
	defer r.Unlock()
	log.Println("TODO - persistState")
	// Save: currentTerm, votedFor, log
}

func (r *RaftNode) recoverFromDisk() {
	r.Lock()
	defer r.Unlock()
	log.Println("TODO - recoverFromDisk")
	// right at the restart of the node, check the standard place for
	// a persisted state object. If exists, apply it, otherwise, just startup normally
}

// NOTE - important that for all the shiftTo...() functions, we must first set our state variable
func (r *RaftNode) shiftToFollower(t Term, leaderID HostID) {
	if r.verbose {
		log.Printf("############\nSHIFT TO FOLLOWER, Term: %d, LeaderID: %d", t, leaderID)
	}
	r.state = follower
	r.resetTickers()
	r.currentTerm = t
	r.currentLeader = leaderID
	r.nextIndex = nil
	r.matchIndex = nil
	r.indexIncrements = nil
	r.votedFor = -1
}

// NOTE - We only become leader by doing shiftToCandidate() and voting for ourself
// Therefore, we know who we voted for.
// We have already adjusted our currentTerm (during shiftToCandidare())
func (r *RaftNode) shiftToLeader() {
	r.Lock()
	defer r.Unlock()
	defer r.heartbeatAppendEntriesRPC() // We need to confirm leadership with all nodes
	if r.verbose {
		log.Printf("############\nSHIFT TO LEADER. Term: %d", r.currentTerm)
	}
	r.state = leader
	r.currentLeader = r.id
	r.votedFor = -1

	// Reset leader volatile state
	r.nextIndex = make(map[HostID]LogIndex)
	r.matchIndex = make(map[HostID]LogIndex)
	r.indexIncrements = make(map[HostID]int)

	for peerID := range r.hosts {
		r.nextIndex[peerID] = r.getLastLogIndex() // TODO - the paper figure 2 "State" says this should be "leader last log index + 1" ???
		r.matchIndex[peerID] = LogIndex(0)
	}
}

func (r *RaftNode) election() {
	r.shiftToCandidate()
	r.multiRequestVoteRPC()
}

func (r *RaftNode) shiftToCandidate() {
	r.Lock()
	defer r.Unlock()

	if r.verbose {
		log.Println("############\nSHIFT TO CANDIDATE")
	}
	r.votes = make(electionResults)
	r.votes[r.id] = true
	for hostID := range r.hosts {
		if hostID != r.id {
			r.votes[hostID] = false
		}
	}
	r.state = candidate
	r.resetTickers()
	r.currentTerm++
	r.votedFor = r.id
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
	defer r.executeLog()
	reply.Leader = r.currentLeader
	reply.Success = false // by default, assume we will fail

	if r.state != leader {
		return nil
	}

	// We are the leader. Attempt to replicate this to all peer logs
	entry := LogEntry{
		Term:            r.currentTerm,
		ClientData:      cd.Data,
		ClientID:        cd.ClientID,
		ClientSerialNum: cd.ClientSerialNum}

	// Try to short-circuit based on the client serial num
	if haveNewer, prevReply := r.haveNewerSerialNum(entry); haveNewer {
		reply.Success = prevReply.Success
		// reply.leader = prevReply.leader
		// NOTE - we do not want to notify about the previous leader, because if it is not us, the client will
		// just get confused and contact the wrong node next time
		// this situation only arises if the client's previous attempt was partially successful, but leader crashed before replying
		return nil
	}

	// TODO - critical change for reliability from POV of client: we need to somehow determine when the change has been committed to the cluster, and THEN respond to the client
	r.append(entry)
	majorityStored := false
	for !majorityStored {
		select {
		case <-time.After(r.giveUpTimeout): // leader gives up and reports failure to client
			break
		default:
			majorityStored = r.multiAppendEntriesRPC([]LogEntry{entry})
			if majorityStored {
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
		logIndex := logAppendStruct.Idx
		logEntry := logAppendStruct.Entry
		if r.log[logIndex].Term != logEntry.Term {
			if r.verbose {
				log.Printf("Found entries with conflict: had %s, want %s. Deleting suffix...", r.log[logIndex].String(), logEntry.String())
			}
			// TODO - when we slice a log suffix, we need to also change our clientSerialNums info somehow???
			r.log = r.log[:logIndex] // delete slice suffix, including item at logIndex. slice len changes, while slice cap does not
		}
		r.append(logEntry)
	}
	if r.verbose {
		log.Printf("log after: %s", r.log.String())
	}
	return r.getLastLogIndex()
}

// After sending updates to other nodes, we try to advance our commitIndex
// At the end, we try to execute log
func (r *RaftNode) updateCommitIndex() {
	// If there exists an N such that:
	//   1) N > commitIndex,
	//   2) a majority of matchIndex[i] >= N, and
	//   3) log[N].term == currentTerm
	// Then:
	//   set commitIndex = N
	defer r.executeLog()
	for n := r.commitIndex + 1; n <= r.getLastLogIndex(); n++ {
		if r.log[n].Term != r.currentTerm {
			log.Printf("commitIndex %d ineligible because of log entry %s", n, r.log[n].String())
			continue
		}
		peersAtThisLevel := make(map[HostID]bool)
		for hostID := range r.hosts {
			if hostID == r.id {
				peersAtThisLevel[hostID] = true
			} else {
				peersAtThisLevel[hostID] = r.matchIndex[hostID] >= n
			}
		}
		if haveMajority(peersAtThisLevel) {
			r.commitIndex = n
		}
	}
}

// Based on our commit index, apply any log entries that are ready for commit
func (r *RaftNode) executeLog() {
	r.Lock()
	defer r.Unlock()
	// TODO - this function should be idempotent and safe to apply often
	// TODO - need some stateMachine variable that represents a set of applied log entries
	// TODO - update commit index
	if r.state == leader {
		for r.commitIndex > r.lastApplied {
			r.lastApplied++
			r.stateMachine.apply(r.log[r.lastApplied])
		}
	}
}

// AppendEntries is called by RPC from the leader to modify the log of a follower.
// TODO - some amount of duplicated logic in AppendEntries() and Vote()
// Returns false if entries were rejected, or true if accepted
func (r *RaftNode) AppendEntries(ae AppendEntriesStruct, response *RPCResponse) error {
	r.resetTickers()
	r.persistState()
	defer r.executeLog()
	response.Term = r.currentTerm

	if r.verbose {
		log.Printf("AppendEntries().")
	}

	// AppendEntries requires same or future term
	if ae.Term < r.currentTerm {
		if r.verbose {
			log.Println("AE from stale term")
		}
		response.Success = false
		return nil
	}

	// TODO - shifting to follower each time is slightly inefficient, but keeps things uniform
	r.shiftToFollower(ae.Term, ae.LeaderID)

	// Check if the AE matches our last log term
	if r.log[ae.PrevLogIndex].Term != ae.PrevLogTerm {
		// TODO - does this work for the very first log entry?
		if r.verbose {
			log.Println("my PrevLogTerm does not match theirs")
		}
		response.Success = false
		return nil
	}

	if r.verbose {
		log.Println("Applying entries...")
	}
	response.Success = true
	lastIndex := r.applyNewLogEntries(ae.Entries)

	// Now we need to decide how to set our local commit index
	if ae.LeaderCommit > r.commitIndex {
		r.commitIndex = min(lastIndex, ae.LeaderCommit)
	}
	return nil
}

// CandidateLooksEligible allows a raft node to decide whether another host's log is sufficiently up-to-date to become leader
// Returns true if the incoming RequestVote shows that the peer is at least as up-to-date as we are
// See paper section 5.4
func (r *RaftNode) CandidateLooksEligible(candLastLogIdx LogIndex, candLastLogTerm Term) bool {
	ourLastLogIdx := r.getLastLogIndex()
	ourLastLogEntry := r.log[ourLastLogIdx]
	if r.verbose {
		log.Printf("Comparing our lastLogEntry (term %d, index %d) VS theirs (term %d, idx %d)", ourLastLogEntry.Term, ourLastLogIdx, candLastLogTerm, candLastLogIdx)
	}

	if int(ourLastLogEntry.Term) > int(candLastLogTerm) {
		return false
	} else if ourLastLogEntry.Term == candLastLogTerm && int(ourLastLogIdx) > int(candLastLogIdx) {
		return false
	}
	return true
}

// TODO - need to decide where to compare lastApplied to commitIndex, and apply log entries to our local state machine

// Vote is called by RPC from a candidate. We can observe the following from the raft.github.io simulation:
//	1) If we get a requestVoteRPC from a future term, we immediately jump to that term and send our vote
//	2) If we are already collecting votes for the next election, and simultaneously get a request from another node to vote for them, we do NOT give them our vote
//    (we've already voted for ourselves!)
//	3) if we've been offline, and wakeup and try to get votes: we get rejections, that also tell us the new term, and we immediately jump to that term as a follower
func (r *RaftNode) Vote(rv RequestVoteStruct, response *RPCResponse) error {
	r.resetTickers()
	r.persistState()
	response.Term = r.currentTerm

	if r.verbose {
		log.Printf("Vote(). \nRequestVoteStruct: %s. \nMy node: term: %d, votedFor %d, lastLogTerm: %d, lastLogIdx: %d",
			rv.String(), r.currentTerm, r.votedFor, r.getLastLogTerm(), r.getLastLogIndex())
	}

	// Vote requires future term
	if rv.Term <= r.currentTerm {
		if r.verbose {
			log.Println("RV from prior term")
		}
		response.Success = false
		return nil
	}

	// By this point, we know the vote request comes from future term
	// Therefore, we must shift to follower and decide whether to grant vote
	r.shiftToFollower(rv.Term, HostID(-1)) // We do not yet know who is leader for this term

	// If we have already voted, or this peer is not a valid candidate, do not grant vote
	if (r.votedFor != -1) || !r.CandidateLooksEligible(rv.LastLogIdx, rv.LastLogTerm) {
		if r.verbose {
			log.Println("Do not grant vote")
		}
		response.Success = false
		return nil
	}

	// Otherwise, we grant vote
	if r.verbose {
		log.Println("Grant vote")
	}
	response.Success = true
	r.votedFor = rv.CandidateID
	return nil
}

func (r *RaftNode) getLastLogIndex() LogIndex {
	return LogIndex(len(r.log) - 1)
}

func (r *RaftNode) getLastLogTerm() Term {
	return Term(r.log[int(r.getLastLogIndex())].Term)
}

func max(x LogIndex, y LogIndex) LogIndex {
	if x > y {
		return x
	}
	return y
}

func min(x LogIndex, y LogIndex) LogIndex {
	if x < y {
		return x
	}
	return y
}

// Main leader Election Procedure
func (r *RaftNode) protocol() {
	r.resetTickers()
	log.Printf("Begin Protocol. verbose: %t", r.verbose)

	for {
		select {
		case m := <-r.incomingChan:
			if m.response.Term > r.currentTerm {
				if r.verbose {
					log.Printf("Received reply from hostID %d with higher term: %d and leaderid: %d", m.hostID, m.response.Term, m.response.LeaderID)
				}
				r.Lock()
				go r.shiftToFollower(m.response.Term, m.response.LeaderID)
				r.Unlock()
			}

			switch m.msgType {
			case vote:
				if r.state == candidate {
					r.votes[m.hostID] = m.response.Success
					if r.wonElection() {
						r.shiftToLeader()
					}
				}
			case appendEntries:
				if r.state == leader { // We might have been deposed

					// Inspect response and update our tracking variables appropriately
					if !m.response.Success {
						// TODO - When responding to AppendEntries, the follower should return success if they do a new append, OR if they already have appended that entry

						prev := r.nextIndex[m.hostID]
						next := max(0, r.nextIndex[m.hostID]-1)
						log.Printf("Decrement nextIndex for hostID %d from %d to %d", m.hostID, prev, next)
						r.nextIndex[m.hostID] = next
					} else {
						prev := r.nextIndex[m.hostID]
						next := prev + LogIndex(r.indexIncrements[m.hostID])
						log.Printf("Increment nextIndex for hostID %d from %d to %d", m.hostID, prev, next)
						r.matchIndex[m.hostID] = prev
						r.nextIndex[m.hostID] = next
					}
				}

				// TODO - update commit index or nextIndex[] and matchIndex[] based on response
				r.executeLog()
			default:
				panic(fmt.Sprintf("invalid incomingMsg: %d", m.msgType))
			}
		case <-r.heartbeatTicker.C: // Send append entries, either empty or full depending on the peer's log index
			if r.state == leader {
				r.heartbeatAppendEntriesRPC()
				r.updateCommitIndex()
			}
		case <-r.electionTicker.C: // Periodically time out and start a new election
			if r.state == follower {
				if r.verbose {
					log.Println("FOLLOWER TIMED OUT!")
				}
				r.election()
			}
		case <-r.quitChan:
			r.shutdown()
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

func (r *RaftNode) resetElectionTicker() time.Duration {
	newTimeout := selectElectionTimeout(r.id) * r.timeoutUnits
	if r.verbose {
		log.Printf("new election timeout: %s", newTimeout.String())
	}
	r.electionTicker = *time.NewTicker(newTimeout)
	return newTimeout
}

func (r *RaftNode) resetHeartbeatTicker() time.Duration {
	var newTimeout time.Duration
	newTimeout = heartbeatTimeout * r.timeoutUnits
	if r.verbose {
		log.Printf("new heartbeat timeout: %s", newTimeout.String())
	}
	r.heartbeatTicker = *time.NewTicker(newTimeout)
	return newTimeout
}

func (r *RaftNode) resetTickers() (time.Duration, time.Duration) {
	electionTimeout := r.resetElectionTicker()
	heartbeatTimeout := r.resetHeartbeatTicker()
	return electionTimeout, heartbeatTimeout
}

func (r *RaftNode) printLog() {
	var sb strings.Builder
	for idx, entry := range r.log {
		sb.WriteString(fmt.Sprintf("index %d, entry %s\n", idx, entry.String()))
	}
	log.Print(sb.String())
}

func (r *RaftNode) printStateMachine() {
	var sb strings.Builder
	sb.WriteString("clientSerialNums:[")
	for cid, csn := range r.stateMachine.clientSerialNums {
		sb.WriteString(fmt.Sprintf("client %d, serialNum %d", cid, csn))
	}
	sb.WriteString("].\ncontents:[")
	for idx, entry := range r.stateMachine.contents {
		sb.WriteString(fmt.Sprintf("index %d, entry %s", idx, entry))
	}
	sb.WriteString("]")
	log.Println(sb.String())
}

func (r *RaftNode) printResults() {
	r.printLog()
	r.printStateMachine()
}

func (r *RaftNode) shutdown() {
	log.Println("RAFT NODE SHUTTING DOWN")
	r.quitChan <- true
}

func init() {
	flag.StringVar(&hostfile, "h", "hostfile.json", "name of hostfile")
	flag.IntVar(&duration, "d", 30, "time until node shutdown")
	flag.BoolVar(&verbose, "v", false, "verbose output")

	recvPort = 4321
	rand.Seed(time.Now().UTC().UnixNano())
}

// Raft is the entrypoint function for the raft replicated state machine protocol
func Raft() {
	flag.Parse()
	hosts := make(HostMap)
	clients := make(ClientMap)
	quitChan := make(chan bool)

	id := HostID(ResolveAllPeers(hosts, clients, hostfile, true))

	r := NewRaftNode(id, hosts, clients, quitChan)

	log.Printf("RaftNode: %s", r.String())

	go r.recvDaemon(quitChan)
	go r.protocol()
	go r.quitter(duration)
	<-quitChan
	log.Println("FINISH EXPERIMENT...")
	r.printResults()
}
