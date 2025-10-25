package icc

import (
	"encoding/binary"
	"errors"
	"fmt"
)

var _ = fmt.Print

type MFT struct {
	in_channels, out_channels   int
	grid_points                 []int
	input_curves, output_curves []Curve1D
	clut                        CLUT
	matrix                      ChannelTransformer
	is8bit                      bool
}

func (c MFT) String() string {
	return fmt.Sprintf("MFT{grid_points:%v, matrix:%v input:%v, clut:%v, output:%v }", c.grid_points, c.matrix, c.input_curves, c.clut, c.output_curves)
}

func (c *MFT) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return num_input_channels == int(c.in_channels) && num_output_channels == c.out_channels && num_input_channels <= 6
}

var _ ChannelTransformer = (*MFT)(nil)

func (mft *MFT) Transform(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	// Apply matrix
	r, g, b = mft.matrix.Transform(r, g, b)
	// Apply input curves with linear interpolation
	r = mft.input_curves[0].Transform(r)
	g = mft.input_curves[1].Transform(g)
	b = mft.input_curves[2].Transform(b)
	// Apply CLUT
	r, g, b = mft.clut.Transform(r, g, b)
	// Apply output curves with interpolation
	r = mft.output_curves[0].Transform(r)
	g = mft.output_curves[1].Transform(g)
	b = mft.output_curves[2].Transform(b)
	return r, g, b
}

func load_8bit_table(raw []byte, n int) (output []unit_float, leftover []byte, err error) {
	if len(raw) < n {
		return nil, raw, fmt.Errorf("mft2 tag too short")
	}
	output = make([]unit_float, n)
	for i := range n {
		output[i] = unit_float(raw[0]) / 255
		raw = raw[1:]
	}
	return output, raw, nil
}

func load_16bit_table(raw []byte, n int) (output []unit_float, leftover []byte, err error) {
	if len(raw) < 2*n {
		return nil, raw, fmt.Errorf("mft2 tag too short")
	}
	output = make([]unit_float, n)
	for i := range n {
		output[i] = unit_float(binary.BigEndian.Uint16(raw[:2])) / 65535
		raw = raw[2:]
	}
	return output, raw, nil
}

func load_mft_header(raw []byte) (ans *MFT, leftover []byte, err error) {
	if len(raw) < 48 {
		return nil, raw, errors.New("mft tag too short")
	}
	a := MFT{}
	var grid_points int
	a.in_channels, a.out_channels, grid_points = int(raw[8]), int(raw[9]), int(raw[10])
	if grid_points < 2 {
		return nil, raw, fmt.Errorf("mft tag has invalid number of CLUT grid points: %d", a.grid_points)
	}
	a.grid_points = make([]int, a.in_channels)
	for i := range a.in_channels {
		a.grid_points[i] = grid_points
	}
	ma, err := embeddedMatrixDecoder(raw[12:48])
	if err != nil {
		return nil, nil, err
	}
	a.matrix = ma.(ChannelTransformer)
	return &a, raw[48:], nil
}

func load_mft_body(a *MFT, raw []byte, load_table func([]byte, int) ([]unit_float, []byte, error), input_table_entries, output_table_entries int, output_colorspace ColorSpace, bytes_per_channel int) (err error) {
	a.input_curves = make([]Curve1D, a.in_channels)
	a.output_curves = make([]Curve1D, a.out_channels)
	var fp []unit_float
	for i := range a.in_channels {
		if fp, raw, err = load_table(raw, input_table_entries); err != nil {
			return err
		}
		if a.input_curves[i], err = load_points_curve(fp); err != nil {
			return err
		}
	}
	fp, consumed, err := decode_clut_table(raw, int(bytes_per_channel), a.out_channels, a.grid_points, output_colorspace, true)
	if err != nil {
		return err
	}
	raw = raw[consumed:]
	a.clut = make_clut(a.grid_points, a.in_channels, a.out_channels, fp)
	for i := range a.out_channels {
		if fp, raw, err = load_table(raw, output_table_entries); err != nil {
			return err
		}
		if a.output_curves[i], err = load_points_curve(fp); err != nil {
			return err
		}
	}
	return nil
}

func decode_mft8(raw []byte, input_colorspace, output_colorspace ColorSpace) (ans any, err error) {
	var a *MFT
	if a, raw, err = load_mft_header(raw); err != nil {
		return nil, err
	}
	err = load_mft_body(a, raw, load_8bit_table, 256, 256, output_colorspace, 1)
	a.is8bit = true
	return a, err
}

func decode_mft16(raw []byte, input_colorspace, output_colorspace ColorSpace) (ans any, err error) {
	var a *MFT
	if a, raw, err = load_mft_header(raw); err != nil {
		return nil, err
	}
	input_table_entries, output_table_entries := binary.BigEndian.Uint16(raw[:2]), binary.BigEndian.Uint16(raw[2:4])
	err = load_mft_body(a, raw[4:], load_16bit_table, int(input_table_entries), int(output_table_entries), output_colorspace, 2)
	return a, err
}
