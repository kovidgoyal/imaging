//go:build lcms2cgo

package prism

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kovidgoyal/imaging/prism/meta/autometa"
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
	// sRGB XYZ v4 profile matrix/TRC based distributed in Linux
	"sRGB.icc": {srgb_tolerance: 0.4 * THRESHOLD8},
	// sRGB XYZ v4 profile matrix/TRC based, small size
	"sRGB-v4.icc": {srgb_tolerance: 0.35 * THRESHOLD8},
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
	"cmyk.icc": {pcs_tolerance: 2 * THRESHOLD16, inv_tolerance: 0.05 * THRESHOLD8, srgb_tolerance: 0.2 * THRESHOLD8},
	"cmyk.jpg": {pcs_tolerance: 2 * THRESHOLD16, inv_tolerance: 0.05 * THRESHOLD8, srgb_tolerance: 0.05 * THRESHOLD8},
	// Adobe RGB matrix/TRC PCS=XYZ profile
	"adobergb.icc": {inv_tolerance: 0.2 * THRESHOLD8, srgb_tolerance: 0.5 * THRESHOLD8},
	// Apple Display P3 matrix/TRC PCS=XYZ
	"display-p3-v4-with-v2-desc.icc": {srgb_tolerance: 0.45 * THRESHOLD8},
	// Apple Display P3 matrix/TRC PCS=XYZ
	"displayp3.icc": {srgb_tolerance: 0.45 * THRESHOLD8},
	// Adobe RGB matrix/TRC PCS=XYZ profile
	"prophoto.icc": {inv_tolerance: 0.1 * THRESHOLD8, srgb_tolerance: 0.7 * THRESHOLD8},
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

func image_data(t *testing.T, name string) []byte {
	ans, err := os.ReadFile(filepath.Join(testDir(t), "test-images", name))
	require.NoError(t, err)
	return ans
}

func profile(t *testing.T, name string) *icc.Profile {
	ans, err := icc.DecodeProfile(bytes.NewReader(profile_data(t, name)))
	require.NoError(t, err)
	return ans
}

func image_and_profile(t *testing.T, name string) (image.Image, *icc.Profile, *CMSProfile) {
	data := image_data(t, name)
	md, r, err := autometa.Load(bytes.NewReader(data))
	require.NoError(t, err)
	require.NotNil(t, md)
	img, err := jpeg.Decode(r)
	require.NoError(t, err)
	p, err := md.ICCProfile()
	require.NoError(t, err)
	require.NotNil(t, p)
	pd, err := md.ICCProfileData()
	require.NoError(t, err)
	lcms, err := CreateCMSProfile(pd)
	require.NoError(t, err)
	return img, p, lcms
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
func xyz(x float64) float64    { return x / 2. }

var lab_scalers = [3]func(float64) float64{lab_l, lab_ab, lab_ab}

func max_lab_diff(expected, actual []float64) (ans float64) {
	for i := range expected {
		ans = max(ans, math.Abs(lab_scalers[i](expected[i])-lab_scalers[i](actual[i])))
	}
	return
}

func max_xyz_diff(expected, actual []float64) (ans float64) {
	for i := range expected {
		ans = max(ans, math.Abs(xyz(expected[i])-xyz(actual[i])))
	}
	return
}

func in_delta_rgb(t *testing.T, desc string, input_channels, output_channels int, pts, expected, actual []float64, tolerance float64, differ func(e, a []float64) float64) {
	t.Helper()
	var worst struct {
		pts, expected, actual []float64
		max_diff              float64
	}
	for range len(actual) / output_channels {
		if d := differ(expected[:output_channels], actual[:output_channels]); d > tolerance {
			if d > worst.max_diff {
				worst.max_diff = d
				worst.pts, worst.expected, worst.actual = pts[:input_channels], expected[:output_channels], actual[:output_channels]
			}
		}
		expected = expected[output_channels:]
		actual = actual[output_channels:]
		pts = pts[input_channels:]
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

func run_general(p *icc.Pipeline, inp, outp []float64, ni, no, np int) {
	var i, o [4]float64
	for range np {
		copy(i[:], inp[:ni])
		p.TransformGeneral(o[:], i[:])
		copy(outp, o[:no])
		inp = inp[ni:]
		outp = outp[no:]
	}
}

func img_to_floats(img image.Image) []float64 {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	numChannels := 4 // Assuming 4 channels for both CMYK and RGBA

	normalized := make([]float64, width*height*numChannels)
	i := 0

	// Use a type switch to handle CMYK images differently.
	switch img := img.(type) {
	case *image.CMYK:
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				// The At() method for image.CMYK returns a color.CMYK.
				cmykColor := img.At(x, y).(color.CMYK)

				// The C, M, Y, K values are uint8s in the range [0, 255].
				// Normalize them to [0.0, 1.0].
				normalized[i] = float64(cmykColor.C) / 255.0
				normalized[i+1] = float64(cmykColor.M) / 255.0
				normalized[i+2] = float64(cmykColor.Y) / 255.0
				normalized[i+3] = float64(cmykColor.K) / 255.0
				i += numChannels
			}
		}
	default:
		// Fallback for all other image types (RGBA, Gray, etc.).
		// This logic converts them to RGBA.
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				// The RGBA() method returns color values as uint32s in the range [0, 65535].
				r, g, b, a := img.At(x, y).RGBA()

				// Normalize the values to the [0.0, 1.0] range.
				normalized[i] = float64(r) / 65535.0
				normalized[i+1] = float64(g) / 65535.0
				normalized[i+2] = float64(b) / 65535.0
				normalized[i+3] = float64(a) / 65535.0
				i += numChannels
			}
		}
	}
	return normalized
}

func test_profile(t *testing.T, name string) {
	opt := options_for_profile(name)
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		ext := filepath.Ext(name)
		is_image := ext != ".icc" && ext != ".icm"
		var p *icc.Profile
		var pts []float64
		var lcms *CMSProfile
		if is_image {
			var img image.Image
			img, p, lcms = image_and_profile(t, name)
			pts = img_to_floats(img)
		} else {
			p = profile(t, name)
			lcms = lcms_profile(t, name)
		}
		inv, err := p.CreateDefaultTransformerToDevice()
		dev_channels := 3
		if !opt.skip_inv {
			require.NoError(t, err)
			_, dev_channels = inv.IOSig()
		}
		actual_bp := p.BlackPoint(p.Header.RenderingIntent, nil)
		expected_bp, ok := lcms.DetectBlackPoint(p.Header.RenderingIntent)
		require.True(t, ok)
		require.InDeltaSlice(t, []float64{expected_bp.X, expected_bp.Y, expected_bp.Z}, []float64{actual_bp.X, actual_bp.Y, actual_bp.Z}, THRESHOLD16, fmt.Sprintf("blackpoint is not equal: %.6v != %.6v", expected_bp, actual_bp))
		pcs, err := p.CreateDefaultTransformerToPCS(dev_channels)
		require.NoError(t, err)
		srgb, err := p.CreateTransformerToSRGB(p.Header.RenderingIntent, dev_channels, false, false, true)
		require.NoError(t, err)
		if !is_image {
			if dev_channels == 3 {
				pts = icc.Points_for_transformer_comparison3()
			} else {
				pts = icc.Points_for_transformer_comparison4()
			}
		}
		var actual, expected struct{ pcs, inv, srgb []float64 }
		num_pixels := len(pts) / dev_channels
		actual.pcs, actual.inv, actual.srgb = make([]float64, 0, num_pixels*3), make([]float64, 0, num_pixels*dev_channels), make([]float64, 0, num_pixels*3)
		if dev_channels == 3 {
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
			actual.pcs, actual.inv, actual.srgb = actual.pcs[:cap(actual.pcs)], actual.inv[:cap(actual.inv)], actual.srgb[:cap(actual.srgb)]
			run_general(pcs, pts, actual.pcs, dev_channels, 3, num_pixels)
			if !opt.skip_inv {
				run_general(inv, actual.pcs, actual.inv, 3, dev_channels, num_pixels)
			}
			run_general(srgb, pts, actual.srgb, dev_channels, 3, num_pixels)
		}
		lcms_pts := pts
		if dev_channels == 4 {
			lcms_pts = make([]float64, len(pts))
			for i, x := range pts {
				lcms_pts[i] = x * 100
			}
		}
		expected.pcs, err = lcms.TransformFloatToPCS(lcms_pts, p.Header.RenderingIntent)
		require.NoError(t, err)
		if !opt.skip_inv {
			expected.inv, err = lcms.TransformFloatToDevice(actual.pcs, p.Header.RenderingIntent)
			require.NoError(t, err)
			if dev_channels == 4 {
				for i := range expected.inv {
					expected.inv[i] /= 100.
				}
			}
		}
		expected.srgb, err = lcms.TransformFloatToSRGB(lcms_pts, p.Header.RenderingIntent)
		require.NoError(t, err)
		d := icc.IfElse(p.Header.ProfileConnectionSpace == icc.ColorSpaceLab, max_lab_diff, max_xyz_diff)
		in_delta_rgb(t, "to pcs", dev_channels, 3, pts, expected.pcs, actual.pcs, opt.pcs_tolerance, d)
		if !opt.skip_inv {
			in_delta_rgb(t, "to device", 3, dev_channels, actual.pcs, expected.inv, actual.inv, opt.inv_tolerance, max_diff)
		}
		in_delta_rgb(t, "to sRGB", dev_channels, 3, pts, expected.srgb, actual.srgb, opt.srgb_tolerance, max_diff)
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

func develop_to_srgb(p *icc.Profile, lcms *CMSProfile, t *testing.T, name string, tolerance float64) {
	const r, g, b float64 = 1, 0.0666667, 0.2
	l, err := lcms.TransformFloatToSRGB([]float64{r, g, b}, p.Header.RenderingIntent)
	tr, err := p.CreateTransformerToSRGB(p.Header.RenderingIntent, 3, false, false, false)
	require.NoError(t, err)
	x, y, z := tr.TransformDebug(r, g, b, transform_debug)
	fmt.Println()

	in_delta_rgb(t, name+":srgb", 3, 3, []float64{r, g, b}, l, []float64{x, y, z}, tolerance, max_diff)
}

var _ = develop_to_srgb

func develop_inverse(p *icc.Profile, lcms *CMSProfile, t *testing.T, name string, tolerance float64) {
	var r, g, b float64 = 78.2471, -57.496, 10.4908
	l, err := lcms.TransformFloatToDevice([]float64{r, g, b}, p.Header.RenderingIntent)
	fmt.Println()
	tr, err := p.CreateTransformerToDevice(p.Header.RenderingIntent, false)
	require.NoError(t, err)
	x, y, z := tr.TransformDebug(r, g, b, transform_debug)
	fmt.Println()
	in_delta_rgb(t, name+":inverse", 3, 3, []float64{r, g, b}, l, []float64{x, y, z}, tolerance, max_diff)
}

var _ = develop_inverse

func develop_pcs(p *icc.Profile, lcms *CMSProfile, t *testing.T, name string, tolerance float64) {
	const r, g, b float64 = 0.933333, 0.666667, 0.666667
	l, err := lcms.TransformFloatToPCS([]float64{r, g, b}, p.Header.RenderingIntent)
	fmt.Println()
	tr, err := p.CreateTransformerToPCS(p.Header.RenderingIntent, 3, false)
	require.NoError(t, err)
	x, y, z := tr.TransformDebug(r, g, b, transform_debug)
	fmt.Println()
	d := icc.IfElse(p.Header.ProfileConnectionSpace == icc.ColorSpaceLab, max_lab_diff, max_xyz_diff)
	in_delta_rgb(t, name+": pcs", 3, 3, []float64{r, g, b}, l, []float64{x, y, z}, tolerance, d)
}

func develop_pcs4(p *icc.Profile, lcms *CMSProfile, t *testing.T, name string, tolerance float64) {
	const c, m, y, k = 0.7406228373702423, 0.6828604382929645, 0.6656516724336797, 0.8982698961937714
	inp := []float64{c, m, y, k}
	l, err := lcms.TransformFloatToPCS([]float64{c * 100, m * 100, y * 100, k * 100}, p.Header.RenderingIntent)
	fmt.Println()
	tr, err := p.CreateTransformerToPCS(p.Header.RenderingIntent, 4, false)
	require.NoError(t, err)
	out := []float64{0, 0, 0, 0}
	tr.TransformGeneralDebug(out, inp, transform_general_debug)
	fmt.Println()
	in_delta_rgb(t, name+": pcs", 4, 3, inp, l, out[:3], tolerance, max_lab_diff)
}

func develop_inverse4(p *icc.Profile, lcms *CMSProfile, t *testing.T, name string, tolerance float64) {
	var r, g, b float64 = 41.3376, 39.8013, 25.6128
	l, err := lcms.TransformFloatToDevice([]float64{r, g, b}, p.Header.RenderingIntent)
	for i := range l {
		l[i] /= 100.
	}
	fmt.Println()
	tr, err := p.CreateTransformerToDevice(p.Header.RenderingIntent, false)
	require.NoError(t, err)
	cmyk := []float64{0, 0, 0, 0}
	tr.TransformGeneralDebug(cmyk, []float64{r, g, b, 0}, transform_general_debug)
	fmt.Println()
	in_delta_rgb(t, name+":inverse", 3, 4, []float64{r, g, b, 0}, l, cmyk, tolerance, max_diff)
}

func develop_to_srgb4(p *icc.Profile, lcms *CMSProfile, t *testing.T, name string, tolerance float64) {
	const c, m, y, k = 1, 0, 0, 0
	l, err := lcms.TransformFloatToSRGB([]float64{c * 100, m * 100, y * 100, k * 100}, p.Header.RenderingIntent)
	fmt.Println()
	tr, err := p.CreateTransformerToSRGB(p.Header.RenderingIntent, 4, false, false, false)
	require.NoError(t, err)
	out := []float64{0, 0, 0, 0}
	tr.TransformGeneralDebug(out, []float64{c, m, y, k}, transform_general_debug)
	fmt.Println()
	in_delta_rgb(t, name+": srgb", 4, 3, []float64{c, m, y, k}, l, out[:3], tolerance, max_diff)
}

var _ = develop_pcs
var _ = develop_pcs4
var _ = develop_inverse4
var _ = develop_to_srgb4
var _ = develop_blackpoint

func develop_blackpoint(p *icc.Profile, lcms *CMSProfile, t *testing.T) {
	lbp, ok := lcms.DetectBlackPoint(p.Header.RenderingIntent)
	bp := p.BlackPoint(p.Header.RenderingIntent, transform_general_debug)
	if ok {
		require.InDeltaSlice(t, []float64{lbp.X, lbp.Y, lbp.Z}, []float64{bp.X, bp.Y, bp.Z}, THRESHOLD16)
	}
}

// Run this with ./custom-lcms.sh
func TestDevelop(t *testing.T) {
	const name = "prophoto.icc"
	opt := options_for_profile(name)
	p := profile(t, name)
	lcms := lcms_profile(t, name)
	// develop_blackpoint(p, lcms, t)
	if p.Header.DataColorSpace == icc.ColorSpaceCMYK {
		develop_pcs4(p, lcms, t, name, opt.pcs_tolerance)
		// if !opt.skip_inv {
		// 	develop_inverse4(p, lcms, t, name, opt.inv_tolerance)
		// }
		// develop_to_srgb4(p, lcms, t, name, opt.srgb_tolerance)
	} else {
		// develop_pcs(p, lcms, t, name, opt.pcs_tolerance)
		// if !opt.skip_inv {
		// 	develop_inverse(p, lcms, t, name, opt.inv_tolerance)
		// }
		develop_to_srgb(p, lcms, t, name, opt.srgb_tolerance)
	}
}
