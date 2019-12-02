package client

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/rpc"
	"raft"
	"strings"
	"sync"
	"time"
)

var hostfile string
var datafile string
var duration int
var verbose bool

// ClientNode represents a client who stores data in the raft cluster
type ClientNode struct {
	sync.Mutex
	retryTimeout  time.Duration
	hosts         raft.HostMap
	serialNum     raft.ClientSerialNum
	clients       raft.ClientMap
	currentLeader raft.HostID
	verbose       bool
	id            raft.ClientID
	recvPort      int
	Datafile      string
	data          []raft.ClientData
	quitChan      chan bool
}

func (c *ClientNode) readDataFile() {
	contents, err := ioutil.ReadFile(c.Datafile)
	if err != nil {
		panic(err)
	}
	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		c.data = append(c.data, raft.ClientData(line))
	}
}

// returns true if success
func (c *ClientNode) syncSendDataToHost(data raft.ClientData, host raft.HostID) (raft.HostID, bool) {
	log.Printf("Sending data: %s to host: %d\n", data, host)

	// Dial the host
	server, err := rpc.Dial("tcp", fmt.Sprintf("%s:%d", c.hosts[host].IP.String(), c.hosts[host].Port))
	if err != nil {
		log.Print("Warning: problem dialing host:", err)
		return raft.HostID(-1), false
	}

	// Prepare input for server
	args := &raft.ClientDataStruct{
		ClientID:        c.id,
		ClientSerialNum: c.serialNum,
		Data:            data}

	// Sync RPC
	response := raft.ClientResponse{}
	err = server.Call("RaftNode.StoreClientData", args, &response)
	if err != nil {
		panic(err)
	}
	return response.Leader, response.Success
}

func (c *ClientNode) trySendLeader(data raft.ClientData, possibleLeader raft.HostID) bool {
	leader, success := c.syncSendDataToHost(data, possibleLeader)
	if success {
		return true
	} else if leader != -1 { // we were informed of a new leader
		c.currentLeader = leader
		return false
	}
	return false
}

// The client tries to send the current data to the appropriate leader.
// It will retry until success, each time making the RPC asynchronously and waiting for the timeout period to allow a response
func (c *ClientNode) sendData(data raft.ClientData) {
	if c.verbose {
		log.Println("client begin send data loop")
	}
	// Choose the host
	done := false
	for !done {
		if c.currentLeader != -1 { // we know who to send to
			done = c.trySendLeader(data, c.currentLeader)
			if c.verbose {
				log.Printf("tried sending to known leader: %d, result %t", c.currentLeader, done)
			}
		} else {
			if c.verbose {
				log.Printf("Unknown leader...")
			}
			for possibleLeader := range c.hosts {
				done = c.trySendLeader(data, possibleLeader)
				if c.verbose {
					log.Printf("Tried leader %d, result %t", possibleLeader, done)
				}
				if done {
					break
				}
			}
		}
		time.Sleep(c.retryTimeout)
	}
}

func (c *ClientNode) QuitChan() chan bool {
	return c.quitChan
}

func (c *ClientNode) Start() {
	c.readDataFile()
	go c.protocol()
	go c.quitter(duration)
	<-c.quitChan
	log.Println("CLIENT FINISHED")
}

func (c *ClientNode) protocol() {

	for _, data := range c.data {
		select {
		case <-c.quitChan:
			return
		default:
			// Prepare next iteration
			c.serialNum++
			c.sendData(data)
		}
	}
}

func (c *ClientNode) quitter(quitTime int) {
	for {
		select {
		case <-c.quitChan: // the node decided it should quit
			return
		case <-time.After(time.Duration(quitTime) * time.Second): // we decide the node should quit
			c.quitChan <- true
			return
		}
	}
}

func init() {
	// TODO - redefining the same flags as used in raft causes panic...
	flag.StringVar(&hostfile, "hostfile", "hostfile.json", "name of hostfile")
	flag.StringVar(&datafile, "data", "datafile.txt", "name of data file")
	flag.IntVar(&duration, "duration", 30, "time until node shutdown")
	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	fmt.Printf("datafile: %s\n", datafile)

	rand.Seed(time.Now().UTC().UnixNano())
}

func NewClientNode(id raft.ClientID, recvPort int, hosts raft.HostMap, clients raft.ClientMap, quitChan chan bool) *ClientNode {
	c := ClientNode{
		id:            id,
		recvPort:      recvPort,
		hosts:         hosts,
		clients:       clients,
		serialNum:     0,
		verbose:       verbose,
		currentLeader: -1, // Clients start assuming node 0 is leader
		Datafile:      datafile,
		quitChan:      quitChan,
		retryTimeout:  time.Duration(1 * time.Second)}
	return &c
}

// Client sends lines from a fixed data file to the raft leader
func Client() {
	flag.Parse()

	hosts := make(raft.HostMap)
	clients := make(raft.ClientMap)
	quitChan := make(chan bool)

	intID, recvPort := raft.ResolveAllPeers(hosts, clients, hostfile, false)
	id := raft.ClientID(intID)
	c := NewClientNode(id, recvPort, hosts, clients, quitChan)
	c.Start()
}
