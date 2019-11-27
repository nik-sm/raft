package raft

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"
	"time"
)

// ResolveAllPeers gets the net address of all raft hosts and clients, and stores these in
// the provided HostMap and ClientMap structures.
// Notice that all nodes must be alive to begin the protocol; thus We loop infinitely here,
// until we have information for all nodes.
// Returns our own id as integer. Caller (either host or client) must cast to HostID or ClientID appropriately
func ResolveAllPeers(hosts HostMap, clients ClientMap, hostfile string, amHost bool) int {
	decodedJSON := readHostfileJSON(hostfile)
	myHost := HostID(-1)
	myClient := ClientID(-1)

	// Lookup our own ID, depending whether we are a raft node or a client node
	containerName := os.Getenv("CONTAINER_NAME")
	if amHost {
		for i, name := range decodedJSON.RaftNodes {
			if name == containerName {
				myHost = HostID(i)
			}
		}
	} else {
		for i, name := range decodedJSON.ClientNodes {
			if name == containerName {
				myClient = ClientID(i)
			}
		}
	}
	if int(myHost) == -1 && int(myClient) == -1 {
		panic("Did not find our own name in the hostfile")
	}

	// Make maps of all known hosts and clients
	h, c := makeHostStringMap(decodedJSON)

	// Continue resolving until all hosts and clients found
	for {
		allFound := ResolvePeersOnce(hosts, clients, h, c)
		if allFound {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if amHost {
		return int(myHost)
	}
	return int(myClient)
}

// ResolvePeersOnce makes one attempt to identify all hosts and clients in the provided maps.
// As they are found, peers get removed from these maps.
// NOTE - We do not handle the errors from LookupHost, because we are waiting for nodes to come online.
func ResolvePeersOnce(hosts HostMap, clients ClientMap, h hostStringMap, c clientStringMap) bool {
	// Resolve raft hosts
	for i, hostname := range h {
		log.Println("resolve host: ", hostname)

		sendAddrs, err := net.LookupHost(hostname)
		if err == nil {
			ip := net.ParseIP(sendAddrs[0])
			log.Println("found: ", ip.String())
			hosts[i] = peer{IP: ip, Port: recvPort, Hostname: hostname}
			delete(h, i)
		}
	}

	// Resolve clients
	for i, clientname := range c {
		log.Println("resolve client: ", clientname)
		sendAddrs, err := net.LookupHost(clientname)
		if err == nil {
			ip := net.ParseIP(sendAddrs[0])
			log.Println("found: ", ip.String())
			clients[i] = peer{IP: ip, Port: recvPort, Hostname: clientname}
			delete(c, i)
		}
	}
	return len(h) == 0 && len(c) == 0
}

type hostsAndClients struct {
	RaftNodes   []string `json:"servers"`
	ClientNodes []string `json:"clients"`
}

func readHostfileJSON(hostfile string) hostsAndClients {
	// get contents of JSON file
	jsonFile, err := os.Open(hostfile)
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)

	var hc hostsAndClients
	json.Unmarshal(byteValue, &hc)
	return hc
}

func makeHostStringMap(hc hostsAndClients) (hostStringMap, clientStringMap) {
	// Find our own ID and build a map for DNS lookup of other hosts
	h := make(hostStringMap)
	c := make(clientStringMap)
	for i, name := range hc.RaftNodes {
		h[HostID(i)] = name
	}
	for i, name := range hc.ClientNodes {
		c[ClientID(i)] = name
	}
	return h, c
}

// AppendEntriesStruct holds the input arguments for the RPC AppendEntries
type AppendEntriesStruct struct {
	T            Term
	LeaderID     HostID
	PrevLogIndex LogIndex
	PrevLogTerm  Term
	Entries      LogAppendList
	LeaderCommit LogIndex
}

// TODO - for now, assume that all RPC should be done sync (using "c.Call" instead of "c.Go"). In reality this should be async
// to make the logic of handling rejections and peer failures more obvious. In order to do async, need waitgroup or shared channel
//
// The leader uses this function to append specific entries to the other nodes
// This lets us reply to a client in realtime to give some feedback after attempting to store their current request
// Returns true if a majority of nodes appended
func (r *RaftNode) multiAppendEntriesRPC(entries []LogEntry) bool {
	if r.verbose {
		log.Println("multiAppendEntriesRPC")
	}
	// responses := make(map[HostID]bool)
	for hostID := range r.hosts {
		if hostID != r.id {
			go r.appendEntriesRPC(hostID, entries)
			/* TODO - for storing client data, do we need to collect the responses in a particular way to decide if we received a majority??
			response := r.appendEntriesRPC(hostID, entries)
			if response.Term > r.currentTerm { // We fail immediately because we should be a follower
				log.Printf("Received reply from hostID: %d with higher term: %d and leader: %d", hostID, response.Term, response.LeaderID)
				r.Lock()
				r.shiftToFollower(response.Term, response.LeaderID)
				r.Unlock()
				return false
			}
			// TODO - should we check response.term to see if we need to jump ahead or something?
			responses[hostID] = response.Success
			if response.Success {
				// We know exactly what index this follower's log is at
				r.nextIndex[hostID]++
				r.matchIndex[hostID] = r.getLastLogIndex()
				log.Printf("For follower %d, set nextIndex to: %d, and matchIndex to: %d", hostID, r.nextIndex[hostID], r.matchIndex[hostID])
			}
			*/
		}
	}
	return false
	// return haveMajority(responses)
	// TODO - if success, update
}

// The leader uses this during heartbeats to slowly bring all other logs up to date, or to maintain leadership.
// For each follower,
//   if they are up to date, we send an empty message
//   if they are trailing behind, we send them the log entry at nextIndex[hostID].
//     if they reject this entry, we decrement their index
func (r *RaftNode) heartbeatAppendEntriesRPC() {
	if r.verbose {
		log.Println("heartbeatAppendEntriesRPC")
	}
	for hostID := range r.hosts {
		if hostID != r.id {
			// Get a suitable entry to send to this follower
			theirNextIdx := r.nextIndex[hostID]
			var entries []LogEntry
			if int(theirNextIdx) == int(r.getLastLogIndex())+1 {
				entries = make([]LogEntry, 0, 0) // empty entries because they are up-to-date
			} else {
				theirNextEntry := r.log[theirNextIdx]
				entries = []LogEntry{theirNextEntry}
			}

			// Send the entries and get response
			go r.appendEntriesRPC(hostID, entries)
		}
	}
}

func (r *RaftNode) appendEntriesRPC(hostID HostID, entries []LogEntry) {
	p := r.hosts[hostID]
	prevLogIdx := r.getLastLogIndex()
	logAppends := make([]LogAppend, 0, 0)
	for i, entry := range entries {
		logAppends = append(logAppends, LogAppend{Idx: LogIndex(int(prevLogIdx) + i), Entry: entry})
	}

	args := AppendEntriesStruct{T: r.currentTerm,
		LeaderID:     r.id,
		PrevLogIndex: prevLogIdx,
		PrevLogTerm:  r.getLastLogTerm(),
		Entries:      logAppends,
		LeaderCommit: r.commitIndex}

	response := RPCResponse{}
	conn, err := rpc.Dial("tcp", fmt.Sprintf("%s:%d", p.IP, p.Port))
	if err != nil {
		log.Printf("WARNING: problem dialing peer: %s, err: %s", p.String(), err)
		response = RPCResponse{Term: r.currentTerm, Success: false, LeaderID: r.currentLeader}
	} else {
		err = conn.Call("RaftNode.AppendEntries", args, &response)
		if err != nil {
			panic(err)
		}
	}

	r.incomingChan <- incomingMsg{msgType: appendEntries, hostID: hostID, response: response}
}

// RequestVoteStruct holds the parameters used during the Vote() RPC
type RequestVoteStruct struct {
	Term        Term
	CandidateID HostID
	LastLogIdx  LogIndex
	LastLogTerm Term
}

func (rv RequestVoteStruct) String() string {
	return fmt.Sprintf("Term: %d, CandidateID: %d, LastLogIdx: %d, LastLogTerm: %d", rv.Term, rv.CandidateID, rv.LastLogIdx, rv.LastLogTerm)
}

// Send a requestVoteRPC to all peers, storing their responses
func (r *RaftNode) multiRequestVoteRPC() {
	if r.verbose {
		log.Println("MultiRequestVote")
	}
	for hostID := range r.hosts {
		if hostID != r.id {
			go r.requestVoteRPC(hostID)
		}
	}
}

// Send out an RPC to the method "Vote" on the remote host
func (r *RaftNode) requestVoteRPC(hostID HostID) {
	p := r.hosts[hostID]
	args := RequestVoteStruct{
		Term:        r.currentTerm,
		CandidateID: r.id,
		LastLogIdx:  r.getLastLogIndex(),
		LastLogTerm: r.getLastLogTerm()}
	// Fill response with default values in case of early return
	response := RPCResponse{}

	conn, err := rpc.Dial("tcp", fmt.Sprintf("%s:%d", p.IP, p.Port))
	if err != nil {
		// We do not crash here, because we don't care if that peer might be down
		log.Printf("WARNING: problem dialing peer: %s. err: %s", p.String(), err)
		response = RPCResponse{Term: r.currentTerm, Success: false, LeaderID: r.currentLeader}
	} else {
		err = conn.Call("RaftNode.Vote", args, &response)
		if err != nil {
			// We crash here, because we do not tolerate RPC errors
			panic(fmt.Sprintf("requestVoteRPC: %s", err))
		}
	}
	r.incomingChan <- incomingMsg{msgType: vote, hostID: hostID, response: response}
}

// TODO - // We need to check the clientSerialNum 2 times:
//	1) When receiving a request, we check the statemachine before trying to put the request into the log
//  2) When applying the log to the statemachine (which still needs to happen somewhere!!!!), we first
// 			check a log entry's serialnum against the most recent serial num per client

// ClientSerialNum is a unique, monotonically increasing integer that each client attaches to their requests
// The state machine includes a map of clients and their most recently executed serial num
// If a request is received with a stale ClientSerialNum, the leader can immediately reply "success"
type ClientSerialNum int

// ClientDataStruct holds the inputs that a client sends when they want to store information in the statemachine
type ClientDataStruct struct {
	ClientID        ClientID
	Data            ClientData
	ClientSerialNum ClientSerialNum
}

// TODO - confirm that we need a pointer receiver here, so that rpc can
// invoke methods on the same RaftNode object we use elsewhere?
func (r *RaftNode) recvDaemon(quitChan <-chan bool) {
	rpc.Register(r)
	l, err := net.Listen("tcp", ":"+fmt.Sprintf("%d", recvPort))
	if err != nil {
		panic(fmt.Sprintf("listen error: %s", err))
	}
	for {
		select {
		case <-quitChan:
			log.Println("QUIT RECV DAEMON")
			return
		default:
			conn, err := l.Accept()
			if err != nil {
				panic(fmt.Sprintf("accept error: %s", err))
			}
			// TODO - Do we need to do extra work to kill this goroutine if we want to kill this raftnode?
			// should this be used without goroutine?
			go rpc.ServeConn(conn)
		}
	}
}
