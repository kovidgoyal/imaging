package icc

import (
	"errors"
	"fmt"
	"math"
)

// CLUTTag represents a color lookup table tag (TagColorLookupTable)
type CLUTTag struct {
	d      *interpolation_data
	legacy bool
}

type CLUT3D struct {
	d      *interpolation_data
	legacy bool
}

type CLUT interface {
	ChannelTransformer
	Samples() []unit_float
}

func (c *CLUTTag) Samples() []unit_float { return c.d.samples }
func (c *CLUT3D) Samples() []unit_float  { return c.d.samples }

func (c CLUT3D) String() string {
	return fmt.Sprintf("CLUT3D{ outp:%v grid:%v values[:9]:%v }", c.d.num_outputs, c.d.grid_points, c.d.samples[:min(9, len(c.d.samples))])
}

func (c CLUTTag) String() string {
	return fmt.Sprintf("CLUTTag{ inp:%v outp:%v grid:%v values[:9]:%v }", c.d.num_inputs, c.d.num_outputs, c.d.grid_points, c.d.samples[:min(9, len(c.d.samples))])
}

var _ CLUT = (*CLUTTag)(nil)
var _ CLUT = (*CLUT3D)(nil)

func decode_clut_table8(raw []byte, ans []unit_float) {
	for i, x := range raw {
		ans[i] = unit_float(x) / math.MaxUint8
	}
}

func decode_clut_table16(raw []byte, ans []unit_float) {
	for i := range ans {
		ans[i] = unit_float((uint16(raw[0])<<8)|uint16(raw[1])) / math.MaxUint16
		raw = raw[2:]
	}
}

func decode_clut_table(raw []byte, bytes_per_channel, OutputChannels int, grid_points []int, output_colorspace ColorSpace) (ans []unit_float, consumed int, err error) {
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
		decode_clut_table8(raw[:consumed], ans)
	} else {
		decode_clut_table16(raw[:consumed], ans)
	}
	return
}

func make_clut(grid_points []int, num_inputs, num_outputs int, samples []unit_float, legacy bool) CLUT {
	if num_inputs == 3 {
		return &CLUT3D{make_interpolation_data(num_inputs, num_outputs, grid_points, samples), legacy}
	}
	return &CLUTTag{make_interpolation_data(num_inputs, num_outputs, grid_points, samples), legacy}
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
	values, _, err := decode_clut_table(raw, int(bytes_per_channel), OutputChannels, gridPoints, output_colorspace)
	if err != nil {
		return nil, err
	}
	return make_clut(gridPoints, InputChannels, OutputChannels, values, false), nil
}

func expectedValues(gridPoints []int, outputChannels int) int {
	expectedPoints := 1
	for _, g := range gridPoints {
		expectedPoints *= int(g)
	}
	return expectedPoints * outputChannels
}

func (c *CLUTTag) IOSig() (int, int)                    { return c.d.num_inputs, c.d.num_outputs }
func (c *CLUT3D) IOSig() (int, int)                     { return 3, c.d.num_outputs }
func (c *CLUTTag) Iter(f func(ChannelTransformer) bool) { f(c) }
func (c *CLUT3D) Iter(f func(ChannelTransformer) bool)  { f(c) }

func tgg(num_in, num_out int, f func(inbuf, outbuf []unit_float), o, i []unit_float) {
	limit := len(i) / num_in
	_ = o[num_out*limit-1]
	for range limit {
		f(i[0:num_in:num_in], o[0:num_out:num_out])
		i, o = i[num_in:], o[num_out:]
	}
}

func (c *CLUTTag) Transform(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	var obuf [3]unit_float
	var ibuf = [3]unit_float{r, g, b}
	c.d.trilinear_interpolate(ibuf[:], obuf[:])
	return obuf[0], obuf[1], obuf[2]
}
func (m *CLUTTag) TransformGeneral(o, i []unit_float) {
	tgg(m.d.num_inputs, m.d.num_outputs, m.d.trilinear_interpolate, o, i)
}

func (c *CLUT3D) Trilinear_interpolate(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	var obuf [3]unit_float
	var ibuf = [3]unit_float{r, g, b}
	c.d.trilinear_interpolate(ibuf[:], obuf[:])
	return obuf[0], obuf[1], obuf[2]
}

func (c *CLUT3D) Tetrahedral_interpolate(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	var obuf [3]unit_float
	c.d.tetrahedral_interpolation(r, g, b, obuf[:])
	return obuf[0], obuf[1], obuf[2]
}

func (c *CLUT3D) Tetrahedral_interpolate_g(i, o []unit_float) {
	c.d.tetrahedral_interpolation(i[0], i[1], i[2], o)
}

func (c *CLUT3D) Transform(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	return c.Tetrahedral_interpolate(r, g, b)
}
func (m *CLUT3D) TransformGeneral(o, i []unit_float) {
	tgg(m.d.num_inputs, m.d.num_outputs, m.Tetrahedral_interpolate_g, o, i)
}

func clamp01(v unit_float) unit_float {
	return max(0, min(v, 1))
}
