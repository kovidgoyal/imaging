package icc

import (
	"encoding/binary"
	"github.com/stretchr/testify/assert"
	"testing"
)

func encodeS15Fixed16BE(value unit_float) []byte {
	if value > 32767 {
		value = 32767
	} else if value < -32768 {
		value = -32768
	}
	intPart := int16(value)
	fracPart := uint16((value - unit_float(intPart)) * 65536.0)
	result := make([]byte, 4)
	binary.BigEndian.PutUint16(result[0:2], uint16(intPart))
	binary.BigEndian.PutUint16(result[2:4], fracPart)
	return result
}

func TestReadS15Fixed16BE(t *testing.T) {
	t.Run("PositiveWhole", func(t *testing.T) {
		val := readS15Fixed16BE([]byte{0x00, 0x01, 0x00, 0x00}) // 1.0
		in_delta(t, 1.0, val, 0.0001)
	})
	t.Run("PositiveFraction", func(t *testing.T) {
		val := readS15Fixed16BE([]byte{0x00, 0x02, 0x80, 0x00}) // 2.5
		in_delta(t, 2.5, val, 0.0001)
	})
	t.Run("NegativeWhole", func(t *testing.T) {
		val := readS15Fixed16BE([]byte{0xFF, 0xFF, 0x00, 0x00}) // -1.0
		in_delta(t, -1.0, val, 0.0001)
	})
	t.Run("NegativeFraction", func(t *testing.T) {
		val := readS15Fixed16BE([]byte{0xFF, 0xFE, 0x80, 0x00}) // -1.5
		in_delta(t, -1.5, val, 0.0001)
	})
	t.Run("Zero", func(t *testing.T) {
		val := readS15Fixed16BE([]byte{0x00, 0x00, 0x00, 0x00})
		assert.Equal(t, unit_float(0), val)
	})
}
