# Test cases
- Freeze the cluster with a certain node as leader, send client request to each
  node and record their responses
- Send a client request during an election


# TODO

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

- Servers should send their first heartbeat immediately after winning


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


- NOTE - Not handling:
  - cluster membership changes!
    - this requires an extra type of log entry, and a period during which there
      can be multiple simultaneous leaders (WTF?)
  - log compaction
    - only required when log size is problematic relative to memory/storage.
      this will not be the case for now

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




# Logic for applying log entries to state machine
- When we try to apply a log entry to statemachine, we check the list of previously applied serialNums







# Current issues
## Robust client interaction

Currently:
- r0 becomes leader
- client detects r0 and starts sending there
- test case disconnects r0 from network
- r0 continues replying "success=true" to the client

Instead, want:
- r0 waits for clientReplyTimeout, checking if there exists a majority of nodes with successful storage
  - if found, replies true
  - if not found and time runs out, replies false


motivating example:
- client sends data
  - cluster is slow, leader waits too long to hear back from majority of nodes
  - either client or leader gives up and considers this a "failed" transaction
- after that RPC disconnects, cluster succeeds and the data gets stored
- client resends, same data, same serial number
  - now we need to determine that their previous message eventually succeeded, so that the re-submission is a duplicate


- leader receives StoreClientData RPC
  - dispatches goroutines to do appendentriesrpc to all followers
- leader dispatches goroutine to do appendentriesrpc, and it passes the reply back into incomingMsg chan
- leader protocol loop receives msg on incomingMsg chan, updates indices, and checks commitIndex
  - if committed, puts notification in clientReply chan



## leader election
There is still some dispute where a node has just received a heartbeat appendEntriesRPC from leader, resets their tickers, and STILL times out almost immediately
this looks like might be one of the following:
- log output is buffered in such a way that the timing is not accurate to the execution time
- tickers are not being reset properly
- tickers need to be reset with locks for some reason


## state machine
- not being updated on most nodes yet
  - Should do: "if commitIndex > lastApplied, increment lastApplied and apply log[lastApplied] to stateMachine"
- only statemachine should store prevClientResponse (log should not have this)



## IndexIncrements
- We want a FIFO channel with each client; really we want a single goroutine for each client that keeps sending an RPC
  - upon successful receipt, it can update that specific coordinate of NextIndex[]
  - upon failure, it decrements

# Notes
- Figure 2 does not specify which to do first:
  - reply as described in a certain "Receiver implementation" section, or
  - convert to follower if the term is > than our currentTerm
  - presumably, we should convert to follower first
 
