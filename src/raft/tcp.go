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

// All nodes must be alive to begin the protocol
// We loop infinitely here, until we have network information for all nodes
// 'peers' arg will be filled with appropriate info upon return
// Returns our own id as integer. Caller (either host or client) must cast appropriately
func ResolveAllPeers(hosts HostMap, clients ClientMap, hostfile string, amHost bool) int {
	decodedJson := ReadHostfileJson(hostfile)
	myHost := Host(-1)
	myClient := Client(-1)

	// Lookup our own ID, depending whether we are a raft node or a client node
	containerName := os.Getenv("CONTAINER_NAME")
	if amHost {
		for i, name := range decodedJson.RaftNodes {
			if name == containerName {
				myHost = Host(i)
			}
		}
	} else {
		for i, name := range decodedJson.ClientNodes {
			if name == containerName {
				myClient = Client(i)
			}
		}
	}
	if int(myHost) == -1 && int(myClient) == -1 {
		log.Fatal("Did not find our own name in the hostfile")
	}

	// Make maps of all known hosts and clients
	h, c := makeHostStringMap(decodedJson)

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

// NOTE - We do not handle the errors from LookupHost, because we are waiting for nodes to come online
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

type Hosts struct {
	RaftNodes   []string `json:"servers"`
	ClientNodes []string `json:"clients"`
}

// TODO - return hostStringMap and clientStringMap
func ReadHostfileJson(hostfile string) Hosts {
	// get contents of JSON file
	jsonFile, err := os.Open(hostfile)
	if err != nil {
		fmt.Println(err)
	}
	defer jsonFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)

	var hosts Hosts
	json.Unmarshal(byteValue, &hosts)
	return hosts
}

func makeHostStringMap(hosts Hosts) (hostStringMap, clientStringMap) {
	// Find our own ID and build a map for DNS lookup of other hosts
	h := make(hostStringMap)
	c := make(clientStringMap)
	for i, name := range hosts.RaftNodes {
		h[Host(i)] = name
	}
	for i, name := range hosts.ClientNodes {
		c[Client(i)] = name
	}
	return h, c
}

type AppendEntriesStruct struct {
	T            Term
	LeaderID     Host
	PrevLogIndex LogIndex
	PrevLogTerm  Term
	Entries      LogAppendList
	LeaderCommit LogIndex
}

// TODO - for now, assume that all RPC should be done sync (using "c.Call" instead of "c.Go"). In reality this should be async
// to make the logic of handling rejections and peer failures more obvious
// The leader uses this function to append to the log on all other nodes
// Returns true if a majority of nodes appended
func (r *RaftNode) MultiAppendEntriesRPC(entries []LogEntry) bool {
	if r.verbose {
		log.Println("MultiAppendEntries")
	}
	responses := make(map[Host]bool)
	for hostID, h := range r.hosts {
		if hostID == r.id {
			continue
		} else {
			response := r.AppendEntriesRPC(h, entries)
			// TODO - should we check response.term to see if we need to jump ahead or something?
			responses[hostID] = response.Success
		}
	}
	return haveMajority(responses)
	// TODO - if success, update
}

func (r *RaftNode) AppendEntriesRPC(p peer, entries []LogEntry) RPCResponse {
	log.Fatal("TODO - after each individual AppendEntriesRPC, need to update tracking info about that follower's log indices")

	prevLogIdx := r.getPrevLogIndex()
	logAppends := make([]LogAppend, 0, 0)
	for i, entry := range entries {
		logAppends = append(logAppends, LogAppend{idx: LogIndex(int(prevLogIdx) + i), entry: entry})
	}

	args := AppendEntriesStruct{T: r.currentTerm,
		LeaderID:     r.id,
		PrevLogIndex: prevLogIdx,
		PrevLogTerm:  r.getPrevLogTerm(),
		Entries:      logAppends,
		LeaderCommit: r.commitIndex}
	reply := RPCResponse{}
	conn, err := rpc.Dial("tcp", fmt.Sprintf("%s:%d", p.IP, p.Port))
	if err != nil {
		log.Fatal("error dialing peer,", err) // TODO - this should not be fatal, we should just take note of the missing peer and maybe retry later
	}
	err = conn.Call("RaftNode.AppendEntries", args, &reply)
	if err != nil {
		log.Fatal("AppendEntriesRPC:", err)
	}
	return reply
}

type RequestVoteStruct struct {
	T           Term
	CandidateID Host
	LastLogIdx  LogIndex
	LastLogTerm Term
}

// Send a RequestVoteRPC to all peers, storing their responses
func (r *RaftNode) MultiRequestVoteRPC() {
	if r.verbose {
		log.Println("MultiRequestVote")
	}
	r.votes = make(map[Host]bool)
	for hostID, h := range r.hosts {
		// TODO - confirm that we do not need to worry about receiving an AppendEntriesRPC from current leader mid-election
		// if this DOES happen, because we lagged behind, then there must be a majority of peers who have moved on
		// and they will just reject our RequestVoteRPC.
		if hostID == r.id { // we always vote for ourself
			r.votes[hostID] = true
		} else {
			response := r.RequestVoteRPC(h)
			// TODO - should we check response.term??
			r.votes[hostID] = response.Success
		}
	}
	if r.wonElection() {
		r.shiftToLeader()
	}
}

// Send out an RPC to the method "Vote" on the remote host
func (r RaftNode) RequestVoteRPC(p peer) RPCResponse {
	args := RequestVoteStruct{T: r.currentTerm,
		CandidateID: r.id,
		LastLogIdx:  r.getPrevLogIndex(),
		LastLogTerm: r.getPrevLogTerm()}
	reply := RPCResponse{}
	conn, err := rpc.Dial("tcp", fmt.Sprintf("%s:%d", p.IP, p.Port))
	if err != nil {
		log.Fatal("error dialing peer,", err) // TODO should this be fatal?
	}
	err = conn.Call("RaftNode.Vote", args, &reply)
	if err != nil {
		log.Fatal("RequestVoteRPC:", err)
	}
	return reply
}

// Every time a client makes a request, they must attach a unique serial number, monotonically increasing
// The state machine includes a map of clients and their most recently executed serial num
// If a request is received with a stale ClientSerialNum, the leader can immediately reply "success"
type ClientSerialNum int
type ClientDataStruct struct {
	ClientID        Client
	Data            ClientData
	ClientSerialNum ClientSerialNum
}

// TODO - confirm that we need a pointer receiver here, so that rpc can
// invoke methods on the same RaftNode object we use elsewhere?
func (r *RaftNode) recvDaemon(quitChan <-chan bool) {
	rpc.Register(r)
	l, err := net.Listen("tcp", ":"+fmt.Sprintf("%d", recvPort))
	if err != nil {
		log.Fatal("listen error:", err)
	}
	for {
		select {
		case <-quitChan:
			log.Println("QUIT RECV DAEMON")
			return
		default:
			conn, err := l.Accept()
			if err != nil {
				log.Fatal("accept error:", err)
			}
			// TODO - Do we need to do extra work to kill this goroutine if we want to kill this raftnode?
			// should this be used without goroutine?
			go rpc.ServeConn(conn)
		}
	}
}
