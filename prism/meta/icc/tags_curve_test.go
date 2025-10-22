package icc

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

func floatToFixed88(f float64) uint16 {
	// Clamp the value to fit in 8.8 range
	if f < 0 {
		f = 0
	}
	if f > 255.99609375 { // 255 + 255/256
		f = 255.99609375
	}
	v := uint16(math.Round(f * 256))
	return v
}

func curv_bytes(params ...float64) []byte {
	b := bytes.NewBuffer([]byte("curv\x00\x00\x00\x00"))
	binary.Write(b, binary.BigEndian, uint32(len(params)))
	if len(params) == 1 {
		binary.Write(b, binary.BigEndian, floatToFixed88(params[0]))
	} else {
		for _, p := range params {
			u := uint16(p * 65535)
			binary.Write(b, binary.BigEndian, u)
		}
	}
	if extra := b.Len() % 4; extra != 0 {
		b.Write(bytes.Repeat([]byte{0}, 4-extra))
	}
	return b.Bytes()
}

func para_bytes(q uint16, params ...float64) []byte {
	b := bytes.NewBuffer([]byte("para\x00\x00\x00\x00"))
	_ = binary.Write(b, binary.BigEndian, q)
	b.WriteString("\x00\x00")
	for _, p := range params {
		b.Write(encodeS15Fixed16BE(p))
	}
	if extra := b.Len() % 4; extra != 0 {
		b.Write(bytes.Repeat([]byte{0}, 4-extra))
	}
	return b.Bytes()
}

func TestCurveDecoder(t *testing.T) {
	t.Run("IdentityCurve", func(t *testing.T) {
		raw := curv_bytes()
		val, err := curveDecoder(raw)
		require.NoError(t, err)
		q := IdentityCurve(0)
		require.IsType(t, &q, val)
	})
	t.Run("GammaCurve", func(t *testing.T) {
		raw := curv_bytes(1.0)
		val, err := curveDecoder(raw)
		require.NoError(t, err)
		c := val.(*GammaCurve)
		assert.InDelta(t, 1.0, float64(c.gamma), 0.001)
	})
	t.Run("PointsCurve", func(t *testing.T) {
		raw := curv_bytes(0.1, 0.2, 0.3)
		val, err := curveDecoder(raw)
		require.NoError(t, err)
		require.IsType(t, &PointsCurve{}, val)
		c := val.(*PointsCurve)
		assert.InDeltaSlice(t, []float64{0.1, 0.2, 0.3}, c.points, 0.0001)
	})
	t.Run("TooShort", func(t *testing.T) {
		_, err := curveDecoder(make([]byte, 11))
		assert.ErrorContains(t, err, "curv tag too short")
	})
	t.Run("MissingGamma", func(t *testing.T) {
		raw := []byte("curv\x00\x00\x00\x00" +
			"\x00\x00\x00\x01") // count = 1 (but no gamma value)
		_, err := curveDecoder(raw)
		assert.ErrorContains(t, err, "curv tag missing gamma value")
	})
	t.Run("TruncatedPoints", func(t *testing.T) {
		raw := []byte("curv\x00\x00\x00\x00" +
			"\x00\x00\x00\x02" + // count = 2
			"\x00\x10") // missing second uint16
		_, err := curveDecoder(raw)
		assert.ErrorContains(t, err, "curv tag truncated")
	})
}

func TestParametricCurveDecoder(t *testing.T) {
	w := func(t *testing.T, q uint16, expect_error bool, params ...float64) any {
		val, err := parametricCurveDecoder(para_bytes(q, params...))
		if expect_error {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		return val
	}
	t.Run("GammaCurve", func(t *testing.T) {
		val := w(t, 0, false, 1.0)
		q := &GammaCurve{}
		require.IsType(t, q, val)
		p := val.(*GammaCurve)
		assert.InDelta(t, 1.0, p.gamma, 0.0001)
	})
	t.Run("ConditionalZeroCurve", func(t *testing.T) {
		w(t, 1, true, 3, 0, 7)
		val := w(t, 1, false, 3, 1, 2)
		require.IsType(t, &ConditionalZeroCurve{}, val)
		p := val.(*ConditionalZeroCurve)
		assert.InDelta(t, 3.0, p.g, 0.0001)
		assert.InDelta(t, 1.0, p.a, 0.0001)
		assert.InDelta(t, 2.0, p.b, 0.0001)
	})
	t.Run("ConditionalCCurve", func(t *testing.T) {
		w(t, 2, true, 3, 0, 1, 2, 3)
		val := w(t, 2, false, 7, 1, 2, 3, 4)
		require.IsType(t, &ConditionalCCurve{}, val)
		p := val.(*ConditionalCCurve)
		assert.InDelta(t, 7.0, p.g, 0.0001)
		assert.InDelta(t, 1.0, p.a, 0.0001)
		assert.InDelta(t, 2.0, p.b, 0.0001)
		assert.InDelta(t, 3.0, p.c, 0.0001)
	})
	t.Run("SplitCurve", func(t *testing.T) {
		val := w(t, 3, false, 9, 1, 2, 3, 4, 5)
		require.IsType(t, &SplitCurve{}, val)
		p := val.(*SplitCurve)
		assert.InDelta(t, 9.0, p.g, 0.0001)
		assert.InDelta(t, 1.0, p.a, 0.0001)
		assert.InDelta(t, 2.0, p.b, 0.0001)
		assert.InDelta(t, 3.0, p.c, 0.0001)
		assert.InDelta(t, 4.0, p.d, 0.0001)
	})
	t.Run("ComplexCurve", func(t *testing.T) {
		val := w(t, 4, false, 11, 1, 2, 3, 4, 5, 6)
		require.IsType(t, &ComplexCurve{}, val)
		p := val.(*ComplexCurve)
		assert.InDelta(t, 11.0, p.g, 0.0001)
		assert.InDelta(t, 1.0, p.a, 0.0001)
		assert.InDelta(t, 2.0, p.b, 0.0001)
		assert.InDelta(t, 3.0, p.c, 0.0001)
		assert.InDelta(t, 4.0, p.d, 0.0001)
		assert.InDelta(t, 5.0, p.e, 0.0001)
		assert.InDelta(t, 6.0, p.f, 0.0001)
	})
	t.Run("TooShort", func(t *testing.T) {
		_, err := parametricCurveDecoder(make([]byte, 11))
		assert.ErrorContains(t, err, "para tag too short")
	})
	t.Run("UnknownFunction", func(t *testing.T) {
		var buf bytes.Buffer
		buf.WriteString("para")
		buf.Write([]byte{0, 0, 0, 0})
		_ = binary.Write(&buf, binary.BigEndian, uint16(5))
		for i := 0; i < 7; i++ {
			_ = binary.Write(&buf, binary.BigEndian, uint32(0x00010000)) // 1.0
		}
		_, err := parametricCurveDecoder(buf.Bytes())
		require.Error(t, err)
		assert.ErrorContains(t, err, "unknown parametric function type: 5")
	})
	t.Run("TruncatedParameters", func(t *testing.T) {
		raw := []byte("para\x00\x00\x00\x00" +
			"\x00\x02" + // function type 2 (needs 4 params)
			"\x00\x01\x00\x00\x00\x01\x00\x00\x00\x01\x00\x00") // only 3 params
		_, err := parametricCurveDecoder(raw)
		assert.ErrorContains(t, err, "para tag too short")
	})
}

func rt(t *testing.T, c Curve1D, x, y float64) {
	t.Helper()
	require.NoError(t, c.Prepare())
	ans := c.Transform(x)
	assert.InDelta(t, y, ans, 0.0001)
}

func TestCurveTag_Transform(t *testing.T) {
	f := func(c Curve1D, x, y float64) {
		rt(t, c, x, y)
	}
	t.Run("IdentityCurve", func(t *testing.T) {
		c := IdentityCurve(0)
		f(&c, 0.5, 0.5)
	})
	t.Run("GammaCurve", func(t *testing.T) {
		f(&GammaCurve{2.0, 0.5, false}, 0.5, 0.25)
	})
	t.Run("PointsCurve_ExactIndex", func(t *testing.T) {
		c := &PointsCurve{points: []float64{0, 0.5, 1.}}
		f(c, 0.5, 0.5)
	})
	t.Run("PointsCurve_Interpolation", func(t *testing.T) {
		c := &PointsCurve{points: []float64{0, 0.5, 1.}}
		f(c, 0.25, 0.25)
	})
}

func TestParametricCurveTag_Transform(t *testing.T) {
	t.Run("ConditionalZeroFunction", func(t *testing.T) {
		rt(t, &ConditionalZeroCurve{g: 2, a: 1, b: 0}, -0.5, 0)
	})
	t.Run("ConditionalZeroFunction_PositiveBranch", func(t *testing.T) {
		rt(t, &ConditionalZeroCurve{g: 2, a: 1, b: 0}, 0.5, 0.25)
	})
	t.Run("ConditionalCFunction", func(t *testing.T) {
		rt(t, &ConditionalCCurve{a: 1, b: 0, c: 0.1, g: 2}, -0.5, 0.1)
	})
	t.Run("ConditionalCFunction_PositiveBranch", func(t *testing.T) {
		rt(t, &ConditionalCCurve{a: 1, b: 0, c: 0.1, g: 2}, 0.5, 0.35)
	})
	t.Run("SplitFunction", func(t *testing.T) {
		rt(t, &SplitCurve{a: 1, b: 0, c: 2.0, d: 0.5, g: 2}, 0.4, 0.8)
	})
	t.Run("SplitFunction_PositiveBranch", func(t *testing.T) {
		rt(t, &SplitCurve{a: 1, b: 0, c: 0.5, d: 0.4, g: 2}, 0.5, 0.25)
	})
	t.Run("ComplexFunction", func(t *testing.T) {
		rt(t, &ComplexCurve{a: 1, b: 0, c: 2, d: 0.5, e: 0.1, f: 0.2, g: 2}, 0.6, math.Pow(0.6, 2)+0.1)
	})
	t.Run("ComplexFunction_NegativeBranch", func(t *testing.T) {
		rt(t, &ComplexCurve{a: 1, b: 0, c: 0.5, d: 0.6, e: 0.1, f: 0.2, g: 2}, 0.5, 0.45)
	})
}
