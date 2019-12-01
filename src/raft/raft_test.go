package raft

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Prepare a test, specifying term, lastLogTerm, and lastLogIdx of voter
func mockRaftNode(vTerm Term, vLastLogTerm Term, vLastLogIdx int, vLeader HostID) *RaftNode {
	// Build dummy log for voter
	log := make([]LogEntry, 0)
	for i := 0; i <= vLastLogIdx; i++ {
		log = append(log, NewLogEntry(
			vLastLogTerm,                   // Term
			"",                             // clientData
			ClientID(0),                    // clientID
			ClientSerialNum(0),             // clientSerialNum
			ClientResponse{Success: true})) // clientResponse
	}

	// Construct voter node
	voter := NewRaftNode(
		HostID(0),
		make(HostMap),   // Empty hosts map
		make(ClientMap), // Empty clients map
		nil)             // Empty 'quit' channel

	// Set voter attributes as specified
	voter.CurrentTerm = vTerm
	voter.Log = log
	voter.verbose = false
	voter.currentLeader = vLeader
	voter.VotedFor = vLeader

	return voter
}

// Prepare a test, specifying term, lastLogTerm, and lastLogIdx of voter
func mockRaftNodeNoLog(vTerm Term, vLeader HostID) *RaftNode {
	// Empty log
	log := make([]LogEntry, 0)

	// Construct voter node
	voter := NewRaftNode(
		HostID(0),
		make(HostMap),   // Empty hosts map
		make(ClientMap), // Empty clients map
		nil)             // Empty 'quit' channel

	// Set voter attributes as specified
	voter.CurrentTerm = vTerm
	voter.Log = log
	voter.verbose = false
	voter.currentLeader = vLeader
	voter.VotedFor = vLeader

	return voter
}

// Ticker tests

func ExampleRaftNode_resetTickers_notTooFast() {
	r := mockRaftNode(Term(0), Term(0), 0, 0)
	r.timeoutUnits = time.Millisecond

	var result strings.Builder
	result.WriteString("begin.")
	nIteration := 5
	for i := 0; i < nIteration; i++ {
		var electionTimeout, _ time.Duration = r.resetTickers()
		var half time.Duration = electionTimeout / 2

	Loop:
		for {
			select {
			case <-time.After(half): // should always trigger first
				break Loop
			case <-r.electionTicker.C:
				result.WriteString("bad.")
				break Loop
			}
		}
	}

	result.WriteString("end")
	fmt.Println(result.String())
	// Output: begin.end
}

func ExampleRaftNode_resetTickers_notTooSlow() {
	r := mockRaftNode(Term(0), Term(0), 0, 0)
	r.timeoutUnits = time.Millisecond

	var result strings.Builder
	result.WriteString("begin.")
	nIteration := 5
	for i := 0; i < nIteration; i++ {
		var electionTimeout, _ time.Duration = r.resetTickers()
		var half time.Duration = electionTimeout / 2

	Loop:
		for {
			select {
			case <-r.electionTicker.C: // should always trigger first
				break Loop
			case <-time.After(electionTimeout + half):
				result.WriteString("bad.")
				break Loop
			}
		}
	}

	result.WriteString("end")
	fmt.Println(result.String())
	// Output: begin.end
}

// Log eligibility tests

func ExampleRaftNode_CandidateLooksEligible_futureLogTermSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		0)       // voter's currentLeader

	candidateLastLogTerm := Term(8)
	candidateLastLogIdx := LogIndex(1)

	result := voter.CandidateLooksEligible(candidateLastLogIdx, candidateLastLogTerm)
	fmt.Println(result)
	// Output: true
}

func ExampleRaftNode_CandidateLooksEligible_futureLogIdxSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		0)       // voter's currentLeader

	candidateLastLogTerm := Term(4)
	candidateLastLogIdx := LogIndex(8)

	result := voter.CandidateLooksEligible(candidateLastLogIdx, candidateLastLogTerm)
	fmt.Println(result)
	// Output: true
}

func ExampleRaftNode_CandidateLooksEligible_sameLogTermLogIdxSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		0)       // voter's currentLeader

	candidateLastLogTerm := Term(4)
	candidateLastLogIdx := LogIndex(3)

	result := voter.CandidateLooksEligible(candidateLastLogIdx, candidateLastLogTerm)
	fmt.Println(result)
	// Output: true
}

func ExampleRaftNode_CandidateLooksEligible_badLogTermFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		0)       // voter's currentLeader

	candidateLastLogTerm := Term(1)
	candidateLastLogIdx := LogIndex(9)

	result := voter.CandidateLooksEligible(candidateLastLogIdx, candidateLastLogTerm)
	fmt.Println(result)
	// Output: false
}

func ExampleRaftNode_CandidateLooksEligible_badLogIdxFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		0)       // voter's currentLeader

	candidateLastLogTerm := Term(4)
	candidateLastLogIdx := LogIndex(2)

	result := voter.CandidateLooksEligible(candidateLastLogIdx, candidateLastLogTerm)
	fmt.Println(result)
	// Output: false
}

///////////////////////////////////////////////////////////////////////////
// Vote tests
///////////////////////////////////////////////////////////////////////////

// TODO - These tests would be better represented with a state table and checking all permutations

func (voter *RaftNode) commonVote(args RequestVoteStruct, reply *RPCResponse) {
	// Cast Vote, and check for errors
	err := voter.Vote(args, reply)
	if err != nil {
		fmt.Println("Error occurred during voting")
	}
}

func ExampleRaftNode_Vote_futureTermSucceedsSameLeader() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        8,
		LastLogTerm: 4,
		LastLogIdx:  3,
		CandidateID: 2}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=5
}

func ExampleRaftNode_Vote_futureTermSucceedsNewLeader() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        8,
		LastLogTerm: 4,
		LastLogIdx:  3,
		CandidateID: 1}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=5
}

func ExampleRaftNode_Vote_sameTermSameLeaderSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        5,
		LastLogTerm: 4,
		LastLogIdx:  3,
		CandidateID: 2}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=5
}

func ExampleRaftNode_Vote_sameTermNewLeaderFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        5,
		LastLogTerm: 4,
		LastLogIdx:  3,
		CandidateID: 1}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=false, reply.Term=5
}

func ExampleRaftNode_Vote_prevTermSameLeaderFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        4,
		LastLogTerm: 4,
		LastLogIdx:  3,
		CandidateID: 2}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=false, reply.Term=5
}

func ExampleRaftNode_Vote_prevTermNewLeaderFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        4,
		LastLogTerm: 4,
		LastLogIdx:  3,
		CandidateID: 1}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=false, reply.Term=5
}

func ExampleRaftNode_Vote_futureLogTermSameLeaderSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        6,
		LastLogTerm: 8,
		LastLogIdx:  3,
		CandidateID: 2}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=5
}

func ExampleRaftNode_Vote_futureLogTermNewLeaderSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        6,
		LastLogTerm: 8,
		LastLogIdx:  3,
		CandidateID: 1}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=5
}

func ExampleRaftNode_Vote_futureLogIdxSameLeaderSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        6,
		LastLogTerm: 8,
		LastLogIdx:  4,
		CandidateID: 2}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=5
}

func ExampleRaftNode_Vote_futureLogIdxNewLeaderSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        6,
		LastLogTerm: 8,
		LastLogIdx:  4,
		CandidateID: 1}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=5
}

func ExampleRaftNode_Vote_badLogTermFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        6,
		LastLogTerm: 3,
		LastLogIdx:  3,
		CandidateID: 2}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=false, reply.Term=5
}

func ExampleRaftNode_Vote_badLogIdxFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		Term:        6,
		LastLogTerm: 4,
		LastLogIdx:  2,
		CandidateID: 2}
	reply := RPCResponse{}

	voter.commonVote(args, &reply)

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=false, reply.Term=5
}

///////////////////////////////////////////////////////////////////////////
// AppendEntries tests
///////////////////////////////////////////////////////////////////////////

// Make a dummy log of the specified length, with all entries from the specified term
func mockLog(lastLogTerm Term, lastIdx int) []LogEntry {
	log := make([]LogEntry, 0)
	for i := 0; i <= lastIdx; i++ {
		log = append(log, NewLogEntry(
			lastLogTerm,        // Term
			"newdata",          // clientData
			ClientID(0),        // clientID
			ClientSerialNum(0), // clientSerialNum
			ClientResponse{}))  // clientResponse
	}
	return log
}

func sameLog(l1 []LogEntry, l2 []LogEntry) bool {
	if len(l1) != len(l2) {
		return false
	}
	for i, e1 := range l1 {
		e2 := l2[i]
		if e1 != e2 {
			//		  e1.Term != e2.Term ||
			//			e1.ClientData != e2.ClientData ||
			//			e1.ClientSerialNum != e2.ClientSerialNum ||
			//			e1.ClientResponse != e2.ClientResponse {
			return false
		}
	}
	return true
}

// Sanity check so we can simply do l1 == l2 for LogEntry structs
func TestLogEntryEquality(t *testing.T) {
	a := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	b := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	if a != b {
		t.Errorf("struct equality on LogEntry fails")
	}
}

func TestLogEntryUnequality_term(t *testing.T) {
	a := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	b := LogEntry{
		Term:            999,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	if a == b {
		t.Errorf("struct equality on LogEntry fails")
	}
}

func TestLogEntryUnequality_clientData(t *testing.T) {
	a := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	b := LogEntry{
		Term:            1,
		ClientData:      "FOOOOOBARRRRRRRR",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	if a == b {
		t.Errorf("struct equality on LogEntry fails")
	}
}

func TestLogEntryUnequality_clientID(t *testing.T) {
	a := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	b := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        9999,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	if a == b {
		t.Errorf("struct equality on LogEntry fails")
	}
}

func TestLogEntryUnequality_clientSerialNum(t *testing.T) {
	a := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	b := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 999,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	if a == b {
		t.Errorf("struct equality on LogEntry fails")
	}
}

func TestLogEntryUnequality_clientResponseBool(t *testing.T) {
	a := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	b := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: false,
			Leader:  4}}

	if a == b {
		t.Errorf("struct equality on LogEntry fails")
	}
}

func TestLogEntryUnequality_clientResponseLeader(t *testing.T) {
	a := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  4}}

	b := LogEntry{
		Term:            1,
		ClientData:      "contents",
		ClientID:        2,
		ClientSerialNum: 3,
		ClientResponse: ClientResponse{
			Success: true,
			Leader:  999}}

	if a == b {
		t.Errorf("struct equality on LogEntry fails")
	}
}

func setupAppendEntriesTest() (*RaftNode, []LogEntry) {
	// Templated LogEntries to use
	a := LogEntry{
		Term:       1,
		ClientData: "aaaaa"}

	b := LogEntry{
		Term:       2,
		ClientData: "bbbbb"}

	// Setup follower
	follower := mockRaftNodeNoLog(
		Term(5), // follower's Term
		2)       // follower's currentLeader

	followerLog := make([]LogEntry, 0)
	for i := 0; i < 3; i++ {
		followerLog = append(followerLog, a)
	}
	followerLog = append(followerLog, b)
	follower.Log = followerLog

	// Setup leaderLog
	leaderLog := make([]LogEntry, 0)
	for i := 0; i < 5; i++ {
		leaderLog = append(leaderLog, a)
	}

	return follower, leaderLog
}

// Setup dummy follower, and also give the result of appending to their log
// starting after "prevIdx"
// NOTE - the entire leaderLog is appended, because this is what we are requesting during
// an AppendEntriesRPC
// result = append(followerLog[:split+1], leaderLog...)
func setupAppendEntriesTestWithSplice(prevIdx int) (*RaftNode, []LogEntry, []LogEntry) {
	// Templated LogEntries to use
	a := LogEntry{
		Term:       1,
		ClientData: "aaaaa"}

	// Setup follower
	follower := mockRaftNodeNoLog(
		Term(5), // follower's Term
		2)       // follower's currentLeader

	followerLog := make([]LogEntry, 0)
	for i := 0; i <= 6; i++ {
		followerLog = append(followerLog, a)
	}
	follower.Log = followerLog

	b := LogEntry{
		Term:       2,
		ClientData: "bbbbb"}

	// Setup leaderLog
	leaderLog := make([]LogEntry, 0)
	for i := 0; i < 5; i++ {
		leaderLog = append(leaderLog, a)
	}
	for i := 0; i < 2; i++ {
		leaderLog = append(leaderLog, b)
	}

	spliceLog := append(followerLog[:prevIdx+1], leaderLog...)
	return follower, leaderLog, spliceLog
}

func ExampleRaftNode_AppendEntries_oldTermFails() {
	follower, leaderLog := setupAppendEntriesTest()

	ae := AppendEntriesStruct{
		Term:         4,
		LeaderID:     2,
		PrevLogIndex: 0,
		PrevLogTerm:  1, // matches the entry of follower's log
		Entries:      leaderLog,
		LeaderCommit: 8}

	response := RPCResponse{}

	follower.AppendEntries(ae, &response)
	fmt.Println(response.Success)
	// Output: false
}

func ExampleRaftNode_AppendEntries_badPrevLogTermFails() {
	follower, leaderLog := setupAppendEntriesTest()

	ae := AppendEntriesStruct{
		Term:         5,
		LeaderID:     2,
		PrevLogIndex: 0,   // follower has this position
		PrevLogTerm:  999, // does not match follower's term at that position
		Entries:      leaderLog,
		LeaderCommit: 8}

	response := RPCResponse{}

	follower.AppendEntries(ae, &response)
	fmt.Println(response.Success)
	// Output: false
}

func ExampleRaftNode_AppendEntries_badPrevLogIdxFails() {
	follower, leaderLog := setupAppendEntriesTest()

	ae := AppendEntriesStruct{
		Term:         5,
		LeaderID:     2,
		PrevLogIndex: 999, // follower does not have this position
		PrevLogTerm:  0,
		Entries:      leaderLog,
		LeaderCommit: 8}

	response := RPCResponse{}

	follower.AppendEntries(ae, &response)
	fmt.Println(response.Success)
	// Output: false
}

func ExampleRaftNode_AppendEntries_extendAndDeleteSuffixSucceeds() {
	prevLogIdx := 2
	follower, leaderLog, resultLog := setupAppendEntriesTestWithSplice(2)

	ae := AppendEntriesStruct{
		Term:         5,
		LeaderID:     2,
		PrevLogIndex: LogIndex(prevLogIdx), // This time, append after position 2
		PrevLogTerm:  1,                    // matches follower's term at that position
		Entries:      leaderLog,
		LeaderCommit: 8}
	response := RPCResponse{}

	follower.AppendEntries(ae, &response)

	fmt.Println(sameLog(follower.Log, resultLog))
	// Output: true
}

func ExampleRaftNode_AppendEntries_unusedIdxSucceeds() {
	prevLogIdx := 6
	follower, leaderLog, resultLog := setupAppendEntriesTestWithSplice(prevLogIdx)

	ae := AppendEntriesStruct{
		Term:         5,
		LeaderID:     2,
		PrevLogIndex: LogIndex(prevLogIdx), // follower has this position
		PrevLogTerm:  1,                    // matches follower's term at that position
		Entries:      leaderLog,
		LeaderCommit: 8}

	response := RPCResponse{}

	follower.AppendEntries(ae, &response)
	fmt.Println(sameLog(follower.Log, resultLog))
	// Output: true
}

func ExampleRaftNode_AppendEntries_validHeartbeatSucceeds() {
	prevLogIdx := 6
	follower, _, _ := setupAppendEntriesTestWithSplice(prevLogIdx)

	ae := AppendEntriesStruct{
		Term:         5,
		LeaderID:     2,
		PrevLogIndex: LogIndex(prevLogIdx), // follower has this position
		PrevLogTerm:  1,                    // matches follower's term at that position
		Entries:      make([]LogEntry, 0),
		LeaderCommit: 8}

	response := RPCResponse{}

	follower.AppendEntries(ae, &response)
	fmt.Println(response.Success)
	// Output: true
}

func ExampleRaftNode_AppendEntries_invalidHeartbeatPrevLogTermFails() {
	prevLogIdx := 6
	follower, _, _ := setupAppendEntriesTestWithSplice(prevLogIdx)

	ae := AppendEntriesStruct{
		Term:         5,
		LeaderID:     2,
		PrevLogIndex: LogIndex(prevLogIdx), // follower has this position
		PrevLogTerm:  0,                    // does NOT match follower's term at that position
		Entries:      make([]LogEntry, 0),
		LeaderCommit: 8}

	response := RPCResponse{}

	follower.AppendEntries(ae, &response)
	fmt.Println(response.Success)
	// Output: false
}

func ExampleRaftNode_AppendEntries_invalidHeartbeatPrevLogIdxFails() {
	prevLogIdx := 6
	follower, _, _ := setupAppendEntriesTestWithSplice(prevLogIdx)

	ae := AppendEntriesStruct{
		Term:         5,
		LeaderID:     2,
		PrevLogIndex: LogIndex(prevLogIdx + 1), // follower has this position
		PrevLogTerm:  1,                        // does NOT match follower's term at that position
		Entries:      make([]LogEntry, 0),
		LeaderCommit: 8}

	response := RPCResponse{}

	follower.AppendEntries(ae, &response)
	fmt.Println(response.Success)
	// Output: false
}
