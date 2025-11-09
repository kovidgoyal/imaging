package imaging

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
	"os"
	"testing"

	"github.com/kovidgoyal/imaging/colorconv"
	"github.com/kovidgoyal/imaging/magick"
	"github.com/kovidgoyal/imaging/types"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func compare_images(t *testing.T, a, b *Image, tolerance float64) {
	require.Equal(t, a.Bounds(), b.Bounds())
	require.Equal(t, len(a.Frames), len(b.Frames))
	var total_square float64
	for i, af := range a.Frames {
		bf := b.Frames[i]
		require.Equal(t, af.Bounds(), bf.Bounds())
		require.Equal(t, af.TopLeft, bf.TopLeft)
		for y := af.Bounds().Min.Y; y < af.Bounds().Max.Y; y++ {
			for x := af.Bounds().Min.X; x < af.Bounds().Max.X; x++ {
				ac, bc := af.At(x, y), bf.At(x, y)
				an, bn := color.NRGBAModel.Convert(ac).(color.NRGBA), color.NRGBAModel.Convert(bc).(color.NRGBA)
				d := colorconv.DeltaEBetweenSrgb(float64(an.R)/255., float64(an.G)/255., float64(an.B)/255., float64(bn.R)/255., float64(bn.G)/255., float64(bn.B)/255.)
				total_square += d * d
			}
		}
		denominator := (float64(af.Bounds().Dx()) * float64(af.Bounds().Dy()))
		rms_delta := math.Sqrt(total_square / denominator)
		if rms_delta > tolerance {
			a.SaveAsPNG("/tmp/a.png", 0o666)
			b.SaveAsPNG("/tmp/b.png", 0o666)
		}
		require.LessOrEqual(t, rms_delta, tolerance, fmt.Sprintf("frame %d, images saved as /tmp/a.png and /tmp/b.png", i))
	}
}

func TestMagick(t *testing.T) {
	if !magick.HasMagick() {
		t.Skip("ImageMagick not available, skipping test")
	}
	const o3 = "testdata/orientation_3.jpg"
	var no_changes_cfg = NewDecodeConfig(ColorSpace(NO_CHANGE_OF_COLORSPACE), AutoOrientation(false))
	img, err := decode_all_magick(&types.Input{Path: o3}, nil, no_changes_cfg)
	require.NoError(t, err)
	require.Equal(t, len(img.Frames), 1)
	f, err := os.Open(o3)
	require.NoError(t, err)
	defer f.Close()
	img2, err := decode_all_magick(&types.Input{Reader: f}, nil, no_changes_cfg)
	require.NoError(t, err)
	require.Equal(t, len(img2.Frames), 1)
	data, err := os.ReadFile(o3)
	require.NoError(t, err)
	img2, err = decode_all_magick(&types.Input{Reader: bytes.NewReader(data)}, nil, no_changes_cfg)
	require.NoError(t, err)
	require.Equal(t, len(img2.Frames), 1)

	test_image := func(path string, threshold float64) {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			cmyk, err := OpenAll(path, Backends(MAGICK_IMAGE))
			require.NoError(t, err)
			cmyk_go, err := OpenAll(path, Backends(GO_IMAGE))
			require.NoError(t, err)
			compare_images(t, cmyk_go, cmyk, threshold)
		})
	}
	test_image("prism/test-images/cmyk.jpg", 0.6)
	test_image("prism/test-images/pizza-rgb8-adobergb.jpg", 0.5)
	test_image("prism/test-images/pizza-rgb8-srgb.jpg", 0.4)
}
