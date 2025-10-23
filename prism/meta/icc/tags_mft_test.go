package icc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

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
		for _, x := range []float32{1, 0, 0, 0, 1, 0, 0, 0, 1} {
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
	var writeval func(float32)
	if m.is8bit {
		writeval = func(x float32) { buf.WriteByte(uint8(x * 255)) }
		for _, c := range m.input_curves {
			if len(c) != 256 {
				panic("mft1 must have curves of length 256")
			}
		}
		for _, c := range m.output_curves {
			if len(c) != 256 {
				panic("mft1 must have curves of length 256")
			}
		}
	} else {
		binary.Write(&buf, binary.BigEndian, []uint16{uint16(len(m.input_curves[0])), uint16(len(m.output_curves[0]))})
		writeval = func(x float32) { binary.Write(&buf, binary.BigEndian, uint16(x*65535)) }
	}
	for _, curve := range m.input_curves {
		for _, x := range curve {
			writeval(x)
		}
	}
	for _, x := range m.clut {
		writeval(x)
	}
	for _, curve := range m.output_curves {
		for _, x := range curve {
			writeval(x)
		}
	}
	return buf.Bytes()
}

func (a *MFT) require_equal(t *testing.T, b *MFT) {
	require.Equal(t, a.in_channels, b.in_channels)
	require.Equal(t, a.out_channels, b.out_channels)
	require.Equal(t, a.grid_points, b.grid_points)
	require.Equal(t, len(a.input_curves), len(b.input_curves))
	require.Equal(t, a.matrix, b.matrix)
	require.Equal(t, a.is8bit, b.is8bit)
	tolerance := IfElse(a.is8bit, 0.01, 0.0001)
	for i := range a.input_curves {
		require.InDeltaSlice(t, a.input_curves[i], b.input_curves[i], tolerance)
	}
	require.InDeltaSlice(t, a.clut, b.clut, tolerance)
	for i := range a.output_curves {
		require.InDeltaSlice(t, a.output_curves[i], b.output_curves[i], tolerance)
	}
}

func make_curve(l int) []float32 {
	curve := make([]float32, l)
	for i := range len(curve) {
		curve[i] = float32(i) / float32(l)
	}
	return curve
}

func TestMFTTag(t *testing.T) {
	c := make_curve(13)
	gp := []int{2, 2, 2}
	im := IdentityMatrix(0)
	m := MFT{
		in_channels: 3, out_channels: 3, grid_points: gp,
		input_curves: [][]float32{c, c, c}, output_curves: [][]float32{c, c, c},
		clut: make_curve(expectedValues(gp, 3)), matrix: &im,
	}

	roundtrip := func() {
		f := IfElse(m.is8bit, decode_mft8, decode_mft16)
		r, err := f(m.as_bytes())
		if err != nil {
			t.Fatal(err)
		}
		m.require_equal(t, r.(*MFT))
	}
	roundtrip()
	m.is8bit = true
	c = make_curve(256)
	m.input_curves = [][]float32{c, c, c}
	m.output_curves = [][]float32{c, c, c}
	roundtrip()
}
