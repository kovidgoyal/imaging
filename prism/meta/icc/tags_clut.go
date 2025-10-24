package icc

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// CLUTTag represents a color lookup table tag (TagColorLookupTable)
type CLUTTag struct {
	GridPoints     []int // e.g., [17,17,17] for 3D CLUT
	InputChannels  int
	OutputChannels int
	Values         []unit_float // flattened [in1, in2, ..., out1, out2, ...]
}

func (c CLUTTag) String() string {
	return fmt.Sprintf("CLUTTag{ %v num_values:%v }", c.GridPoints, len(c.Values))
}

func make_clut(grid_points []int, inp, outp int, values []unit_float) *CLUTTag {
	return &CLUTTag{grid_points, inp, outp, values}
}

var _ ChannelTransformer = (*CLUTTag)(nil)

// section 10.12.3 (CLUT) in ICC.1-2202-05.pdf
func embeddedClutDecoder(raw []byte, InputChannels, OutputChannels int) (any, error) {
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
	// expected size: (product of grid points) * output channels * bytes_per_channel
	expected_num_of_values := expectedValues(gridPoints, OutputChannels)
	values := make([]unit_float, expected_num_of_values)
	if len(values)*int(bytes_per_channel) > len(raw) {
		return nil, fmt.Errorf("CLUT unexpected body length: expected %d, got %d", expected_num_of_values*int(bytes_per_channel), len(raw))
	}

	switch bytes_per_channel {
	case 1:
		for i, b := range raw[:len(values)] {
			values[i] = unit_float(b) / 255
		}
	case 2:
		for i := range len(values) {
			values[i] = unit_float(binary.BigEndian.Uint16(raw[i*2:i*2+2])) / 65535
		}
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
// Input values should be normalized between 0.0 and 1.0.
func (c *CLUTTag) Lookup(input, workspace, output []unit_float) {
	// output MUST be zero initialized
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
	// Initialize the final output color array
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

func (c *CLUTTag) Transform(workspace []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float) {
	var obuf [3]unit_float
	var ibuf = [3]unit_float{r, g, b}
	c.Lookup(ibuf[:], workspace, obuf[:])
	return obuf[0], obuf[1], obuf[2]
}

func clamp01(v unit_float) unit_float {
	return max(0, min(v, 1))
}
