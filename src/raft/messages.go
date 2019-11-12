package raft

import "fmt"

type messageType int

type GenericMessage interface {
	String() string
	GetType() messageType
}

// NOTE - To prevent errors, we make the `msgType` an unexported field.
// However, this means that to encode/decode using Gob, we also need to provide custom encoder/decoder methods
// Therefore, we use a wrapper type and implement MarshalBinary() and UnmarshalBinary() methods, as suggested at:
// https://stackoverflow.com/questions/44290639/how-to-marshal-array-to-binary-and-unmarshal-binary-to-array-in-golang
// We can keep all this complexity in a separate file, using the GetType() method

// ViewChange message
type ViewChange struct {
	msgType   messageType // Must be equal to 2
	ServerID  Host
	Attempted Host
}

const (
	ViewChangeType      messageType = 2
	ViewChangeProofType messageType = 3
)

func NewViewChange(s Host, a Host) ViewChange {
	return ViewChange{msgType: ViewChangeType, ServerID: s, Attempted: a}
}

func (v ViewChange) String() string {
	return fmt.Sprintf("VC. Type: %d, ServerID: %d, Attempted: %d", v.msgType, v.ServerID, v.Attempted)
}

func (v ViewChange) GetType() messageType {
	return v.msgType
}

// ViewChangeProof message
type ViewChangeProof struct {
	msgType   messageType // Must be equal to 3
	ServerID  Host
	Installed Host
}

func NewViewChangeProof(s Host, i Host) ViewChangeProof {
	return ViewChangeProof{
		msgType:   ViewChangeProofType,
		ServerID:  s,
		Installed: i}
}

func (v ViewChangeProof) String() string {
	return fmt.Sprintf("VCP. Type: %d, ServerID: %d, Installed: %d", v.msgType, v.ServerID, v.Installed)
}

func (v ViewChangeProof) GetType() messageType {
	return v.msgType
}
