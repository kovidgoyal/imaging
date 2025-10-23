package icc

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func encode_matrix_vals(values ...float64) string {
	var buf bytes.Buffer
	for _, v := range values {
		buf.Write(encodeS15Fixed16BE(v))
	}
	return buf.String()
}

func identity_matrix(offset1, offset2, offset3 float64) []float64 {
	return []float64{
		1.0, 0.0, 0.0,
		0.0, 1.0, 0.0,
		0.0, 0.0, 1.0,
		offset1, offset2, offset3,
	}

}

func TestMtxDecoder(t *testing.T) {
	t.Run("SuccessWithOffsets", func(t *testing.T) {
		var buf bytes.Buffer
		buf.WriteString("mtx ")       // Name
		buf.Write([]byte{0, 0, 0, 0}) // Reserved
		// Write 9 matrix values
		values := []float64{
			1.0, 0.0, 0.0,
			0.0, 1.0, 0.0,
			0.0, 0.0, 1.0,
		}
		for _, v := range values {
			buf.Write(encodeS15Fixed16BE(v))
		}
		// Write 3 offset values
		offsets := []float64{0.1, 0.2, 0.3}
		for _, v := range offsets {
			buf.Write(encodeS15Fixed16BE(v))
		}
		val, err := matrixDecoder(buf.Bytes())
		require.NoError(t, err)
		require.IsType(t, &MatrixWithOffset{}, val)
		mtx := val.(*MatrixWithOffset)
		// Check matrix values
		expectedMatrix := IdentityMatrix(0)
		assert.Equal(t, &expectedMatrix, mtx.m)
		// Check offset values
		assert.InDelta(t, 0.1, mtx.offset1, 0.0001)
		assert.InDelta(t, 0.2, mtx.offset2, 0.0001)
		assert.InDelta(t, 0.3, mtx.offset3, 0.0001)
	})
	t.Run("SuccessWithoutOffsets", func(t *testing.T) {
		var buf bytes.Buffer
		buf.WriteString("mtx ")       // Name
		buf.Write([]byte{0, 0, 0, 0}) // Reserved
		// Write only the 9 matrix values (no offsets)
		values := []float64{
			1.0, 2.0, 3.0,
			4.0, 5.0, 6.0,
			7.0, 8.0, 9.0,
		}
		for _, v := range values {
			buf.Write(encodeS15Fixed16BE(v))
		}
		val, err := matrixDecoder(buf.Bytes())
		require.NoError(t, err)
		require.IsType(t, &Matrix3{}, val)
		mtx := val.(*Matrix3)
		expectedMatrix := Matrix3{
			{1.0, 2.0, 3.0},
			{4.0, 5.0, 6.0},
			{7.0, 8.0, 9.0},
		}
		assert.Equal(t, &expectedMatrix, mtx)
	})
	t.Run("TooShort", func(t *testing.T) {
		data := make([]byte, 20)
		_, err := matrixDecoder(data)
		assert.ErrorContains(t, err, "mtx tag too short")
	})
}

func TestMatrixTag_Transform(t *testing.T) {
	output := make([]float64, 3)
	t.Run("SuccessWithoutOffset", func(t *testing.T) {
		matrix := &Matrix3{
			{1, 0, 0},
			{0, 1, 0},
			{0, 0, 1},
		}
		input := []float64{0.5, 0.25, 0.75}
		err := matrix.Transform(output, nil, input...)
		require.NoError(t, err)
		assert.InDeltaSlice(t, input, output, 0.0001)
	})
	t.Run("SuccessWithOffset", func(t *testing.T) {
		matrix := &MatrixWithOffset{
			m: &Matrix3{
				{1, 0, 0},
				{0, 1, 0},
				{0, 0, 1},
			},
			offset1: 0.1, offset2: 0.2, offset3: 0.3,
		}
		input := []float64{0.5, 0.25, 0.75}
		expected := []float64{0.6, 0.45, 1.05} // input + offset
		err := matrix.Transform(output, nil, input...)
		require.NoError(t, err)
		assert.InDeltaSlice(t, expected, output, 0.0001)
	})
	t.Run("MatrixApplied", func(t *testing.T) {
		matrix := &Matrix3{
			{2, 0, 0},
			{0, 3, 0},
			{0, 0, 4},
		}
		input := []float64{1, 1, 1}
		expected := []float64{2, 3, 4}
		err := matrix.Transform(output, nil, input...)
		require.NoError(t, err)
		assert.InDeltaSlice(t, expected, output, 0.0001)
	})
}
