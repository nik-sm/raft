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
var duration int
var verbose bool

// returns true when an agent has a majority of votes for the proposed view
func (r *RaftNode) wonElection() bool {
	return haveMajority(r.votes, "ELECTION", r.verbose)
}

func haveMajority(votes map[HostID]bool, label string, verbose bool) bool {
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
	if verbose {
		log.Printf("Checking %s majority. nVoters: %d, nReq: %d, nFound: %d, Votes: %s", label, nVoters, nReq, nFound, sb.String())
	}
	return nFound >= nReq
}

// NOTE - important that for all the shiftTo...() functions, we must first set our state variable
func (r *RaftNode) shiftToFollower(t Term, leaderID HostID) {
	if r.verbose {
		log.Printf("############  SHIFT TO FOLLOWER, Term: %d, LeaderID: %d", t, leaderID)
	}
	r.state = follower
	r.CurrentTerm = t
	r.currentLeader = leaderID
	r.nextIndex = nil
	r.matchIndex = nil
	//r.indexIncrements = nil
	r.VotedFor = -1
}

// NOTE - We only become leader by doing shiftToCandidate() and voting for ourself
// Therefore, we know who we voted for.
// We have already adjusted our currentTerm (during shiftToCandidare())
func (r *RaftNode) shiftToLeader() {
	defer r.heartbeatAppendEntriesRPC() // We need to confirm leadership with all nodes
	if r.verbose {
		log.Printf("############  SHIFT TO LEADER. Term: %d", r.CurrentTerm)
	}
	r.state = leader
	r.currentLeader = r.id

	// Reset leader volatile state
	r.nextIndex = make(map[HostID]LogIndex)
	r.matchIndex = make(map[HostID]LogIndex)
	//r.indexIncrements = make(map[HostID]int)

	for peerID := range r.hosts {
		r.nextIndex[peerID] = r.getLastLogIndex() + 1
		r.matchIndex[peerID] = LogIndex(0)
		//r.indexIncrements[peerID] = 0
	}
}

func (r *RaftNode) election() {
	r.shiftToCandidate()
	r.multiRequestVoteRPC()
}

func (r *RaftNode) shiftToCandidate() {
	r.resetTickers()
	if r.verbose {
		log.Println("############  SHIFT TO CANDIDATE")
	}
	r.votes = make(electionResults)
	r.votes[r.id] = true
	for hostID := range r.hosts {
		if hostID != r.id {
			r.votes[hostID] = false
		}
	}
	r.state = candidate
	r.CurrentTerm++
	r.VotedFor = r.id
}

// StoreClientData allows a client to send data to the raft cluster via RPC for storage
// We fill the reply struct with "success = true" if we are leader and store the data successfully.
// If we are not leader, we will reply with the id of another node, and the client
// must detect this and retry at that node.
// If we do not know or do not yet have a leader, we will reply with leader = -1 and
// client may choose to retry at us or another random node.
// TODO - need a version of StoreClientData that ensures some form of commitment after leader responds to a message?
func (r *RaftNode) StoreClientData(cd ClientDataStruct, response *ClientResponse) error {
	if r.verbose {
		log.Println("############ StoreClientData()")
	}
	// NOTE - if we do not yet know leader, client will see response.leader = -1.
	// They should wait before recontact, and may recontact us or another random node
	defer r.persistState()
	defer r.executeLog()
	response.Leader = r.currentLeader
	response.Success = false // by default, assume we will fail

	if r.state != leader {
		return nil
	}

	// Try to short-circuit based on the client serial num
	if haveNewer, prevReply := r.haveNewerSerialNum(cd.ClientID, cd.ClientSerialNum); haveNewer {
		response.Success = prevReply.Success
		// response.leader = prevReply.leader
		// NOTE - we do not want to notify about the previous leader, because if it is not us, the client will
		// just get confused and contact the wrong node next time
		// this situation only arises if the client's previous attempt was partially successful, but leader crashed before replying
		return nil
	}

	// We are the leader and this is a new entry. Attempt to replicate this to all peer logs
	response.Success = true
	entry := LogEntry{
		Term:            r.CurrentTerm,
		ClientData:      cd.Data,
		ClientID:        cd.ClientID,
		ClientSerialNum: cd.ClientSerialNum,
		ClientResponse: ClientResponse{
			Success: response.Success,
			Leader:  r.id}}
	r.append(entry)

	go r.heartbeatAppendEntriesRPC()

	return nil
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
	for n := r.commitIndex + 1; n <= r.getLastLogIndex(); n++ {
		if r.Log[n].Term != r.CurrentTerm {
			if r.verbose {
				log.Printf("commitIndex %d ineligible because of log entry %s", n, r.Log[n].String())
			}
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
		if haveMajority(peersAtThisLevel, "COMMIT IDX", r.verbose) {
			r.commitIndex = n
		}
	}
}

// Based on our commit index, apply any log entries that are ready for commit
func (r *RaftNode) executeLog() {
	// TODO - this function should be idempotent and safe to apply often
	// TODO - need some stateMachine variable that represents a set of applied log entries
	// TODO - update commit index
	for r.commitIndex > r.lastApplied {
		r.lastApplied++
		r.StateMachine.apply(r.Log[r.lastApplied])
	}
}

// AppendEntries is called by RPC from the leader to modify the log of a follower.
// TODO - some amount of duplicated logic in AppendEntries() and Vote()
// Returns false if entries were rejected, or true if accepted
func (r *RaftNode) AppendEntries(ae AppendEntriesStruct, response *RPCResponse) error {
	r.Lock()
	defer r.Unlock()
	if r.verbose {
		log.Printf("AppendEntries(). ae: %s", ae.String())
		log.Printf("My log: %s", r.Log.String())
	}

	response.Term = r.CurrentTerm

	if ae.LeaderID == r.currentLeader {
		if r.verbose {
			log.Println("AppendEntries from leader - reset tickers")
		}
		r.resetTickers()
	}

	// Reply false if term < currentTerm
	if ae.Term < r.CurrentTerm {
		if r.verbose {
			log.Println("AE from stale term")
		}
		response.Term = r.CurrentTerm
		response.Success = false
		return nil
	}

	// TODO - shifting to follower each time is slightly inefficient, but keeps things uniform
	r.shiftToFollower(ae.Term, ae.LeaderID)

	// Reply false if log doesn't contain an entry at prevLogIndex whose term matches prevLogTerm
	if int(ae.PrevLogIndex) >= len(r.Log) || // index out-of-bounds
		r.Log[ae.PrevLogIndex].Term != ae.PrevLogTerm {
		if r.verbose {
			log.Println("my PrevLogTerm does not match theirs")
		}
		response.Term = r.CurrentTerm
		response.Success = false
		return nil
	}

	// If an existing entry conflicts with a new one (same index, but different terms),
	// delete the existing entry and all that follow it
	if r.verbose {
		log.Println("Applying entries...")
	}
	offset := int(ae.PrevLogIndex) + 1
	for i, entry := range ae.Entries {
		if i+offset >= len(r.Log) { // We certainly have no conflict
			if r.verbose {
				log.Printf("Apply without conflict: index=%d", i+offset)
			}
			r.append(entry)
		} else {
			if r.Log[i+offset].Term != ae.Entries[i].Term { // We have conflicting entry
				if r.verbose {
					log.Printf("Conflict - delete suffix! (we have term=%d, they have term=%d). Delete our log from index=%d onwards.", r.Log[i+offset].Term, ae.Entries[i].Term, i+offset)
				}
				r.Log = r.Log[:i+offset] // delete the existing entry and all that follow it
				r.append(entry)          // append the current entry
				log.Printf("\n\nLog: %s\n\n", stringOneLog(r.Log))
			} else if r.Log[i+offset] != entry {
				log.Printf("\nOURS: %s\n\nTHEIRS: %s", r.Log[i+offset].String(), entry.String())
				panic("log safety violation occurred somewhere")
			}
		}
	}

	response.Success = true
	lastIndex := r.getLastLogIndex()

	// Now we need to decide how to set our local commit index
	if ae.LeaderCommit > r.commitIndex {
		r.commitIndex = min(lastIndex, ae.LeaderCommit)
	}
	r.executeLog()
	r.persistState()
	return nil
}

// CandidateLooksEligible allows a raft node to decide whether another host's log is sufficiently up-to-date to become leader
// Returns true if the incoming RequestVote shows that the peer is at least as up-to-date as we are
// See paper section 5.4
func (r *RaftNode) CandidateLooksEligible(candLastLogIdx LogIndex, candLastLogTerm Term) bool {
	ourLastLogTerm := r.getLastLogTerm()
	ourLastLogIdx := r.getLastLogIndex()
	if r.verbose {
		log.Printf("We have: lastLogTerm=%d, lastLogIdx=%d. They have: lastLogTerm=%d, lastLogIdx=%d", ourLastLogTerm, ourLastLogIdx, candLastLogTerm, candLastLogIdx)
	}

	if ourLastLogTerm == candLastLogTerm {
		return candLastLogIdx >= ourLastLogIdx
	}
	return candLastLogTerm >= ourLastLogTerm
}

// TODO - need to decide where to compare lastApplied to commitIndex, and apply log entries to our local state machine

// Vote is called by RPC from a candidate. We can observe the following from the raft.github.io simulation:
//	1) If we get a requestVoteRPC from a future term, we immediately jump to that term and send our vote
//	2) If we are already collecting votes for the next election, and simultaneously get a request from another node to vote for them, we do NOT give them our vote
//    (we've already voted for ourselves!)
//	3) if we've been offline, and wakeup and try to get votes: we get rejections, that also tell us the new term, and we immediately jump to that term as a follower
func (r *RaftNode) Vote(rv RequestVoteStruct, response *RPCResponse) error {
	r.Lock()
	defer r.Unlock()
	if r.verbose {
		log.Println("Vote()")
	}

	defer r.persistState()

	response.Term = r.CurrentTerm

	myLastLogTerm := r.getLastLogTerm()
	myLastLogIdx := r.getLastLogIndex()

	if r.verbose {
		log.Printf("RequestVoteStruct: %s. \nMy node: term: %d, votedFor %d, lastLogTerm: %d, lastLogIdx: %d",
			rv.String(), r.CurrentTerm, r.VotedFor, myLastLogTerm, myLastLogIdx)
	}

	looksEligible := r.CandidateLooksEligible(rv.LastLogIdx, rv.LastLogTerm)

	if rv.Term > r.CurrentTerm {
		r.shiftToFollower(rv.Term, HostID(-1)) // We do not yet know who is leader for this term
	}

	if rv.Term < r.CurrentTerm {
		if r.verbose {
			log.Println("RV from prior term - do not grant vote")
		}
		response.Success = false
	} else if (r.VotedFor == -1 || r.VotedFor == rv.CandidateID) && looksEligible {
		if r.verbose {
			log.Println("Grant vote")
		}
		r.resetTickers()
		response.Success = true
		r.VotedFor = rv.CandidateID
	} else {
		if r.verbose {
			log.Println("Do not grant vote")
		}
		response.Success = false
	}

	return nil
}

func (r *RaftNode) getLastLogIndex() LogIndex {
	if len(r.Log) > 0 {
		return LogIndex(len(r.Log) - 1)
	}
	return LogIndex(0)
}

func (r *RaftNode) getLastLogTerm() Term {
	return Term(r.Log[int(r.getLastLogIndex())].Term)
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

func (r *RaftNode) QuitChan() chan bool {
	return r.quitChan
}

// Start is the entrypoint for a raft node (constructed here or in a test) to begin the protocol
func (r *RaftNode) Start() {
	go r.recvDaemon()
	go r.protocol()
	go r.quitter(duration)
	<-r.quitChan
	log.Println("FINISH EXPERIMENT...")
	r.printResults()
}

// Main Raft protocol
func (r *RaftNode) protocol() {
	r.resetTickers()
	log.Printf("Begin Protocol. verbose: %t", r.verbose)

	for {
		select {
		case m := <-r.incomingChan:
			if m.response.Term > r.CurrentTerm {
				if r.verbose {
					log.Printf("Received reply from hostID %d with higher term: %d and leaderid: %d", m.hostID, m.response.Term, m.response.LeaderID)
				}
				r.shiftToFollower(m.response.Term, m.response.LeaderID)
			}

			switch m.msgType {
			case vote:
				if r.verbose {
					log.Printf("processing vote reply, hostID=%d: response=%s", m.hostID, m.response.String())
				}
				r.Lock()
				if r.state == candidate {
					r.votes[m.hostID] = m.response.Success
					if r.wonElection() {
						r.shiftToLeader()
					}
				}
				r.Unlock()
			case appendEntries:
				r.Lock()
				if r.state == leader { // We might have been deposed
					// Inspect response and update our tracking variables appropriately
					if !m.response.Success {
						// TODO - When responding to AppendEntries, the follower should return success if they do a new append, OR if they already have appended that entry

						prev := r.nextIndex[m.hostID]
						next := max(0, r.nextIndex[m.hostID]-1)
						if r.verbose {
							log.Printf("Decrement nextIndex for hostID %d from %d to %d", m.hostID, prev, next)
						}
						r.nextIndex[m.hostID] = next
					} else {
						prev := r.nextIndex[m.hostID]
						next := prev + LogIndex(m.aeLength)
						if r.verbose {
							log.Printf("Increment nextIndex for hostID %d from %d to %d", m.hostID, prev, next)
						}
						r.matchIndex[m.hostID] = prev
						r.nextIndex[m.hostID] = next
					}
					// TODO - update commit index or nextIndex[] and matchIndex[] based on response
					r.executeLog()
				}
				r.Unlock()
			default:
				panic(fmt.Sprintf("invalid msg type. %s", m.String()))
			}
		case <-r.heartbeatTicker.C: // Send append entries, either empty or full depending on the peer's log index
			if r.state == leader {
				r.Lock()
				r.heartbeatAppendEntriesRPC()
				r.updateCommitIndex()
				r.executeLog()
				r.Unlock()
			}
		case <-r.electionTicker.C: // Periodically time out and start a new election
			if r.state == follower || r.state == candidate {
				if r.verbose {
					log.Println("TIMED OUT - STARTING ELECTION")
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
			return
		}
	}
}

func (r *RaftNode) resetElectionTicker() time.Duration {
	var newTimeout time.Duration = selectElectionTimeout(r.id) * r.timeoutUnits
	if r.verbose {
		log.Printf("new election timeout: %s", newTimeout.String())
	}
	r.electionTicker = *time.NewTicker(newTimeout)
	return newTimeout
}

func (r *RaftNode) resetHeartbeatTicker() time.Duration {
	var newTimeout time.Duration = heartbeatTimeout * r.timeoutUnits
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

func (r *RaftNode) shutdown() {
	log.Println("RAFT NODE SHUTTING DOWN")
	r.quitChan <- true
}

func init() {
	flag.StringVar(&hostfile, "h", "hostfile.json", "name of hostfile")
	flag.IntVar(&duration, "d", 30, "time until node shutdown")
	flag.BoolVar(&verbose, "v", false, "verbose output")

	rand.Seed(time.Now().UTC().UnixNano())
}

// Raft is the entrypoint function for the raft replicated state machine protocol
func Raft() {
	flag.Parse()
	hosts := make(HostMap)
	clients := make(ClientMap)
	quitChan := make(chan bool)

	intID, recvPort := ResolveAllPeers(hosts, clients, hostfile, true)
	id := HostID(intID)

	r := NewRaftNode(id, recvPort, hosts, clients, quitChan)

	log.Printf("RaftNode: %s", r.String())
	r.Start()
}
