package icc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Println

func in_delta(t *testing.T, expected, actual any, delta float64, msgAndArgs ...any) bool {
	t.Helper()
	if e, ok := expected.(unit_float); ok {
		expected = float64(e)
	}
	if e, ok := actual.(unit_float); ok {
		actual = float64(e)
	}
	return assert.InDelta(t, expected, actual, delta, msgAndArgs...)
}

func in_delta_slice(t *testing.T, expected, actual any, delta float64, msgAndArgs ...any) bool {
	t.Helper()
	if expected == nil || actual == nil ||
		reflect.TypeOf(actual).Kind() != reflect.Slice ||
		reflect.TypeOf(expected).Kind() != reflect.Slice {
		return assert.Fail(t, "Parameters must be slice", msgAndArgs...)
	}
	actualSlice := reflect.ValueOf(actual)
	expectedSlice := reflect.ValueOf(expected)
	if actualSlice.Len() != expectedSlice.Len() {
		return assert.Fail(t, fmt.Sprintf("Slices have different lengths: want %d got %d", expectedSlice.Len(), actualSlice.Len()), msgAndArgs...)
	}
	for i := 0; i < actualSlice.Len(); i++ {
		e, a := expectedSlice.Index(i).Interface(), actualSlice.Index(i).Interface()
		result := in_delta(t, e, a, delta, msgAndArgs...)
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
		val := unit_float(i) / unit_float(8*3-1)
		_ = binary.Write(&buf, binary.BigEndian, uint16(val*math.MaxUint16))
	}
	if extra := buf.Len() % 4; extra > 0 {
		buf.WriteString(strings.Repeat("\x00", 4-extra))
	}
	return buf.Bytes()
}

func TestCLUTDecoder(t *testing.T) {
	t.Run("Success3D16bit", func(t *testing.T) {
		val, err := embeddedClutDecoder(encode_clut16bit(), 3, 3, ColorSpaceXYZ, false)
		require.NoError(t, err)
		require.IsType(t, &TetrahedralInterpolate{}, val)
		clut := val.(*TetrahedralInterpolate)
		assert.Equal(t, (3), clut.d.num_outputs)
		assert.Equal(t, []int{2, 2, 2}, clut.d.grid_points)
		assert.Len(t, clut.d.samples, 8*3)
		in_delta(t, 0.0, clut.d.samples[0], 0.001)
		in_delta(t, 1.0, clut.d.samples[len(clut.d.samples)-1], 0.001) // <-- Now will pass!
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
		val, err := embeddedClutDecoder(buf.Bytes(), 3, 3, ColorSpaceXYZ, false)
		require.NoError(t, err)
		require.IsType(t, &TetrahedralInterpolate{}, val)
		clut := val.(*TetrahedralInterpolate)
		assert.Equal(t, (3), clut.d.num_outputs)
		assert.Equal(t, []int{2, 2, 2}, clut.d.grid_points)
		assert.Len(t, clut.d.samples, 8*3)
		in_delta(t, 0.0, clut.d.samples[0], 0.001)
		in_delta(t, 1.0, clut.d.samples[len(clut.d.samples)-1], 1e-6)
	})
	t.Run("TooShort", func(t *testing.T) {
		data := make([]byte, 19) // should be at least 20 bytes
		_, err := embeddedClutDecoder(data, 1, 1, ColorSpaceXYZ, false)
		assert.ErrorContains(t, err, "clut tag too short")
	})
	t.Run("UnexpectedBodyLength", func(t *testing.T) {
		var buf bytes.Buffer
		buf.Write([]byte{2, 2, 2})             // grid points for each input (2×2×2)
		buf.Write(bytes.Repeat([]byte{0}, 13)) // rest of grid points unused
		buf.WriteString("\x01\x00\x00\x00")    // bytes_per_channel
		// 2x2x2 = 8 grid points, 3 outputs per point = 24 outputs
		for i := range 8*3 - 1 {
			buf.WriteByte(uint8(i))
		}
		_, err := embeddedClutDecoder(buf.Bytes(), 3, 3, ColorSpaceXYZ, false)
		assert.ErrorContains(t, err, "CLUT table too short 23 < 24")
	})
}

func TestCLUTTransform(t *testing.T) {
	var output [16]unit_float
	out := output[:]
	t.Run("HappyPath_3D", func(t *testing.T) {
		clut := &TetrahedralInterpolate{make_interpolation_data(3, 3, []int{2, 2, 2},
			[]unit_float{
				0.0, 0.0, 0.0,
				0.1, 0.1, 0.1,
				0.2, 0.2, 0.2,
				0.3, 0.3, 0.3,
				0.4, 0.4, 0.4,
				0.5, 0.5, 0.5,
				0.6, 0.6, 0.6,
				1, 1, 1,
			}, // 8 points per output
		), false}
		out[0], out[1], out[2] = clut.Transform(0.0, 0.0, 0.0) // Should hit [0.0]
		in_delta(t, 0.0, out[0], 1e-6)
		out[0], out[1], out[2] = clut.Transform(1.0, 1.0, 1.0) // Should hit [1.0]
		in_delta(t, 1.0, out[0], 1e-6)
	})
	t.Run("RGB->1-RGB", func(t *testing.T) {
		clut := &TetrahedralInterpolate{make_interpolation_data(3, 3, []int{2, 2, 2},
			// The table below has the output on the left and input in the comment on the right.
			// As per section 10.12.3 of of ICC.1-2022-5.pdf spec the first input channel (R)
			// varies least rapidly and the last (B) varies most rapidly
			[]unit_float{
				// Output <-   Input
				1, 1, 1, // <- R=0, G=0, B=0
				1, 1, 0, // <- R=0, G=0, B=1
				1, 0, 1, // <- R=0, G=1, B=0
				1, 0, 0, // <- R=0, G=1, B=1

				0, 1, 1, // <- R=1, G=0, B=0
				0, 1, 0, // <- R=1, G=0, B=0
				0, 0, 1, // <- R=1, G=1, B=0
				0, 0, 0, // <- R=1, G=1, B=1
			}, // 8 points per output
		), false}
		type u = [3]unit_float
		for _, c := range []u{
			{0, 0, 0}, {1, 1, 1}, {1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {0.5, 0.5, 0.5},
			{0.5, 0, 0}, {0.5, 1, 1}, {0.25, 0.5, 0.75}, {0.75, 0.5, 0.25},
		} {
			expected := []unit_float{1 - c[0], 1 - c[1], 1 - c[2]}
			r, g, b := clut.Trilinear_interpolate(c[0], c[1], c[2])
			actual := []unit_float{r, g, b}
			in_delta_slice(t, expected, actual, FLOAT_EQUALITY_THRESHOLD, fmt.Sprintf("trilinear: %v -> %v != %v", c, actual, expected))
			r, g, b = clut.Tetrahedral_interpolate(c[0], c[1], c[2])
			actual = []unit_float{r, g, b}
			in_delta_slice(t, expected, actual, FLOAT_EQUALITY_THRESHOLD, fmt.Sprintf("tetrahedral: %v -> %v != %v", c, actual, expected))
		}
	})
}

func TestClamp01(t *testing.T) {
	require.Equal(t, unit_float(1), clamp01(1.0001))
	require.Equal(t, unit_float(0), clamp01(-1))
	require.Equal(t, unit_float(0.5), clamp01(0.5))
}
