- Setup the cluster with a fixed node as leader and freeze all tickers, send all possible RPCs 
  - For each RPC, describe the possible meaningful regimes of each parameter, and try all combinations, 
  verifying the effect on the state diagram of each node
- Send RPCs during transitions, and during elections

- Issue with lack of robust client interaction. motivating example:
  - client sends data
    - cluster is slow, leader waits too long to hear back from majority of nodes
    - either client or leader gives up and considers this a "failed" transaction
  - after that RPC disconnects, cluster succeeds and the data gets stored
  - client resends, same data, same serial number
    - now we need to determine that their previous message eventually succeeded, so that the re-submission is a duplicate
  - Finally, notice that we could have disconnected the leader after their reply "success=true", 
  but before the message was relayed to the cluster, so that the client's data is lost!
- Possible approach:
  - leader receives StoreClientData RPC
    - dispatches goroutines to do appendentriesrpc to all followers
  - leader dispatches goroutine to do appendentriesrpc, and it passes the reply back into incomingMsg chan
  - leader protocol loop receives msg on incomingMsg chan, updates indices, and checks commitIndex
    - if committed, puts notification in clientReply chan

- Figure 2 does not specify which to do first:
  - reply as described in a certain "Receiver implementation" section, or
  - convert to follower if the term is > than our currentTerm
  - presumably, we should convert to follower first
