package raft

import (
	"math/rand"
	"time"
)

//const electionTimeoutMinimum = time.Duration(150 * time.Millisecond)
//const electionTimeoutMaximum = time.Duration(300 * time.Millisecond)
//const heartbeatTimeout = time.Duration(40 * time.Millisecond)

const electionTimeoutMinimum = 3
const electionTimeoutMaximum = 6
const timeoutUnits = time.Second
const heartbeatTimeout = electionTimeoutMinimum / 2

const fakeHeartbeatTimeout = 10 * electionTimeoutMaximum // TODO - this is a workaround to avoid handling null tickers on followers

func selectElectionTimeout(id HostID) time.Duration {
	return time.Duration(rand.Intn(electionTimeoutMaximum-electionTimeoutMinimum+1) + electionTimeoutMinimum)
}
