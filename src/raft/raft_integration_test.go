package raft_test

import (
	"net"
	"raft"
	"sync"
	"testing"
)

type HostMap = raft.HostMap
type ClientMap = raft.ClientMap
type Peer = raft.Peer

type Startable interface {
	Start()
	QuitChan() chan bool
}

func TestMultipleNodes(t *testing.T) {
	t.Error("TODO")
	localhost := net.IPv4(127, 0, 0, 1)

	hosts := make(HostMap)
	hosts[0] = Peer{
		IP:       localhost,
		Port:     10000,
		Hostname: ""}
	hosts[1] = Peer{
		IP:       localhost,
		Port:     10001,
		Hostname: ""}
	hosts[2] = Peer{
		IP:       localhost,
		Port:     10002,
		Hostname: ""}

	clients := make(ClientMap)
	clients[0] = Peer{
		IP:       localhost,
		Port:     20000,
		Hostname: ""}
	clients[1] = Peer{
		IP:       localhost,
		Port:     20001,
		Hostname: ""}

	quitChan := make(chan bool)

	r0 := raft.NewRaftNode(0, 10000, hosts, clients, quitChan)
	r1 := raft.NewRaftNode(1, 10001, hosts, clients, quitChan)
	r2 := raft.NewRaftNode(2, 10002, hosts, clients, quitChan)

	//	c0 := client.NewClientNode(0, 20000, hosts, clients, quitChan)
	//	c0.Datafile = "/Users/nik/Desktop/neu/courses/2year/fall/7610-distributed-systems/hw/project_3/datafile.0.txt"

	//	c1 := client.NewClientNode(1, 20001, hosts, clients, quitChan)
	//	c1.Datafile = "/Users/nik/Desktop/neu/courses/2year/fall/7610-distributed-systems/hw/project_3/datafile.1.txt"

	var wg sync.WaitGroup
	waitForNode := func(s Startable, wg *sync.WaitGroup) {
		go s.Start()
		<-s.QuitChan()
		wg.Done()
	}

	go waitForNode(r0, &wg)
	wg.Add(1)

	go waitForNode(r1, &wg)
	wg.Add(1)

	go waitForNode(r2, &wg)
	wg.Add(1)

	//	go waitForNode(c0, &wg)
	//	wg.Add(1)

	//	go waitForNode(c1, &wg)
	//	wg.Add(1)

	wg.Wait()
	t.Log("all done")

	ok := stateMachineSafety(r0.StateMachine, r1.StateMachine) &&
		stateMachineSafety(r0.StateMachine, r2.StateMachine)

	if !ok {
		t.Error("state machine safety violation")
	}
}

// "If a server has applied a log entry at a given index,
// no other server will ever apply a different log entry for the same index"
func stateMachineSafety(s1 raft.StateMachine, s2 raft.StateMachine) bool {
	var longer raft.StateMachine
	var shorter raft.StateMachine
	if len(s1.Contents) > len(s2.Contents) {
		longer = s1
		shorter = s2
	} else {
		longer = s2
		shorter = s1
	}

	for idx, entry := range longer.Contents {
		if idx < len(shorter.Contents) {
			if shorter.Contents[idx] != entry {
				return false
			}
		}
	}
	return true
}
