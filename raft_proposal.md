Raft Project Proposal

# Main Goals
I will implement the full RAFT protocol, as described in https://raft.github.io/raft.pdf.

To demonstrate that my implementation is correct, I will add the ability to:
- restart nodes
- randomly drop messages 
- shuffle the incoming message buffer on a client node (or otherwise find a way to simulate shuffled message arrival order)
- send lines from a datafile on a client to the raft network, and compare the datafile before and after the protocol

## Key Implementation Notes
I will take a test-driven approach to this project, meaning that I will first design and write tests that measure the desired behaviors, and then write implementations.

When the network is built using `docker-compose`, I will provide a dedicated docker volume for each node, to allow persistent data that simulates node failure+recovery.

Client data will simply be consecutive lines from a fixed data file, such as a few pages of a book.

# Tests
The final end-to-end test will be to compare the contents of a datafile on the client node to the final log on the raft nodes, and see that the contents are the same, despite all restarts of nodes, dropped messages, and reordering of messages.

Each of these tasks will consist of writing 1 or more test functions, and simultaneously designing the go structs to perform the task.

These tests will be divided into two main groups: single-node tests, and multi-node tests.

## Single-Node Tests
Single node tests will establish the required behaviors for a single node.
These will be run in one of the following regimes, which requires more detailed design in the future:
- running go code locally
- launching a single container, perform a setup() function that establishes a UDP listener on a known test port, and performing tests by sending messages to that port

[] Send and receive UDP messages
[] Encode message, send via UDP messages, and decode (repeat for each message type)
[] Restart a RAFT node, recovering saved state from docker volume
[] Run client node, broadcast data messages
[] Add log entries
[] Compare log entries with known datafile for correctness

## Multi-Node Tests
Multi node tests will designate one of the nodes as the "tester".
Several containers will be launched, and the "tester" node will send messages, request messages, etc.
More detailed design work is necessary to think about how to fully test the "send" and "recv" of each message type. 
In particular:
- if a message always results in some form of ACK, then this should be sufficient (and a test flag can cause the ACK to include extra debug information). 
- if a message may NOT cause a response, then we need the recipient node to also be running test code and collect its test output somehow.

[] Resolve peers
[] Send each message type
[] Receive each message type
[] Drop a message
[] Shuffle list of messages

[] Perform leader election under benign conditions
[] Perform leader election with dropped messages
[] Perform leader election with shuffled messages
[] Perform leader election with restarted node
[] Perform leader election with all 3 error conditions

[] Accept a data message from client under benign conditions
[] Accept list of data messages
[] Accept list of data messages with drops, shuffles, restarts
[] Protocol blocks with too many failures
[] Attempt leader election from skewed log state, show that only valid leaders are chosen
