package convert

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"testing"

	"github.com/kovidgoyal/imaging"
	"github.com/kovidgoyal/imaging/prism/meta/icc"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func pixel_setter(img_ image.Image) func(x, y int, c color.Color) {
	switch img := img_.(type) {
	case *image.YCbCr:
		if img.SubsampleRatio != image.YCbCrSubsampleRatio444 {
			panic("setting not supported for YCbCr images with non 4:4:4 sampling")
		}
		return func(x, y int, c color.Color) {
			cc := color.YCbCrModel.Convert(c).(color.YCbCr)
			yi := img.YOffset(x, y)
			ci := img.COffset(x, y)
			img.Y[yi] = cc.Y
			img.Cb[ci] = cc.Cb
			img.Cr[ci] = cc.Cr
		}
	case draw.Image:
		return img.Set
	}
	panic(fmt.Sprintf("unhandled image type to set colors in: %T", img_))
}

func image_compare(t *testing.T, a, b image.Image, ct *icc.Translation) {
	wa, ha := a.Bounds().Dx(), a.Bounds().Dy()
	wb, hb := b.Bounds().Dx(), b.Bounds().Dy()
	require.Equal(t, wa, wb)
	require.Equal(t, ha, hb)
	maxval := float64(math.MaxUint8)
	switch a.(type) {
	case *image.NRGBA64, *image.RGBA64:
		maxval = math.MaxUint16
	}
	cvt := func(x float64) uint { return uint(x * maxval) }
	f := func(x uint16) float64 { return float64(x) / math.MaxUint16 }
	for y := range ha {
		ya, yb := a.Bounds().Min.Y+y, b.Bounds().Min.Y+y
		for x := range wa {
			xa, xb := a.Bounds().Min.X+x, b.Bounds().Min.X+x
			ac, bc := a.At(xa, ya), b.At(xb, yb)
			qa := color.NRGBA64Model.Convert(ac).(color.NRGBA64)
			qb := color.NRGBA64Model.Convert(bc).(color.NRGBA64)
			orig := []float64{f(qa.R), f(qa.G), f(qa.B)}
			actual := []uint{cvt(f(qb.R)), cvt(f(qb.G)), cvt(f(qb.B))}
			r, g, b := ct.Transform(orig[0], orig[1], orig[2])
			expected := []uint{cvt(r), cvt(g), cvt(b)}
			require.Equal(t, expected, actual, fmt.Sprintf("pixel at x=%d y=%d orig=%.6v", x, y, []uint{cvt(orig[0]), cvt(orig[1]), cvt(orig[2])}))
		}
	}
}

func populate_test_image(img image.Image, dr, dg, db uint8) image.Image {
	r := img.Bounds()
	offset := r.Min.Y + r.Min.X
	c := func(base uint8) color.Color {
		return imaging.NRGBColor{R: base + dr, G: base + 1 + dg, B: base + 2 + db}
	}
	if p, ok := img.(*image.Paletted); ok {
		max_base := uint8(0)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				base := uint8(x + y - offset)
				max_base = max(base, max_base)
				p.Palette[base] = c(base)
				p.SetColorIndex(x, y, base)
			}
		}
		p.Palette = p.Palette[:max_base+1]
		return img
	}
	s := pixel_setter(img)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			base := uint8(x + y - offset)
			s(x, y, c(base))
		}
	}
	return img
}

func TestProfileApplication(t *testing.T) {
	r := image.Rect(-11, -7, -11+13, -7+37)
	ct := &icc.Translation{6 / 255., 7 / 255., 8 / 255.}
	run := func(img image.Image) {
		t.Run(fmt.Sprintf("%T", img), func(t *testing.T) {
			t.Parallel()
			_, is_cmyk := img.(*image.CMYK)
			p := &icc.Pipeline{}
			if is_cmyk {
				p.Append(icc.NewCMYKToRGB())
			}
			p.Append(ct)
			p.Finalize(true)
			img = populate_test_image(img, 0, 0, 0)
			cimg := imaging.ClonePreservingType(img)
			cimg, err := convert(p, cimg)
			require.NoError(t, err)
			image_compare(t, img, cimg, ct)
		})
	}
	run(imaging.NewNRGB(r))
	run(image.NewNRGBA(r))
	run(image.NewNRGBA64(r))
	run(image.NewRGBA(r))
	run(image.NewRGBA64(r))
	run(image.NewYCbCr(r, image.YCbCrSubsampleRatio444))
	run(image.NewPaletted(r, make(color.Palette, 256)))
}
