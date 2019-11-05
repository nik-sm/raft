package raft

import (
	"bytes"
	"encoding/gob"
)

// ViewChange Wrappers
type WrapViewChange struct {
	MsgType   MessageType
	ServerID  int
	Attempted int
}

func (v ViewChange) MarshalBinary() ([]byte, error) {
	// Wrap struct
	w := WrapViewChange{MsgType: v.msgType, ServerID: v.ServerID, Attempted: v.Attempted}

	// use default gob encoder
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(w); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (v *ViewChange) UnmarshalBinary(data []byte) error {
	w := WrapViewChange{}

	// Use default gob decoder
	reader := bytes.NewReader(data)
	dec := gob.NewDecoder(reader)
	if err := dec.Decode(&w); err != nil {
		return err
	}

	v.msgType = w.MsgType
	v.ServerID = w.ServerID
	v.Attempted = w.Attempted
	return nil
}

// ViewChangeProof Wrappers
type WrapViewChangeProof struct {
	MsgType   MessageType
	ServerID  int
	Installed int
}

func (v ViewChangeProof) MarshalBinary() ([]byte, error) {
	// Wrap struct
	w := WrapViewChangeProof{MsgType: v.msgType, ServerID: v.ServerID, Installed: v.Installed}

	// use default gob encoder
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(w); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (v *ViewChangeProof) UnmarshalBinary(data []byte) error {
	w := WrapViewChangeProof{}

	// Use default gob decoder
	reader := bytes.NewReader(data)
	dec := gob.NewDecoder(reader)
	if err := dec.Decode(&w); err != nil {
		return err
	}

	v.msgType = w.MsgType
	v.ServerID = w.ServerID
	v.Installed = w.Installed
	return nil
}
