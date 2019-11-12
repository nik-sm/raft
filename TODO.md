# Test cases
- Freeze the cluster with a certain node as leader, send client request to each node and record their responses
- Send a client request during an election


# TODO
- need layer of indirection so that leader can collect responses before responding to client
  - need to balance client's timeout, leader's timeout, and response times from other raft nodes
- resolve needs to also locate the client node
