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

func decode_mft8(raw []byte) (ans any, err error) {
	if len(raw) < 48 {
		return nil, errors.New("mft tag too short")
	}
	a := MFT{}
	var grid_points int
	a.in_channels, a.out_channels, grid_points = int(raw[8]), int(raw[9]), int(raw[10])
	if grid_points < 2 {
		return nil, fmt.Errorf("mft tag has invalid number of CLUT grid points: %d", a.grid_points)
	}
	a.grid_points = make([]int, a.in_channels)
	for i := range a.in_channels {
		a.grid_points[i] = grid_points
	}
	ma, _ := embeddedMatrixDecoder(raw[12:48])
	a.matrix = ma.(MatrixTag).Matrix
	a.matrix_is_identity = is_identity_matrix(a.matrix)
	raw = raw[48:]
	a.input_curves = make([][]float64, a.in_channels)
	t := make([]uint8, 256)
	for i := range a.in_channels {
		n, err := binary.Decode(raw, binary.BigEndian, t)
		if err != nil {
			return nil, err
		}
		raw = raw[n:]
		a.input_curves[i] = make([]float64, len(t))
		for e, x := range t {
			a.input_curves[i][e] = float64(x) / 255
		}
	}
	num_clut := grid_points * a.in_channels * a.out_channels
	a.clut = make([]float64, num_clut)
	if len(raw) < num_clut {
		return nil, errors.New("mft tag too short")
	}
	for i := range num_clut {
		a.clut[i] = float64(raw[i]) / 255
	}
	raw = raw[num_clut:]
	a.output_curves = make([][]float64, a.out_channels)
	for i := range a.out_channels {
		n, err := binary.Decode(raw, binary.BigEndian, t)
		if err != nil {
			return nil, err
		}
		raw = raw[n:]
		a.output_curves[i] = make([]float64, len(t))
		for e, x := range t {
			a.output_curves[i][e] = float64(x) / 255
		}
	}
	return &a, nil
}

func decode_mft16(raw []byte) (ans any, err error) {
	if len(raw) < 52 {
		return nil, errors.New("mft2 tag too short")
	}
	a := MFT{}
	var grid_points int
	a.in_channels, a.out_channels, grid_points = int(raw[8]), int(raw[9]), int(raw[10])
	if grid_points < 2 {
		return nil, fmt.Errorf("mft2 tag has invalid number of CLUT grid points: %d", a.grid_points)
	}
	a.grid_points = make([]int, a.in_channels)
	for i := range a.in_channels {
		a.grid_points[i] = grid_points
	}
	ma, _ := embeddedMatrixDecoder(raw[12:48])
	a.matrix = ma.(MatrixTag).Matrix
	a.matrix_is_identity = is_identity_matrix(a.matrix)
	input_table_entries, output_table_entries := binary.BigEndian.Uint16(raw[48:50]), binary.BigEndian.Uint16(raw[50:52])
	raw = raw[52:]
	a.input_curves = make([][]float64, a.in_channels)
	table := make([]uint16, input_table_entries)
	for i := range a.in_channels {
		n, err := binary.Decode(raw, binary.BigEndian, table)
		if err != nil {
			return nil, err
		}
		raw = raw[n:]
		a.input_curves[i] = make([]float64, len(table))
		for e, cval := range table {
			a.input_curves[i][e] = float64(cval) / 65535
		}
	}

	num_clut := int(math.Pow(float64(grid_points), float64(a.in_channels))) * a.out_channels
	if len(raw) < 2*num_clut {
		return nil, errors.New("mft2 tag too short")
	}
	a.clut = make([]float64, num_clut)
	for i := range num_clut {
		a.clut[i] = float64(binary.BigEndian.Uint16(raw[:2])) / 65535
		raw = raw[2:]
	}
	a.output_curves = make([][]float64, a.out_channels)
	table = make([]uint16, output_table_entries)
	for i := range a.in_channels {
		n, err := binary.Decode(raw, binary.BigEndian, table)
		if err != nil {
			return nil, err
		}
		raw = raw[n:]
		a.input_curves[i] = make([]float64, len(table))
		for e, cval := range table {
			a.output_curves[i][e] = float64(cval) / 65535
		}
	}
	return &a, nil
}
