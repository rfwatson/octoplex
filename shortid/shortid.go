package shortid

import (
	"crypto/rand"
	"fmt"
)

const defaultLenBytes = 6

// ID is a short ID. It is not intended to guarantee uniqueness or any other
// cryptographic property and should only be used for labelling and other
// non-critical purposes.
type ID []byte

// New generates a new short ID, of length 6 bytes.
func New() ID {
	p := make([]byte, defaultLenBytes)
	_, _ = rand.Read(p)
	return ID(p)
}

// String implements the fmt.Stringer interface.
func (id ID) String() string {
	return fmt.Sprintf("%x", []byte(id))
}
