package raft

import (
	"math/rand"
	"time"
)

//const electionTimeoutMinimum = time.Duration(150 * time.Millisecond)
//const electionTimeoutMaximum = time.Duration(300 * time.Millisecond)
//const heartbeatTimeout = time.Duration(40 * time.Millisecond)

const electionTimeoutMinimum = time.Duration(5 * time.Second)
const electionTimeoutMaximum = time.Duration(10 * time.Second)
const heartbeatTimeout = time.Duration(1 * time.Millisecond)

const fakeHeartbeatTimeout = time.Duration(100 * time.Second) // TODO - this is a workaround to avoid handling null tickers on followers

func selectElectionTimeout() time.Duration {
	electionTimeoutMinimum := 150
	electionTimeoutMaximum := 300
	return time.Duration((rand.Intn(electionTimeoutMaximum-electionTimeoutMinimum+1) + electionTimeoutMinimum)) * time.Millisecond
}
