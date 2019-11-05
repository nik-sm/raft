package raft

import (
	"encoding/gob"
	"flag"
	"log"
	"math"
	"math/rand"
	"time"
)

var hostfile string
var testScenario int
var recvPort string
var duration int
var verbose bool
var tfrom int
var ttil int

func selectTimeout(min int, max int) time.Duration {
	return time.Duration(rand.Intn(max-min+1) + min)
}

// returns true when an agent has a majority of votes for the proposed view
func (a agent) preinstallReady(view host) bool {
	nPeers := len(a.peers)
	nReq := int(math.Floor(float64(nPeers)/2)) + 1
	nFound := 0
	for _, v := range a.vcMessages {
		if v.Attempted == view {
			nFound++
		}
	}
	if nFound >= nReq {
		return true
	}
	return false
}

// agent shifts into the election state, restarts its electionTimeout, and updates its
// election-related local variables
func (a *agent) shiftToLeaderElection(view host) {
	if a.verbose {
		log.Printf("shiftToLeaderElection, vote for %d", view)
	}
	a.resetElectionTicker()
	a.state = election
	vcMessages := make(map[host]ViewChange)
	a.lastAttempted = view
	v := NewViewChange(a.id, view)
	a.multicast(v)
	vcMessages[a.id] = v
	a.vcMessages = vcMessages
}

func (a agent) isLeader(v host) bool {
	return int(v)%len(a.peers) == int(a.id)
}

// this function returns true if a message should be discarded
func (a agent) shouldDiscard(msg incomingUDPMessage) (bool, string) {
	switch t := msg.Contents.GetType(); t {
	case ViewChangeType: // ViewChange
		v, ok := msg.Contents.(ViewChange)
		if !ok {
			log.Fatal("error casting to ViewChange")
		}
		if msg.SourcePeerID == a.id {
			return true, "our own message"
		}
		if a.state != election {
			return true, "not in election state"
		}
		if v.Attempted <= a.lastInstalled {
			return true, "attempted lower or equal to our latest"
		}
		if _, haveEntry := a.vcMessages[msg.SourcePeerID]; haveEntry {
			return true, "already have message from this peer"
		}
		return false, ""
	case ViewChangeProofType: // ViewChangeProof
		v, ok := msg.Contents.(ViewChangeProof)
		if !ok {
			log.Fatal("error casting to ViewChangeProof")
		}
		if msg.SourcePeerID == a.id {
			return true, "our own message"
		}
		if a.state != election {
			return true, "not in election state"
		}
		if v.Installed <= a.lastInstalled {
			return true, "Already installed greater or equal view"
		}
		return false, ""
	default:
		log.Fatalf("unknown message type: %d", t)
	}
	return false, ""
}

// wrapper function for checking incoming messages for conflicts
// and dispatching an appropriate method based on the decoded message type
func (a *agent) handleMessage(msg incomingUDPMessage) {
	if a.verbose {
		log.Printf("Received message. %s", msg.String())
	}
	if b, reason := a.shouldDiscard(msg); b {
		if a.verbose {
			log.Print("discard message: " + reason)
		}
		return
	}

	switch t := msg.Contents.GetType(); t {
	case ViewChangeType:
		v, ok := msg.Contents.(ViewChange)
		if !ok {
			log.Fatal("error casting to ViewChange")
		}
		a.handleViewChange(msg.SourcePeerID, v)
	case ViewChangeProofType:
		v, ok := msg.Contents.(ViewChangeProof)
		if !ok {
			log.Fatal("error casting to ViewChangeProof")
		}
		a.handleViewChangeProof(v)
	default:
		log.Fatalf("Unknown message type: %d", t)
	}
}

func (a *agent) handleViewChange(senderID host, v ViewChange) {
	if a.verbose {
		log.Printf("Received ViewChange: %s\n", v.String())
	}
	att := v.Attempted
	// NOTE - if we somehow get to the state where no majority of nodes is
	// attempting the same view, it is necessary for us to bring them back
	// "into sync" somehow. Either we can add randomness to their timers,
	// or more simply, we can allow lagging nodes to catch up to the most recent
	// election
	if att > a.lastAttempted {
		a.shiftToLeaderElection(att)
		a.vcMessages[senderID] = v
	} else if att == a.lastAttempted {
		// store the message only if we do not already have a value for this peer
		if _, ok := a.vcMessages[senderID]; !ok {
			// log.Printf("Store message for peer %d", senderID)
			a.vcMessages[senderID] = v
			if a.verbose {
				log.Printf("Current vcMessages: %s\n", a.vcMessages.String())
			}
		}
		if a.preinstallReady(att) {
			if a.verbose {
				log.Printf("Preinstall Ready for %d\n", att)
			}
			//a.timeout *= 2

			if a.isLeader(att) {
				a.lastInstalled = a.lastAttempted
				a.shiftToLeader(true)
			} else {
				a.lastInstalled = a.lastAttempted
				a.shiftToFollower(true)
			}
		}
	}
}

func (a *agent) handleViewChangeProof(v ViewChangeProof) {
	if v.Installed > a.lastInstalled {
		//prev := a.lastInstalled
		if a.verbose {
			log.Printf("CHANGE VIEW from %d to %d", a.lastInstalled, v.Installed)
		}
		a.lastInstalled = v.Installed

		if a.isLeader(a.lastInstalled) {
			a.shiftToLeader(false)
		} else {
			a.shiftToFollower(false)
		}
	}
}

// entries in an agent's viewHistory either contain a list of votes that
// constituted a majority, or all "-1" if the view was installed due to
// `ViewChangeProof` message
func (a *agent) appendHistory(view host, withVotes bool) {
	if a.verbose {
		log.Printf("history before append: %s", a.vh.String())
	}
	votes := make([]int, len(a.peers))
	if withVotes { // record what evidence we received for installing this view
		for i := range a.peers {
			if _, ok := a.vcMessages[i]; ok { // we have a viewchange from peer i
				votes[i] = 1
			} else {
				votes[i] = 0
			}
		}
	} else { // we must have reached this view due to ViewChangeProof
		for i := range a.peers {
			votes[i] = -1
		}
	}
	a.vh = append(a.vh, viewEvent{view: view, votesReceived: votes})
	if a.verbose {
		log.Printf("history after append: %s", a.vh.String())
	}
}

func (a *agent) shiftToLeader(withVotes bool) {
	if a.killSwitch {
		log.Println("TEST CASE: node terminating")
		a.shutdown() // NOTE - this puts a message on quitChan, but we will still appendHistory to show our evidence for this installation
	}
	a.state = leader
	a.appendHistory(a.lastInstalled, withVotes)
	log.Printf("%d SHIFT TO LEADER. VIEW=%d", a.id, a.lastInstalled)
}

func (a *agent) shiftToFollower(withVotes bool) {
	a.state = follower
	a.appendHistory(a.lastInstalled, withVotes)
	log.Printf("%d SHIFT TO FOLLOWER. VIEW=%d", a.id, a.lastInstalled)
}

func containsH(s []host, i host) bool {
	for _, v := range s {
		if v == i {
			return true
		}
	}
	return false
}

func containsI(s []int, i int) bool {
	for _, v := range s {
		if v == i {
			return true
		}
	}
	return false
}

// Main leader Election Procedure
func (a *agent) protocol() {
	log.Printf("Begin Protocol. verbose: %t", a.verbose)

	//if a.id == 0 {
	//	log.Printf("SLEEP 30!")
	//	time.Sleep(30 * time.Second)
	//}

	for {
		select {
		case msg, ok := <-a.recvChan:
			if !ok {
				log.Printf("RecvChan might be closed: %s", msg.String())
			}
			a.handleMessage(msg)
		case <-a.proofTicker.C:
			// Periodically send out ViewChangeProof messages for servers that are behind or re-joining
			a.multicast(NewViewChangeProof(a.id, a.lastInstalled))
		case <-a.electionTicker.C:
			// Periodically time out and start a new election
			if a.verbose {
				log.Println("TIMED OUT!")
			}
			a.shiftToLeaderElection(a.lastAttempted + 1)
		}
	}
}

// Quit the protocol on a timer (to be run in separate goroutine)
func (a agent) quitter(quitTime int) {
	for {
		select {
		case <-a.quitChan:
			// the agent decided it should quit
			return
		case <-time.After(time.Duration(quitTime) * time.Second):
			// we decide the agent should quit
			a.quitChan <- true
		}
	}
}

func getKillSwitch(nodeID host, testScenario int) bool {
	var killNodes []host
	if testScenario == 3 {
		killNodes = []host{1}
	} else if testScenario == 4 {
		killNodes = []host{1, 2}
	} else if testScenario == 5 {
		killNodes = []host{1, 2, 3}
	}

	if len(killNodes) != 0 && containsH(killNodes, nodeID) {
		log.Println("TEST CASE: This node will die upon becoming leader")
		return true
	}
	return false
}

func (a *agent) initTickers() {
	a.electionTicker = *time.NewTicker(a.electionTimeout * time.Second)
	a.proofTicker = *time.NewTicker(a.vcpTimeout * time.Second)
}

func (a *agent) resetElectionTicker() {
	a.electionTicker = *time.NewTicker(a.electionTimeout * time.Second)
}

func (a *agent) shutdown() {
	log.Println("AGENT SHUTTING DOWN")
	a.quitChan <- true
}

func init() {
	defer flag.Parse()
	flag.StringVar(&hostfile, "h", "hostfile.txt", "name of hostfile")
	flag.IntVar(&testScenario, "t", 0, "test scenario to run")
	flag.IntVar(&duration, "d", 30, "time until node shutdown")
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.IntVar(&tfrom, "tfrom", 5, "low end of election timeout range")
	flag.IntVar(&ttil, "ttil", 10, "high end of election timeout range")

	recvPort = "4321"
	gob.Register(ViewChange{})
	gob.Register(ViewChangeProof{})
	rand.Seed(time.Now().UTC().UnixNano())
}

func Raft() {
	peers := make(peerMap)
	if !containsI([]int{1, 2, 3, 4, 5}, testScenario) {
		log.Fatalf("invalid test scenario: %d", testScenario)
	} else {
		log.Printf("TEST SCENARIO: %d", testScenario)
	}

	recvChan := make(chan incomingUDPMessage)
	id, peerStringMap, err := readHostfile(hostfile)
	if err != nil {
		log.Fatal(err)
	}
	for {
		allFound, err := resolvePeers(peers, peerStringMap)
		if err != nil {
			log.Fatal(err)
		}
		if allFound {
			break
		}
		time.Sleep(1 * time.Second)
	}
	vcMessages := make(vcMap)

	quitChan := make(chan bool)

	go recvDaemon(recvChan, quitChan, peers, id)

	a := agent{id: id,
		peers:           peers,
		vcMessages:      vcMessages,
		electionTimeout: selectTimeout(tfrom, ttil),
		vcpTimeout:      selectTimeout(1, 1),
		state:           follower,
		lastAttempted:   -1, // Starting with -1 so we run an initial election for node 0
		lastInstalled:   -1,
		recvChan:        recvChan,
		quitChan:        quitChan,
		killSwitch:      getKillSwitch(id, testScenario),
		verbose:         verbose} // NOTE - set to true for verbose mode
	a.initTickers()
	log.Printf("Agent details: %s", a.String())

	go a.protocol()
	go a.quitter(duration)
	<-quitChan
	log.Printf("FINAL VIEW HISTORY: %s", a.vh.String())
}
