package client

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/rpc"
	"os"
	"raft"
	"time"
)

var hostfile string
var datafile string
var recvPort string

// Client sends lines from a fixed data file to the raft leader
func Client() {
	flag.Parse()
	hosts := make(raft.HostMap)
	clients := make(raft.ClientMap)
	myClientID := raft.Client(raft.ResolveAllPeers(hosts, clients, hostfile, false))

	// Begin assuming node 0 is leader
	server, err := rpc.Dial("tcp", fmt.Sprintf("%s:%d", hosts[0].IP.String(), hosts[0].Port))
	if err != nil {
		log.Fatal("dialing:", err)
	}

	// Setup data file
	dataFile, err := os.Open(datafile)
	if err != nil {
		log.Fatal("could not open data file")
	}
	defer dataFile.Close()
	scanner := bufio.NewScanner(dataFile)

	// Loop variables
	var response raft.ClientResponse
	var serialNum raft.ClientSerialNum = 0
	var data string

	for scanner.Scan() {
		// Prepare input for server
		data = scanner.Text()
		log.Printf("Sending data: %s\n", data)
		args := &raft.ClientDataStruct{ClientID: myClientID,
			ClientSerialNum: serialNum,
			Data:            raft.ClientData(data)}

		// Do RPC
		err = server.Call("RaftNode.StoreClientData", args, &response)
		if err != nil {
			log.Fatal("client send data:", err)
		}
		log.Printf("Received response: %s\n", response)
		if !response.Success {
			// TODO - should retry after delay or something
			log.Println("server failed to store...")
		}

		// Prepare next iteration
		serialNum++
		time.Sleep(1 * time.Second)
	}
}

func init() {
	// TODO - reusing '-h' here causes panic, probably because of import raft
	flag.StringVar(&hostfile, "hostfile", "hostfile.json", "name of hostfile")
	flag.StringVar(&datafile, "data", "datafile.txt", "name of data file")
	fmt.Printf("datafile: %s\n", datafile)

	recvPort = "4321"
	//TODO gob.register?

	rand.Seed(time.Now().UTC().UnixNano())
}
