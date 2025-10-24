//go:build lcms2cgo

package prism

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/kovidgoyal/imaging/prism/meta/icc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func TestCGOConversion(t *testing.T) {
	xyz, err := CreateCMSProfile(icc.Srgb_xyz_profile_data)
	require.NoError(t, err)
	require.Equal(t, xyz.DeviceColorSpace, icc.RGBSignature)
	require.Equal(t, xyz.PCSColorSpace, icc.XYZSignature)
	defer xyz.Close()
	lab, err := CreateCMSProfile(icc.Srgb_lab_profile_data)
	require.Equal(t, lab.DeviceColorSpace, icc.RGBSignature)
	require.Equal(t, lab.PCSColorSpace, icc.LabSignature)
	require.NoError(t, err)
	defer lab.Close()
	_, err = CreateCMSProfile([]byte("invalid profile"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Got 15 bytes, block should be")

	expected := []float32{0.2569, 0.1454, 0.7221}
	pcs, err := xyz.TransformRGB8bitToPCS([]byte{128, 64, 255}, icc.RelativeColorimetricRenderingIntent)
	require.NoError(t, err)
	assert.InDeltaSlice(t, pcs, expected, 0.001)
	pcs, err = xyz.TransformFloatToPCS([]float32{128 / 255., 64 / 255., 255 / 255.}, icc.RelativeColorimetricRenderingIntent)
	require.NoError(t, err)
	assert.InDeltaSlice(t, pcs, expected, 0.001)
	pcs, err = lab.TransformRGB8bitToPCS([]byte{128, 64, 255}, icc.RelativeColorimetricRenderingIntent)
	require.NoError(t, err)
	assert.InDeltaSlice(t, pcs, []float32{45.2933, 58.3075, -85.6426}, 0.001)

	for _, p := range []*CMSProfile{xyz, lab} {
		inp := []byte{128, 64, 255}
		out, err := p.TransformRGB8(inp, p, icc.RelativeColorimetricRenderingIntent)
		require.NoError(t, err)
		assert.Equal(t, inp, out)
	}
}

var pts_for_lcms2 = sync.OnceValue(func() []float32 {
	pts := icc.Points_for_transformer_comparison()
	temp := make([]float32, 0, len(pts)*3)
	for _, pt := range pts {
		temp = append(temp, float32(pt.X), float32(pt.Y), float32(pt.Z))
	}
	return temp
})

func test_profile(t *testing.T, name string, profile_data []byte) {
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		p, err := icc.NewProfileReader(bytes.NewReader(profile_data)).ReadProfile()
		require.NoError(t, err)
		lcms, err := CreateCMSProfile(profile_data)
		require.NoError(t, err)
		tr, err := p.CreateTransformerToPCS(icc.PerceptualRenderingIntent)
		require.NoError(t, err)
		inv, err := p.CreateTransformerToDevice(icc.PerceptualRenderingIntent)
		require.NoError(t, err)
		pts := icc.Points_for_transformer_comparison()
		actual := make([]float32, 0, len(pts)*3)
		for _, pt := range pts {
			r, g, b := tr.Transform(pt.X, pt.Y, pt.Z)
			actual = append(actual, float32(r), float32(g), float32(b))
			r, g, b = inv.Transform(r, g, b)
			require.InDeltaSlice(
				t, []float32{pt.X, pt.Y, pt.Z}, []float32{r, g, b}, icc.FLOAT_EQUALITY_THRESHOLD,
				"b2a of a2b result differs from original color")
		}
		expected, err := lcms.TransformFloatToPCS(pts_for_lcms2(), icc.RelativeColorimetricRenderingIntent)
		require.NoError(t, err)
		require.InDeltaSlice(t, expected, actual, icc.FLOAT_EQUALITY_THRESHOLD)
		expected = pts_for_lcms2()
	})
}

func TestDevelop(t *testing.T) {
	p := icc.Srgb_lab_profile()
	tr, err := p.CreateTransformerToPCS(icc.PerceptualRenderingIntent)
	require.NoError(t, err)
	r, g, b := tr.Transform(0.5, 0.25, 1)
	fmt.Println(1111111, r, g, b, []float32{45.2933, 58.3075, -85.6426})
}

func TestAgainstLCMS2(t *testing.T) {
	// test_profile(t, "srgb_lab", icc.Srgb_lab_profile_data)
	test_profile(t, "srgb_xyz", icc.Srgb_xyz_profile_data)
}
