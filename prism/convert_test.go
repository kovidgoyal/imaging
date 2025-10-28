//go:build lcms2cgo

package prism

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kovidgoyal/imaging/prism/meta/icc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

const srgb_lab_profile_name = "sRGB_ICC_v4_Appearance.icc"
const srgb_xyz_profile_name = "sRGB2014.icc"

// testDir returns the absolute path to the directory containing the test file.
// It searches the call stack for the first frame whose filename ends with "_test.go".
// Falls back to os.Getwd() if nothing is found (very unlikely).
func testDir(t *testing.T) string {
	t.Helper()
	for skip := range 20 {
		_, file, _, ok := runtime.Caller(skip)
		if !ok {
			break
		}
		if strings.HasSuffix(file, "_test.go") {
			dir, err := filepath.Abs(filepath.Dir(file))
			if err != nil {
				t.Fatalf("failed to get abs path: %v", err)
			}
			return dir
		}
	}
	// Fallback: go test usually runs with working dir == package dir
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}
	return wd
}

func profile_data(t *testing.T, name string) []byte {
	ans, err := os.ReadFile(filepath.Join(testDir(t), "meta", "icc", "test-profiles", name))
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
		require.InDeltaSlice(t, expected[:3], actual[:3], tolerance, fmt.Sprintf("%s: the %dth pixel does not match. Want %.6v got %.6v", desc, i, expected[:3], actual[:3]))
	}
}

func test_profile(t *testing.T, name string, tolerance float64) {
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		p := profile(t, name)
		input_channels := icc.IfElse(p.Header.DataColorSpace == icc.ColorSpaceCMYK, 4, 3)
		lcms := lcms_profile(t, name)
		actual_bp := p.BlackPoint(p.Header.RenderingIntent)
		expected_bp, ok := lcms.DetectBlackPoint(p.Header.RenderingIntent)
		require.True(t, ok)
		require.Equal(t, expected_bp, actual_bp)
		pcs, err := p.CreateDefaultTransformerToPCS(input_channels)
		require.NoError(t, err)
		inv, err := p.CreateDefaultTransformerToDevice()
		require.NoError(t, err)
		srgb, err := p.CreateTransformerToSRGB(p.Header.RenderingIntent, input_channels)
		require.NoError(t, err)
		var pts []float64
		if input_channels == 3 {
			pts = icc.Points_for_transformer_comparison3()
		} else {
			pts = icc.Points_for_transformer_comparison4()
		}
		var actual, expected struct{ pcs, inv, srgb []float64 }
		actual.pcs, actual.inv, actual.srgb = make([]float64, 0, len(pts)), make([]float64, 0, len(pts)), make([]float64, 0, len(pts))
		num_pixels := len(pts) / input_channels
		if input_channels == 3 {
			pos := pts
			for range num_pixels {
				x, y, z := pos[0], pos[1], pos[2]
				r, g, b := pcs.Transform(x, y, z)
				actual.pcs = append(actual.pcs, r, g, b)
				ir, ig, ib := inv.Transform(x, y, z)
				actual.inv = append(actual.inv, ir, ig, ib)
				r, g, b = srgb.Transform(x, y, z)
				actual.srgb = append(actual.srgb, r, g, b)
				pos = pos[3:]
			}
		} else {
			actual.pcs, actual.inv, actual.srgb = actual.pcs[:3*num_pixels], actual.inv[:3*num_pixels], actual.srgb[:3*num_pixels]
			pcs.TransformGeneral(actual.pcs, pts)
			inv.TransformGeneral(actual.inv, pts)
			srgb.TransformGeneral(actual.srgb, pts)
		}
		expected.pcs, err = lcms.TransformFloatToPCS(pts, p.Header.RenderingIntent)
		require.NoError(t, err)
		expected.inv, err = lcms.TransformFloatToDevice(pts, p.Header.RenderingIntent)
		require.NoError(t, err)
		expected.srgb, err = lcms.TransformFloatToSRGB(pts, p.Header.RenderingIntent)
		require.NoError(t, err)
		in_delta_rgb(t, "to pcs", expected.pcs, actual.pcs, tolerance)
		in_delta_rgb(t, "to device", expected.inv, actual.inv, tolerance)
		in_delta_rgb(t, "to sRGB", expected.srgb, actual.srgb, tolerance)
	})
}

func TestAgainstLCMS2(t *testing.T) {
	// simplest case: matrix/trc profile
	test_profile(t, srgb_xyz_profile_name, icc.FLOAT_EQUALITY_THRESHOLD)
	// LutAtoBType profile with PCS=XYZ
	// test_profile(t, "jpegli.icc", icc.FLOAT_EQUALITY_THRESHOLD)
	// LutAtoBType profile with PCS=LAB
	// test_profile(t, srgb_lab_profile_name, 0.0005)
}

func transform_debug(r, g, b, x, y, z float64, t icc.ChannelTransformer) {
	fmt.Printf("\x1b[34m%s\x1b[m\n", t)
	fmt.Printf("  %.6v → %.6v\n", []float64{r, g, b}, []float64{x, y, z})
}

var _ = transform_debug

func develop_to_srgb(t *testing.T, name string) {
	const r, g, b float64 = 0.1, 0.2, 0.3
	p := profile(t, name)
	lcms := lcms_profile(t, name)
	l, err := lcms.TransformFloatToSRGB([]float64{r, g, b}, p.Header.RenderingIntent)
	tr, err := p.CreateTransformerToSRGB(p.Header.RenderingIntent, 3)
	require.NoError(t, err)
	x, y, z := tr.TransformDebug(r, g, b, transform_debug)
	fmt.Println()

	in_delta_rgb(t, name, l, []float64{x, y, z}, 0.001)
}

var _ = develop_to_srgb

func develop_inverse(t *testing.T, name string) {
	const r, g, b float64 = 0.1, 0.2, 0.3
	p := profile(t, name)
	lcms := lcms_profile(t, name)
	l, err := lcms.TransformFloatToDevice([]float64{r, g, b}, p.Header.RenderingIntent)
	fmt.Println()
	tr, err := p.CreateDefaultTransformerToDevice()
	require.NoError(t, err)
	x, y, z := tr.TransformDebug(r, g, b, transform_debug)
	fmt.Println()

	in_delta_rgb(t, name, l, []float64{x, y, z}, 0.001)
}

var _ = develop_inverse

func develop_pcs(t *testing.T, name string) {
	const r, g, b float64 = 0.1, 0.2, 0.3
	p := profile(t, name)
	lcms := lcms_profile(t, name)
	l, err := lcms.TransformFloatToPCS([]float64{r, g, b}, p.Header.RenderingIntent)
	fmt.Println()
	tr, err := p.CreateDefaultTransformerToPCS(3)
	require.NoError(t, err)
	x, y, z := tr.TransformDebug(r, g, b, transform_debug)
	fmt.Println()
	in_delta_rgb(t, name, l, []float64{x, y, z}, 0.001)
}

var _ = develop_pcs

func TestDevelop(t *testing.T) {
	const name = srgb_xyz_profile_name
	develop_to_srgb(t, name)
	develop_pcs(t, name)
	develop_inverse(t, name)
}
