//go:build lcms2cgo

package prism

import (
	"bytes"
	"fmt"
	"math"
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
const THRESHOLD16 = 1. / math.MaxUint16
const THRESHOLD8 = 1. / math.MaxUint8

var profiles = map[string]opt{
	// simplest case: matrix/trc profile
	srgb_xyz_profile_name: {srgb_tolerance: 0.4 * THRESHOLD8, inv_tolerance: 6 * THRESHOLD16},
	// LutAtoBType profile with PCS=XYZ (need higher tolerance for
	// because of lcms does interpolation and matrix calculation using 16bit
	// numbers, we use 64bit numbers)
	"jpegli.icc": {pcs_tolerance: 0.07 * THRESHOLD8, inv_tolerance: 0.25 * THRESHOLD8, srgb_tolerance: 0.94 * THRESHOLD8},
	// LutAtoBType profile with PCS=LAB. Their is some numerical
	// instability in the lcms code because of conversion to and from float
	// and fixed point representations, hence higher tolerances.
	srgb_lab_profile_name: {inv_tolerance: 0.05 * THRESHOLD8, srgb_tolerance: 0.35 * THRESHOLD8},
	// profile created by lcms to check browser compatibility using V2 LUTs
	"lcms-check-lut.icc": {skip_inv: true, srgb_tolerance: 5 * THRESHOLD16},
	// profile created by lcms to check browser compatibility uses both matrix and LUTs
	"lcms-check-full.icc": {skip_inv: true, srgb_tolerance: 5 * THRESHOLD16},
	// profile created by lcms to check browser compatibility uses v4
	// constructs uses LutAtoBType with PCS=LAB
	"lcms-stress.icc": {skip_inv: true, pcs_tolerance: 2 * THRESHOLD16, srgb_tolerance: 4 * THRESHOLD16},
	// CMYK profile using LAB space
	"cmyk.icc": {skip_inv: true, pcs_tolerance: 2 * THRESHOLD16, srgb_tolerance: 4 * THRESHOLD16},
}

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

func max_diff(expected, actual []float64) (ans float64) {
	for i := range expected {
		ans = max(ans, math.Abs(expected[i]-actual[i]))
	}
	return
}

func lab_l(l float64) float64  { return l / 100 }
func lab_ab(a float64) float64 { return a*(1./255) + 128./255 }

var lab_scalers = [3]func(float64) float64{lab_l, lab_ab, lab_ab}

func max_lab_diff(expected, actual []float64) (ans float64) {
	for i := range expected {
		ans = max(ans, math.Abs(lab_scalers[i](expected[i])-lab_scalers[i](actual[i])))
	}
	return
}

func in_delta_rgb(t *testing.T, desc string, num_channels int, pts, expected, actual []float64, tolerance float64, is_lab bool) {
	t.Helper()
	var worst struct {
		pts, expected, actual []float64
		max_diff              float64
	}
	df := icc.IfElse(is_lab, max_lab_diff, max_diff)
	for range len(actual) / num_channels {
		if d := df(expected[:num_channels], actual[:num_channels]); d > tolerance {
			if d > worst.max_diff {
				worst.max_diff = d
				worst.pts, worst.expected, worst.actual = pts[:num_channels], expected[:num_channels], actual[:num_channels]
			}
		}
		expected = expected[num_channels:]
		actual = actual[num_channels:]
		pts = pts[num_channels:]
	}
	if worst.max_diff > 0 {
		t.Fatalf("%s: the pixel %.6v does not match.\nThe max difference is %d times the tolerance of %.2v.\nWant %.6v got %.6v",
			desc, worst.pts, int(math.Ceil(worst.max_diff/tolerance)), tolerance, worst.expected, worst.actual)
	}
}

type opt struct {
	pcs_tolerance, inv_tolerance, srgb_tolerance float64
	skip_inv                                     bool
}

func options_for_profile(name string) opt {
	opt := profiles[name]
	if opt.pcs_tolerance == 0 {
		opt.pcs_tolerance = THRESHOLD16
	}
	if opt.inv_tolerance == 0 {
		opt.inv_tolerance = THRESHOLD16
	}
	if opt.srgb_tolerance == 0 {
		opt.srgb_tolerance = THRESHOLD16
	}
	return opt
}

func test_profile(t *testing.T, name string) {
	opt := options_for_profile(name)
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
		if !opt.skip_inv {
			require.NoError(t, err)
		}
		srgb, err := p.CreateTransformerToSRGB(p.Header.RenderingIntent, input_channels, false, false, true)
		require.NoError(t, err)
		var pts []float64
		if input_channels == 3 {
			pts = icc.Points_for_transformer_comparison3()
		} else {
			pts = icc.Points_for_transformer_comparison4()
		}
		var actual, expected struct{ pcs, inv, srgb []float64 }
		num_pixels := len(pts) / input_channels
		actual.pcs, actual.inv, actual.srgb = make([]float64, 0, num_pixels*3), make([]float64, 0, num_pixels*input_channels), make([]float64, 0, num_pixels*3)
		if input_channels == 3 {
			pos := pts
			for range num_pixels {
				x, y, z := pos[0], pos[1], pos[2]
				r, g, b := pcs.Transform(x, y, z)
				actual.pcs = append(actual.pcs, r, g, b)
				if !opt.skip_inv {
					ir, ig, ib := inv.Transform(r, g, b)
					actual.inv = append(actual.inv, ir, ig, ib)
				}
				r, g, b = srgb.Transform(x, y, z)
				actual.srgb = append(actual.srgb, r, g, b)
				pos = pos[3:]
			}
		} else {
			actual.pcs, actual.inv, actual.srgb = actual.pcs[:3*cap(actual.pcs)], actual.inv[:cap(actual.inv)], actual.srgb[:cap(actual.srgb)]
			pcs.TransformGeneral(actual.pcs, pts)
			if !opt.skip_inv {
				inv.TransformGeneral(actual.inv, actual.pcs)
			}
			srgb.TransformGeneral(actual.srgb, pts)
		}
		expected.pcs, err = lcms.TransformFloatToPCS(pts, p.Header.RenderingIntent)
		require.NoError(t, err)
		if !opt.skip_inv {
			expected.inv, err = lcms.TransformFloatToDevice(actual.pcs, p.Header.RenderingIntent)
			require.NoError(t, err)
		}
		expected.srgb, err = lcms.TransformFloatToSRGB(pts, p.Header.RenderingIntent)
		require.NoError(t, err)
		in_delta_rgb(t, "to pcs", input_channels, pts, expected.pcs, actual.pcs, opt.pcs_tolerance, p.Header.ProfileConnectionSpace == icc.ColorSpaceLab)
		if !opt.skip_inv {
			in_delta_rgb(t, "to device", input_channels, actual.pcs, expected.inv, actual.inv, opt.inv_tolerance, false)
		}
		in_delta_rgb(t, "to sRGB", input_channels, pts, expected.srgb, actual.srgb, opt.srgb_tolerance, false)
	})
}

func TestAgainstLCMS2(t *testing.T) {
	for name := range profiles {
		test_profile(t, name)
	}
}

func transform_debug(r, g, b, x, y, z float64, t icc.ChannelTransformer) {
	fmt.Printf("\x1b[34m%s\x1b[m\n", t)
	fmt.Printf("  %.6v → %.6v\n", []float64{r, g, b}, []float64{x, y, z})
}

func transform_general_debug(inp, outp []float64, t icc.ChannelTransformer) {
	fmt.Printf("\x1b[34m%s\x1b[m\n", t)
	fmt.Printf("  %.6v → %.6v\n", inp, outp)
}

var _ = transform_debug

func develop_to_srgb(p *icc.Profile, t *testing.T, name string, tolerance float64) {
	const r, g, b float64 = 1, 0.0666667, 0.2
	lcms := lcms_profile(t, name)
	l, err := lcms.TransformFloatToSRGB([]float64{r, g, b}, p.Header.RenderingIntent)
	tr, err := p.CreateTransformerToSRGB(p.Header.RenderingIntent, 3, false, false, false)
	require.NoError(t, err)
	x, y, z := tr.TransformDebug(r, g, b, transform_debug)
	fmt.Println()

	in_delta_rgb(t, name+":srgb", 3, []float64{r, g, b}, l, []float64{x, y, z}, tolerance, false)
}

var _ = develop_to_srgb

func develop_inverse(p *icc.Profile, t *testing.T, name string, tolerance float64) {
	var r, g, b float64 = 78.2471, -57.496, 10.4908
	lcms := lcms_profile(t, name)
	l, err := lcms.TransformFloatToDevice([]float64{r, g, b}, p.Header.RenderingIntent)
	fmt.Println()
	tr, err := p.CreateTransformerToDevice(p.Header.RenderingIntent, false)
	require.NoError(t, err)
	x, y, z := tr.TransformDebug(r, g, b, transform_debug)
	fmt.Println()
	in_delta_rgb(t, name+":inverse", 3, []float64{r, g, b}, l, []float64{x, y, z}, tolerance, false)
}

var _ = develop_inverse

func develop_pcs(p *icc.Profile, t *testing.T, name string, tolerance float64) {
	const r, g, b float64 = 0.933333, 0.666667, 0.666667
	lcms := lcms_profile(t, name)
	l, err := lcms.TransformFloatToPCS([]float64{r, g, b}, p.Header.RenderingIntent)
	fmt.Println()
	tr, err := p.CreateTransformerToPCS(p.Header.RenderingIntent, 3, false)
	require.NoError(t, err)
	x, y, z := tr.TransformDebug(r, g, b, transform_debug)
	fmt.Println()
	in_delta_rgb(t, name+": pcs", 3, []float64{r, g, b}, l, []float64{x, y, z}, tolerance, true)
}

func develop_pcs4(p *icc.Profile, t *testing.T, name string, tolerance float64) {
	const c, m, y, k = 0.933333, 0.666667, 0.666667, 0.5
	lcms := lcms_profile(t, name)
	inp := []float64{c, m, y, k}
	l, err := lcms.TransformFloatToPCS(inp, p.Header.RenderingIntent)
	fmt.Println()
	tr, err := p.CreateTransformerToPCS(p.Header.RenderingIntent, 4, false)
	require.NoError(t, err)
	out := []float64{0, 0, 0, 0}
	tr.TransformGeneralDebug(out, inp, transform_general_debug)
	fmt.Println()
	in_delta_rgb(t, name+": pcs", 4, inp, l, out[:3], tolerance, true)
}

var _ = develop_pcs

// Run this with ./custom-lcms.sh
func TestDevelop(t *testing.T) {
	const name = "cmyk.icc"
	opt := options_for_profile(name)
	p := profile(t, name)
	if p.Header.DataColorSpace == icc.ColorSpaceCMYK {
		develop_pcs4(p, t, name, opt.pcs_tolerance)
	} else {
		develop_pcs(p, t, name, opt.pcs_tolerance)
		// if !opt.skip_inv {
		// 	develop_inverse(p, t, name, opt.inv_tolerance)
		// }
		// develop_to_srgb(p, t, name, opt.srgb_tolerance)
	}
}
