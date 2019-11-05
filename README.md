# Usage
Run the project with `make`.

The Makefile sets several environment variables to control the basic operation:
- `$PRJ2_TEST_CASE`: Valid values are 1,2,3,4,5. This selects one of the test cases as described in the assignment
- `$PRJ2_DURATION`: This integer value specifies how many seconds each node will run before shutting themselves down and printing their view history. (Notice that nodes may terminate early, based on the test case being run)
- `$PRJ2_VERBOSE`: This boolean controls the amount of log information printed by each node
- `$PRJ2_TFROM`, `$PRJ2_TTIL`: Each node will select a random election timeout interval in this range. Set these to the same number to have a deterministic election timeout interval.

NOTE - during the basic operation, for example in Test Case 1, the network will not simply stop after a single view change; instead it will continue changing in a ring until the full `$PRJ2_DURATION` has elapsed.
NOTE - `docker-compose` version 1.17 (latest available through `apt`) is insufficient; requires [`1.24.1`](https://docs.docker.com/compose/install/)
NOTE - the timeout range specified for the `electionTicker` must be larger than the values used for `proofTicker`

# Test Results
Test results file created the following bash one-liner, and then cutting out `docker-compose` build output with a text editor for clarity:
```bash
rm TEST_RESULTS.txt; for t in test1 test2 test3 test4 test5; do { printf "\n\n##########\n${t}\n##########\n"; make $t; } >> TEST_RESULTS.txt; done
```

NOTE - for test case 5, where we reach a blocking state, we have two possible outcomes, based on the way that nodes are shutdown.
- If the final node to shutdown (node 3) is the one that collects an initial majority of votes, then it will shutdown before broadcasting a `ViewChangeProof`, and the other nodes will never install view 3.
- If another node (say node 4) collects an initial majority, then the group can successfully install view 3, but will block afterwards.

Recommended is to `cat TEST_RESULTS.txt`, since the output is color-coded.

# Contents
.
├── Dockerfile
├── Makefile
├── README.md
├── docker-compose.yaml
├── hostfile.txt
└── src
    ├── messages
    │   ├── encoding.go # Provide implementations of MarshalBinary and UnmarshalBinary to enable encoding/decoding with Gob  (see [note](#message-encoding) below)
    │   └── main.go     # Message structs and exported constructors
    └── server
        ├── main.go     # Main program logic
        ├── types.go    # Struct definitions
        └── udp.go      # All functions for interacting with the network

# Main Logic 
Each agent holds onto two tickers.
When `proofTicker` ticks, the agent multicasts a `ViewChangeProof` announcing its `lastInstalled` view.
When `electionTicker` ticks, the agent begins a new leader election using `lastAttempted + 1`.

## Election Message Logic
- Nodes reject election-related messages until entering the `election` state. During an election, nodes wait for a majority of `ViewChange` votes, or a `ViewChangeProof` message before installing a particular view.
- During the election state, nodes that have multicast `ViewChange(i)` and receive a `ViewChangeProof(j)` message for a later view (`j>i`) will also advance to join that future view.
- Finally, as with the previous rule, nodes that have multicast `ViewChange(i)` and receive `ViewChange(j)` for a later view (`j>i`) will accept that future `ViewChange`, and join an election for `j`. 
  Note that they will begin that election with 2 votes, from the original peer and from themself. 
  This behavior is an important mechanism to restore nearly-synchronous operation when one or more nodes become delayed; without either this simple mechanism or some randomization on the `electionTicker` timeouts, the group may become
  stuck in an infinite loop where no majority of nodes are voting on the same view id, and subsets of the group increment at approximately the same time.

  For example, we might reach a state where 2 nodes are voting for `5`, 2 nodes are voting for `6` and 1 for `7`. 
  Then, all nodes can simultaneously increment, so that we have votes for `6`, `6`, `7`, `7`, `8`, etc

- Notice that, overall, in order for our protocol to succeed, we require that at each view, at least one node must receive a majority of votes, so that it will successfully install the view and broadcast `ViewChangeProof` messages to bring other nodes up to speed.

# State Machine

0. LEADER{id: i, lastInstalled: v, lastAttempt: a}
  - ProofTimer runs out; broadcast ViewChangeProof(v) and stay in state 0
  - ProgressTimer runs out; go to state 2 with lastAttempt := a+1

1. FOLLOWER{id: i, lastInstalled: v, lastAttempt: a}
  - ProofTimer runs out; broadcast ViewChangeProof(v) and stay in state 1
  - ProgressTimer runs out; go to state 2 with lastAttempt := a+1

2. ELECTION{id: i, lastInstalled: v, lastAttempt: a, votes} (Node is voting for `a`, and adds themself to the `votes` list)
  - ProofTimer runs out; broadcast ViewChangeProof(v) and stay in state 2
  - Recieve ViewChangeProof(w) with w > v; 
    - if w mod n == i; go to state 0 with lastInstalled := w, lastAttempt := w
    - if w mod n != i; go to state 1 with lastInstalled := w, lastAttempt := w
  - Receive ViewChange(w) with w > a; go to state 2 with lastAttempt := w and include originating peer in votes
  - Receive ViewChange(w) with w == a
    - if have majority and w mod n == i;  go to state 0 with lastInstalled := w, lastAttempt := w
    - if have majority and w mod n != i; go to state 0 with lastInstalled := w, lastAttempt := w
  - ProgressTimer runs out; stay in state 2 with lastAttempt := a+1

Note that all other `ViewChange` and `ViewChangeProof` messages not mentioned here are ignored and cause no transition.

# Implementation Details
Agents are initialized with `lastAttempted = -1` and `lastInstalled = -1`, so that the first election will be to try to install view 0.

Each node runs a main thread and 3 goroutines. These 3 goroutines are:
1. A RECV daemon, responsible for accepting UDP packets, decoding them as a `GenericMessage` struct, and making them available in the "work queue" of an agent by putting them onto `recvChan`.
2. A `quitter()` goroutine, which enforces the timeout specified by `PRJ2_DURATION`
3. A `protocol()` goroutine, which runs the main logic of the leader election

Upon startup, each agent will use the hostfile and the value of the environment variable `$CONTAINER_NAME` to resolve its own IP as well as the IP of all peers. 
The agent will wait until all peers are available before beginning the main protocol.
As mentioned above, each agent will run the main protocol for a specified number of seconds, keeping track of the history of views installed, then print this view history and exit.

As an example, the view history might look like this:

```
FINAL VIEW HISTORY: 
view 0: [-1,-1,-1,-1,-1,]
view 1: [0,1,0,1,1,]
```

This indicates that the node installed view 0 from a `ViewChangeProof` message (so that the value of each vote is just set to `-1`), and the node installed view 1 based on a vote from nodes 1, 3, and 4.

Notice that in a test scenario when a node shuts down, we will no longer see votes from that node. 
The shutdown is implemented such that we will still the election evidence from that node for their final view.

## Message encoding

First, we want to ensure that the contents of messages are correct according to the specification given; therefore we do not export the `msgType` field, and instead only export constructor methods for each message type that use the correct value.

However, we also want to encode and decode messages to the wire in a safe manner by avoiding manual parsing of byte streams.
We seek to use the Gob library for encoding and decoding, but the unexported `msgType` field prevents this.
Therefore, we simply make a wrapper type for each message type that implements the `Encoder` interface (by implementing `MarshalBinary()` and `UnmarshalBinary()` methods).
Finally, we must register the concrete types that will be used at the beginning of our program; this is handled during the `init()` method of `main.go`.
