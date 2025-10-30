package icc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func curve_points(c Curve1D) []unit_float {
	return c.(*PointsCurve).points
}

func curve_len(c Curve1D) int {
	return len(curve_points(c))
}

func (m *MFT) as_bytes() []byte {
	sig := IfElse(m.is8bit, "mft1", "mft2")
	var buf bytes.Buffer
	buf.WriteString(sig)
	buf.WriteString("\x00\x00\x00\x00")
	buf.WriteByte(uint8(m.in_channels))
	buf.WriteByte(uint8(m.out_channels))
	buf.WriteByte(uint8(m.grid_points[0]))
	buf.WriteByte(0)
	switch m := m.matrix.(type) {
	case *Matrix3:
		for _, row := range m {
			for _, x := range row {
				buf.Write(encodeS15Fixed16BE(x))
			}
		}
	case *IdentityMatrix:
		for _, x := range []unit_float{1, 0, 0, 0, 1, 0, 0, 0, 1} {
			buf.Write(encodeS15Fixed16BE(x))
		}
	case *MatrixWithOffset:
		for _, row := range m.m.(*Matrix3) {
			for _, x := range row {
				buf.Write(encodeS15Fixed16BE(x))
			}
		}
		buf.Write(encodeS15Fixed16BE(m.offset1))
		buf.Write(encodeS15Fixed16BE(m.offset2))
		buf.Write(encodeS15Fixed16BE(m.offset3))
	default:
		panic(fmt.Sprintf("unknown type of matrix: %T", m))
	}
	var writeval func(unit_float)
	if m.is8bit {
		writeval = func(x unit_float) { buf.WriteByte(uint8(x * 255)) }
		for _, c := range m.input_curve.Curves() {
			if curve_len(c) != 256 {
				panic("mft1 must have curves of length 256")
			}
		}
		for _, c := range m.output_curve.Curves() {
			if curve_len(c) != 256 {
				panic("mft1 must have curves of length 256")
			}
		}
	} else {
		binary.Write(&buf, binary.BigEndian, []uint16{uint16(curve_len(m.input_curve.Curves()[0])), uint16(curve_len(m.output_curve.Curves()[0]))})
		writeval = func(x unit_float) { binary.Write(&buf, binary.BigEndian, uint16(x*65535)) }
	}
	for _, curve := range m.input_curve.Curves() {
		for _, x := range curve_points(curve) {
			writeval(x)
		}
	}
	for _, x := range m.clut.Samples() {
		writeval(x)
	}
	for _, curve := range m.output_curve.Curves() {
		for _, x := range curve_points(curve) {
			writeval(x)
		}
	}
	return buf.Bytes()
}

func (a *MFT) require_equal(t *testing.T, b *MFT) {
	require.Equal(t, a.in_channels, b.in_channels)
	require.Equal(t, a.out_channels, b.out_channels)
	require.Equal(t, a.grid_points, b.grid_points)
	require.Equal(t, len(a.input_curve.Curves()), len(b.input_curve.Curves()))
	require.Equal(t, a.matrix, b.matrix)
	require.Equal(t, a.is8bit, b.is8bit)
	tolerance := IfElse(a.is8bit, 0.01, 0.0001)
	for i := range a.input_curve.Curves() {
		in_delta_slice(t, curve_points(a.input_curve.Curves()[i]), curve_points(b.input_curve.Curves()[i]), tolerance)
	}
	in_delta_slice(t, a.clut.Samples(), b.clut.Samples(), tolerance)
	for i := range a.output_curve.Curves() {
		in_delta_slice(t, curve_points(a.output_curve.Curves()[i]), curve_points(b.output_curve.Curves()[i]), tolerance)
	}
}

func make_curve(l int) Curve1D {
	curve := make([]unit_float, l)
	for i := range len(curve) {
		curve[i] = unit_float(i) / unit_float(l)
	}
	p, _ := load_points_curve(curve)
	return p
}

func TestMFTTag(t *testing.T) {
	c := make_curve(13)
	gp := []int{2, 2, 2}
	im := IdentityMatrix(0)
	m := MFT{
		in_channels: 3, out_channels: 3, grid_points: gp,
		input_curve: NewCurveTransformer("test", c, c, c), output_curve: NewCurveTransformer("test", c, c, c),
		clut: make_clut(gp, 3, 3, curve_points(make_curve(expectedValues(gp, 3))), true, false), matrix: &im,
	}

	roundtrip := func() {
		f := IfElse(m.is8bit, decode_mft8, decode_mft16)
		r, err := f(m.as_bytes(), ColorSpaceRGB, ColorSpaceXYZ)
		if err != nil {
			t.Fatal(err)
		}
		m.require_equal(t, r.(*MFT))
	}
	roundtrip()
	m.is8bit = true
	c = make_curve(256)
	m.input_curve = NewCurveTransformer("test", c, c, c)
	m.output_curve = NewCurveTransformer("test", c, c, c)
	roundtrip()
}
