package icc

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

var gray_profiles = []string{
	"gray-v4-srgb.icc", "gray-v2-gamma1.8-d65.icc", "gray-v2-tabulated-prtr.icc",
	"gray-v4-lab-gamma2.2.icc", "gray-v4-lut.icc", "gray-v2-lut.icc",
}

var all_intents = []RenderingIntent{PerceptualRenderingIntent, RelativeColorimetricRenderingIntent, SaturationRenderingIntent, AbsoluteColorimetricRenderingIntent}

func gray_profile_data(t *testing.T, name string) []byte {
	data, err := os.ReadFile("test-profiles/" + name)
	require.NoError(t, err)
	return data
}

func decode_test_profile(t *testing.T, data []byte) *Profile {
	p, err := DecodeProfile(bytes.NewReader(data))
	require.NoError(t, err)
	return p
}

// replace the only occurrence of old in data with new
func patch_profile(t *testing.T, data []byte, old, new []byte) []byte {
	require.Equal(t, 1, bytes.Count(data, old), "%q not found exactly once", old)
	return bytes.Replace(bytes.Clone(data), old, new, 1)
}

func srgb_decode(v unit_float) unit_float {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// run a gray pipeline via both Transform and TransformGeneral checking that
// they agree
func gray_transform(t *testing.T, p *Pipeline, v unit_float) (ans [3]unit_float) {
	t.Helper()
	var i, o [4]unit_float
	i[0] = v
	p.TransformGeneral(o[:], i[:])
	r, g, b := p.Transform(v, 0, 0)
	_, no := p.IOSig()
	require.InDeltaSlice(t, o[:no], []unit_float{r, g, b}[:no], 1e-12, "Transform() and TransformGeneral() differ for %v in %s", v, p)
	copy(ans[:], o[:3])
	return
}

func TestGrayProfileHeaderAndTags(t *testing.T) {
	for _, name := range gray_profiles {
		p := decode_test_profile(t, gray_profile_data(t, name))
		require.Equal(t, ColorSpaceGray, p.Header.DataColorSpace, name)
		is_lut := name == "gray-v4-lut.icc" || name == "gray-v2-lut.icc"
		require.Equal(t, !is_lut, p.IsMatrixShaper(), name)
		require.Equal(t, name == "gray-v4-srgb.icc", p.IsSRGB(), name)
		for _, intent := range all_intents {
			pcs, err := p.CreateTransformerToPCS(intent, 1, true)
			require.NoError(t, err)
			i, o := pcs.IOSig()
			require.Equal(t, [2]int{1, 3}, [2]int{i, o}, "%s: %s", name, pcs)
			dev, err := p.CreateTransformerToDevice(intent, false, true)
			require.NoError(t, err)
			i, o = dev.IOSig()
			require.Equal(t, [2]int{3, 1}, [2]int{i, o}, "%s: %s", name, dev)
			srgb, err := p.CreateTransformerToSRGB(intent, false, 1, true, true, true)
			require.NoError(t, err)
			i, o = srgb.IOSig()
			require.Equal(t, [2]int{1, 3}, [2]int{i, o}, "%s: %s", name, srgb)
			_, err = p.CreateTransformerToSRGB(intent, false, 3, true, true, true)
			require.Error(t, err, "a monochrome profile must not be usable with 3 input channels")
		}
	}
}

func TestGrayMatrixTRCToPCS(t *testing.T) {
	// v2 curv tags store gamma as u8Fixed8Number
	gamma18 := func(v unit_float) unit_float { return math.Pow(v, 460./256) }
	gamma22 := func(v unit_float) unit_float { return math.Pow(v, 2.2) }
	d65 := XYZType{0.9505, 1, 1.0890}
	for _, tc := range []struct {
		name    string
		trc     func(unit_float) unit_float
		white   XYZType // for the absolute intent
		patches [][2][]byte
	}{
		{name: "gray-v4-srgb.icc", trc: srgb_decode, white: lcms_d50},
		{name: "gray-v2-gamma1.8-d65.icc", trc: gamma18, white: lcms_d50},
		// the media white point of v2 display profiles is taken to be D50,
		// but not for other classes
		{name: "gray-v2-gamma1.8-d65.icc", trc: gamma18, white: d65, patches: [][2][]byte{{[]byte("mntrGRAY"), []byte("prtrGRAY")}}},
		{name: "gray-v2-gamma1.8-d65.icc", trc: func(v unit_float) unit_float { return v }, white: lcms_d50, patches: [][2][]byte{
			{[]byte("curv\x00\x00\x00\x00\x00\x00\x00\x01\x01\xcc"), []byte("curv\x00\x00\x00\x00\x00\x00\x00\x01\x01\x00")}}},
		{name: "gray-v4-lab-gamma2.2.icc", trc: gamma22, white: lcms_d50},
	} {
		data := gray_profile_data(t, tc.name)
		for _, p := range tc.patches {
			data = patch_profile(t, data, p[0], p[1])
		}
		p := decode_test_profile(t, data)
		is_lab := p.Header.ProfileConnectionSpace == ColorSpaceLab
		for _, intent := range all_intents {
			pcs, err := p.CreateTransformerToPCS(intent, 1, true)
			require.NoError(t, err)
			white := IfElse(intent == AbsoluteColorimetricRenderingIntent, tc.white, lcms_d50)
			for i := range 256 {
				v := unit_float(i) / 255
				y := tc.trc(v)
				actual := gray_transform(t, pcs, v)
				var expected [3]unit_float
				if is_lab {
					// for LAB the TRC gives L* directly
					expected = [3]unit_float{100 * y, 0, 0}
					require.InDeltaSlice(t, expected[:], actual[:], 1e-4, "%s: %s: gray: %v", tc.name, intent, v)
				} else {
					expected = [3]unit_float{white.X * y, white.Y * y, white.Z * y}
					// table based curves are evaluated with 16 bit precision
					require.InDeltaSlice(t, expected[:], actual[:], 1e-4, "%s: %s: gray: %v", tc.name, intent, v)
				}
			}
		}
	}
}

func TestGrayRoundTrip(t *testing.T) {
	for _, name := range gray_profiles {
		if name == "gray-v4-lut.icc" || name == "gray-v2-lut.icc" {
			// The B2A LUTs in these profiles are too coarse to accurately
			// invert the A2B LUTs, they are tested against lcms instead
			continue
		}
		p := decode_test_profile(t, gray_profile_data(t, name))
		// no black point compensation happens for these intents
		for _, intent := range []RenderingIntent{RelativeColorimetricRenderingIntent, AbsoluteColorimetricRenderingIntent} {
			pcs, err := p.CreateTransformerToPCS(intent, 1, true)
			require.NoError(t, err)
			dev, err := p.CreateTransformerToDevice(intent, false, true)
			require.NoError(t, err)
			for i := range 1024 {
				v := unit_float(i) / 1023
				x := gray_transform(t, pcs, v)
				var in, o [4]unit_float
				copy(in[:], x[:])
				dev.TransformGeneral(o[:], in[:])
				r, _, _ := dev.Transform(x[0], x[1], x[2])
				require.InDelta(t, o[0], r, 1e-12, "Transform() and TransformGeneral() differ for %v in %s", x, dev)
				require.InDelta(t, v, o[0], 5e-4, "%s: %s: gray: %v", name, intent, v)
			}
		}
	}
}

func TestGrayToSRGBIsNeutralAndMonotonic(t *testing.T) {
	for _, name := range gray_profiles {
		p := decode_test_profile(t, gray_profile_data(t, name))
		is_lut := name == "gray-v4-lut.icc" || name == "gray-v2-lut.icc"
		for _, intent := range all_intents {
			srgb, err := p.CreateTransformerToSRGB(intent, false, 1, true, true, true)
			require.NoError(t, err)
			prev := unit_float(-1)
			for i := range 1024 {
				v := unit_float(i) / 1023
				c := gray_transform(t, srgb, v)
				if !is_lut {
					// the LUT based profiles are tinted. The tolerance
					// is for the difference between D50 and the
					// s15Fixed16Number encoded PCS illuminant in the
					// profile header used for the conversion to sRGB
					require.InDelta(t, c[0], c[1], 1e-5, "%s: %s: gray: %v -> %v", name, intent, v, c)
					require.InDelta(t, c[1], c[2], 1e-5, "%s: %s: gray: %v -> %v", name, intent, v, c)
				}
				require.GreaterOrEqual(t, c[1], prev, "%s: %s: gray: %v -> %v", name, intent, v, c)
				prev = c[1]
			}
			black, white := gray_transform(t, srgb, 0), gray_transform(t, srgb, 1)
			if !is_lut {
				require.InDelta(t, 1, white[1], 1e-5, "%s: %s", name, intent)
			}
			bp := p.BlackPoint(intent, nil)
			if intent == AbsoluteColorimetricRenderingIntent || bp == (XYZType{}) {
				require.InDelta(t, 0, black[1], 1e-5, "%s: %s", name, intent)
			}
		}
	}
	// sRGB gray is the identity
	p := decode_test_profile(t, gray_profile_data(t, "gray-v4-srgb.icc"))
	srgb, err := p.CreateTransformerToSRGB(RelativeColorimetricRenderingIntent, false, 1, true, true, true)
	require.NoError(t, err)
	for i := range 256 {
		v := unit_float(i) / 255
		c := gray_transform(t, srgb, v)
		require.InDeltaSlice(t, []unit_float{v, v, v}, c[:], 1e-5)
	}
}

func TestGrayBlackPoint(t *testing.T) {
	for _, name := range gray_profiles {
		p := decode_test_profile(t, gray_profile_data(t, name))
		for _, intent := range all_intents {
			bp := p.BlackPoint(intent, nil)
			if name == "gray-v4-lut.icc" && (intent == PerceptualRenderingIntent || intent == SaturationRenderingIntent) {
				// v4 LUT based profiles use the fixed perceptual black point
				require.Equal(t, XYZType{0.00336, 0.0034731, 0.00287}, bp)
			} else {
				// the gray value of 0 maps to PCS black in these profiles
				// or the intent is not supported by the profile
				require.Equal(t, XYZType{}, bp, "%s: %s", name, intent)
			}
		}
	}
}

func TestGrayProfileErrors(t *testing.T) {
	data := gray_profile_data(t, "gray-v2-gamma1.8-d65.icc")
	// missing grayTRC tag
	p := decode_test_profile(t, patch_profile(t, data, []byte("kTRC"), []byte("xTRC")))
	_, err := p.CreateTransformerToSRGB(PerceptualRenderingIntent, false, 1, true, true, true)
	require.Error(t, err)
	require.False(t, p.IsSRGB())
	_, err = p.CreateTransformerToDevice(PerceptualRenderingIntent, false, true)
	require.Error(t, err)
	require.False(t, p.IsMatrixShaper())
	// missing media white point is treated as D50
	p = decode_test_profile(t, patch_profile(t, data, []byte("wtpt"), []byte("xtpt")))
	require.Equal(t, lcms_d50, p.media_white_point())
	_, err = p.CreateTransformerToSRGB(AbsoluteColorimetricRenderingIntent, false, 1, true, true, true)
	require.NoError(t, err)
	// unsupported PCS
	p = decode_test_profile(t, data)
	p.Header.ProfileConnectionSpace = ColorSpaceLuv
	_, err = p.createTransformerToPCS(PerceptualRenderingIntent)
	require.Error(t, err)
}
