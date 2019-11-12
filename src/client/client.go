package client

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/rpc"
	"raft"
	"time"
)

var hostfile string
var datafile string
var recvPort string

// Client sends lines from a fixed data file to the raft leader
func Client() {
	flag.Parse()
	peers := make(raft.PeerMap)
	raft.ResolveAllPeers(peers, hostfile)

	// Begin assuming node 0 is leader
	server, err := rpc.DialHTTP("tcp", peers[0].IP.String()+fmt.Sprintf(":%d", peers[0].Port))
	if err != nil {
		log.Fatal("dialing:", err)
	}
	args := &raft.ClientData{Contents: 12345}
	var reply int
	err = server.Call("Raft.ClientSendData", args, &reply)
	if err != nil {
		log.Fatal("client send data:", err)
	}
	fmt.Printf("Client Send Data. Sent %d. Recv %d", args.Contents, reply)
}

func init() {
	// TODO - reusing '-h' here causes panic, probably because of import raft
	flag.StringVar(&hostfile, "hostfile", "hostfile.txt", "name of hostfile")
	flag.StringVar(&datafile, "data", "datafile.txt", "name of data file")
	fmt.Printf("datafile: %s\n", datafile)

	recvPort = "4321"
	//TODO gob.register?

	rand.Seed(time.Now().UTC().UnixNano())
}
