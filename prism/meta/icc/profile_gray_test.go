package icc

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func gray_test_xyz_bytes(x, y, z unit_float) []byte {
	b := bytes.NewBuffer([]byte("XYZ \x00\x00\x00\x00"))
	b.Write(encodeS15Fixed16BE(x))
	b.Write(encodeS15Fixed16BE(y))
	b.Write(encodeS15Fixed16BE(z))
	return b.Bytes()
}

func build_gray_test_profile(t *testing.T) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	writeTestHeader(buf, [4]byte{'a', 'c', 's', 'p'}, [4]byte{'G', 'R', 'A', 'Y'})
	writeTestTagTable(buf, map[[4]byte][]byte{
		{'k', 'T', 'R', 'C'}: para_bytes(0, 2.2),
		{'w', 't', 'p', 't'}: gray_test_xyz_bytes(0.9642, 1.0, 0.8249),
	})
	return buf.Bytes()
}

func TestGrayProfile(t *testing.T) {
	data := build_gray_test_profile(t)
	p, err := DecodeProfile(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, ColorSpaceGray, p.Header.DataColorSpace)

	to_pcs, err := p.CreateDefaultTransformerToPCS(1)
	require.NoError(t, err)

	to_device, err := p.CreateDefaultTransformerToDevice()
	require.NoError(t, err)

	for _, gray := range []unit_float{0, 0.25, 0.5, 0.75, 1} {
		var xyz, in, out [4]unit_float
		in[0] = gray
		to_pcs.TransformGeneral(xyz[:], in[:])
		require.Greater(t, xyz[1], unit_float(-1e-9), "Y should be non-negative for gray=%v", gray)

		to_device.TransformGeneral(out[:], xyz[:])
		require.InDelta(t, gray, out[0], 1e-3, "round-tripped gray value should match original for gray=%v", gray)
	}
}
