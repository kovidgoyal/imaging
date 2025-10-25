package icc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// CLUTTag represents a color lookup table tag (TagColorLookupTable)
type CLUTTag struct {
	GridPoints     []int // e.g., [17,17,17] for 3D CLUT
	InputChannels  int
	OutputChannels int
	Values         []unit_float // flattened [in1, in2, ..., out1, out2, ...]
}

func (c CLUTTag) String() string {
	return fmt.Sprintf("CLUTTag{ inp:%v outp:%v grid:%v values[:9]:%v }", c.InputChannels, c.OutputChannels, c.GridPoints, c.Values[:min(9, len(c.Values))])
}

func make_clut(grid_points []int, inp, outp int, values []unit_float) *CLUTTag {
	return &CLUTTag{grid_points, inp, outp, values}
}

var _ ChannelTransformer = (*CLUTTag)(nil)

func default8(x uint8) unit_float         { return unit_float(x) / math.MaxUint8 }
func default16(x uint16) unit_float       { return unit_float(x) / math.MaxUint16 }
func u1Fixed15Number(x uint16) unit_float { return unit_float(x) / unit_float(1<<15) }
func lab_l8(x uint8) unit_float           { return (unit_float(x) / math.MaxUint8) * 100 }
func lab_ab8(x uint8) unit_float          { return (unit_float(x)/math.MaxUint8)*(127+128) - 128 }
func lab_l16(x uint16) unit_float         { return (unit_float(x) / math.MaxUint16) * 100 }
func lab_ab16(x uint16) unit_float        { return (unit_float(x)/math.MaxUint16)*(127+128) - 128 }
func lab_l16_legacy(x uint16) unit_float  { return (unit_float(x) / 0xff00) * 100 }
func lab_ab16_legacy(x uint16) unit_float { return (unit_float(x)/0xff00)*(127+128) - 128 }

type clut_decoder_func16 = func(src []uint16, dest []unit_float)
type clut_decoder_func8 = func(src []uint8, dest []unit_float)

func uniform_decoder16(d func(uint16) unit_float) clut_decoder_func16 {
	return func(src []uint16, dest []unit_float) {
		for i, x := range src {
			dest[i] = d(x)
		}
	}
}

func uniform_decoder8(d func(uint8) unit_float) clut_decoder_func8 {
	return func(src []uint8, dest []unit_float) {
		for i, x := range src {
			dest[i] = d(x)
		}
	}
}

func lab_decoder8(src []uint8, dest []unit_float) {
	dest[0], dest[1], dest[2] = lab_l8(src[0]), lab_ab8(src[1]), lab_ab8(src[2])
}
func lab_decoder16(src []uint16, dest []unit_float) {
	dest[0], dest[1], dest[2] = lab_l16(src[0]), lab_ab16(src[1]), lab_ab16(src[2])
}
func lab_decoder16_legacy(src []uint16, dest []unit_float) {
	dest[0], dest[1], dest[2] = lab_l16_legacy(src[0]), lab_ab16_legacy(src[1]), lab_ab16_legacy(src[2])
}

func decode_clut_table8(raw []byte, OutputChannels int, output_colorspace ColorSpace, ans []unit_float) {
	var d clut_decoder_func8
	switch output_colorspace {
	case ColorSpaceLab:
		d = lab_decoder8
	default:
		d = uniform_decoder8(default8)
	}
	temp := make([]uint8, OutputChannels)
	for range len(ans) / OutputChannels {
		n, _ := binary.Decode(raw, binary.BigEndian, temp)
		d(temp, ans)
		raw = raw[n:]
		ans = ans[OutputChannels:]
	}

}

func decode_clut_table16(raw []byte, OutputChannels int, output_colorspace ColorSpace, legacy bool, ans []unit_float) {
	var d clut_decoder_func16
	switch output_colorspace {
	case ColorSpaceXYZ:
		d = uniform_decoder16(u1Fixed15Number)
	case ColorSpaceLab:
		d = IfElse(legacy, lab_decoder16_legacy, lab_decoder16)
	default:
		d = uniform_decoder16(default16)
	}
	temp := make([]uint16, OutputChannels)
	for range len(ans) / OutputChannels {
		n, _ := binary.Decode(raw, binary.BigEndian, temp)
		d(temp, ans)
		raw = raw[n:]
		ans = ans[OutputChannels:]
	}
}

func decode_clut_table(raw []byte, bytes_per_channel, OutputChannels int, grid_points []int, output_colorspace ColorSpace, legacy bool) (ans []unit_float, consumed int, err error) {
	expected_num_of_output_channels := 3
	switch output_colorspace {
	case ColorSpaceCMYK:
		expected_num_of_output_channels = 4
	}
	if expected_num_of_output_channels != OutputChannels {
		return nil, 0, fmt.Errorf("CLUT table number of output channels %d inappropriate for output_colorspace: %s", OutputChannels, output_colorspace)
	}
	expected_num_of_values := expectedValues(grid_points, OutputChannels)
	consumed = bytes_per_channel * expected_num_of_values
	if len(raw) < consumed {
		return nil, 0, fmt.Errorf("CLUT table too short %d < %d", len(raw), bytes_per_channel*expected_num_of_values)
	}
	ans = make([]unit_float, expected_num_of_values)
	if bytes_per_channel == 1 {
		decode_clut_table8(raw[:consumed], OutputChannels, output_colorspace, ans)
	} else {
		decode_clut_table16(raw[:consumed], OutputChannels, output_colorspace, legacy, ans)
	}
	return
}

// section 10.12.3 (CLUT) in ICC.1-2202-05.pdf
func embeddedClutDecoder(raw []byte, InputChannels, OutputChannels int, output_colorspace ColorSpace) (any, error) {
	if len(raw) < 20 {
		return nil, errors.New("clut tag too short")
	}
	if InputChannels > 4 {
		return nil, fmt.Errorf("clut supports at most 4 input channels not: %d", InputChannels)
	}
	gridPoints := make([]int, InputChannels)
	for i, b := range raw[:InputChannels] {
		gridPoints[i] = int(b)
	}
	for i, nPoints := range gridPoints {
		if nPoints < 2 {
			return nil, fmt.Errorf("CLUT input channel %d has invalid grid points: %d", i, nPoints)
		}
	}
	bytes_per_channel := raw[16]
	raw = raw[20:]
	values, _, err := decode_clut_table(raw, int(bytes_per_channel), OutputChannels, gridPoints, output_colorspace, false)
	if err != nil {
		return nil, err
	}
	ans := &CLUTTag{
		GridPoints:     gridPoints,
		InputChannels:  InputChannels,
		OutputChannels: OutputChannels,
		Values:         values,
	}
	return ans, nil
}

func expectedValues(gridPoints []int, outputChannels int) int {
	expectedPoints := 1
	for _, g := range gridPoints {
		expectedPoints *= int(g)
	}
	return expectedPoints * outputChannels
}

func (c *CLUTTag) WorkspaceSize() int { return 0 }

func (c *CLUTTag) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return num_input_channels == int(c.InputChannels) && num_output_channels == c.OutputChannels && num_input_channels <= 6
}

// Lookup performs an n-linear interpolation on the CLUT for the given input color using an iterative method.
// Input values should be normalized between 0.0 and 1.0. Output MUST be zero initialized.
func (c *CLUTTag) Lookup(input, output []unit_float) {
	// Pre-allocate slices for indices and weights
	var buf [4]int
	var wbuf [4]unit_float
	indices := buf[:c.InputChannels]
	weights := wbuf[:c.InputChannels]
	input = input[:c.InputChannels]

	// Calculate the base indices and interpolation weights for each dimension.
	for i, val := range input {
		gp := c.GridPoints[i]
		val = clamp01(val)
		// Scale the value to the grid dimensions
		pos := val * unit_float(gp-1)
		// The base index is the floor of the position.
		idx := int(pos)
		// The weight is the fractional part of the position.
		weight := pos - unit_float(idx)
		// Clamp index to be at most the second to last grid point.
		if idx >= gp-1 {
			idx = gp - 2
			weight = 1 // set weight to 1 for border index
		}
		indices[i] = idx
		weights[i] = weight
	}
	// Iterate through all 2^InputChannels corners of the n-dimensional hypercube
	for i := range 1 << c.InputChannels {
		// Calculate the combined weight for this corner
		cornerWeight := unit_float(1)
		// Calculate the N-dimensional index to look up in the table
		tableIndex := 0
		multiplier := unit_float(1)

		for j := c.InputChannels - 1; j >= 0; j-- {
			// Check the j-th bit of i to decide if we are at the lower or upper bound for this dimension
			if (i>>j)&1 == 1 {
				// Upper bound for this dimension
				cornerWeight *= weights[j]
				tableIndex += int(unit_float(indices[j]+1) * multiplier)
			} else {
				// Lower bound for this dimension
				cornerWeight *= (1.0 - weights[j])
				tableIndex += int(unit_float(indices[j]) * multiplier)
			}
			multiplier *= unit_float(c.GridPoints[j])
		}
		// Get the color value from the table for the current corner
		offset := tableIndex * c.OutputChannels
		// Add the weighted corner color to the output
		for k, v := range c.Values[offset : offset+c.OutputChannels] {
			output[k] += v * cornerWeight
		}
	}
}

func (c *CLUTTag) Transform(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	var obuf [3]unit_float
	var ibuf = [3]unit_float{r, g, b}
	c.Lookup(ibuf[:], obuf[:])
	return obuf[0], obuf[1], obuf[2]
}

func clamp01(v unit_float) unit_float {
	return max(0, min(v, 1))
}
