package raft

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"
	"strings"
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
	err = json.Unmarshal(byteValue, &hc)
	if err != nil {
		panic(err)
	}
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
	Term         Term
	LeaderID     HostID
	PrevLogIndex LogIndex
	PrevLogTerm  Term
	Entries      []LogEntry
	LeaderCommit LogIndex
}

func (ae AppendEntriesStruct) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("AEStruct. {Term: %d, LeaderID: %d, PrevLogIndex: %d, PrevLogTerm: %d, LeaderCommit: %d, Entries: [",
		ae.Term, ae.LeaderID, ae.PrevLogIndex, ae.PrevLogTerm, ae.LeaderCommit))
	for idx, entry := range ae.Entries {
		sb.WriteString(fmt.Sprintf("{idx: %d, entry: %s}\n", idx, entry.String()))
	}
	sb.WriteString("]}")
	return sb.String()
}

// The leader uses this during heartbeats (or after receiving from client) to slowly bring all other logs up to date, and to maintain leadership.
// For each follower,
//   if they are up to date, we send an empty message
//   if they are trailing behind, we send them the log entry at nextIndex[hostID].
//     if they reject this entry, we decrement their index
func (r *RaftNode) heartbeatAppendEntriesRPC() {
	if r.verbose {
		log.Println("heartbeatAppendEntriesRPC")
	}

	leaderLastLogIdx := r.getLastLogIndex()

	for hostID := range r.hosts {
		if hostID != r.id {

			// By default, we assume the peer is up-to-date and will get an empty list
			var entries []LogEntry = make([]LogEntry, 0)

			// Check their "nextIndex" against our last log index
			theirNextIdx := r.nextIndex[hostID]
			if leaderLastLogIdx >= theirNextIdx {
				for i := theirNextIdx; i <= leaderLastLogIdx; i++ {
					entries = append(entries, r.Log[i])
				}
			}

			// Send the entries and get response
			r.Lock()
			if r.verbose {
				log.Printf("set indexIncrements for host %d to %d. (previously %d)", hostID, len(entries), r.indexIncrements[hostID])
			}
			r.indexIncrements[hostID] = len(entries)
			r.Unlock()
			go r.appendEntriesRPC(hostID, entries)
		}
	}
}

func (r *RaftNode) appendEntriesRPC(hostID HostID, entries []LogEntry) {
	p := r.hosts[hostID]
	prevLogIdx := max(0, r.nextIndex[hostID]-1)
	prevLogTerm := r.Log[prevLogIdx].Term

	args := AppendEntriesStruct{
		Term:         r.CurrentTerm,
		LeaderID:     r.id,
		PrevLogIndex: prevLogIdx,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: r.commitIndex}

	response := RPCResponse{}
	conn, err := rpc.Dial("tcp", fmt.Sprintf("%s:%d", p.IP, p.Port))
	if err != nil {
		log.Printf("WARNING: problem dialing peer: %s, err: %s", p.String(), err)
		response = RPCResponse{Term: r.CurrentTerm, Success: false, LeaderID: r.currentLeader}
	} else {
		err = conn.Call("RaftNode.AppendEntries", args, &response)
		if err != nil {
			panic(err)
		}
	}

	if r.verbose {
		log.Printf("appendEntriesRPC result. host: %d, success: %t", hostID, response.Success)
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
		Term:        r.CurrentTerm,
		CandidateID: r.id,
		LastLogIdx:  r.getLastLogIndex(),
		LastLogTerm: r.getLastLogTerm()}
	// Fill response with default values in case of early return
	response := RPCResponse{}

	conn, err := rpc.Dial("tcp", fmt.Sprintf("%s:%d", p.IP, p.Port))
	if err != nil {
		// We do not crash here, because we don't care if that peer might be down
		log.Printf("WARNING: problem dialing peer: %s. err: %s", p.String(), err)
		response = RPCResponse{Term: r.CurrentTerm, Success: false, LeaderID: r.currentLeader}
	} else {
		err = conn.Call("RaftNode.Vote", args, &response)
		if err != nil {
			// We crash here, because we do not tolerate RPC errors
			panic(fmt.Sprintf("requestVoteRPC: %s", err))
		}
	}
	if r.verbose {
		log.Printf("received vote reply from hostID: %d, response.success=%t", hostID, response.Success)
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
	err := rpc.Register(r)
	if err != nil {
		panic(err)
	}
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
