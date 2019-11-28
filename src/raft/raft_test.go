package raft

import (
	"fmt"
	"strings"
	"time"
)

// Prepare a test, specifying term, lastLogTerm, and lastLogIdx of voter
func mockRaftNode(vTerm Term, vLastLogTerm Term, vLastLogIdx int) *RaftNode {
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
	voter.currentTerm = vTerm
	voter.log = log
	voter.verbose = false

	return voter
}

// Ticker tests

func ExampleRaftNode_resetTickers_notTooFast() {
	r := mockRaftNode(Term(0), Term(0), 0)
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
	r := mockRaftNode(Term(0), Term(0), 0)
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
		3)       // voter's lastLogIndex

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
		3)       // voter's lastLogIndex

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
		3)       // voter's lastLogIndex

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
		3)       // voter's lastLogIndex

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
		3)       // voter's lastLogIndex

	candidateLastLogTerm := Term(4)
	candidateLastLogIdx := LogIndex(2)

	result := voter.CandidateLooksEligible(candidateLastLogIdx, candidateLastLogTerm)
	fmt.Println(result)
	// Output: false
}

// Vote tests

func ExampleRaftNode_Vote_futureTermSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3)       // voter's lastLogIndex

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		CandidateID: 1,
		Term:        8,
		LastLogTerm: 4,
		LastLogIdx:  3}
	reply := RPCResponse{}

	// Cast Vote, and check for errors
	err := voter.Vote(args, &reply)
	if err != nil {
		fmt.Println("Error occurred during voting")
	}

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=8
}

func ExampleRaftNode_Vote_sameTermFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3)       // voter's lastLogIndex

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		CandidateID: 1,
		Term:        5,
		LastLogTerm: 4,
		LastLogIdx:  3}
	reply := RPCResponse{}

	// Cast Vote, and check for errors
	err := voter.Vote(args, &reply)
	if err != nil {
		fmt.Println("Error occurred during voting")
	}

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=false, reply.Term=5
}

func ExampleRaftNode_Vote_prevTermFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3)       // voter's lastLogIndex

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		CandidateID: 1,
		Term:        4,
		LastLogTerm: 4,
		LastLogIdx:  3}
	reply := RPCResponse{}

	// Cast Vote, and check for errors
	err := voter.Vote(args, &reply)
	if err != nil {
		fmt.Println("Error occurred during voting")
	}

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=false, reply.Term=5
}

func ExampleRaftNode_Vote_futureLogTermSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3)       // voter's lastLogIndex

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		CandidateID: 1,
		Term:        9,
		LastLogTerm: 8,
		LastLogIdx:  3}
	reply := RPCResponse{}

	// Cast Vote, and check for errors
	err := voter.Vote(args, &reply)
	if err != nil {
		fmt.Println("Error occurred during voting")
	}

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=9
}

func ExampleRaftNode_Vote_futureLogIdxSucceeds() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3)       // voter's lastLogIndex

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		CandidateID: 1,
		Term:        9,
		LastLogTerm: 4,
		LastLogIdx:  8}
	reply := RPCResponse{}

	// Cast Vote, and check for errors
	err := voter.Vote(args, &reply)
	if err != nil {
		fmt.Println("Error occurred during voting")
	}

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=9
}

func ExampleRaftNode_Vote_badLogTermFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3)       // voter's lastLogIndex

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		CandidateID: 1,
		Term:        8,
		LastLogTerm: 3,
		LastLogIdx:  3}
	reply := RPCResponse{}

	// Cast Vote, and check for errors
	err := voter.Vote(args, &reply)
	if err != nil {
		fmt.Println("Error occurred during voting")
	}

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=false, reply.Term=8
}

func ExampleRaftNode_Vote_badLogIdxFails() {
	// Mock voter
	voter := mockRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3)       // voter's lastLogIndex

	// Prepare inputs for the RPC
	args := RequestVoteStruct{
		CandidateID: 1,
		Term:        8,
		LastLogTerm: 4,
		LastLogIdx:  2}
	reply := RPCResponse{}

	// Cast Vote, and check for errors
	err := voter.Vote(args, &reply)
	if err != nil {
		fmt.Println("Error occurred during voting")
	}

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=false, reply.Term=8
}
