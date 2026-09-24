package imaging

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/kovidgoyal/imaging/nrgb"
	"github.com/kovidgoyal/imaging/prism/meta/icc"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func gray_test_profile_data(t *testing.T, name string) []byte {
	data, err := os.ReadFile(filepath.Join("prism", "meta", "icc", "test-profiles", name))
	require.NoError(t, err)
	return data
}

func gray_test_profile(t *testing.T, name string) *icc.Profile {
	p, err := icc.DecodeProfile(bytes.NewReader(gray_test_profile_data(t, name)))
	require.NoError(t, err)
	return p
}

// the expected sRGB values for a 16 bit gray value
func gray_expected(t *testing.T, p *icc.Profile, intent icc.RenderingIntent, bpc bool) func(uint16) [3]uint16 {
	f := gray_expected_float(t, p, intent, bpc)
	return func(v uint16) [3]uint16 {
		c := f(float64(v) / math.MaxUint16)
		return [3]uint16{f16r(c[0]), f16r(c[1]), f16r(c[2])}
	}
}

// the expected sRGB values for an 8 bit gray value
func gray_expected8(t *testing.T, p *icc.Profile, intent icc.RenderingIntent, bpc bool) func(uint8) [3]uint8 {
	f := gray_expected_float(t, p, intent, bpc)
	return func(v uint8) [3]uint8 {
		c := f(float64(v) / math.MaxUint8)
		return [3]uint8{f8r(c[0]), f8r(c[1]), f8r(c[2])}
	}
}

func gray_expected_float(t *testing.T, p *icc.Profile, intent icc.RenderingIntent, bpc bool) func(float64) [3]float64 {
	tr, err := p.CreateTransformerToSRGB(intent, bpc, 1, true, true, true)
	require.NoError(t, err)
	return func(v float64) [3]float64 {
		var i, o [4]float64
		i[0] = v
		tr.TransformGeneral(o[:], i[:])
		return [3]float64{o[0], o[1], o[2]}
	}
}

// read a pixel from one of the output image types without loss of precision
func nrgba64_at(img image.Image, x, y int) color.NRGBA64 {
	switch img := img.(type) {
	case *NRGB:
		c := img.NRGBAt(x, y)
		return color.NRGBA64{R: uint16(c.R) * 257, G: uint16(c.G) * 257, B: uint16(c.B) * 257, A: 0xffff}
	case *image.NRGBA:
		c := img.NRGBAAt(x, y)
		return color.NRGBA64{R: uint16(c.R) * 257, G: uint16(c.G) * 257, B: uint16(c.B) * 257, A: uint16(c.A) * 257}
	case *image.NRGBA64:
		return img.NRGBA64At(x, y)
	}
	panic(fmt.Sprintf("unexpected output image type: %T", img))
}

func require_close(t *testing.T, expected, actual []int, tolerance int, msg string, args ...any) {
	t.Helper()
	for i := range expected {
		d := expected[i] - actual[i]
		if d < -tolerance || d > tolerance {
			require.Failf(t, "values differ", "%v != %v (tolerance: %d): %s", expected, actual, tolerance, fmt.Sprintf(msg, args...))
		}
	}
}

// Golden values from lcms, see TransformToSRGBInt()
func TestGrayConversionAgainstGoldenValues(t *testing.T) {
	levels := []byte{0, 1, 16, 32, 64, 100, 128, 160, 192, 224, 254, 255}
	type key struct {
		name   string
		intent icc.RenderingIntent
	}
	golden := map[key][]byte{
		{"gray-v2-gamma1.8-d65.icc", icc.PerceptualRenderingIntent}:             {0, 0, 0, 0, 0, 0, 20, 20, 20, 43, 43, 43, 82, 82, 82, 119, 119, 119, 147, 147, 147, 176, 176, 176, 204, 204, 204, 230, 230, 230, 254, 254, 254, 255, 255, 255},
		{"gray-v2-gamma1.8-d65.icc", icc.RelativeColorimetricRenderingIntent}:   {0, 0, 0, 0, 0, 0, 20, 20, 20, 43, 43, 43, 82, 82, 82, 119, 119, 119, 147, 147, 147, 176, 176, 176, 204, 204, 204, 230, 230, 230, 254, 254, 254, 255, 255, 255},
		{"gray-v2-tabulated-prtr.icc", icc.PerceptualRenderingIntent}:           {0, 0, 0, 3, 3, 3, 32, 32, 32, 51, 51, 51, 84, 84, 84, 118, 118, 118, 143, 143, 143, 172, 172, 172, 200, 200, 200, 228, 228, 228, 254, 254, 254, 255, 255, 255},
		{"gray-v2-tabulated-prtr.icc", icc.RelativeColorimetricRenderingIntent}: {0, 0, 0, 3, 3, 3, 32, 32, 32, 51, 51, 51, 84, 84, 84, 118, 118, 118, 143, 143, 143, 172, 172, 172, 200, 200, 200, 228, 228, 228, 254, 254, 254, 255, 255, 255},
		{"gray-v4-lab-gamma2.2.icc", icc.PerceptualRenderingIntent}:             {0, 0, 0, 0, 0, 0, 1, 1, 1, 4, 4, 4, 16, 16, 16, 33, 33, 33, 53, 53, 53, 84, 84, 84, 128, 128, 128, 185, 185, 185, 253, 253, 253, 255, 255, 255},
		{"gray-v4-lab-gamma2.2.icc", icc.RelativeColorimetricRenderingIntent}:   {0, 0, 0, 0, 0, 0, 1, 1, 1, 4, 4, 4, 16, 16, 16, 33, 33, 33, 53, 53, 53, 84, 84, 84, 128, 128, 128, 185, 185, 185, 253, 253, 253, 255, 255, 255},
		{"gray-v4-lut.icc", icc.PerceptualRenderingIntent}:                      {0, 0, 0, 0, 0, 0, 0, 0, 0, 13, 13, 13, 59, 58, 57, 100, 99, 97, 131, 128, 124, 166, 160, 155, 200, 192, 183, 235, 223, 210, 255, 251, 235, 255, 252, 236},
		{"gray-v4-lut.icc", icc.RelativeColorimetricRenderingIntent}:            {0, 0, 0, 0, 0, 0, 5, 5, 5, 21, 21, 21, 61, 60, 60, 102, 100, 98, 132, 128, 125, 166, 160, 155, 201, 192, 184, 235, 223, 211, 255, 251, 235, 255, 252, 236},
		{"gray-v2-lut.icc", icc.PerceptualRenderingIntent}:                      {0, 0, 0, 0, 0, 0, 16, 15, 15, 39, 39, 39, 82, 81, 80, 122, 119, 116, 150, 146, 141, 181, 174, 168, 212, 202, 192, 241, 228, 215, 255, 251, 235, 255, 252, 236},
		{"gray-v2-lut.icc", icc.RelativeColorimetricRenderingIntent}:            {0, 0, 0, 0, 0, 0, 16, 15, 15, 39, 39, 39, 82, 81, 80, 122, 119, 116, 150, 146, 141, 181, 174, 168, 212, 202, 192, 241, 228, 215, 255, 251, 235, 255, 252, 236},
	}
	for k, expected := range golden {
		p := gray_test_profile(t, k.name)
		img := image.NewGray(image.Rect(0, 0, len(levels), 1))
		copy(img.Pix, levels)
		out, err := ConvertToSRGB(p, k.intent, false, img)
		require.NoError(t, err)
		n, ok := out.(*NRGB)
		require.True(t, ok, "unexpected output type: %T", out)
		for i := range levels {
			if levels[i] > 224 && (k.name == "gray-v4-lut.icc" || k.name == "gray-v2-lut.icc") {
				// These colors are out of the sRGB gamut, we do gamut
				// mapping by reducing chroma, lcms clips
				continue
			}
			e := expected[3*i : 3*i+3]
			a := n.Pix[3*i : 3*i+3]
			require_close(t, []int{int(e[0]), int(e[1]), int(e[2])}, []int{int(a[0]), int(a[1]), int(a[2])}, 1, "%s: %s: gray: %d", k.name, k.intent, levels[i])
		}
	}
}

// An image type not specifically handled by the conversion code
type gray_unknown_image struct{ *image.Gray16 }

func TestGrayConversionImageTypes(t *testing.T) {
	const w, h = 37, 11
	outer := image.Rect(-7, -3, w+5, h+4)
	r := image.Rect(-2, -1, w-2, h-1)
	// gray value and alpha for a pixel
	gv := func(x, y int) uint16 { return uint16(((x-r.Min.X)*h + (y - r.Min.Y)) * 65535 / (w*h - 1)) }
	av := func(x, y int) uint16 { return uint16((x - r.Min.X + y - r.Min.Y) * 65535 / (w + h - 2)) }
	g8 := func(x, y int) uint8 { return uint8(gv(x, y) >> 8) }
	a8 := func(x, y int) uint8 { return uint8(av(x, y) >> 8) }

	type test_case struct {
		name string
		// return an image with gray pixels
		make func() image.Image
		// the gray value and alpha that the conversion should see
		pixel func(x, y int) (gray uint16, alpha uint16)
		// the 8 bit or 16 bit output type
		is16          bool
		has_alpha     bool
		premultiplied bool
		tolerance     int
	}
	sub := func(img image.Image) image.Image {
		return img.(interface {
			SubImage(image.Rectangle) image.Image
		}).SubImage(r)
	}
	opaque8 := func(x, y int) (uint16, uint16) { v := uint16(g8(x, y)); return v<<8 | v, 0xffff }
	cases := []test_case{
		{name: "Gray", make: func() image.Image {
			img := image.NewGray(outer)
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					img.SetGray(x, y, color.Gray{Y: g8(x, y)})
				}
			}
			return sub(img)
		}, pixel: opaque8},
		{name: "Gray16", make: func() image.Image {
			img := image.NewGray16(outer)
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					img.SetGray16(x, y, color.Gray16{Y: gv(x, y)})
				}
			}
			return sub(img)
		}, pixel: func(x, y int) (uint16, uint16) { return gv(x, y), 0xffff }, is16: true},
		{name: "NRGB", make: func() image.Image {
			img := nrgb.NewNRGB(outer)
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					v := g8(x, y)
					img.Set(x, y, NRGBColor{R: v, G: v, B: v})
				}
			}
			return sub(img)
		}, pixel: opaque8},
		{name: "NRGBA", make: func() image.Image {
			img := image.NewNRGBA(outer)
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					v := g8(x, y)
					img.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: a8(x, y)})
				}
			}
			return sub(img)
		}, pixel: func(x, y int) (uint16, uint16) {
			v, a := uint16(g8(x, y)), uint16(a8(x, y))
			return v<<8 | v, a<<8 | a
		}, has_alpha: true},
		{name: "NRGBA64", make: func() image.Image {
			img := image.NewNRGBA64(outer)
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					v := gv(x, y)
					img.SetNRGBA64(x, y, color.NRGBA64{R: v, G: v, B: v, A: av(x, y)})
				}
			}
			return sub(img)
		}, pixel: func(x, y int) (uint16, uint16) { return gv(x, y), av(x, y) }, is16: true, has_alpha: true},
		{name: "RGBA64", make: func() image.Image {
			img := image.NewRGBA64(outer)
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					img.Set(x, y, color.NRGBA64{R: gv(x, y), G: gv(x, y), B: gv(x, y), A: av(x, y)})
				}
			}
			return sub(img)
		}, pixel: func(x, y int) (uint16, uint16) { return gv(x, y), av(x, y) }, is16: true, has_alpha: true,
			// premultiplication loses precision
			premultiplied: true, tolerance: 64},
		{name: "YCbCr", make: func() image.Image {
			img := image.NewYCbCr(outer, image.YCbCrSubsampleRatio420)
			for i := range img.Cb {
				img.Cb[i], img.Cr[i] = 128, 128
			}
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					img.Y[img.YOffset(x, y)] = g8(x, y)
				}
			}
			return sub(img)
		}, pixel: opaque8},
		{name: "NYCbCrA", make: func() image.Image {
			img := image.NewNYCbCrA(outer, image.YCbCrSubsampleRatio422)
			for i := range img.Cb {
				img.Cb[i], img.Cr[i] = 128, 128
			}
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					img.Y[img.YOffset(x, y)] = g8(x, y)
					img.A[img.AOffset(x, y)] = a8(x, y)
				}
			}
			return sub(img)
		}, pixel: func(x, y int) (uint16, uint16) {
			v, a := uint16(g8(x, y)), uint16(a8(x, y))
			return v<<8 | v, a<<8 | a
		}, has_alpha: true},
		{name: "unknown", make: func() image.Image {
			img := image.NewGray16(r)
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					img.SetGray16(x, y, color.Gray16{Y: gv(x, y)})
				}
			}
			return gray_unknown_image{img}
		}, pixel: func(x, y int) (uint16, uint16) { return gv(x, y), 0xffff }, is16: true},
	}
	for _, name := range []string{"gray-v2-gamma1.8-d65.icc", "gray-v4-lut.icc", "gray-v4-lab-gamma2.2.icc"} {
		p := gray_test_profile(t, name)
		expected := gray_expected(t, p, icc.RelativeColorimetricRenderingIntent, true)
		expected8 := gray_expected8(t, p, icc.RelativeColorimetricRenderingIntent, true)
		for _, tc := range cases {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				img := tc.make()
				out, err := ConvertToSRGB(p, icc.RelativeColorimetricRenderingIntent, true, img)
				require.NoError(t, err)
				require.Equal(t, r, out.Bounds())
				for y := r.Min.Y; y < r.Max.Y; y++ {
					for x := r.Min.X; x < r.Max.X; x++ {
						v, a := tc.pixel(x, y)
						c := nrgba64_at(out, x, y)
						if tc.has_alpha {
							require.Equal(t, a, c.A, "alpha not preserved at: %d, %d", x, y)
						} else {
							require.Equal(t, uint16(0xffff), c.A, "not opaque at: %d, %d", x, y)
						}
						if tc.premultiplied && a < 0x2000 {
							// premultiplied images lose precision at low alpha
							continue
						}
						actual := []int{int(c.R), int(c.G), int(c.B)}
						var e []int
						if tc.is16 {
							q := expected(v)
							e = []int{int(q[0]), int(q[1]), int(q[2])}
						} else {
							q := expected8(uint8(v >> 8))
							e = []int{int(q[0]) * 257, int(q[1]) * 257, int(q[2]) * 257}
						}
						require_close(t, e, actual, tc.tolerance, "pixel at: %d, %d with gray: %d", x, y, v)
					}
				}
			})
		}
	}
}

func TestGrayConversionPaletted(t *testing.T) {
	p := gray_test_profile(t, "gray-v2-gamma1.8-d65.icc")
	expected8 := gray_expected8(t, p, icc.PerceptualRenderingIntent, false)
	palette := make(color.Palette, 0, 256)
	for i := range 256 {
		palette = append(palette, color.NRGBA{R: uint8(i), G: uint8(i), B: uint8(i), A: uint8(255 - i/2)})
	}
	img := image.NewPaletted(image.Rect(0, 0, 16, 16), palette)
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	out, err := ConvertToSRGB(p, icc.PerceptualRenderingIntent, false, img)
	require.NoError(t, err)
	require.Same(t, img, out)
	for i, c := range img.Palette {
		n := c.(color.NRGBA64)
		require.Equal(t, uint16(255-i/2)*257, n.A)
		// the palette colors are 8 bit and premultiplied by RGBA() so compare
		// at 8 bit precision
		e := expected8(uint8(i))
		require_close(t, []int{int(e[0]), int(e[1]), int(e[2])}, []int{(int(n.R) + 128) / 257, (int(n.G) + 128) / 257, (int(n.B) + 128) / 257}, 1, "palette entry: %d", i)
	}
}

func TestGrayConversionGray16LUT(t *testing.T) {
	// large images use a lookup table, check it gives the same results as
	// direct evaluation for all possible values
	p := gray_test_profile(t, "gray-v2-tabulated-prtr.icc")
	expected := gray_expected(t, p, icc.PerceptualRenderingIntent, false)
	img := image.NewGray16(image.Rect(0, 0, 256, 256))
	for i := range 65536 {
		binary.BigEndian.PutUint16(img.Pix[2*i:], uint16(i))
	}
	out, err := ConvertToSRGB(p, icc.PerceptualRenderingIntent, false, img)
	require.NoError(t, err)
	n := out.(*image.NRGBA64)
	for i := range 65536 {
		s := n.Pix[8*i:]
		e := expected(uint16(i))
		require.Equal(t, e, [3]uint16{binary.BigEndian.Uint16(s), binary.BigEndian.Uint16(s[2:]), binary.BigEndian.Uint16(s[4:])}, "gray: %d", i)
		require.Equal(t, uint16(0xffff), binary.BigEndian.Uint16(s[6:]))
	}
	// and small images use direct evaluation
	small := image.NewGray16(image.Rect(0, 0, 3, 1))
	for i, v := range []uint16{0, 12345, 65535} {
		binary.BigEndian.PutUint16(small.Pix[2*i:], v)
	}
	out, err = ConvertToSRGB(p, icc.PerceptualRenderingIntent, false, small)
	require.NoError(t, err)
	for i, v := range []uint16{0, 12345, 65535} {
		c := out.(*image.NRGBA64).NRGBA64At(i, 0)
		require.Equal(t, expected(v), [3]uint16{c.R, c.G, c.B})
	}
}

func TestGrayConversionSpecialCases(t *testing.T) {
	// sRGB gray needs no conversion
	p := gray_test_profile(t, "gray-v4-srgb.icc")
	img := image.NewGray(image.Rect(0, 0, 4, 4))
	out, err := ConvertToSRGB(p, icc.PerceptualRenderingIntent, false, img)
	require.NoError(t, err)
	require.Same(t, img, out)

	p = gray_test_profile(t, "gray-v2-gamma1.8-d65.icc")
	// CMYK images cannot have a gray profile
	_, err = ConvertToSRGB(p, icc.PerceptualRenderingIntent, false, image.NewCMYK(image.Rect(0, 0, 4, 4)))
	require.Error(t, err)
	// empty images
	for _, img := range []image.Image{image.NewGray(image.Rect(0, 0, 0, 5)), image.NewGray16(image.Rect(3, 3, 3, 3)), image.NewNRGBA(image.Rect(0, 0, 5, 0))} {
		out, err := ConvertToSRGB(p, icc.PerceptualRenderingIntent, false, img)
		require.NoError(t, err)
		require.Equal(t, img.Bounds(), out.Bounds())
	}
	// a gray image that is actually sRGB gray, converted using the sRGB
	// profile gives the same values
	p = gray_test_profile(t, "gray-v4-srgb.icc")
	srgb, err := p.CreateTransformerToSRGB(icc.PerceptualRenderingIntent, false, 1, true, true, true)
	require.NoError(t, err)
	img = image.NewGray(image.Rect(0, 0, 16, 16))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	out, err = convert_gray(srgb, img)
	require.NoError(t, err)
	for i, v := range img.Pix {
		require.Equal(t, []uint8{v, v, v}, out.(*NRGB).Pix[3*i:3*i+3])
	}
}

// A minimal PNG encoder for gray and gray + alpha images since the standard
// library encoder cannot create these
func encode_gray_png(t *testing.T, w, h, depth int, has_alpha bool, pixel func(x, y int) (gray, alpha uint16)) []byte {
	var out bytes.Buffer
	chunk := func(typ string, data []byte) {
		c := append([]byte(typ), data...)
		_ = binary.Write(&out, binary.BigEndian, uint32(len(data)))
		out.Write(c)
		_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(c))
	}
	out.WriteString("\x89PNG\r\n\x1a\n")
	var ihdr [13]byte
	binary.BigEndian.PutUint32(ihdr[0:], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(h))
	ihdr[8] = uint8(depth)
	ihdr[9] = icc.IfElse[uint8](has_alpha, 4, 0)
	chunk("IHDR", ihdr[:])
	var raw bytes.Buffer
	for y := range h {
		raw.WriteByte(0) // no filtering
		var bits, nbits uint
		for x := range w {
			v, a := pixel(x, y)
			switch depth {
			case 16:
				_ = binary.Write(&raw, binary.BigEndian, v)
				if has_alpha {
					_ = binary.Write(&raw, binary.BigEndian, a)
				}
			case 8:
				raw.WriteByte(uint8(v >> 8))
				if has_alpha {
					raw.WriteByte(uint8(a >> 8))
				}
			default:
				bits = bits<<uint(depth) | uint(v>>(16-depth))
				if nbits += uint(depth); nbits == 8 {
					raw.WriteByte(uint8(bits))
					bits, nbits = 0, 0
				}
			}
		}
		if nbits > 0 {
			raw.WriteByte(uint8(bits << (8 - nbits)))
		}
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, err := zw.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	chunk("IDAT", z.Bytes())
	chunk("IEND", nil)
	return out.Bytes()
}

// Insert an iCCP chunk into PNG data
func png_with_icc(t *testing.T, data, profile []byte) []byte {
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	_, err := w.Write(profile)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	chunk := append([]byte("iCCP"), []byte("gray\x00\x00")...)
	chunk = append(chunk, z.Bytes()...)
	var b bytes.Buffer
	const ihdr_end = 8 + 8 + 13 + 4
	b.Write(data[:ihdr_end])
	_ = binary.Write(&b, binary.BigEndian, uint32(len(chunk)-4))
	b.Write(chunk)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	b.Write(data[ihdr_end:])
	return b.Bytes()
}

// Insert an APP2 ICC_PROFILE marker into JPEG data
func jpeg_with_icc(data, profile []byte) []byte {
	var b bytes.Buffer
	b.Write(data[:2])
	b.Write([]byte{0xff, 0xe2})
	_ = binary.Write(&b, binary.BigEndian, uint16(2+12+2+len(profile)))
	b.WriteString("ICC_PROFILE\x00")
	b.Write([]byte{1, 1})
	b.Write(profile)
	b.Write(data[2:])
	return b.Bytes()
}

func TestGrayDecodeWithICCProfile(t *testing.T) {
	const w, h = 64, 48
	pixel := func(x, y int) (uint16, uint16) {
		return uint16((y*w + x) * 65535 / (w*h - 1)), uint16(x * 1000)
	}
	gray8 := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			v, _ := pixel(x, y)
			gray8.SetGray(x, y, color.Gray{Y: uint8(v >> 8)})
		}
	}
	var pb bytes.Buffer
	require.NoError(t, png.Encode(&pb, gray8))
	var jb bytes.Buffer
	require.NoError(t, jpeg.Encode(&jb, gray8, &jpeg.Options{Quality: 95}))

	for _, profile_name := range []string{"gray-v2-gamma1.8-d65.icc", "gray-v4-lut.icc"} {
		profile_data := gray_test_profile_data(t, profile_name)
		p := gray_test_profile(t, profile_name)
		// the default decode settings
		expected := gray_expected(t, p, icc.RelativeColorimetricRenderingIntent, true)
		expected8 := gray_expected8(t, p, icc.RelativeColorimetricRenderingIntent, true)
		for _, tc := range []struct {
			name     string
			data     []byte
			raw_type string
		}{
			{"png-gray8-stdlib", png_with_icc(t, pb.Bytes(), profile_data), "*image.Gray"},
			{"png-gray4", png_with_icc(t, encode_gray_png(t, w, h, 4, false, pixel), profile_data), "*image.Gray"},
			{"png-gray8", png_with_icc(t, encode_gray_png(t, w, h, 8, false, pixel), profile_data), "*image.Gray"},
			{"png-gray16", png_with_icc(t, encode_gray_png(t, w, h, 16, false, pixel), profile_data), "*image.Gray16"},
			{"png-gray-alpha8", png_with_icc(t, encode_gray_png(t, w, h, 8, true, pixel), profile_data), "*image.NRGBA"},
			{"png-gray-alpha16", png_with_icc(t, encode_gray_png(t, w, h, 16, true, pixel), profile_data), "*image.NRGBA64"},
			{"jpeg-gray", jpeg_with_icc(jb.Bytes(), profile_data), "*image.Gray"},
		} {
			t.Run(profile_name+"/"+tc.name, func(t *testing.T) {
				md, _, err := DecodeAll(bytes.NewReader(tc.data), Backends(GO_IMAGE), ColorSpace(NO_CHANGE_OF_COLORSPACE))
				require.NoError(t, err)
				embedded, err := md.Metadata.ICCProfileData()
				require.NoError(t, err)
				require.Equal(t, profile_data, embedded, "ICC profile not found in image")
				raw := md.SingleFrame()
				require.Equal(t, tc.raw_type, fmt.Sprintf("%T", raw))
				converted, err := Decode(bytes.NewReader(tc.data), Backends(GO_IMAGE))
				require.NoError(t, err)
				require.Equal(t, raw.Bounds(), converted.Bounds())
				// the profile is not sRGB so the colors must change
				mid := color.NRGBA64Model.Convert(converted.At(w/2, h/2)).(color.NRGBA64)
				mr, _, _, _ := raw.At(w/2, h/2).RGBA()
				require.NotEqual(t, uint16(mr), mid.R)
				_, is16 := raw.(*image.Gray16)
				if _, ok := raw.(*image.NRGBA64); ok {
					is16 = true
				}
				for y := range h {
					for x := range w {
						r, g, b, a := raw.At(x, y).RGBA()
						require.Equal(t, r, g)
						require.Equal(t, g, b)
						c := nrgba64_at(converted, x, y)
						require.Equal(t, uint16(a), c.A)
						if a == 0 {
							continue
						}
						var v uint16
						switch raw := raw.(type) {
						case *image.Gray:
							v = uint16(raw.GrayAt(x, y).Y) * 257
						case *image.Gray16:
							v = raw.Gray16At(x, y).Y
						case *image.NRGBA:
							v = uint16(raw.NRGBAAt(x, y).R) * 257
						case *image.NRGBA64:
							v = raw.NRGBA64At(x, y).R
						default:
							require.Fail(t, "unexpected decoded image type", "%T", raw)
						}
						var e []int
						if is16 {
							q := expected(v)
							e = []int{int(q[0]), int(q[1]), int(q[2])}
						} else {
							q := expected8(uint8(v >> 8))
							e = []int{int(q[0]) * 257, int(q[1]) * 257, int(q[2]) * 257}
						}
						require_close(t, e, []int{int(c.R), int(c.G), int(c.B)}, 0, "pixel at: %d, %d gray: %d", x, y, v)
					}
				}
			})
		}
	}
}
