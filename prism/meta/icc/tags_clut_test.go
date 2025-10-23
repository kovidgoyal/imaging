package icc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Println

func in_delta(t *testing.T, expected, actual interface{}, delta float64, msgAndArgs ...interface{}) bool {
	t.Helper()
	if e, ok := expected.(unit_float); ok {
		expected = float64(e)
	}
	if e, ok := actual.(unit_float); ok {
		actual = float64(e)
	}
	return assert.InDelta(t, expected, actual, delta, msgAndArgs...)
}

func in_delta_slice(t *testing.T, expected, actual interface{}, delta float64, msgAndArgs ...interface{}) bool {
	t.Helper()
	if expected == nil || actual == nil ||
		reflect.TypeOf(actual).Kind() != reflect.Slice ||
		reflect.TypeOf(expected).Kind() != reflect.Slice {
		return assert.Fail(t, "Parameters must be slice", msgAndArgs...)
	}
	actualSlice := reflect.ValueOf(actual)
	expectedSlice := reflect.ValueOf(expected)
	for i := 0; i < actualSlice.Len(); i++ {
		result := in_delta(t, actualSlice.Index(i).Interface(), expectedSlice.Index(i).Interface(), delta, msgAndArgs...)
		if !result {
			return result
		}
	}
	return true
}

func encode_clut16bit() []byte {
	var buf bytes.Buffer
	buf.Write([]byte{2, 2, 2})             // grid points for each input (2×2×2)
	buf.Write(bytes.Repeat([]byte{0}, 13)) // rest of grid points unused
	buf.WriteString("\x02\x00\x00\x00")    // bytes_per_channel
	// 2x2x2 = 8 grid points, 3 outputs per point = 24 outputs
	for i := range 8 * 3 {
		val := uint16((i * 65535) / (8*3 - 1)) // Spread nicely 0..65535
		_ = binary.Write(&buf, binary.BigEndian, val)
	}
	if extra := buf.Len() % 4; extra > 0 {
		buf.WriteString(strings.Repeat("\x00", 4-extra))
	}
	return buf.Bytes()
}

func TestCLUTDecoder(t *testing.T) {
	t.Run("Success3D16bit", func(t *testing.T) {
		val, err := embeddedClutDecoder(encode_clut16bit(), 3, 3)
		require.NoError(t, err)
		require.IsType(t, &CLUTTag{}, val)
		clut := val.(*CLUTTag)
		assert.Equal(t, (3), clut.InputChannels)
		assert.Equal(t, (3), clut.OutputChannels)
		assert.Equal(t, []int{2, 2, 2}, clut.GridPoints)
		assert.Len(t, clut.Values, 8*3)
		in_delta(t, 0.0, clut.Values[0], 0.001)
		in_delta(t, 1.0, clut.Values[len(clut.Values)-1], 0.001) // <-- Now will pass!
	})
	t.Run("Success3D8bit", func(t *testing.T) {
		var buf bytes.Buffer
		buf.Write([]byte{2, 2, 2})             // grid points for each input (2×2×2)
		buf.Write(bytes.Repeat([]byte{0}, 13)) // rest of grid points unused
		buf.WriteString("\x01\x00\x00\x00")    // bytes_per_channel
		// 2x2x2 = 8 grid points, 3 outputs per point = 24 outputs
		for i := range 8*3 - 1 {
			buf.WriteByte(uint8(i))
		}
		buf.WriteByte(255)
		val, err := embeddedClutDecoder(buf.Bytes(), 3, 3)
		require.NoError(t, err)
		require.IsType(t, &CLUTTag{}, val)
		clut := val.(*CLUTTag)
		assert.Equal(t, (3), clut.InputChannels)
		assert.Equal(t, (3), clut.OutputChannels)
		assert.Equal(t, []int{2, 2, 2}, clut.GridPoints)
		assert.Len(t, clut.Values, 8*3)
		in_delta(t, 0.0, clut.Values[0], 0.001)
		in_delta(t, 1.0, clut.Values[len(clut.Values)-1], 1e-6)
	})
	t.Run("TooShort", func(t *testing.T) {
		data := make([]byte, 19) // should be at least 20 bytes
		_, err := embeddedClutDecoder(data, 1, 1)
		assert.ErrorContains(t, err, "clut tag too short")
	})
	t.Run("UnexpectedBodyLength", func(t *testing.T) {
		var buf bytes.Buffer
		buf.Write([]byte{2, 2, 2})             // grid points for each input (2×2×2)
		buf.Write(bytes.Repeat([]byte{0}, 13)) // rest of grid points unused
		buf.WriteString("\x01\x00\x00\x00")    // bytes_per_channel
		// 2x2x2 = 8 grid points, 3 outputs per point = 24 outputs
		for i := 0; i < 8*3-1; i++ {
			buf.WriteByte(uint8(i))
		}
		_, err := embeddedClutDecoder(buf.Bytes(), 3, 3)
		assert.ErrorContains(t, err, "CLUT unexpected body length")
	})
}

func TestCLUTTransform(t *testing.T) {
	var output, workspace [16]unit_float
	out, work := output[:], workspace[:]
	t.Run("HappyPath_1D", func(t *testing.T) {
		clut := &CLUTTag{
			InputChannels:  1,
			OutputChannels: 1,
			GridPoints:     []int{2},
			Values:         []unit_float{0.0, 1.0}, // 2 values: for 1D input, 1 output channel
		}
		// Test input 0.0 → should return 0.0
		clut.Transform(out, work, 0.0)
		in_delta(t, 0.0, out[0], 1e-6)
		// Test input 1.0 → should return 1.0
		clut.Transform(out, work, 1.0)
		in_delta(t, 1.0, out[0], 1e-6)
		// Test input 0.5 → should return 0.5 via interpolation
		clut.Transform(out, work, 0.5)
		in_delta(t, 0.5, out[0], 1e-6)
	})
	t.Run("HappyPath_3D", func(t *testing.T) {
		clut := &CLUTTag{
			InputChannels:  3,
			OutputChannels: 1,
			GridPoints:     []int{2, 2, 2},
			Values: []unit_float{
				0.0, 0.1, 0.2, 0.3,
				0.4, 0.5, 0.6, 1.0,
			}, // 8 points, 1 output each
		}
		clut.Transform(out, work, 0.0, 0.0, 0.0) // Should hit [0.0]
		in_delta(t, 0.0, out[0], 1e-6)
		clut.Transform(out, work, 1.0, 1.0, 1.0) // Should hit [1.0]
		in_delta(t, 1.0, out[0], 1e-6)
	})
}

func TestClamp01(t *testing.T) {
	require.Equal(t, unit_float(1), clamp01(1.0001))
	require.Equal(t, unit_float(0), clamp01(-1))
	require.Equal(t, unit_float(0.5), clamp01(0.5))
}
