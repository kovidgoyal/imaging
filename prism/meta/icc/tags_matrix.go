package icc

import (
	"errors"
	"fmt"
	"math"
)

type Matrix3 [3][3]unit_float
type IdentityMatrix int

type MatrixWithOffset struct {
	m                         ChannelTransformer
	offset1, offset2, offset3 unit_float
}

func is_identity_matrix(m *Matrix3) bool {
	for r := range 3 {
		for c := range 3 {
			q := IfElse(r == c, unit_float(1), unit_float(0))
			if math.Abs(float64(m[r][c]-q)) > FLOAT_EQUALITY_THRESHOLD {
				return false
			}
		}
	}
	return true
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

func (c IdentityMatrix) String() string { return "IdentityMatrix" }

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

func (m *Matrix3) Transform(workspace []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float) {
	return m[0][0]*r + m[0][1]*g + m[0][2]*b, m[1][0]*r + m[1][1]*g + m[1][2]*b, m[2][0]*r + m[2][1]*g + m[2][2]*b
}

func (m Matrix3) Transpose() Matrix3 {
	return Matrix3{
		{m[0][0], m[1][0], m[2][0]},
		{m[0][1], m[1][1], m[2][1]},
		{m[0][2], m[1][2], m[2][2]},
	}
}

func Dot(v1, v2 [3]unit_float) unit_float {
	return v1[0]*v2[0] + v1[1]*v2[1] + v1[2]*v2[2]
}

func (m Matrix3) String() string {
	return fmt.Sprintf("Matrix3{ %v, %v, %v }", m[0], m[1], m[2])
}

// Return m * o
func (m Matrix3) Multiply(o Matrix3) Matrix3 {
	t := o.Transpose()
	return Matrix3{
		{Dot(t[0], m[0]), Dot(t[1], m[0]), Dot(t[2], m[0])},
		{Dot(t[0], m[1]), Dot(t[1], m[1]), Dot(t[2], m[1])},
		{Dot(t[0], m[2]), Dot(t[1], m[2]), Dot(t[2], m[2])},
	}
}

func (m Matrix3) Inverted() (ans Matrix3, err error) {
	o := Matrix3{
		{
			m[1][1]*m[2][2] - m[2][1]*m[1][2],
			-(m[0][1]*m[2][2] - m[2][1]*m[0][2]),
			m[0][1]*m[1][2] - m[1][1]*m[0][2],
		},
		{
			-(m[1][0]*m[2][2] - m[2][0]*m[1][2]),
			m[0][0]*m[2][2] - m[2][0]*m[0][2],
			-(m[0][0]*m[1][2] - m[1][0]*m[0][2]),
		},
		{
			m[1][0]*m[2][1] - m[2][0]*m[1][1],
			-(m[0][0]*m[2][1] - m[2][0]*m[0][1]),
			m[0][0]*m[1][1] - m[1][0]*m[0][1],
		},
	}

	det := m[0][0]*o[0][0] + m[1][0]*o[0][1] + m[2][0]*o[0][2]
	if abs(det) < FLOAT_EQUALITY_THRESHOLD {
		return ans, fmt.Errorf("matrix is singular and cannot be inverted, det=%v", det)
	}
	det = 1 / det

	o[0][0] *= det
	o[0][1] *= det
	o[0][2] *= det
	o[1][0] *= det
	o[1][1] *= det
	o[1][2] *= det
	o[2][0] *= det
	o[2][1] *= det
	o[2][2] *= det
	return o, nil
}

func (m IdentityMatrix) Transform(workspace []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float) {
	return r, g, b
}

func (m *MatrixWithOffset) Transform(workspace []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float) {
	r, g, b = m.m.Transform(workspace, r, g, b)
	r += m.offset1
	g += m.offset2
	b += m.offset3
	return r, g, b
}
