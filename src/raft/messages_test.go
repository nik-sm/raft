package raft

import "fmt"

func ExampleNewViewChange() {
	v := NewViewChange(host(5), host(6))
	fmt.Println(v.GetType())
	// Output: 2
}

func ExampleNewViewChangeProof() {
	v := NewViewChange(host(5), host(6))
	fmt.Println(v.GetType())
	// Output: 3
}
