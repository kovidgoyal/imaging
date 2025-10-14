package icc

import (
	"encoding/binary"
	"fmt"
)

type TextDescription struct {
	ASCII string
}

func parseTextDescription(data []byte) (TextDescription, error) {
	desc := TextDescription{}
	var b [3]uint32
	if n, err := binary.Decode(data, binary.BigEndian, b[:]); err != nil {
		return desc, err
	} else {
		data = data[n:]
	}

	if s := Signature(b[0]); s != DescSignature {
		return desc, fmt.Errorf("expected %v but got %v", DescSignature, s)
	}
	asciiCount := b[2]

	if asciiCount > 1 {
		desc.ASCII = string(data[:asciiCount-1]) // skip terminating null
	}

	return desc, nil
}
