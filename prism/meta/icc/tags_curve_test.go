package icc

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed test-profiles/sRGB2014.icc
var srgb_xyz_profile_data []byte

var Srgb_xyz_profile = sync.OnceValue(func() *Profile {
	p, _ := DecodeProfile(bytes.NewReader(srgb_xyz_profile_data))
	return p
})

func floatToFixed88(f unit_float) uint16 {
	// Clamp the value to fit in 8.8 range
	if f < 0 {
		f = 0
	}
	if f > 255.99609375 { // 255 + 255/256
		f = 255.99609375
	}
	v := uint16(math.Round(float64(f * 256)))
	return v
}

func curv_bytes(params ...unit_float) []byte {
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

func para_bytes(q uint16, params ...unit_float) []byte {
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
		_, ok := val.(*IdentityCurve)
		require.True(t, ok)
	})
	t.Run("PointsCurve", func(t *testing.T) {
		raw := curv_bytes(0.1, 0.2, 0.3)
		val, err := curveDecoder(raw)
		require.NoError(t, err)
		require.IsType(t, &PointsCurve{}, val)
		c := val.(*PointsCurve)
		in_delta_slice(t, []unit_float{0.1, 0.2, 0.3}, c.points, 0.0001)
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
	w := func(t *testing.T, q uint16, expect_error bool, params ...unit_float) any {
		t.Helper()
		val, err := parametricCurveDecoder(para_bytes(q, params...))
		if expect_error {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		return val
	}
	t.Run("GammaCurve", func(t *testing.T) {
		val := w(t, 0, false, 1.2)
		q := &GammaCurve{}
		require.IsType(t, q, val)
		p := val.(*GammaCurve)
		in_delta(t, 1.2, p.gamma, 0.0001)
	})
	t.Run("ConditionalZeroCurve", func(t *testing.T) {
		w(t, 1, false, 3, 0, 7)
		val := w(t, 1, false, 3, 1, 2)
		require.IsType(t, &ConditionalZeroCurve{}, val)
		p := val.(*ConditionalZeroCurve)
		in_delta(t, 3.0, p.g, 0.0001)
		in_delta(t, 1.0, p.a, 0.0001)
		in_delta(t, 2.0, p.b, 0.0001)
	})
	t.Run("ConditionalCCurve", func(t *testing.T) {
		w(t, 2, false, 3, 0, 1, 2, 3)
		val := w(t, 2, false, 7, 1, 2, 3, 4)
		require.IsType(t, &ConditionalCCurve{}, val)
		p := val.(*ConditionalCCurve)
		in_delta(t, 7.0, p.g, 0.0001)
		in_delta(t, 1.0, p.a, 0.0001)
		in_delta(t, 2.0, p.b, 0.0001)
		in_delta(t, 3.0, p.c, 0.0001)
	})
	t.Run("SplitCurve", func(t *testing.T) {
		val := w(t, 3, false, 9, 1, 2, 3, 4, 5)
		require.IsType(t, &SplitCurve{}, val)
		p := val.(*SplitCurve)
		in_delta(t, 9.0, p.g, 0.0001)
		in_delta(t, 1.0, p.a, 0.0001)
		in_delta(t, 2.0, p.b, 0.0001)
		in_delta(t, 3.0, p.c, 0.0001)
		in_delta(t, 4.0, p.d, 0.0001)
	})
	t.Run("ComplexCurve", func(t *testing.T) {
		val := w(t, 4, false, 11, 1, 2, 3, 4, 5, 6)
		require.IsType(t, &ComplexCurve{}, val)
		p := val.(*ComplexCurve)
		in_delta(t, 11.0, p.g, 0.0001)
		in_delta(t, 1.0, p.a, 0.0001)
		in_delta(t, 2.0, p.b, 0.0001)
		in_delta(t, 3.0, p.c, 0.0001)
		in_delta(t, 4.0, p.d, 0.0001)
		in_delta(t, 5.0, p.e, 0.0001)
		in_delta(t, 6.0, p.f, 0.0001)
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

func rt(t *testing.T, c Curve1D, x, y unit_float) {
	t.Helper()
	require.NoError(t, c.Prepare())
	ans := c.Transform(x)
	in_delta(t, y, ans, 0.0001)
}

func TestCurveTag_Transform(t *testing.T) {
	f := func(c Curve1D, x, y unit_float) {
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
		c := &PointsCurve{points: []unit_float{0, 0.5, 1.}}
		f(c, 0.5, 0.5)
	})
	t.Run("PointsCurve_Interpolation", func(t *testing.T) {
		c := &PointsCurve{points: []unit_float{0, 0.5, 1.}}
		f(c, 0.25, 0.25)
	})
}

func curve_inverse(t *testing.T, c Curve1D, delta float64) {
	const num = 256
	t.Run(c.String(), func(t *testing.T) {
		t.Parallel()
		require.NoError(t, c.Prepare(), fmt.Sprintf("failed to prepare %s", c))
		for i := range num {
			x := unit_float(i) / unit_float(num-1)
			y := c.Transform(x)
			nx := c.InverseTransform(y)
			in_delta(t, x, nx, delta, fmt.Sprintf("inversion of x=%f in curve %s failed: y=%f inv(y)=%f", x, c, y, nx))
		}
	})
}

func generate_sampled_curve(c Curve1D) *PointsCurve {
	const num = 256 * 16
	points := make([]unit_float, num)
	for i := range num {
		x := unit_float(i) / unit_float(num-1)
		points[i] = c.Transform(x)
	}
	return &PointsCurve{points: points}
}

func srgb_sampled_curve() *PointsCurve {
	p := Srgb_xyz_profile()
	rte := p.TagTable.entries[RedTRCTagSignature]
	raw := rte.data
	count := int(binary.BigEndian.Uint32(raw[8:12]))
	points := make([]uint16, count)
	binary.Decode(raw[12:], binary.BigEndian, points)
	fp := make([]unit_float, len(points))
	for i, p := range points {
		fp[i] = unit_float(p) / math.MaxUint16
	}
	c := &PointsCurve{points: fp}
	c.Prepare()
	return c
}

func TestCurveInverse(t *testing.T) {
	const delta = 1e-6
	curve_inverse(t, IdentityCurve(0), delta)
	curve_inverse(t, &GammaCurve{gamma: 2}, delta)
	curve_inverse(t, &ConditionalZeroCurve{a: 2, b: 1, g: 0.5}, delta)
	curve_inverse(t, &ConditionalCCurve{a: 1, b: 2, c: 3, g: 2}, delta)
	curve_inverse(t, &SplitCurve{a: 1, b: 2, c: 3, d: 4, g: 2}, delta)
	curve_inverse(t, &ComplexCurve{a: 1, b: 2, c: 3, d: 4, e: 5, f: 6, g: 2}, delta)
	curve_inverse(t, generate_sampled_curve(&GammaCurve{gamma: 2}), 5e-3)
	curve_inverse(t, srgb_sampled_curve(), 1e-3)
	identity := make([]unit_float, 256)
	for i := range identity {
		identity[i] = unit_float(i) / 255
	}
	curve_inverse(t, &PointsCurve{points: identity}, 1e-5)
}

func srgb_to_linear(v unit_float) unit_float {
	if v <= 0.0031308 {
		return v * 12.92
	}
	return 1.055*pow(unit_float(v), 1/2.4) - 0.055
}

func linear_to_srgb(v unit_float) unit_float {
	if v <= 0.0031308*12.92 {
		return v / 12.92
	}
	return pow((v+0.055)/1.055, 2.4)
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
		rt(t, &ComplexCurve{a: 1, b: 0, c: 2, d: 0.5, e: 0.1, f: 0.2, g: 2}, 0.6, pow(0.6, 2)+0.1)
	})
	t.Run("ComplexFunction_NegativeBranch", func(t *testing.T) {
		rt(t, &ComplexCurve{a: 1, b: 0, c: 0.5, d: 0.6, e: 0.1, f: 0.2, g: 2}, 0.5, 0.45)
	})
	rc := SRGBCurve()
	sc := srgb_sampled_curve()
	num := 4 * len(sc.points)
	for i := range num {
		x := unit_float(i) / unit_float(num-1)
		in_delta(t, rc.Transform(x), linear_to_srgb(x), 1e-6, fmt.Sprintf("failed for analytic curve i=%d x=%f", i, x))
		in_delta(t, rc.Transform(x), sc.Transform(x), 1e-5, fmt.Sprintf("failed for sampled curve i=%d x=%f", i, x))
		// now test inverse transforms
		in_delta(t, rc.InverseTransform(x), srgb_to_linear(x), 1e-6, fmt.Sprintf("failed for analytic inverse curve i=%d x=%f", i, x))
		in_delta(t, rc.InverseTransform(x), sc.InverseTransform(x), 0.002, fmt.Sprintf("failed for sampled curve inverse i=%d x=%f", i, x))
	}
}
