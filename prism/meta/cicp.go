package meta

import (
	"fmt"
)

var _ = fmt.Print

type WellKnownColorSpace int

const (
	UnknownColorSpace WellKnownColorSpace = iota
	SRGB
	DisplayP3
	BT2020SpacePQ
)

type CodingIndependentCodePoints struct {
	ColorPrimaries, TransferCharacteristics, MatrixCoefficients, VideoFullRange uint8
}

func (c CodingIndependentCodePoints) WellKnownColorSpace() WellKnownColorSpace {
	if c.MatrixCoefficients != 0 && c.VideoFullRange != 1 {
		return UnknownColorSpace
	}
	switch c.TransferCharacteristics {
	case 13:
		switch c.ColorPrimaries {
		case 1:
			return SRGB
		case 12:
			return DisplayP3
		}
	case 16:
		switch c.ColorPrimaries {
		case 9:
			return BT2020SpacePQ
		}
	}
	return UnknownColorSpace
}
