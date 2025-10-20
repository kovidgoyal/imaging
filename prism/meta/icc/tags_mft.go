package icc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

var _ = fmt.Print

type MFT struct {
	in_channels, out_channels   int
	grid_points                 []int
	input_curves, output_curves [][]float64
	clut                        []float64
	matrix                      [3][3]float64
	matrix_is_identity          bool
}

func (c *MFT) WorkspaceSize() int { return c.in_channels * 2 }

func (c *MFT) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return num_input_channels == int(c.in_channels) && num_output_channels == c.out_channels && num_input_channels <= 6
}

var _ ChannelTransformer = (*MFT)(nil)

func linear_interpolate_1d(val float64, table []float64) float64 {
	val = clamp01(val)
	pos := val * float64(len(table)-1)
	lof := math.Trunc(pos)
	lo := int(lof)
	if lof == pos {
		return table[lo]
	}
	frac := pos - lof
	vlo := table[lo]
	vhi := table[lo+1]
	return vlo + frac*(vhi-vlo)
}

func apply_matrix(m *[3][3]float64, output, input []float64, is_identity_matrix bool) {
	if is_identity_matrix {
		copy(output, input)
	} else {
		for i := range 3 {
			output[i] = 0
			for j := range 3 {
				output[i] += m[i][j] * input[j]
			}
		}
	}
}

func (mft *MFT) Transform(output, workspace []float64, inputs ...float64) error {
	mapped := workspace[0:mft.in_channels]
	workspace = workspace[mft.in_channels:]
	for o := range mft.out_channels {
		output[o] = 0
	}
	// Apply matrix
	apply_matrix(&mft.matrix, output, mapped, mft.matrix_is_identity)
	// Apply input curves with linear interpolation
	for i := range mft.in_channels {
		mapped[i] = linear_interpolate_1d(mapped[i], mft.input_curves[i])
	}
	// Apply CLUT
	clut_transform(mft.in_channels, mft.out_channels, mft.grid_points, mft.clut, output, workspace, mapped)
	// Apply output curves with interpolation
	for i := range mft.out_channels {
		output[i] = linear_interpolate_1d(output[i], mft.output_curves[i])
	}
	return nil
}

func load_8bit_table(raw []byte, n int) (output []float64, leftover []byte, err error) {
	if len(raw) < n {
		return nil, raw, fmt.Errorf("mft2 tag too short")
	}
	output = make([]float64, n)
	for i := range n {
		output[i] = float64(raw[0]) / 255
		raw = raw[1:]
	}
	return output, raw, nil
}

func load_16bit_table(raw []byte, n int) (output []float64, leftover []byte, err error) {
	if len(raw) < 2*n {
		return nil, raw, fmt.Errorf("mft2 tag too short")
	}
	output = make([]float64, n)
	for i := range n {
		output[i] = float64(binary.BigEndian.Uint16(raw[:2])) / 65535
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
	ma, _ := embeddedMatrixDecoder(raw[12:48])
	a.matrix = ma.(MatrixTag).Matrix
	a.matrix_is_identity = is_identity_matrix(a.matrix)
	return &a, raw[48:], nil
}

func load_mft_body(a *MFT, raw []byte, load_table func([]byte, int) ([]float64, []byte, error), input_table_entries, output_table_entries int) (err error) {
	a.input_curves = make([][]float64, a.in_channels)
	a.output_curves = make([][]float64, a.out_channels)
	for i := range a.in_channels {
		if a.input_curves[i], raw, err = load_table(raw, input_table_entries); err != nil {
			return err
		}
	}
	num_clut := expectedValues(a.grid_points, a.out_channels)
	if a.clut, raw, err = load_table(raw, num_clut); err != nil {
		return err
	}
	for i := range a.out_channels {
		if a.output_curves[i], raw, err = load_table(raw, output_table_entries); err != nil {
			return err
		}
	}
	return nil
}

func decode_mft8(raw []byte) (ans any, err error) {
	var a *MFT
	if a, raw, err = load_mft_header(raw); err != nil {
		return nil, err
	}
	err = load_mft_body(a, raw, load_8bit_table, 256, 256)
	return &a, err
}

func decode_mft16(raw []byte) (ans any, err error) {
	var a *MFT
	if a, raw, err = load_mft_header(raw); err != nil {
		return nil, err
	}
	input_table_entries, output_table_entries := binary.BigEndian.Uint16(raw[:2]), binary.BigEndian.Uint16(raw[2:4])
	err = load_mft_body(a, raw[4:], load_16bit_table, int(input_table_entries), int(output_table_entries))
	return &a, err
}
