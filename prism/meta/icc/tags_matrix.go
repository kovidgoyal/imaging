package icc

import (
	"errors"
)

// MatrixTag represents a matrix tag (TagMatrix)
type MatrixTag struct {
	Matrix [3][3]float64
	Offset *[3]float64 // offset is not always present
}

func (c *MatrixTag) WorkspaceSize() int { return 0 }

func (c *MatrixTag) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return num_input_channels == 3 && num_output_channels == 3
}

var _ ChannelTransformer = (*MatrixTag)(nil)

func matrixDecoder(raw []byte) (any, error) {
	const (
		minLength     = 8 + (9 * 4) // 8 bytes for type/reserved + 9 * 4-byte matrix numbers
		offsetsLength = 12 * 4      // 4 * matrix numbers (4-byte) + 3 * offset numbers
	)
	if len(raw) < minLength {
		return nil, errors.New("mtx tag too short")
	}
	result := &MatrixTag{}
	body := raw[8:]
	for i := range 9 {
		result.Matrix[i/3][i%3] = readS15Fixed16BE(body[i*4 : (i+1)*4])
	}
	if len(body) >= offsetsLength {
		offset := [3]float64{}
		for i := range 3 {
			offset[i] = readS15Fixed16BE(body[36+i*4 : 36+(i+1)*4])
		}
		result.Offset = &offset
	}
	return result, nil
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
