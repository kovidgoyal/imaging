package icc

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func join_curves(curves ...[]byte) []byte {
	return bytes.Join(curves, nil)
}

func encode_modular(t *testing.T, a_to_b bool, num_inp, num_outp int, tags map[string][]byte) []byte {
	var buf bytes.Buffer
	sig := IfElse(a_to_b, "mAB ", "mBA ")
	buf.WriteString(sig + "\x00\x00\x00\x00")
	buf.WriteByte(uint8(num_inp))
	buf.WriteByte(uint8(num_outp))
	buf.WriteString("\x00\x00")
	offset_table_size := 5 * 4
	b_off := buf.Len() + offset_table_size
	b_curves := tags["b"]
	matrix_off := b_off + len(b_curves)
	matrix := tags["matrix"]
	m_off := matrix_off + len(matrix)
	m_curves := tags["m"]
	clut_off := m_off + len(m_curves)
	clut := tags["clut"]
	a_off := clut_off + len(clut)
	a_curves := tags["a"]
	if b_curves == nil {
		b_off = 0
	}
	if matrix == nil {
		matrix_off = 0
	}
	if m_curves == nil {
		m_off = 0
	}
	if clut == nil {
		clut_off = 0
	}
	if a_curves == nil {
		a_off = 0
	}
	binary.Write(&buf, binary.BigEndian, []uint32{
		uint32(b_off), uint32(matrix_off), uint32(m_off), uint32(clut_off), uint32(a_off)})
	assert.Equal(t, b_off, IfElse(b_curves == nil, 0, buf.Len()))
	buf.Write(b_curves)
	assert.Equal(t, matrix_off, IfElse(matrix == nil, 0, buf.Len()))
	buf.Write(matrix)
	assert.Equal(t, m_off, IfElse(m_curves == nil, 0, buf.Len()))
	buf.Write(m_curves)
	assert.Equal(t, clut_off, IfElse(clut == nil, 0, buf.Len()))
	buf.Write(clut)
	assert.Equal(t, a_off, IfElse(a_curves == nil, 0, buf.Len()))
	buf.Write(a_curves)
	return buf.Bytes()
}

func TestModularDecoder(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		b := encode_modular(t, true, 3, 3, map[string][]byte{
			"b":      join_curves(curv_bytes(), curv_bytes(1.0), curv_bytes(0.1, 0.2, 0.3)),
			"matrix": []byte(encode_matrix_vals(identity_matrix(0.1, 0.2, 0.3)...)),
			"m":      join_curves(curv_bytes(2.0), curv_bytes(3.0), curv_bytes(0.1, 0.2, 0.3)),
			"clut":   encode_clut16bit(),
			"a":      join_curves(curv_bytes(3.0), curv_bytes(4.0), curv_bytes(0.1, 0.2, 0.3)),
		})
		val, err := modularDecoder(b)
		require.NoError(t, err)
		require.IsType(t, &ModularTag{}, val)
		tag := val.(*ModularTag)
		assert.True(t, tag.is_a_to_b)
		assert.Equal(t, 3, tag.num_input_channels)
		assert.Equal(t, 3, tag.num_output_channels)
		assert.Len(t, tag.transforms, 5)
	})
	t.Run("TooShort", func(t *testing.T) {
		data := []byte{1, 2, 3}
		_, err := modularDecoder(data)
		assert.ErrorContains(t, err, "modular (mAB/mBA) tag too short")
	})
}

func TestModularTag_transformChannels(t *testing.T) {
}
