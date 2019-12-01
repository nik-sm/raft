package raft

import (
	"encoding/json"
	"fmt"
	"os"
)

var raft_storage string = "/persistence/raft_node.%d.json"

func (r *RaftNode) persistState() {
	f, err := os.Create(fmt.Sprintf(raft_storage, r.id))
	if err != nil {
		panic(err)
	}
	enc := json.NewEncoder(f)
	err = enc.Encode(r)
	if err != nil {
		panic(err)
	}
	f.Close()
}

func (r *RaftNode) recoverFromDisk() {
	f, err := os.Open(fmt.Sprintf(raft_storage, r.id))
	if err != nil {
		// TODO - check if exists, otherwise skip
		panic(err)
	}
	dec := json.NewDecoder(f)
	var r2 RaftNode
	err = dec.Decode(&r2)
	if err != nil {
		panic(err)
	}
	f.Close()

	r.CurrentTerm = r2.CurrentTerm
	r.VotedFor = r2.VotedFor
	r.Log = r2.Log
	r.StateMachine = r2.StateMachine
}
