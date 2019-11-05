package raft

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
)

// Populate a map associating peer index to IP, port and hostname
// returns true if all peers were found, and possible error
func resolvePeers(peers peerMap, peerStringsMap peerStringMap) (bool, error) {
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

func readHostfile(hostfile string) (host, peerStringMap, error) {
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
			peerStringsMap[host(i)] = name
		}
	}
	if myID == -1 {
		return host(-1), peerStringsMap, errors.New("did not find our own container name in hostfile")
	}
	return host(myID), peerStringsMap, nil
}

func (a agent) multicast(msg GenericMessage) {
	if a.verbose {
		log.Printf("multicast: %s", msg.String())
	}
	for _, peer := range a.peers {
		send(msg, peer)
	}
}

func (a agent) unicast(msg GenericMessage, p peer) {
	send(msg, p)
}

func send(msg GenericMessage, p peer) {
	// Setup the connection
	writeConn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: p.IP, Port: p.Port})
	if err != nil {
		log.Fatal("dial udp: ", err)
	}
	defer writeConn.Close()
	// log.Printf("Sending %s from local addr %s to remote addr %s\n", msg.String(), writeConn.LocalAddr().String(), writeConn.RemoteAddr().String())

	// Encode the message
	buf := new(bytes.Buffer)
	encoder := gob.NewEncoder(buf)
	err = encoder.Encode(&msg)
	if err != nil {
		log.Fatal("encoding: ", err)
	}

	// Send the message
	_, err = writeConn.Write(buf.Bytes())
	if err != nil {
		log.Fatal("writing :", err)
	}
}

func sameIP(first net.IP, second net.IP) bool {
	if len(first) != len(second) {
		return false
	}
	for i, v := range first {
		if second[i] != v {
			return false
		}
	}
	return true
}

func getPeerByAddr(peers peerMap, addr *net.UDPAddr) (host, peer, error) {
	for idx, p := range peers {
		if sameIP(p.IP, addr.IP) {
			return idx, p, nil
		}
	}
	return 0, peer{}, fmt.Errorf("peer not found! %s", addr.String())
}

// Grab incoming message and push it into recvChan
func recvDaemon(recvChan chan<- incomingUDPMessage, quitChan <-chan bool, peers peerMap, myID host) {
	// setup listener for incoming UDP connection
	defer close(recvChan)
	myself := peers[myID]
	recvConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: myself.IP, Port: myself.Port})
	if err != nil {
		log.Fatal("listen udp: ", err)
	}
	defer recvConn.Close()

	var m GenericMessage
	readBytes := make([]byte, 5000) // TODO - can we avoid specifying size here?? For more complicated message types, we may not know their encoded sizes...
	buf := new(bytes.Buffer)
	for {
		select {
		case <-quitChan:
			log.Println("QUIT RECV DAEMON")
			return
		default:
			n, addr, err := recvConn.ReadFromUDP(readBytes) // fill readBytes with contents from packet
			if err != nil {
				log.Fatal("read udp: ", err)
			}
			buf.Reset()
			buf.Write(readBytes[:n])
			dec := gob.NewDecoder(buf)
			err = dec.Decode(&m)
			if err != nil {
				log.Fatal("decode :", err)
			}

			pid, p, err := getPeerByAddr(peers, addr)
			if err != nil {
				log.Fatal(err)
			}
			recvChan <- incomingUDPMessage{SourcePeerID: pid, SourcePeer: p, Contents: m}
		}
	}
}
