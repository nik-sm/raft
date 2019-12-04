Go implementation of [Raft](https://raft.github.io/) from scratch for CS7610: Distributed Systems

# Project Overview

The goal of this project is to implement the main components of the Raft
distributed state machine protocol, namely, leader election, log replication,
and a basic state machine.

# Raft Protocol Overview

First, we can briefly review the key parts of the Raft protocol.

## State Diagram

The Raft protocol is designed around a simple state diagram:

![Raft Paper Figure 4 - State Diagram](https://github.com/nik-sm/raft/blob/master/figure_4.png)

We can focus on three key functions of the system:
1. Leader Election
  - Raft elects a "distinguished leader", who controls the flow of log entries
    and seeks to bring all other nodes in sync with itself.
2. Log Replication
  - The leader uses AppendEntriesRPC to modify the log on follower nodes.
  - Clients are redirected to communicate only with the current cluster leader.
3. State Machine
  - When entries have been "committed" (appended to a majority of logs), then
    they may be safely applied to each node's copy of the State Machine

## Basic Operation

Briefly, the basic logic of the protocol is as follows:

1. All nodes in a Raft cluster maintain an election ticker, set to a random
   value from a fixed range (e.g. 300ms < t < 600ms).

2. When a follower waits too long without hearing from their current leader or
   granting a vote to a valid candidate, they will time out.

3. The timed-out follower will transition to being a candidate for the next
   term, voting for themself and broadcasting a RequestVoteRPC.

4. Followers will grant votes on a first-come, first-served basis, only to
   "up-to-date" candidates (see the [Election Restriction](#Election-Restriction) 
   property mentioned below).

5. If the candidate receives a majority of votes, it transitions to leader and
   immediately begins broadcasting AppendEntriesRPC to the log.

6. Leader nodes also maintain a heartbeat ticker, set to a sufficiently small
   value (e.g. t = 50ms). If a client sends a new state machien action during
   this interval, the leader will broadcast a corresponding AppendEntriesRPC;
   otherwise, the leader will send an empty AppendEntriesRPC as a heartbeat to
   maintain leadership.

7. A follower only accepts an AppendEntriesRPC if their log agrees with the
   leader's log at the point of insertion.
  - The leader tracks the state of the log for each followers, so that during
    AppendEntriesRPCs it can try to send entries from a point where its log
    agrees with that follower.
  - Based on the responses to AppendEntriesRPCs, the leader will eventually
    learn the correct place to begin log appends for each follower, and then
    bring them into agreement.
  - Notice that when a follower agrees to insert a list of entries a certain
    position, it must first delete its log suffix from that point. This is the 
    key reason for the Election Restriction.

8. Log entries can be considered committed when they are present in the logs of
   a majority of nodes in the cluster. At this point, they can be applied to
   each copy of the State Machine.

### Election Restriction

One of the most subtle parts of the Raft protocol is the election restriction,
which is a key to the protocol's safety properties.  When a follower node (a
"voter") receives a RequestVoteRPC from some other candidate, one requirement
for granting its vote is that the candidate must be at least as "up-to-date" as
  this voter.  "Up-to-date" here means that the candidate's last log _term_
  must be at least as big as the voter's last log _term_, and if they end in
  the same term, then the candidate's last log _index_ must be at least as big
  as the voter's last log _index_.

See the [Leader Completeness](#Leader-Completeness) section below for more
intuition about the benefit of this restriction. 

## Key Safety Properties

Raft provides several safety guarantees, and gives short and intuitive proofs
of these properties.

Since the primary purpose of Raft is to maintain a State Machine in a manner
that is robust to lost messages or failed nodes, we will briefly highlight
these safety properties, focusing especially on the final State Machine Safety
property.  We will see that the State Machine Safety property follows neatly as
a consequence from the Log Matching property and the election restriction on
"up-to-date" leaders.

### Election Safety

> At most one leader can be elected in a given term.

This follows simply from the behavior of each node during election and the
election rules:
1. Nodes cast at most 1 vote per term (they may cast 0 if they do not receive
any RequestVoteRPC or if they are offline).
2. A candidate must receive a majority of votes to become leader.

There cannot exist multiple majorities in a single term, so there cannot be
more than 1 leader who successfully receives a majority.  Notice that based on
dropped messages, or multiple nodes transitioning to candidate at approximately
the same time, it might be the case that no nodes receive a majority.

### Leader Append-Only

> A leader never overwrites or deletes entries in its log; it only appends new entries.

This is simply a rule about how entries are appended by the leader.

### Log Matching

> If two logs contain an entry with the same index and term, then the logs are
> identical in all entries up through the given index.

This is proven inductively in the paper using two properties:

> 1. If two entries in different logs have the same index and term, then they
>    store the same command.
> 2. If two entries in different logs have the same index and term, then the
>    logs are identical in all preceding entries.

We note that leaders will never re-use the same index for multiple commands,
and previous log entries are never edited to use a different term or index,
providing us the first property.

The second property follows from the implementation of AppendEntriesRPC; a
follower accepts this RPC iff their log agrees on the term and index at the
point where the entries are to be inserted.  This serves as an inductive step,
where logs that previously matched will continue to match.  Finally, we note
that while it is possible for logs to have inconsistent suffixes, due to
crashed leaders, the implementation of AppendEntriesRPC also specifies that,
once a valid leader is elected and sends an RPC, if the follower's log is
consistent at the insertion point but has other inconsistent entries afterward,
the follower will delete that suffix and get in-sync with the leader.

### Leader Completeness

> If a log entry is committed in a given term, then that entry will be present
> in the logs of the leaders for all higher-numbered terms.

Given the simple implementation where leaders only append to their logs, this
property follows quite simply from the election restriction explained above.

Intuitively, the purpose of the election restriction is to ensure that elected
leaders will never accidentally force a follower to delete a log suffix
containing some committed entries.  Consider one of these "contentious" log
entries that a voter has but a candidate does not have:

- If the entry exists in a majority of voter logs, then this candidate cannot
  become leader (because none of those voters will grant their vote).

- If the entry exists in only a minority of voter logs, then it is fine for
  this candidate to become leader and eventually cause the entry to be
  overwritten, because this entry was not actually committed yet.

### State Machine Safety

> If a server has applied a log entry at a given index to its state machine, no
> other server will ever apply a different log entry for the same index.

Intuitively, this means that the State Machine on all nodes is eventually
consistent; it is possible for a node to be lagging behind (due to dropped
messages or node downtime), but if it ever recovers, it will apply all the same
log entries in the same order as other nodes in the cluster.


The key danger we want to avoid for the State Machine is that we might apply a
certain entry, and then somehow modify the log and decide that a different
entry should have been applied at that same time. (This would mean we need to
manage roll-backs to our State Machine, etc).

The only way the history of the log can be modified is by a leader.  However,
given the previous safety properties, we already know that

- Once committed, an entry will always be present on all future leader nodes

This guarantees that we can safely apply a committed entry to the State
Machine, knowing that no future leader can ever be elected that would seek to
modify this log entry.

# Implementation Overview

First, we briefly overview the features and design of this implementation.

## Implemented

This project implements only the basic components of the Raft Protocol highlighted above:
- Leader Election
- Log Replication
- State Machine

These components are chosen because they can in principle give us a working distributed state machine.

## Not Implemented
- "Robust" client interaction
  - In a production implementation, when a client submits data for storage, a
    leader should only report success after the data has actually been
    committed to the cluster. Here, this means that it has been appended to a
    majority of logs. This implementation does not yet handle this behavior,
    and the leader reports success as long as the data is added to its own log.
    Implementing the "correct"/robust behavior should in principle be a simple
    extension of the current project; a client must wait after sending data
    until it either receives confirmation of commitment, or until the client
    times-out and decides to retry. On the cluster side of things, a leader
    should spawn a dedicated goroutine for handling each client append, that
    also periodically checks for majority success until some time-out period
    elapses. There are at least 2 extra complications:
  1. If the client times-out first, but the data is successfully committed,
  then the client will resend the same data and the leader needs to choose a
  simple way of detecting this duplicated response. If this is handled by
  inspecting the `clientSerialNum` associated with the message (which will be
  identicalfor any valid client), then it might be sufficient for the leader to
  track the "most recently seen" `clientSerialNum` for each active client in
  ONLY the `StateMachine`. Although the client may cause duplicated `LogEntry`
  insertions, only the first one will be applied to the `StateMachine`.
  2. The leader goroutine that is handling the client interaction needs to do
  some extra locking to be sure that it responds appropriately, even if the
  leader node's local state may have changed.

- Integration testing, especially for deadlocks and race conditions
  - The ideal way of testing for these is not clear. Some tools, such as 
    [this](https://golang.org/pkg/net/http/pprof/) and [this](https://github.com/sasha-s/go-deadlock) 
    exist for detecting potential deadlocks in particular situations, and there is also [active research](https://spiral.imperial.ac.uk/bitstream/10044/1/34139/4/main.pdf) 
    into static detection of deadlocks in Go programs. These resources were not 
    yet explored here, and using these might provide important feedback on the 
    structure and locking done in this project. Anecdotally, existing tools can
    only debug a [complete deadlock](https://yourbasic.org/golang/detect-deadlock/), 
    and not a deadlock resulting from a subset of goroutines - this would not be
    sufficient for this application.

  - Likewise, Golang has tools such as a built-in [Data Race Detector](https://golang.org/doc/articles/race_detector.html) 
    for detecting race conditions. This tool also needs further evaluation to 
    determine if it is right for this application, but using similar tools could 
    help improve the design of this project.
- Log Compaction 
  - For a Raft cluster that stays live for a long time, the log on each node
    will continuously grow, maintaining a full history of all client
    interactions.  This can become unwieldy, and therefore a "production-grade"
    Raft implementation includes the periodic creation of checkpoints so that
    very old log entries can be deleted.  For a demo implementation, this
    feature is not necessary, because the cluster will not stay alive during
    any experiment long enough for log size to become an issue.
- Membership Changes
  - It is possible to allow a Raft cluster to stay alive while growing or
    shrinking.  This optimization feels in general to be an overcomplication;
    an easy alternative for a real production system is to spawn a new, larger
    cluster replica based on a recent checkpoint, redirect client requests to
    both clusters for a period of time until they are sufficiently in-sync, and
    then take down the old cluster.  Therefore, this feature was not considered
    for this implementation.
- Optimized communication
  - When a follower has deviated by many entries, there will be multiple
    back-and-forth failed AppendEntriesRPC, which can be short-circuited by
    trying to return a previous index of agreement, or the first index stored
    for a certain term.  Increasing the complexity of the system is only
      helpful when there is substantial communication overhead, which will also
      not happen on a demo implementation of Raft.

# Implementation Details

Next, we discuss this implementation in detail, including some of the key tools used. 

## Key Data Types

The main structs and their essential fields are listed here, with a brief
explanation where required

- `RaftNode`
  - `Log`
    - Contents - a list of `LogEntry`
  - `StateMachine`
    - `Contents`
    - `ClientSerialNums` - to avoid applying a duplicate entry
  - `CurrentTerm`
  - `VotedFor` - the `HostID` that this node voted for in the current term.
  - `nextIndex` - a list of log indices describing the most recent point of
    agreement for each follower with this leader's log.
  - `electionTicker`

- `ClientNode`
  - `StoreClientData` - the RPC used by clients to store `ClientData` to the
    cluster.

- `AppendEntries` - the function invoked via RPC for modifying the log on a
  node.
  - `AppendEntriesRPC` - the wrapper that performs the RPC.

- `Vote` - the function invoked via RPC for collecting a vote.
  - `RequestVoteRPC` - the wrapper that performs the RPC.

- `RPCResponse` - the struct filled during an RPC to describe the receiver's
  response
  - `Term` - the term of the receiver when the RPC arrived.
  - `Success` - result of the RPC.
  - `LeaderID` - the `HostID` that this node believes to be leader.

## Usage and Overview

Build and run the project with `make`.

The default Makefile target runs a simple experiment with no node failures.
Each node will be run in its own Docker container. 
- Nodes only begin operation after successfully resolving all other nodes
  specified in the `hostfile.json`; that is, we allow failures during the
  experiment, but we assume that the cluster begins with all nodes operating
  correctly.
- The default experiment runs 3 `RaftNode`s and 2 `ClientNode`s.
- Every 1s, each client node will try to send the next line from their data file to
  the cluster to be applied to the state machine.

The Makefile sets several environment variables to control the basic operation:
- `RAFT_DURATION`: int, seconds to run the experiment before each node will
  quit and shutdown
- `RAFT_VERBOSE`: bool, controls verbose output

To change the number of nodes being run:
- modify `docker-compose.yml` to include additional nodes of the specified type
- modify `hostfile.json` to be consistent. 
  - The `name` specified in `hostfile.json` MUST match the `container_name`
    specified in the `docker-compose.yml`.
  - The order of appearance in `hostfile.json` determines a node's unique
    integer `HostID`; to avoid confusion, this order SHOULD be consistent with
    their order of appearance in `docker-compose.yml`.
- if adding client nodes that should have unique data, provide an additional
  `datafile.*.txt`, and `COPY` this additional file into all containers inside
  `Dockerfile`.

Test the project with `go test raft`, and view documentation and testable
examples with `godoc -http=:6060` and browse to `http://localhost:6061/pkg/raft/`.

## Inspecting output

The easiest way to view the "results" of an experiment is to check the final
state of the StateMachine on each Raft node.

The state machine is currently just a dummy implementation consisting of a list
of strings, however this is sufficient for testing correctness:
- If two nodes have the same list of `ClientData` entries in the same order on
  their `StateMachine`, then they are consistent.
- Notice that the protocol only requires nodes to agree on any indices that
  they both have; it is OK for one node to be missing a suffix of entries that
  are found on another node. This can happen if one node shuts down early, or
  if it does not receive some set of `AppendEntriesRPC` messages.

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

## Tools used
- [vim-go](https://github.com/fatih/vim-go/) makes it easy to write Go in Vim;
  in particular, auto-formats and runs [golangci-lint](https://github.com/golangci/golangci-lint) upon every save.

- docker, docker-compose
  - For the "production" scenario (as opposed to the testing scenarios), every
    node in the system is run in its own docker container.  The cluster
    configuration is controlled using `docker-compose`. Ideally, it would be
    better to have more thorough integration tests that more closely mimic the
    production scenario.

- `godoc` - This tool provides an extremely easy documentation server, where
  testable examples can be viewed as well. NOTE - currently, the testable
  examples written here are not runnable in the browser, and this needs to be
  figured out.

# Contents

Here is a table of contents for this repository.


```
.
├── datafile.0.txt                    # Data for client 0
├── datafile.1.txt                    # Data for client 1
├── docker-compose.yaml
├── Dockerfile
├── figure_4.png                      # State Diagram from Raft paper
├── hostfile.json                     # Hostname (docker container name) and receiving port for all Raft and Client nodes
├── main_client.go
├── main_raft.go
├── Makefile
├── raft_proposal.md
├── README.md
├── src
│   ├── client
│   │   └── client.go
│   └── raft
│       ├── persist.go
│       ├── raft.go
│       ├── raft_integration_test.go
│       ├── raft_unit_test.go
│       ├── rpc.go
│       ├── time_constants.go
│       ├── types.go
│       └── utils.go
└── TODO.md
```

## Implementation Notes

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

# Takeaway Messages

## Caveats

- There is still at least one race condition due to insufficient/incorrect
  locking, so experiments can sporadically fail
  - When a follower node replies slowly to a heartbeat and then quickly to the
    next non-trivial AppendEntriesRPC, the leader node may advance that
    follower's `nextIndex[i]` value too far. Instead of advancing by 0 (for
    heartbeat) + N (for the N entries successfully appended), it will do N + N.
- A number of TODO items have been left in the code and in
  - `TODO.md`

## Project Challenges
TODO


# Appendix

## Noteworthy Phrases from Raft Paper

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

## Useful Links

Raft:
- https://raft.github.io/raft.pdf
- https://github.com/ongardie/raft.tla/blob/master/raft.tla
- https://web.stanford.edu/~ouster/cgi-bin/papers/OngaroPhD.pdf

Golang:
- https://gobyexample.com/
- https://golang.org/pkg/net/rpc/
- https://tour.golang.org/welcome/1
