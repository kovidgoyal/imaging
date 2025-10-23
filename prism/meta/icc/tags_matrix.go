package icc

import (
	"errors"
	"fmt"
)

type Matrix3 [3][3]float64
type IdentityMatrix int

type MatrixWithOffset struct {
	m                         ChannelTransformer
	offset1, offset2, offset3 float64
}

func is_identity_matrix(m *Matrix3) bool {
	return m[0][0] == 1 && m[0][1] == 0 && m[0][2] == 0 && m[1][0] == 0 && m[1][1] == 1 && m[1][2] == 0 && m[2][0] == 0 && m[2][1] == 0 && m[2][2] == 1
}

func (c *Matrix3) WorkspaceSize() int { return 0 }

func (c *Matrix3) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return num_input_channels == 3 && num_output_channels == 3
}

func (c *MatrixWithOffset) WorkspaceSize() int { return 0 }

func (c *MatrixWithOffset) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return num_input_channels == 3 && num_output_channels == 3
}

func (c *IdentityMatrix) WorkspaceSize() int { return 0 }

func (c *IdentityMatrix) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return num_input_channels == 3 && num_output_channels == 3
}

var _ ChannelTransformer = (*MatrixWithOffset)(nil)

func embeddedMatrixDecoder(body []byte) (any, error) {
	result := Matrix3{}
	var m ChannelTransformer = &result
	for i := range 9 {
		result[i/3][i%3] = readS15Fixed16BE(body[i*4 : (i+1)*4])
	}
	if is_identity_matrix(&result) {
		t := IdentityMatrix(0)
		m = &t
	}
	body = body[36:]
	if len(body) < 3*4 {
		return m, nil
	}
	r2 := &MatrixWithOffset{m: m}
	if len(body) >= 3*4 {
		r2.offset1 = readS15Fixed16BE(body[:4])
		r2.offset2 = readS15Fixed16BE(body[4:8])
		r2.offset3 = readS15Fixed16BE(body[8:12])
	}
	return r2, nil

}

func matrixDecoder(raw []byte) (any, error) {
	if len(raw) < 8+(9*4) {
		return nil, errors.New("mtx tag too short")
	}
	return embeddedMatrixDecoder(raw[8:])
}

func (m *Matrix3) Transform(output, workspace []float64, inputs ...float64) error {
	output[0] = m[0][0]*inputs[0] + m[0][1]*inputs[1] + m[0][2]*inputs[2]
	output[1] = m[1][0]*inputs[0] + m[1][1]*inputs[1] + m[1][2]*inputs[2]
	output[2] = m[2][0]*inputs[0] + m[2][1]*inputs[1] + m[2][2]*inputs[2]
	return nil
}

func (mat *Matrix3) Inverted() (ans Matrix3, err error) {
	det := mat[0][0]*(mat[1][1]*mat[2][2]-mat[1][2]*mat[2][1]) -
		mat[0][1]*(mat[1][0]*mat[2][2]-mat[1][2]*mat[2][0]) +
		mat[0][2]*(mat[1][0]*mat[2][1]-mat[1][1]*mat[2][0])

	if det == 0 {
		return ans, fmt.Errorf("matrix is singular and cannot be inverted")
	}
	invDet := 1 / det
	adj := Matrix3{
		{
			(mat[1][1]*mat[2][2] - mat[1][2]*mat[2][1]),
			(mat[0][2]*mat[2][1] - mat[0][1]*mat[2][2]), // Note the sign change for cofactor C12
			(mat[0][1]*mat[1][2] - mat[0][2]*mat[1][1]), // Note the sign change for cofactor C13
		},
		{
			(mat[1][2]*mat[2][0] - mat[1][0]*mat[2][2]),
			(mat[0][0]*mat[2][2] - mat[0][2]*mat[2][0]),
			(mat[0][2]*mat[1][0] - mat[0][0]*mat[1][2]),
		},
		{
			(mat[1][0]*mat[2][1] - mat[1][1]*mat[2][0]),
			(mat[0][1]*mat[2][0] - mat[0][0]*mat[2][1]),
			(mat[0][0]*mat[1][1] - mat[0][1]*mat[1][0]),
		},
	}
	for i := range 3 {
		for j := range 3 {
			ans[i][j] = invDet * adj[i][j]
		}
	}
	return
}

func (m IdentityMatrix) Transform(output, workspace []float64, inputs ...float64) error {
	copy(output, inputs)
	return nil
}

func (m *MatrixWithOffset) Transform(output, workspace []float64, inputs ...float64) error {
	m.m.Transform(output, nil, inputs...)
	output[0] += m.offset1
	output[1] += m.offset2
	output[2] += m.offset3
	return nil
}
