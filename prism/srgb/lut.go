package srgb

import (
	"github.com/kovidgoyal/imaging/prism/linear"
	"github.com/kovidgoyal/imaging/prism/linear/lut"
	"sync"
)

var linearToEncoded8LUT = sync.OnceValue(func() []uint8 { v := lut.BuildLinearTo8Bit(linearToEncoded); return v[:] })
var encoded8ToLinearLUT = sync.OnceValue(func() []float32 { v := lut.Build8BitToLinear(encodedToLinear); return v[:] })
var linearToEncoded16LUT = sync.OnceValue(func() []uint16 { v := lut.BuildLinearTo16Bit(linearToEncoded); return v[:] })
var encoded16ToLinearLUT = sync.OnceValue(func() []float32 { v := lut.Build16BitToLinear(encodedToLinear); return v[:] })

// From8Bit converts an 8-bit sRGB encoded value to a normalised linear value
// between 0.0 and 1.0.
//
// This implementation uses a fast look-up table without sacrificing accuracy.
func From8Bit(v uint8) float32 {
	return encoded8ToLinearLUT()[v]
}

// From16Bit converts a 16-bit sRGB encoded value to a normalised linear value
// between 0.0 and 1.0.
//
// This implementation uses a fast look-up table without sacrificing accuracy.
func From16Bit(v uint16) float32 {
	return encoded16ToLinearLUT()[v]
}

// To8Bit converts a linear value to an 8-bit sRGB encoded value, clipping the
// linear value to between 0.0 and 1.0.
//
// This implementation uses a fast look-up table and is approximate. For more
// accuracy, see ConvertLinearTo8Bit.
func To8Bit(v float32) uint8 {
	return linearToEncoded8LUT()[linear.NormalisedTo9Bit(v)]
}

// To16Bit converts a linear value to a 16-bit sRGB encoded value, clipping the
// linear value to between 0.0 and 1.0.
//
// This implementation uses a fast look-up table and is approximate. For more
// accuracy, see ConvertLinearTo16Bit.
func To16Bit(v float32) uint16 {
	return linearToEncoded16LUT()[linear.NormalisedTo16Bit(v)]
}
