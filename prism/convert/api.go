package convert

import (
	"fmt"
)

var _ = fmt.Print

type Convert8 func(rgb []uint8)
type Convert16 func(rgb []uint16)
