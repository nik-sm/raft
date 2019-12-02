Go implementation of [Raft](https://raft.github.io/) from scratch for CS7610: Distributed Systems


# Usage and Overview

Build and run the project with `make`.

The Makefile sets several environment variables to control the basic operation:
- `RAFT_DURATION`: int, seconds to run the experiment before each node will
  quit and shutdown
- `RAFT_VERBOSE`: bool, controls verbose output

The default configuration runs a cluster of 3 Raft nodes, with 2 client nodes.
- Every 1s, each client node will send the next line from their data file to
  the cluster to be applied to the state machine.

View documentation and testable examples with `godoc -http=:6060` and browse to `localhost:6060`.


## Inspecting output

The easiest way to view the "results" of an experiment is to check the final
state of the StateMachine on each Raft node.

The state machine is currently just a dummy implementation consisting of a list
of strings, however this is sufficient for testing correctness.

The state machine safety property in Raft basically just requires that we apply
the same user operations in the same order, and this can easily be checked by
comparing the string lists on each raft node.

For example, we can easily use [jq](https://github.com/stedolan/jq) to diff the
contents:
```bash
make
...
# Once experiment finishes running
...
diff <(cat persistence/*r0* | jq '.StateMachine.Contents') <(cat persistence/*r1* | jq '.StateMachine.Contents') 
diff <(cat persistence/*r0* | jq '.StateMachine.Contents') <(cat persistence/*r2* | jq '.StateMachine.Contents') 
```

Notice that the formal requirement only says we cannot have different actions
at the same index in the StateMachine; it is valid to have one shorter than
another (in this case due to our forced early shutdown procedure).

## Demo and Integration Tests

`make test_stop1` - run the cluster for 60s, and stop one of the nodes after
20s.
- Notice that the cluster successfully runs to completion, and the remaining
  nodes are consistent with each other.

`make test_stop2` - run for 60s and stop two nodes after 20s.
- Notice that progress halts, as desired.

`make test_disconnect1` - run for 60s, and disconnect one node from the network
after 20s
- As above, progress continues and the remaining nodes stay consistent.

`make test_disconnect2` - analogous to stopping 2 above

`make test_disconnect_reconnect` - run for 60s, at 20s disconnect one node, and
then at 40s reconnect the node.
- Notice that it catches up, and the cluster finishes with all nodes
  consistent.

# Contents

```
.
├── Dockerfile
├── Makefile
├── README.md
├── TODO.md
├── datafile.0.txt                     # Data for client 0
├── datafile.1.txt                     # Data for client 1
├── docker-compose.yaml
├── hostfile.json                      # Hostname and receiving port for all Raft and client nodes
├── main_client.go
├── main_raft.go
├── raft_proposal.md                   # Original project proposal
└── src
    ├── client
    │   └── client.go
    └── raft
        ├── persist.go
        ├── raft.go
        ├── raft_integration_test.go   # Work-in-progress integration tests
        ├── raft_unit_test.go
        ├── rpc.go
        ├── time_constants.go
        ├── types.go
        └── utils.go
```

# Notes

So far, this is a simplified implementation, focusing on the basics:
- Leader Election
- Log Replication
- (Very basic) State Machine

Among others, the following aspects of the full Raft protocol are not yet
implemented:
- “Robust” client interaction. 
  - Currently, leader replies success after storing, but should actually wait
    for data to be added to StateMachine (i.e. be replicated on majority of
      logs) first
- Thorough integration tests
  - Can use the JSON outputs from `r.persistState()`
  - Already in-progress in `raft_integration_test.go` is to use different ports
    and run multiple nodes on localhost
    - Alternative: Add layer of indirection before making RPC calls, and
      substitute a local/mocked transport layer during testing
- Test thoroughly for deadlocks and race conditions
  - For example, when sending ApplieEntriesRPC, we need to ensure we are still
    leader, term has not changed, nextIndex[] has not changed, etc
- More interesting StateMachine
  - Straightforward addition; for example using a fixed list of integer
    operators (`+`,`-`,`*`,`/`) on a finite list of integer variables
    (`x`,`y`,`z`)
- Log Compaction
- Resuming from persisted state
- Adding nodes to running cluster (“Membership Changes”)
- Optimizations for fewer back-and-forth failed AppendEntriesRPC when follower
  needs to catch up

# Caveats

- There is still at least one race condition, so experiments can sporadically
  fail
  - When a follower node replies slowly to a heartbeat and then quickly to the
    next non-trivial AppendEntriesRPC, the leader node may advance that
    follower's `nextIndex[i]` value too far. Instead of advancing by 0 (for
    heartbeat) + N (for the N entries successfully appended), it will do N + N.
- Numerous TODOs left, see especially:
  - `TODO.md`
  - `raft_proposal.md`
- Code needs cleanup and more tests

# Key Phrases from Raft Paper

- "servers retry RPCs if they do not receive a response in a timely manner, and
  they issue RPCs in parallel for best performance"
  - we have at least 3 timeouts happening:
    - heartbeat
    - election
    - retryRPC
  - the response from RPC needs to happen fast, can't wait around too long
    collecting other info before responding. Notice that we SHOULD be doing
    some writing to stable storage though.
    - For now, let the "writeToStableStorage" method be a no-op

- "follower and candidate crashes" 
  - "raft handles these failures by retrying indefinitely"... " if the crashed
    server restarts, then the RPC will complete successfully"

- "raft RPCs are idempotent" ... "if a follower receives an appendentries
  request that includes log entries already present in its log, it ignores
  those entries in the new request"

- "timing requirement" "broadcastTime << electionTimeout << MTBF" where MTBF is
  average time between failures for a single server
  - broadcast time should be 10x faster than election timeout
  - election timeout should be "a few orders of magnitude less" than MTBF

- "raft's rpcs typically require the recipient to persist information to stable
  storage, so the broadcast time may range from 0.5ms to 20ms"
  - "election timeout is likely to be somewhere between 10ms and 500ms"

- "client interaction"
  - "if client's first choice is not the leader, that server will reject the
    client's request and supply information about the most recent leader it has
    heard from. (AppendEntries requests include the network address of the
    leader). If the leader crashes, client requests will time out; clients then
    try again with randomly-chosen servers"
    - We have another timeout to consider: clientSendData timeout. After
      timeout, client should select randomly among the remaining servers
      - upon entering the "clientSendDataUnknownLeader" method, we should build
        a list of all the nodes. Client picks one randomly to talk to.  if they
        timeout without response, remove that node from list, and try another,
        etc.  return from the method with the information about the successful
        leader

- Avoiding executing a command multiple times on the state machine:
  - "state machine tracks the latest serial number processed for each client,
    along with the associated response.  if it receives a command whose serial
    number has already been executed, it responds immediately without
    re-executing the request"
  - "[a leader] needs to commit an entry from its term. ... each leader [must]
    commit a blank no-opentry into the log at the start of ites term"

# Useful links

- https://raft.github.io/raft.pdf
- https://golang.org/pkg/net/rpc/
- https://github.com/ongardie/raft.tla/blob/master/raft.tla
- https://web.stanford.edu/~ouster/cgi-bin/papers/OngaroPhD.pdf

