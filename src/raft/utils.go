package raft

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"
)

// ResolveAllPeers gets the net address of all raft hosts and clients, and stores these in
// the provided HostMap and ClientMap structures.
// Notice that all nodes must be alive to begin the protocol; thus We loop infinitely here,
// until we have information for all nodes.
// Returns our own id and our recvPort. id returned as integer. Caller (either host or client) must cast to HostID or ClientID appropriately
func ResolveAllPeers(hosts HostMap, clients ClientMap, hostfile string, amHost bool) (int, int) {
	decodedJSON := readHostfileJSON(hostfile)
	myHost := HostID(-1)
	myClient := ClientID(-1)
	myPort := -1

	// Lookup our own ID, depending whether we are a raft node or a client node
	containerName := os.Getenv("CONTAINER_NAME")
	if amHost {
		for i, node := range decodedJSON.RaftNodes {
			if node.Name == containerName {
				myHost = HostID(i)
				myPort = node.Port
			}
		}
	} else {
		for i, node := range decodedJSON.ClientNodes {
			if node.Name == containerName {
				myClient = ClientID(i)
				myPort = node.Port
			}
		}
	}
	if int(myHost) == -1 && int(myClient) == -1 {
		panic("Did not find our own name in the hostfile")
	}

	// Make maps of all known hosts and clients
	h, c := makeHostStringMap(decodedJSON)

	// Continue resolving until all hosts and clients found
	for {
		allFound := ResolvePeersOnce(hosts, clients, h, c)
		if allFound {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if amHost {
		return int(myHost), myPort
	}
	return int(myClient), myPort
}

// ResolvePeersOnce makes one attempt to identify all hosts and clients in the provided maps.
// As they are found, peers get removed from these maps.
// NOTE - We do not handle the errors from LookupHost, because we are waiting for nodes to come online.
func ResolvePeersOnce(hosts HostMap, clients ClientMap, h hostStringMap, c clientStringMap) bool {
	// Resolve raft hosts
	for i, node := range h {
		hostname := node.Name
		log.Println("resolve host: ", hostname)

		sendAddrs, err := net.LookupHost(hostname)
		if err == nil {
			ip := net.ParseIP(sendAddrs[0])
			log.Println("found: ", ip.String())
			hosts[i] = Peer{IP: ip, Port: node.Port, Hostname: hostname}
			delete(h, i)
		}
	}

	// Resolve clients
	for i, node := range c {
		clientname := node.Name
		log.Println("resolve client: ", clientname)
		sendAddrs, err := net.LookupHost(clientname)
		if err == nil {
			ip := net.ParseIP(sendAddrs[0])
			log.Println("found: ", ip.String())
			clients[i] = Peer{IP: ip, Port: node.Port, Hostname: clientname}
			delete(c, i)
		}
	}
	return len(h) == 0 && len(c) == 0
}

type hostsAndClients struct {
	RaftNodes   []raftNodeJSON   `json:"raftnodes"`
	ClientNodes []clientNodeJSON `json:"clientnodes"`
}

type raftNodeJSON struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

type clientNodeJSON struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

func readHostfileJSON(hostfile string) hostsAndClients {
	// get contents of JSON file
	jsonFile, err := os.Open(hostfile)
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)

	var hc hostsAndClients
	err = json.Unmarshal(byteValue, &hc)
	if err != nil {
		panic(err)
	}
	return hc
}

func makeHostStringMap(hc hostsAndClients) (hostStringMap, clientStringMap) {
	// Find our own ID and build a map for DNS lookup of other hosts
	h := make(hostStringMap)
	c := make(clientStringMap)
	for i, node := range hc.RaftNodes {
		h[HostID(i)] = node
	}
	for i, node := range hc.ClientNodes {
		c[ClientID(i)] = node
	}
	return h, c
}
