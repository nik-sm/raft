package raft

import "fmt"

func ExampleNewViewChange() {
	v := NewViewChange(Host(5), Host(6))
	fmt.Println(v.GetType())
	// Output: 2
}

func ExampleNewViewChangeProof() {
	v := NewViewChange(Host(5), Host(6))
	fmt.Println(v.GetType())
	// Output: 3
}
