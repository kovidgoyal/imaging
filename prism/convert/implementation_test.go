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
	case *image.NYCbCrA:
		if img.SubsampleRatio != image.YCbCrSubsampleRatio444 {
			panic("setting not supported for YCbCr images with non 4:4:4 sampling")
		}
		return func(x, y int, c color.Color) {
			cc := color.YCbCrModel.Convert(c).(color.YCbCr)
			yi := img.YOffset(x, y)
			ci := img.COffset(x, y)
			a := img.AOffset(x, y)
			img.Y[yi] = cc.Y
			img.Cb[ci] = cc.Cb
			img.Cr[ci] = cc.Cr
			img.A[a] = 0xff
		}
	case *unknown_image:
		return img.set
	case draw.Image:
		return img.Set
	}
	panic(fmt.Sprintf("unhandled image type to set colors in: %T", img_))
}

func image_compare(t *testing.T, a, b image.Image, ct icc.ChannelTransformer, allowed_diff uint) {
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
	max_diff := struct {
		d                      uint
		x, y                   int
		orig, actual, expected []uint
	}{}
	md := func(a, b []uint) (ans uint) {
		for i, x := range a {
			y := b[i]
			d := icc.IfElse(x > y, x-y, y-x)
			ans = max(ans, d)
		}
		return
	}
	cc := func(c color.Color) color.NRGBA64 { return color.NRGBA64Model.Convert(c).(color.NRGBA64) }
	for y := range ha {
		ya, yb := a.Bounds().Min.Y+y, b.Bounds().Min.Y+y
		for x := range wa {
			xa, xb := a.Bounds().Min.X+x, b.Bounds().Min.X+x
			qa, qb := cc(a.At(xa, ya)), cc(b.At(xb, yb))
			orig := []float64{f(qa.R), f(qa.G), f(qa.B)}
			actual := []uint{cvt(f(qb.R)), cvt(f(qb.G)), cvt(f(qb.B))}
			r, g, b := ct.Transform(orig[0], orig[1], orig[2])
			expected := []uint{cvt(r), cvt(g), cvt(b)}
			dd := md(actual, expected)
			if dd > max_diff.d {
				max_diff.d = dd
				max_diff.x, max_diff.y = x, y
				max_diff.actual, max_diff.expected = actual, expected
				max_diff.orig = []uint{cvt(orig[0]), cvt(orig[1]), cvt(orig[2])}
			}
		}
		require.LessOrEqual(t, max_diff.d, allowed_diff, fmt.Sprintf("pixel at x=%d y=%d orig=%v\n%v != %v", max_diff.x, max_diff.y, max_diff.orig, max_diff.expected, max_diff.actual))
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
	if p, ok := img.(*image.CMYK); ok {
		for y := range r.Dy() {
			row := p.Pix[y*p.Stride:]
			for x := range r.Dx() {
				base := uint8(x + y)
				row[0], row[1], row[2], row[3] = base, base+1, base+2, base+3
				row = row[4:]
			}
		}
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

type NormalisedCMYKToRGB int

func NewCMYKToRGB() *NormalisedCMYKToRGB {
	a := NormalisedCMYKToRGB(0)
	return &a
}

func (c *NormalisedCMYKToRGB) Transform(l, a, b float64) (float64, float64, float64) {
	panic("need 4 inputs cannot use Transform, must use TransformGeneral")
}
func (m *NormalisedCMYKToRGB) TransformGeneral(o, i []float64) {
	k := 1 - i[3]
	o[0] = (1 - i[0]) * k
	o[1] = (1 - i[1]) * k
	o[2] = (1 - i[2]) * k
}
func (n *NormalisedCMYKToRGB) IOSig() (int, int)                        { return 4, 3 }
func (n *NormalisedCMYKToRGB) String() string                           { return "NormalisedCMYKToRGB" }
func (n *NormalisedCMYKToRGB) Iter(f func(icc.ChannelTransformer) bool) { f(n) }

type unknown_image struct {
	Pix    []uint8
	Stride int
	Rect   image.Rectangle
}

func new_unknown_image(r image.Rectangle) *unknown_image {
	return &unknown_image{
		Pix:    make([]uint8, 3*r.Dx()*r.Dy()),
		Stride: 3 * r.Dx(),
		Rect:   r,
	}
}

func (p *unknown_image) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*3
}
func (p *unknown_image) ColorModel() color.Model { return imaging.NRGBModel }
func (p *unknown_image) Bounds() image.Rectangle { return p.Rect }
func (p *unknown_image) At(x, y int) color.Color {
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+3 : i+3]
	return imaging.NRGBColor{R: s[0], G: s[1], B: s[2]}
}

func (p *unknown_image) set(x, y int, c color.Color) {
	if !(image.Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+3 : i+3]
	q := imaging.NRGBModel.Convert(c).(imaging.NRGBColor)
	s[0], s[1], s[2] = q.R, q.G, q.B
}

type unknown_image_with_set struct {
	Pix    []uint8
	Stride int
	Rect   image.Rectangle
}

func new_unknown_image_with_set(r image.Rectangle) *unknown_image_with_set {
	return &unknown_image_with_set{
		Pix:    make([]uint8, 3*r.Dx()*r.Dy()),
		Stride: 3 * r.Dx(),
		Rect:   r,
	}
}

func (p *unknown_image_with_set) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*3
}
func (p *unknown_image_with_set) ColorModel() color.Model { return imaging.NRGBModel }
func (p *unknown_image_with_set) Bounds() image.Rectangle { return p.Rect }
func (p *unknown_image_with_set) At(x, y int) color.Color {
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+3 : i+3]
	return imaging.NRGBColor{R: s[0], G: s[1], B: s[2]}
}

func (p *unknown_image_with_set) Set(x, y int, c color.Color) {
	if !(image.Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+3 : i+3]
	q := imaging.NRGBModel.Convert(c).(imaging.NRGBColor)
	s[0], s[1], s[2] = q.R, q.G, q.B
}

func TestProfileApplication(t *testing.T) {
	r := image.Rect(-11, -7, -11+13, -7+37)
	run := func(img image.Image, allowed_diff uint) {
		t.Run(fmt.Sprintf("%T", img), func(t *testing.T) {
			t.Parallel()
			_, is_cmyk := img.(*image.CMYK)
			p := &icc.Pipeline{}
			var ct icc.ChannelTransformer
			if is_cmyk {
				p.Append(NewCMYKToRGB())
				ct = icc.NewScaling("", 0.5)
			} else {
				ct = &icc.Translation{6 / 255., 7 / 255., 8 / 255.}
			}
			p.Append(ct)
			p.Finalize(true)
			img = populate_test_image(img, 0, 0, 0)
			cimg := imaging.ClonePreservingType(img)
			cimg, err := convert(p, cimg)
			require.NoError(t, err)
			image_compare(t, img, cimg, ct, allowed_diff)
		})
	}
	run(imaging.NewNRGB(r), 0)
	run(image.NewNRGBA(r), 0)
	run(image.NewNRGBA64(r), 0)
	run(image.NewRGBA(r), 0)
	run(image.NewRGBA64(r), 0)
	run(image.NewCMYK(r), 0)
	run(new_unknown_image(r), 0)
	run(new_unknown_image_with_set(r), 0)
	run(image.NewYCbCr(r, image.YCbCrSubsampleRatio444), 0)
	run(image.NewPaletted(r, make(color.Palette, 256)), 0)
	run(image.NewNYCbCrA(r, image.YCbCrSubsampleRatio444), 0)
}
