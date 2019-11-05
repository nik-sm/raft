package raft

import "fmt"

// type host int

type MessageType int

type GenericMessage interface {
	String() string
	GetType() MessageType
}

// NOTE - To prevent errors, we make the `msgType` an unexported field.
// However, this means that to encode/decode using Gob, we also need to provide custom encoder/decoder methods
// Therefore, we use a wrapper type and implement MarshalBinary() and UnmarshalBinary() methods, as suggested at:
// https://stackoverflow.com/questions/44290639/how-to-marshal-array-to-binary-and-unmarshal-binary-to-array-in-golang
// We can keep all this complexity in a separate file, using the GetType() method

// ViewChange message
type ViewChange struct {
	msgType   MessageType // Must be equal to 2
	ServerID  int
	Attempted int
}

const (
	ViewChangeType      MessageType = 2
	ViewChangeProofType MessageType = 3
)

func NewViewChange(s int, a int) ViewChange {
	return ViewChange{msgType: ViewChangeType, ServerID: s, Attempted: a}
}

func (v ViewChange) String() string {
	return fmt.Sprintf("VC. Type: %d, ServerID: %d, Attempted: %d", v.msgType, v.ServerID, v.Attempted)
}

func (v ViewChange) GetType() MessageType {
	return v.msgType
}

// ViewChangeProof message
type ViewChangeProof struct {
	msgType   MessageType // Must be equal to 3
	ServerID  int
	Installed int
}

func NewViewChangeProof(s int, i int) ViewChangeProof {
	return ViewChangeProof{
		msgType:   ViewChangeProofType,
		ServerID:  s,
		Installed: i}
}

func (v ViewChangeProof) String() string {
	return fmt.Sprintf("VCP. Type: %d, ServerID: %d, Installed: %d", v.msgType, v.ServerID, v.Installed)
}

func (v ViewChangeProof) GetType() MessageType {
	return v.msgType
}
