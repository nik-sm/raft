- Setup the cluster with a fixed node as leader and freeze all tickers, send
  all possible RPCs 
  - For each RPC, describe the possible meaningful regimes of each parameter,
    and try all combinations, verifying the effect on the state diagram of each
    node
- Send RPCs during transitions, and during elections

- Issue with lack of robust client interaction. motivating example:
  - client sends data
    - cluster is slow, leader waits too long to hear back from majority of
      nodes
    - either client or leader gives up and considers this a "failed"
      transaction
  - after that RPC disconnects, cluster succeeds and the data gets stored
  - client resends, same data, same serial number
    - now we need to determine that their previous message eventually
      succeeded, so that the re-submission is a duplicate
  - Finally, notice that we could have disconnected the leader after their
    reply "success=true", but before the message was relayed to the cluster, so
    that the client's data is lost!
- Possible approach:
  - leader receives StoreClientData RPC
    - dispatches goroutines to do appendentriesrpc to all followers
  - leader dispatches goroutine to do appendentriesrpc, and it passes the reply
    back into incomingMsg chan
  - leader protocol loop receives msg on incomingMsg chan, updates indices, and
    checks commitIndex
    - if committed, puts notification in clientReply chan

- Figure 2 does not specify which to do first:
  - reply as described in a certain "Receiver implementation" section, or
  - convert to follower if the term is > than our currentTerm
  - presumably, we should convert to follower first

# Additional Testing
For many tests below, there is a set of attributes that may vary, and
implicitly there is a state table of correct outcomes based on the permutations
of these attributes.

In an ideal world, the most thorough testing would specify exactly what
attributes can vary, the "threshold" values for these attributes that describe
regions of different behavior, and test all permutations. This is a non-trivial
setup, but would give high confidence about the behavior of the code.

## Single-Node Tests
Single-node testing scenarios:
- running go code locally
- launching a single container, perform a setup(), and performing tests by
  sending messages to this node

[] Send and receive RPC messages of each required type
[] Restart a RAFT node, recovering saved state from bind-mounted volume
[] Run a client node, broadcast data messages
[] Add log entries
[] Compare log entries with known datafile for correctness

## Multi-Node Tests
Multi-node testing scenarios:
- running go code locally
- launching multiple containers, plus a final go runtime (either locally or in
  a container), that triggers events

[] Resolve peers
[] Send each message type
[] Receive each message type
[] Drop a message/Detach a node during a particular RPC
[] Reorder RPCs (this may not be applicable due to underlying TCP transport)

[] Perform leader election under benign conditions
[] Perform leader election with dropped messages
[] Perform leader election with restarted node
[] Perform leader election with all 3 error conditions

[] Accept a data message from client under benign conditions
[] Accept list of data messages
[] Accept list of data messages with drops, shuffles, restarts
[] Protocol blocks with too many failures
[] Attempt leader election from skewed log state, show that only valid leaders are chosen

## Tests from Raft Paper
The Raft paper includes several figures describing particular difficult
scenarios - each of these should be included as an integration test.
