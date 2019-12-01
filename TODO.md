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
