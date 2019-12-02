package raft

import (
	"net"
	"testing"
)

func TestMultipleNodes(t *testing.T) {
	localhost := net.IPv4(127, 0, 0, 1)

	hosts := make(HostMap)
	hosts[0] = peer{
		IP:       localhost,
		Port:     10000,
		Hostname: ""}
	hosts[1] = peer{
		IP:       localhost,
		Port:     10001,
		Hostname: ""}
	hosts[2] = peer{
		IP:       localhost,
		Port:     10002,
		Hostname: ""}

	clients := make(ClientMap)
	clients[0] = peer{
		IP:       localhost,
		Port:     20000,
		Hostname: ""}
	clients[1] = peer{
		IP:       localhost,
		Port:     20001,
		Hostname: ""}

	quitChan := make(chan bool)

	r0 := NewRaftNode(0, 10000, hosts, clients, quitChan)
	r1 := NewRaftNode(0, 10001, hosts, clients, quitChan)
	r2 := NewRaftNode(0, 10002, hosts, clients, quitChan)


	c1 := mockClientNode()
	r0 := mockRaftNode()

	r0 := NewRaftNode(
		Term(5), // voter's Term
		Term(4), // voter's lastLogTerm
		3,       // voter's lastLogIndex
		2)       // voter's currentLeader
}
