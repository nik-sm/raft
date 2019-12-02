Go implementation of [Raft](https://raft.github.io/) from scratch for CS7610: Distributed Systems

# Useful links:
- https://github.com/ongardie/raft.tla/blob/master/raft.tla
- https://web.stanford.edu/~ouster/cgi-bin/papers/OngaroPhD.pdf
- https://golang.org/pkg/net/rpc/#Dial
- https://raft.github.io/raft.pdf

# Usage and Overview
Build and run the project with `make`.

The Makefile sets several environment variables to control the basic operation:
- `RAFT_DURATION`: int, seconds to run the experiment before each node will quit and shutdown
- `RAFT_VERBOSE`: bool, controls verbose output

The default configuration runs a cluster of 3 Raft nodes, with 2 client nodes.
- Every 1s, each client node will send the next line from their data file to the cluster to be applied to the state machine.

## Inspecting output
The easiest way to view the "results" of an experiment is to check the final state of the StateMachine on each Raft node.

The state machine is currently just a dummy implementation consisting of a list of strings, however this is sufficient for testing correctness.

The state machine safety property in Raft basically just requires that we apply the same user operations in the same order, and this can easily be checked by comparing the string lists on each raft node.

For example, we can easily use [jq](https://github.com/stedolan/jq) to diff the contents:
```bash
make
...
# Once experiment finishes running
...
diff <(cat persistence/*r0* | jq '.StateMachine.Contents') <(cat persistence/*r1* | jq '.StateMachine.Contents') 
diff <(cat persistence/*r0* | jq '.StateMachine.Contents') <(cat persistence/*r2* | jq '.StateMachine.Contents') 
```

Notice that the formal requirement only says we cannot have different actions at the same index in the StateMachine; it is valid to have one shorter than another (in this case due to our forced early shutdown procedure).

## Demo and Integration Tests
`make test_stop1` - run the cluster for 60s, and stop one of the nodes after 20s.
- Notice that the cluster successfully runs to completion, and the remaining nodes are consistent with each other.

`make test_stop2` - run for 60s and stop two nodes after 20s.
- Notice that progress halts, as desired.

`make test_disconnect1` - run for 60s, and disconnect one node from the network after 20s
- As above, progress continues and the remaining nodes stay consistent.

`make test_disconnect2` - analogous to above

`make test_disconnect_reconnect` - run for 60s, at 20s disconnect one node, and then at 40s reconnect the node.
- Notice that it catches up, and the cluster finishes with all nodes consistent.

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

# Notes and Caveats
- There is still at least one race condition, so experiments can sporadically fail
  - When a follower node replies slowly to a heartbeat and then quickly to the next 
    non-trivial AppendEntriesRPC, the leader node may advance that follower's `nextIndex[i]`
    value too far. Instead of advancing by 0 (for heartbeat) + N (for the N entries successfully appended),
    it will do N + N.
- Numerous TODOs left, see especially:
  - `TODO.md`
  - `raft_proposal.md`
- Code needs cleanup and more tests
