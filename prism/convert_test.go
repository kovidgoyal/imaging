//go:build lcms2cgo

package prism

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kovidgoyal/imaging/prism/meta/icc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

const srgb_lab_profile_name = "sRGB_ICC_v4_Appearance.icc"
const srgb_xyz_profile_name = "sRGB2014.icc"

func profile_data(t *testing.T, name string) []byte {
	ans, err := os.ReadFile(filepath.Join("meta", "icc", "test-profiles", name))
	require.NoError(t, err)
	return ans
}

func profile(t *testing.T, name string) *icc.Profile {
	ans, err := icc.DecodeProfile(bytes.NewReader(profile_data(t, name)))
	require.NoError(t, err)
	return ans
}

func lcms_profile(t *testing.T, name string) *CMSProfile {
	ans, err := CreateCMSProfile(profile_data(t, name))
	require.NoError(t, err)
	return ans
}

func TestCGOConversion(t *testing.T) {
	xyz := lcms_profile(t, srgb_xyz_profile_name)
	require.Equal(t, xyz.DeviceColorSpace, icc.RGBSignature)
	require.Equal(t, xyz.PCSColorSpace, icc.XYZSignature)
	defer xyz.Close()
	lab := lcms_profile(t, srgb_lab_profile_name)
	require.Equal(t, lab.DeviceColorSpace, icc.RGBSignature)
	require.Equal(t, lab.PCSColorSpace, icc.LabSignature)
	defer lab.Close()
	_, err := CreateCMSProfile([]byte("invalid profile"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Got 15 bytes, block should be")

	expected := []float64{0.2569, 0.1454, 0.7221}
	pcs, err := xyz.TransformRGB8bitToPCS([]byte{128, 64, 255}, icc.RelativeColorimetricRenderingIntent)
	require.NoError(t, err)
	assert.InDeltaSlice(t, pcs, expected, 0.001)
	pcs, err = xyz.TransformFloatToPCS([]float64{128 / 255., 64 / 255., 255 / 255.}, icc.RelativeColorimetricRenderingIntent)
	require.NoError(t, err)
	assert.InDeltaSlice(t, pcs, expected, 0.001)
	pcs, err = lab.TransformRGB8bitToPCS([]byte{128, 64, 255}, icc.PerceptualRenderingIntent)
	require.NoError(t, err)
	assert.InDeltaSlice(t, pcs, []float64{43.643852, 46.361866, -73.44747}, 0.001)

	for _, p := range []*CMSProfile{xyz, lab} {
		inp := []byte{128, 64, 255}
		out, err := p.TransformRGB8(inp, p, icc.RelativeColorimetricRenderingIntent)
		require.NoError(t, err)
		assert.Equal(t, inp, out)
	}
}

func in_delta_rgb(t *testing.T, desc string, expected, actual []float64, tolerance float64) {
	t.Helper()
	for i := range len(actual) / 3 {
		require.InDeltaSlice(t, expected[:3], actual[:3], tolerance, fmt.Sprintf("%s: the %dth pixel does not match. Want %v got %v", desc, i, expected[:3], actual[:3]))
	}
}

func test_profile(t *testing.T, name string, tolerance float64, inverse_tolerance float64) {
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		p := profile(t, name)
		input_channels := icc.IfElse(p.Header.DataColorSpace == icc.ColorSpaceCMYK, 4, 3)
		lcms := lcms_profile(t, name)
		actual_bp := p.BlackPoint(p.Header.RenderingIntent)
		expected_bp, ok := lcms.DetectBlackPoint(p.Header.RenderingIntent)
		require.True(t, ok)
		require.Equal(t, expected_bp, actual_bp)
		tr, err := p.CreateDefaultTransformerToPCS(input_channels)
		require.NoError(t, err)
		inv, err := p.CreateDefaultTransformerToDevice()
		require.NoError(t, err)
		var pts []float64
		if input_channels == 3 {
			pts = icc.Points_for_transformer_comparison3()
		} else {
			pts = icc.Points_for_transformer_comparison4()
		}
		num_pixels := len(pts) / input_channels
		actual := make([]float64, 0, len(pts))
		if input_channels == 3 {
			pos := pts
			for range num_pixels {
				sl := pos[0:3:3]
				r, g, b := tr.Transform(sl[0], sl[1], sl[2])
				actual = append(actual, r, g, b)
				r, g, b = inv.Transform(r, g, b)
				in_delta_rgb(t, "a2b + b2a roundtrip", sl, []float64{r, g, b}, inverse_tolerance)
				pos = pos[input_channels:]
			}
		} else {
			actual = actual[:3*num_pixels]
			tr.TransformGeneral(actual, pts)
		}
		expected, err := lcms.TransformFloatToPCS(pts, p.Header.RenderingIntent)
		require.NoError(t, err)
		in_delta_rgb(t, "to pcs", expected, actual, tolerance)
		tr, err = p.CreateTransformerToSRGB(p.Header.RenderingIntent, input_channels)
		require.NoError(t, err)
		expected, err = lcms.TransformFloatToSRGB(pts, p.Header.RenderingIntent)
		require.NoError(t, err)
		if input_channels == 3 {
			actual := actual[:0]
			pos := pts
			for range num_pixels {
				sl := pos[0:3:3]
				r, g, b := tr.Transform(sl[0], sl[1], sl[2])
				actual = append(actual, r, g, b)
				pos = pos[3:]
			}
		} else {
			actual = actual[:3*num_pixels]
			tr.TransformGeneral(actual, pts)
		}
		in_delta_rgb(t, "to sRGB", expected, actual, tolerance)
	})
}

func TestDevelop(t *testing.T) {
}

func TestAgainstLCMS2(t *testing.T) {
	test_profile(t, srgb_lab_profile_name, 0.0005, 0.3)
	test_profile(t, srgb_xyz_profile_name, icc.FLOAT_EQUALITY_THRESHOLD, icc.FLOAT_EQUALITY_THRESHOLD)
}
