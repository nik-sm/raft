package raft

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"
	"strings"
	"time"
)

// All nodes must be alive to begin the protocol
// We loop infinitely here, until we have network information for all nodes
// 'peers' arg will be filled with appropriate info upon return
// Returns our own Host id
func ResolveAllPeers(peers PeerMap, hostfile string) Host {
	id, peerStringMap, err := ReadHostfile(hostfile)
	if err != nil {
		log.Fatal(err)
	}

	for {
		allFound, err := ResolvePeersOnce(peers, peerStringMap)
		if err != nil {
			log.Fatal(err)
		}
		if allFound {
			break
		}
		time.Sleep(1 * time.Second)
	}
	return id
}

func ResolvePeersOnce(peers PeerMap, peerStringMap peerStringMap) (bool, error) {
	for i, hostname := range peerStringMap {
		log.Println("resolve host: ", hostname)
		sendAddrs, err := net.LookupHost(hostname)
		// NOTE - not handling error here. We need to trust the hostfile contents so that
		// we keep checking for the hosts, as specified by the hostfile, until they are alive
		if err == nil {
			// TODO - LookupHost returns a slice that we probably need to handle differently here...
			log.Println("found: ", sendAddrs)
			peers[i] = peer{IP: net.IP(sendAddrs[0]), Port: recvPort, Hostname: hostname}
			delete(peerStringMap, i)
		}
	}
	return len(peerStringMap) == 0, nil
}

func ReadHostfile(hostfile string) (Host, peerStringMap, error) {
	containerName := os.Getenv("CONTAINER_NAME")
	contents, err := ioutil.ReadFile(hostfile)
	if err != nil {
		log.Fatal(err)
	}
	myID := -1
	peerStringsMap := make(peerStringMap)
	for i, name := range strings.Split(string(contents), "\n") {
		if name != "" { // remove blank lines
			if name == containerName {
				myID = i
			}
			peerStringsMap[Host(i)] = name
		}
	}
	if myID == -1 {
		return Host(-1), peerStringsMap, errors.New("did not find our own container name in hostfile")
	}
	return Host(myID), peerStringsMap, nil
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
	for peerID, p := range r.peers {
		if peerID == r.id {
			continue
		} else {
			response := r.AppendEntriesRPC(p, entries)
			// TODO - should we check response.term to see if we need to jump ahead or something?
			responses[peerID] = response.success
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
		log.Fatal("error dialing peer") // TODO - this should not be fatal, we should just take note of the missing peer and maybe retry later
	}
	err = conn.Call("AppendEntries", args, &reply)
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
	responses := make(map[Host]bool)
	for peerID, p := range r.peers {
		// TODO - confirm that we do not need to worry about receiving an AppendEntriesRPC from current leader mid-election
		// if this DOES happen, because we lagged behind, then there must be a majority of peers who have moved on
		// and they will just reject our RequestVoteRPC.
		if peerID == r.id { // we always vote for ourself
			responses[peerID] = true
		} else {
			response := r.RequestVoteRPC(p)
			// TODO - should we check response.term??
			responses[peerID] = response.success
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
		log.Fatal("error dialing peer") // TODO should this be fatal?
	}
	err = conn.Call("Vote", args, &reply)
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
	for {
		select {
		case <-quitChan:
			log.Println("QUIT RECV DAEMON")
			return
		default:
			l, err := net.Listen("tcp", ":"+fmt.Sprintf("%d", recvPort))
			if err != nil {
				log.Fatal("listen error:", err)
			}
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
