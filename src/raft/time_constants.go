package raft

import (
	"math/rand"
	"time"
)

//const electionTimeoutMinimum = time.Duration(150 * time.Millisecond)
//const electionTimeoutMaximum = time.Duration(300 * time.Millisecond)
//const heartbeatTimeout = time.Duration(40 * time.Millisecond)

const electionTimeoutMinimum = 300
const electionTimeoutMaximum = 600
const heartbeatTimeout = 1

const fakeHeartbeatTimeout = time.Duration(100 * time.Second) // TODO - this is a workaround to avoid handling null tickers on followers

func selectElectionTimeout() time.Duration {
	return time.Duration(rand.Intn(electionTimeoutMaximum-electionTimeoutMinimum+1) + electionTimeoutMinimum)
}
