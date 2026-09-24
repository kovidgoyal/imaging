//go:build lcms2cgo

package imaging

import (
	"encoding/binary"
	"fmt"
	"image"
	"testing"

	"github.com/kovidgoyal/imaging/prism"
	"github.com/kovidgoyal/imaging/prism/meta/icc"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

// Compare the conversion of all possible 8 and 16 bit gray values with lcms
func TestGrayConversionAgainstLCMS2(t *testing.T) {
	// The LUT based profiles are not tested here since they produce out of
	// gamut colors where our gamut mapping differs from the clipping done by
	// lcms. They are tested against lcms in the prism package instead.
	for name, tolerance16 := range map[string]int{
		"gray-v4-srgb.icc":         0,
		"gray-v2-gamma1.8-d65.icc": 1,
		// lcms interpolates tabulated curves using 16 bit integer arithmetic
		// which is amplified by the slope of 12.92 of the sRGB curve near black
		"gray-v2-tabulated-prtr.icc": 13,
		"gray-v4-lab-gamma2.2.icc":   1,
	} {
		data := gray_test_profile_data(t, name)
		p := gray_test_profile(t, name)
		lcms, err := prism.CreateCMSProfile(data)
		require.NoError(t, err)
		defer lcms.Close()
		for _, intent := range []icc.RenderingIntent{icc.PerceptualRenderingIntent, icc.RelativeColorimetricRenderingIntent, icc.SaturationRenderingIntent, icc.AbsoluteColorimetricRenderingIntent} {
			t.Run(name+"/"+intent.String(), func(t *testing.T) {
				img8 := image.NewGray(image.Rect(0, 0, 256, 1))
				for i := range img8.Pix {
					img8.Pix[i] = uint8(i)
				}
				expected, err := lcms.TransformToSRGBInt(img8.Pix, 1, intent)
				require.NoError(t, err)
				out, err := ConvertToSRGB(p, intent, false, img8)
				require.NoError(t, err)
				var actual []uint8
				switch img := out.(type) {
				case *NRGB:
					actual = img.Pix
				case *image.Gray:
					// sRGB gray is not converted
					for _, v := range img.Pix {
						actual = append(actual, v, v, v)
					}
				default:
					require.Fail(t, "unexpected output type", "%T", out)
				}
				for i := range expected {
					require.InDelta(t, int(expected[i]), int(actual[i]), 1, "gray: %d", i/3)
				}

				img16 := image.NewGray16(image.Rect(0, 0, 256, 256))
				for i := range 65536 {
					binary.BigEndian.PutUint16(img16.Pix[2*i:], uint16(i))
				}
				expected, err = lcms.TransformToSRGBInt(img16.Pix, 2, intent)
				require.NoError(t, err)
				out, err = ConvertToSRGB(p, intent, false, img16)
				require.NoError(t, err)
				for i := range 65536 {
					e := expected[6*i:]
					var a [3]uint16
					switch img := out.(type) {
					case *image.NRGBA64:
						s := img.Pix[8*i:]
						a = [3]uint16{binary.BigEndian.Uint16(s), binary.BigEndian.Uint16(s[2:]), binary.BigEndian.Uint16(s[4:])}
					case *image.Gray16:
						v := binary.BigEndian.Uint16(img.Pix[2*i:])
						a = [3]uint16{v, v, v}
					default:
						require.Fail(t, "unexpected output type", "%T", out)
					}
					for c := range 3 {
						require.InDelta(t, int(binary.BigEndian.Uint16(e[2*c:])), int(a[c]), float64(tolerance16), "gray: %d", i)
					}
				}
			})
		}
	}
}
