package raft_test

import (
	"fmt"
	"raft"
)

type term = raft.Term

// Prepare a test, specifying:
// - term, lastLogTerm, and lastLogIdx of candidate
// - term, lastLogTerm, and number of log entries for voter
// Notice that we need to populate the voter's log with dummy entries
func setupVoteTests(
	vTerm term, vLastLogTerm term, vNumLogEntries int,
	cTerm term, cLastLogTerm term, cLastLogIdx raft.LogIndex) raft.RPCResponse {

	// Build list of log entries of specified length
	entries := make([]raft.LogEntry, 0, 0)
	for i := 0; i < vNumLogEntries; i++ {
		entries = append(entries, raft.NewLogEntry(
			vLastLogTerm,                        // term
			"",                                  // clientData
			raft.ClientID(0),                    // clientID
			raft.ClientSerialNum(0),             // clientSerialNum
			raft.ClientResponse{Success: true})) // clientResponse
	}

	// Construct voter node
	voter := raft.NewRaftNodeSpecial(
		raft.HostID(0),
		make(raft.HostMap),   // Empty hosts map
		make(raft.ClientMap), // Empty clients map
		nil,                  // Empty 'quit' channel
		vTerm,                // Voter starting term
		entries)

	// Sanity check the ballot info we will use
	if !voter.TheyAreUpToDate(cLastLogIdx, cLastLogTerm) {
		fmt.Println("Test setup turned out to be invalid")
	}

	// Prepare inputs for the RPC
	args := raft.RequestVoteStruct{T: cTerm,
		CandidateID: 1,
		LastLogIdx:  cLastLogIdx,
		LastLogTerm: cLastLogTerm}
	reply := raft.RPCResponse{}

	// Cast the Vote, and check for errors
	err := voter.Vote(args, &reply)
	if err != nil {
		fmt.Println("Error occurred during voting")
	}

	return reply
}

// TODO - resume here
func ExampleVote() {
	reply := setupVoteTests(
		term(5),          // voter's term
		term(4),          // voter's lastLogTerm
		3,                // voter's lastLogIndex
		term(5),          // candidate's term
		term(4),          // candidate's lastLogTerm
		raft.LogIndex(3)) // candidate's lastLogIndex

	fmt.Printf("reply.Success=%t, reply.Term=%d", reply.Success, reply.Term)
	// Output: reply.Success=true, reply.Term=5
}

func ExampleVote_logOutdated() {

}

//func ExampleVote_termOutdated() {}
