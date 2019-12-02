package raft_test

import (
	"client"
	"net"
	"raft"
	"testing"
)

type HostMap = raft.HostMap
type ClientMap = raft.ClientMap
type Peer = raft.Peer

func TestMultipleNodes(t *testing.T) {
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

	c0 := client.NewClientNode(0, 20000, hosts, clients, quitChan)
	c1 := client.NewClientNode(1, 20001, hosts, clients, quitChan)

	r0.protocol()
	r1.protocol()
	r2.protocol()

	c0.protocol()
	c1.protocol()
}
