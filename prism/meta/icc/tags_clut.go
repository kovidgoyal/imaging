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
	if ans.InputChannels > 6 {
		return nil, fmt.Errorf("unsupported num of CLUT input channels: %d", ans.InputChannels)
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

func (c *CLUTTag) WorkspaceSize() int { return c.InputChannels }

func (c *CLUTTag) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return num_input_channels == int(c.InputChannels) && num_output_channels == c.OutputChannels && num_input_channels <= 6
}

func clut_trilinear_interpolate(input_channels int, grid_points []int, values []unit_float, out []unit_float, gridPos []int, gridFrac []unit_float) {
	numCorners := 1 << input_channels // 2^inputs
	// walk all corners of the hypercube
	for corner := range numCorners {
		weight := unit_float(1.0)
		idx := 0
		stride := 1
		for dim := input_channels - 1; dim >= 0; dim-- {
			bit := (corner >> dim) & 1
			pos := gridPos[dim] + bit
			idx += pos * stride
			stride *= grid_points[dim]
			if bit == 0 {
				weight *= 1 - gridFrac[dim]
			} else {
				weight *= gridFrac[dim]
			}
		}
		base := idx * len(out)
		for o := range len(out) {
			out[o] += weight * values[base+o]
		}
	}
}

func clut_trilinear_interpolate3(grid_points []int, values []unit_float, gridPos []int, gridFrac []unit_float) (or unit_float, og unit_float, ob unit_float) {
	const input_channels = 3
	numCorners := 1 << input_channels // 2^inputs
	// walk all corners of the hypercube
	for corner := range numCorners {
		weight := unit_float(1.0)
		idx := 0
		stride := 1
		for dim := input_channels - 1; dim >= 0; dim-- {
			bit := (corner >> dim) & 1
			pos := gridPos[dim] + bit
			idx += pos * stride
			stride *= grid_points[dim]
			if bit == 0 {
				weight *= 1 - gridFrac[dim]
			} else {
				weight *= gridFrac[dim]
			}
		}
		base := idx * input_channels
		or += weight * values[base]
		og += weight * values[base+1]
		ob += weight * values[base+2]
	}
	return
}

func clut_transform(input_channels, output_channels int, grid_points []int, values []unit_float, output, workspace []unit_float, inputs []unit_float) {
	gridFrac := workspace[0:input_channels]
	var buf [6]int
	gridPos := buf[:]
	for i, v := range inputs {
		nPoints := grid_points[i]
		pos := clamp01(v) * unit_float(nPoints-1)
		gridPos[i] = int(pos)
		if gridPos[i] >= nPoints-1 {
			gridPos[i] = nPoints - 2 // clamp
			gridFrac[i] = 1.0
		} else {
			gridFrac[i] = pos - unit_float(gridPos[i])
		}
	}
	for i := range output_channels {
		output[i] = 0
	}
	clut_trilinear_interpolate(input_channels, grid_points, values, output[:output_channels], gridPos, gridFrac)
}

func clut_transform3(grid_points []int, values []unit_float, workspace []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float) {
	const input_channels = 3
	gridFrac := workspace[0:input_channels]
	var buf [6]int
	var ibuf = [3]unit_float{r, g, b}
	gridPos := buf[:]
	for i, v := range ibuf {
		nPoints := grid_points[i]
		pos := clamp01(v) * unit_float(nPoints-1)
		gridPos[i] = int(pos)
		if gridPos[i] >= nPoints-1 {
			gridPos[i] = nPoints - 2 // clamp
			gridFrac[i] = 1.0
		} else {
			gridFrac[i] = pos - unit_float(gridPos[i])
		}
	}
	return clut_trilinear_interpolate3(grid_points, values, gridPos, gridFrac)
}

func (c *CLUTTag) Transform(workspace []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float) {
	return clut_transform3(c.GridPoints, c.Values, workspace, r, g, b)
}

func clamp01(v unit_float) unit_float {
	return max(0, min(v, 1))
}
