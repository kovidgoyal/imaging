package icc

import (
	"errors"
)

// MatrixTag represents a matrix tag (TagMatrix)
type MatrixTag struct {
	Matrix [3][3]float64
	Offset *[3]float64 // offset is not always present
}

func is_identity_matrix(m [3][3]float64) bool {
	return m[0][0] == 1 && m[0][1] == 0 && m[0][2] == 0 && m[1][0] == 0 && m[1][1] == 1 && m[1][2] == 0 && m[2][0] == 0 && m[2][1] == 0 && m[2][2] == 1
}

func (c *MatrixTag) WorkspaceSize() int { return 0 }

func (c *MatrixTag) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return num_input_channels == 3 && num_output_channels == 3
}

var _ ChannelTransformer = (*MatrixTag)(nil)

func embeddedMatrixDecoder(body []byte) (any, error) {
	result := &MatrixTag{}
	for i := range 9 {
		result.Matrix[i/3][i%3] = readS15Fixed16BE(body[i*4 : (i+1)*4])
	}
	body = body[36:]
	if len(body) >= 3*4 {
		offset := [3]float64{}
		for i := range 3 {
			offset[i] = readS15Fixed16BE(body[i*4 : (i+1)*4])
		}
		result.Offset = &offset
	}
	return result, nil

}

func matrixDecoder(raw []byte) (any, error) {
	if len(raw) < 8+(9*4) {
		return nil, errors.New("mtx tag too short")
	}
	return embeddedMatrixDecoder(raw[8:])
}

func (m *MatrixTag) Transform(output, workspace []float64, inputs ...float64) error {
	for i := range 3 {
		output[i] = 0
	}
	for i := range 3 {
		for j := range 3 {
			output[i] += m.Matrix[i][j] * inputs[j]
		}
	}
	if m.Offset != nil {
		for i := range 3 {
			output[i] += m.Offset[i]
		}
	}
	return nil
}
