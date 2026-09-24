//go:build lcms2cgo

package prism

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kovidgoyal/imaging/prism/meta/icc"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

var d50 = icc.XYZType{X: 0.9642, Y: 1, Z: 0.8249}
var d65 = icc.XYZType{X: 0.9505, Y: 1, Z: 1.0890}

type gray_test_case struct {
	name    string
	spec    GrayProfileSpec
	is_srgb bool
	// multiples of THRESHOLD16, defaulting to 1
	pcs_tolerance, inv_tolerance, srgb_tolerance float64
}

func (tc gray_test_case) tolerances() (pcs, inv, srgb float64) {
	d := func(x float64) float64 { return icc.IfElse(x == 0, 1, x) * THRESHOLD16 }
	return d(tc.pcs_tolerance), d(tc.inv_tolerance), d(tc.srgb_tolerance)
}

func gray_test_cases() []gray_test_case {
	xyz, lab := icc.XYZSignature, icc.LabSignature
	return []gray_test_case{
		{name: "v4-xyz-d50-gamma2.2", spec: GrayProfileSpec{Version: 4.3, WhitePoint: d50, Gamma: 2.2, PCS: xyz}},
		{name: "v4-xyz-d50-srgb", spec: GrayProfileSpec{Version: 4.3, WhitePoint: d50, Curve: SRGBGrayCurve, PCS: xyz}, is_srgb: true},
		// v2 has no parametric curves so lcms stores the sRGB curve as a table,
		// lcms evaluates tables with 16 bit precision
		{name: "v2-xyz-d50-srgb", spec: GrayProfileSpec{Version: 2.1, WhitePoint: d50, Curve: SRGBGrayCurve, PCS: xyz}, is_srgb: true, inv_tolerance: 12, srgb_tolerance: 10},
		{name: "v4-xyz-d50-tabulated", spec: GrayProfileSpec{Version: 4.3, WhitePoint: d50, Curve: TabulatedGrayCurve, PCS: xyz}, inv_tolerance: 24, srgb_tolerance: 5},
		{name: "v4-xyz-d50-identity", spec: GrayProfileSpec{Version: 4.3, WhitePoint: d50, Curve: IdentityGrayCurve, PCS: xyz}},
		{name: "v2-xyz-d65-gamma1.8-display", spec: GrayProfileSpec{Version: 2.1, WhitePoint: d65, Gamma: 1.8, PCS: xyz}},
		{name: "v2-xyz-d65-gamma2.2-output", spec: GrayProfileSpec{Version: 2.1, WhitePoint: d65, Gamma: 2.2, PCS: xyz, DeviceClass: icc.Signature(icc.DeviceClassOutput)}},
		{name: "v4-lab-d50-gamma2.2", spec: GrayProfileSpec{Version: 4.3, WhitePoint: d50, Gamma: 2.2, PCS: lab}},
		{name: "v2-lab-d50-srgb", spec: GrayProfileSpec{Version: 2.1, WhitePoint: d50, Curve: SRGBGrayCurve, PCS: lab}, pcs_tolerance: 2, inv_tolerance: 12, srgb_tolerance: 3},
		// lcms quantizes the inputs of 16 bit CLUTs to 16 bits even for
		// floating point transforms
		{name: "v4-lut-lab", spec: GrayProfileSpec{Version: 4.3, WhitePoint: d50, Gamma: 2.2, PCS: lab, UseLUT: true}, pcs_tolerance: 5, inv_tolerance: 64, srgb_tolerance: 8},
		{name: "v4-lut-xyz", spec: GrayProfileSpec{Version: 4.3, WhitePoint: d50, Curve: SRGBGrayCurve, PCS: xyz, UseLUT: true}, pcs_tolerance: 2, inv_tolerance: 18, srgb_tolerance: 72},
		{name: "v2-lut-lab", spec: GrayProfileSpec{Version: 2.1, WhitePoint: d50, Gamma: 1.8, PCS: lab, UseLUT: true}, pcs_tolerance: 5, inv_tolerance: 25, srgb_tolerance: 8},
		{name: "v2-lut-xyz", spec: GrayProfileSpec{Version: 2.1, WhitePoint: d50, Curve: TabulatedGrayCurve, PCS: xyz, UseLUT: true}, pcs_tolerance: 2, inv_tolerance: 8, srgb_tolerance: 72},
	}
}

var all_intents = []icc.RenderingIntent{icc.PerceptualRenderingIntent, icc.RelativeColorimetricRenderingIntent, icc.SaturationRenderingIntent, icc.AbsoluteColorimetricRenderingIntent}

func gray_points() []float64 {
	ans := make([]float64, 0, 1024)
	for i := range 1024 {
		ans = append(ans, float64(i)/1023)
	}
	return ans
}

// Run a pipeline using both Transform and TransformGeneral, ensuring they give the same result
func run_both(t *testing.T, p *icc.Pipeline, inp []float64, ni, no int) []float64 {
	t.Helper()
	np := len(inp) / ni
	ans := make([]float64, np*no)
	run_general(p, inp, ans, ni, no, np)
	for i := range np {
		var in [3]float64
		copy(in[:], inp[i*ni:(i+1)*ni])
		r, g, b := p.Transform(in[0], in[1], in[2])
		require.InDeltaSlice(t, ans[i*no:(i+1)*no], []float64{r, g, b}[:no], 1e-12, "Transform() and TransformGeneral() differ for input: %v in pipeline: %s", in[:ni], p)
	}
	return ans
}

func TestGrayAgainstLCMS2(t *testing.T) {
	for _, tc := range gray_test_cases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := CreateGrayProfile(tc.spec)
			require.NoError(t, err)
			p, err := icc.DecodeProfile(bytes.NewReader(data))
			require.NoError(t, err)
			require.Equal(t, icc.ColorSpaceGray, p.Header.DataColorSpace)
			require.Equal(t, tc.is_srgb, p.IsSRGB())
			lcms, err := CreateCMSProfile(data)
			require.NoError(t, err)
			defer lcms.Close()
			pts := gray_points()
			pcs_diff := icc.IfElse(p.Header.ProfileConnectionSpace == icc.ColorSpaceLab, max_lab_diff, max_xyz_diff)
			for _, intent := range all_intents {
				t.Run(intent.String(), func(t *testing.T) {
					pcs_tolerance, inv_tolerance, srgb_tolerance := tc.tolerances()
					if intent != icc.AbsoluteColorimetricRenderingIntent {
						// lcms returns a zero black point when it fails to detect it
						expected_bp, _ := lcms.DetectBlackPoint(intent)
						actual_bp := p.BlackPoint(intent, nil)
						require.InDeltaSlice(t, []float64{expected_bp.X, expected_bp.Y, expected_bp.Z}, []float64{actual_bp.X, actual_bp.Y, actual_bp.Z}, THRESHOLD16, "blackpoint is not equal: %.6v != %.6v", expected_bp, actual_bp)
					}

					pcs, err := p.CreateTransformerToPCS(intent, 1, true)
					require.NoError(t, err)
					actual := run_both(t, pcs, pts, 1, 3)
					// lcms does not apply the absolute colorimetric
					// adaptation when transforming to the PCS as
					// that is done when linking two profiles. The
					// absolute intent is tested by the conversion to sRGB.
					if intent != icc.AbsoluteColorimetricRenderingIntent {
						expected, err := lcms.TransformFloatToPCS(pts, intent)
						require.NoError(t, err)
						in_delta_rgb(t, "to PCS", 1, 3, pts, expected, actual, pcs_tolerance, pcs_diff)
					}

					inv, err := p.CreateTransformerToDevice(intent, false, true)
					require.NoError(t, err)
					actual_inv := run_both(t, inv, actual, 3, 1)
					expected, err := lcms.TransformFloatToDevice(actual, intent)
					require.NoError(t, err)
					in_delta_rgb(t, "to device", 3, 1, actual, expected, actual_inv, inv_tolerance, max_diff)
					if !tc.spec.UseLUT && (p.BlackPoint(intent, nil) == (icc.XYZType{}) || intent == icc.AbsoluteColorimetricRenderingIntent || intent == icc.RelativeColorimetricRenderingIntent) {
						// black point compensation is not done when
						// transforming to the PCS, so only do a round trip
						// test when it has no effect. The B2A LUTs in the
						// test profiles are too coarse to invert the A2B
						// LUTs accurately.
						in_delta_rgb(t, "round trip", 1, 1, pts, pts, actual_inv, inv_tolerance, max_diff)
					}

					srgb, err := p.CreateTransformerToSRGB(intent, false, 1, false, false, true)
					require.NoError(t, err)
					actual = run_both(t, srgb, pts, 1, 3)
					expected, err = lcms.TransformFloatToSRGB(pts, intent)
					require.NoError(t, err)
					in_delta_rgb(t, "to sRGB", 1, 3, pts, expected, actual, srgb_tolerance, max_diff)
				})
			}
		})
	}
}

// The monochrome profiles in test-profiles are generated with lcms using:
// GENERATE_GRAY_PROFILES=1 go test -tags lcms2cgo -run TestGenerateGrayProfiles ./prism
var gray_fixture_profiles = map[string]GrayProfileSpec{
	"gray-v4-srgb.icc":           {Version: 4.3, WhitePoint: d50, Curve: SRGBGrayCurve, PCS: icc.XYZSignature},
	"gray-v2-gamma1.8-d65.icc":   {Version: 2.1, WhitePoint: d65, Gamma: 1.8, PCS: icc.XYZSignature},
	"gray-v2-tabulated-prtr.icc": {Version: 2.1, WhitePoint: d50, Curve: TabulatedGrayCurve, PCS: icc.XYZSignature, DeviceClass: icc.Signature(icc.DeviceClassOutput)},
	"gray-v4-lab-gamma2.2.icc":   {Version: 4.3, WhitePoint: d50, Gamma: 2.2, PCS: icc.LabSignature},
	"gray-v4-lut.icc":            {Version: 4.3, WhitePoint: d50, Gamma: 2.2, PCS: icc.LabSignature, UseLUT: true},
	"gray-v2-lut.icc":            {Version: 2.1, WhitePoint: d50, Gamma: 1.8, PCS: icc.LabSignature, UseLUT: true},
}

func TestGenerateGrayProfiles(t *testing.T) {
	if os.Getenv("GENERATE_GRAY_PROFILES") != "1" {
		t.Skip("set GENERATE_GRAY_PROFILES=1 to generate the monochrome test profiles")
	}
	for name, spec := range gray_fixture_profiles {
		data, err := CreateGrayProfile(spec)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(testDir(t), "meta", "icc", "test-profiles", name), data, 0o644))
	}
}
