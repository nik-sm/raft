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
)

// For anyone in the peerStringMap, try to resolve and add them to the peerMap
// returns true if all peers were found, and possible error
func ResolvePeers(peers PeerMap, peerStringsMap peerStringMap) (bool, error) {
	for i, hostname := range peerStringsMap {
		log.Println("resolve host: ", hostname)
		sendAddr, err := net.ResolveUDPAddr("udp", hostname+":"+recvPort)
		// NOTE - not handling error here. We need to trust the hostfile contents so that
		// we can keep checking for the hosts, as specified by the hostfile, until they are alive
		if err == nil {
			log.Println("found: ", sendAddr)
			peers[i] = peer{IP: sendAddr.IP, Port: sendAddr.Port, Hostname: hostname}
			delete(peerStringsMap, i)
		}
	}
	return len(peerStringsMap) == 0, nil
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
	T               Term
	LeaderID        Host
	PrevLogIndex    int
	PrevLogTerm     Term
	Entries         []LogEntry
	LeaderCommitIdx int
}

func (a agent) MultiAppendEntriesRPC(async bool, entries []LogEntry) (Term, bool) {
	if a.verbose {
		log.Println("MultiAppendEntries")
	}
	results = make(map[Host]bool)
	for peerID, p := range a.peers {
		results[peerID] = a.AppendEntriesRPC(p)
	}
	log.Fatal('do something with results')
	return nil, nil
}


func (a agent) AppendEntriesRPC(p peer, entries []LogEntry) (Term, bool) {
	args = AppendEntriesStruct{T: a.term,
		LeaderID:        a.id,
		PrevLogIndex:    a.getPrevLogIndex(),
		PrevLogTerm:     a.getPrevLogTerm(),
		Entries:         entries,
		LeaderCommitIdx: a.commitIdx}
	var reply bool
	var err error
	err = p.Call("AppendEntries", args, &reply)
	if err != nil {
		log.Fatal("AppendEntriesRPC:", err)
	}
	return reply
}

type RequestVoteStruct struct {
	T           Term
	CandidateID Host
	LastLogIdx  int
	LastLogTerm Term
}

func (a agent) MultiRequestVoteRPC(async bool) (Term, bool) {
	if a.verbose {
		log.Println("MultiRequestVote")
	}
	for _, p := range a.peers {
		RequestVoteRPC(async, p)
	}
}

func (a agent) RequestVoteRPC(async bool, p peer) (Term, bool) {
	args = RequestVoteStruct{T: a.term,
		CandidateID: a.id
		LastLogIdx:	a.getLastLogIndex(),
		LastLogTerm: a.getLastLogTerm()}
	var reply bool
	var err error
	err = p.Call("RequestVote", args, &reply)
	if err != nil {
		log.Fatal("RequestVoteRPC:", err)
	}
	return reply
}

func recvDaemon(quitChan <-chan bool, peers PeerMap, myID Host) {
	for {
		select {
		case <-quitChan:
			log.Println("QUIT RECV DAEMON")
			return
		default:
			l, err := net.Listen("tcp", ":"+recvPort)
			if err != nil {
				log.Fatal("listen error:", err)
			}
			conn, err := l.Accept()
			if err != nil {
				log.Fatal("accept error:", err)
			}
			go rpc.ServeConn(conn)
		}
	}
}
