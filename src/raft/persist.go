package raft

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

var raftStorage string = "/persistence/raft_node.%d.json"

func (r *RaftNode) persistState() {
	f, err := os.Create(fmt.Sprintf(raftStorage, r.id))
	if err != nil {
		log.Println("WARNING - cannot persistState:", err)
		return
	}
	enc := json.NewEncoder(f)
	err = enc.Encode(r)
	if err != nil {
		panic(err)
	}
	f.Close()
}

func (r *RaftNode) recoverFromDisk() {
	f, err := os.Open(fmt.Sprintf(raftStorage, r.id))
	if err != nil {
		// TODO - check if exists, otherwise skip
		log.Println("WARNING - cannot persistState:", err)
		return
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
