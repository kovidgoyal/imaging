package convert

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/kovidgoyal/go-parallel"
	"github.com/kovidgoyal/imaging"
	"github.com/kovidgoyal/imaging/prism/meta/icc"
)

var _ = fmt.Print

func premultiply8(fr float64, a uint8) uint8 {
	return f8i(fr * float64(a) / math.MaxUint8)
}

func unpremultiply8(r, a uint8) float64 {
	return float64((uint16(r) * math.MaxUint8) / uint16(a))
}

func unpremultiply(r, a uint32) float64 {
	return float64((r * math.MaxUint16) / a)
}

func premultiply(r float64, a uint32) uint16 {
	return uint16((r * float64(a)) / math.MaxUint16)
}

func f8(x uint8) float64    { return float64(x) / math.MaxUint8 }
func f8i(x float64) uint8   { return uint8(x * math.MaxUint8) }
func f16(x uint16) float64  { return float64(x) / math.MaxUint16 }
func f16i(x float64) uint16 { return uint16(x * math.MaxUint16) }

// Convert to SRGB based on the supplied ICC color profile. The result
// may be either the original image unmodified if no color
// conversion was needed, the original image modified, or a new image (when the original image
// is not in a supported format).
func ConvertToSRGB(p *icc.Profile, image_any image.Image) (ans image.Image, err error) {
	if p.IsSRGB() {
		return image_any, nil
	}
	num_channels := 3
	if _, is_cmyk := image_any.(*image.CMYK); is_cmyk {
		num_channels = 4
	}
	tr, err := p.CreateTransformerToSRGB(p.Header.RenderingIntent, num_channels, true, true, true)
	if err != nil {
		return nil, err
	}
	return convert_to_srgb(tr, image_any)
}

func convert_to_srgb(tr *icc.Pipeline, image_any image.Image) (ans image.Image, err error) {
	t := tr.Transform
	b := image_any.Bounds()
	width, height := b.Dx(), b.Dy()
	ans = image_any
	var f func(start, limit int)
	switch img := image_any.(type) {
	case *imaging.NRGB:
		f = func(start, limit int) {
			for y := start; y < limit; y++ {
				row := img.Pix[img.Stride*y:]
				_ = row[3*(width-1)]
				for range width {
					r := row[0:3:3]
					fr, fg, fb := t(f8(r[0]), f8(r[1]), f8(r[2]))
					r[0], r[1], r[2] = f8i(fr), f8i(fg), f8i(fb)
					row = row[3:]
				}
			}
		}
	case *image.NRGBA:
		f = func(start, limit int) {
			for y := start; y < limit; y++ {
				row := img.Pix[img.Stride*y:]
				_ = row[4*(width-1)]
				for range width {
					r := row[0:3:3]
					fr, fg, fb := t(f8(r[0]), f8(r[1]), f8(r[2]))
					r[0], r[1], r[2] = f8i(fr), f8i(fg), f8i(fb)
					row = row[4:]
				}
			}
		}
	case *image.NRGBA64:
		f = func(start, limit int) {
			for y := start; y < limit; y++ {
				row := img.Pix[img.Stride*y:]
				_ = row[8*(width-1)]
				for range width {
					s := row[0:8:8]
					fr := f16(uint16(s[0])<<8 | uint16(s[1]))
					fg := f16(uint16(s[2])<<8 | uint16(s[3]))
					fb := f16(uint16(s[4])<<8 | uint16(s[5]))
					fr, fg, fb = t(fr, fg, fb)
					r, g, b := f16i(fr), f16i(fg), f16i(fb)
					s[0], s[1] = uint8(r>>8), uint8(r)
					s[2], s[3] = uint8(g>>8), uint8(g)
					s[4], s[5] = uint8(b>>8), uint8(b)
					row = row[8:]
				}
			}
		}
	case *image.RGBA:
		f = func(start, limit int) {
			for y := start; y < limit; y++ {
				row := img.Pix[img.Stride*y:]
				_ = row[4*(width-1)]
				for range width {
					r := row[0:3:3]
					if a := row[4]; a != 0 {
						fr, fg, fb := t(unpremultiply8(r[0], a), unpremultiply8(r[1], a), unpremultiply8(r[2], a))
						r[0], r[1], r[2] = premultiply8(fr, a), premultiply8(fg, a), premultiply8(fb, a)
					}
					row = row[4:]
				}
			}
		}
	case *image.RGBA64:
		f = func(start, limit int) {
			for y := start; y < limit; y++ {
				row := img.Pix[img.Stride*y:]
				_ = row[8*(width-1)]
				for range width {
					s := row[0:8:8]
					a := uint32(uint16(s[6])<<8 | uint16(s[7]))
					if a != 0 {
						fr := unpremultiply(uint32(uint16(s[0])<<8|uint16(s[1])), a)
						fg := unpremultiply(uint32(uint16(s[2])<<8|uint16(s[3])), a)
						fb := unpremultiply(uint32(uint16(s[4])<<8|uint16(s[5])), a)
						fr, fg, fb = t(fr, fg, fb)
						r, g, b := premultiply(fr, a), premultiply(fg, a), premultiply(fb, a)
						s[0], s[1] = uint8(r>>8), uint8(r)
						s[2], s[3] = uint8(g>>8), uint8(g)
						s[4], s[5] = uint8(b>>8), uint8(b)
					}
					row = row[8:]
				}
			}
		}
	case *image.Paletted:
		for i, c := range img.Palette {
			r, g, b, a := c.RGBA()
			if a != 0 {
				fr, fg, fb := unpremultiply(r, a), unpremultiply(g, a), unpremultiply(b, a)
				fr, fg, fb = t(fr, fg, fb)
				img.Palette[i] = &color.NRGBA64{R: f16i(fr), G: f16i(fg), B: f16i(fb)}
			}
		}
		return
	case *image.CMYK:
		g := tr.TransformGeneral
		f = func(start, limit int) {
			var inp, outp [4]float64
			i, o := inp[:], outp[:]
			for y := start; y < limit; y++ {
				row := img.Pix[img.Stride*y:]
				_ = row[4*(width-1)]
				for range width {
					r := row[0:4:4]
					inp[0], inp[1], inp[2], inp[3] = f8(r[0]), f8(r[1]), f8(r[2]), f8(r[3])
					g(o, i)
					r[0], r[1], r[2], r[3] = f8i(o[0]), f8i(o[1]), f8i(o[2]), f8i(o[3])
					row = row[4:]
				}
			}
		}
	case *image.YCbCr:
		d := imaging.NewNRGB(b)
		ans = d
		f = func(start, limit int) {
			for y := start; y < limit; y++ {
				ybase := y * img.YStride
				row := d.Pix[d.Stride*y:]
				for x := b.Min.X; x < b.Max.X; x++ {
					iy := ybase + (x - b.Min.X)
					ic := img.COffset(x, y+b.Min.Y)
					r, g, b := color.YCbCrToRGB(img.Y[iy], img.Cb[ic], img.Cr[ic])
					fr, fg, fb := t(f8(r), f8(g), f8(b))
					row[0], row[1], row[2] = f8i(fr), f8i(fg), f8i(fb)
					row = row[3:]
				}
			}
		}
	case draw.Image:
		f = func(start, limit int) {
			for y := b.Min.Y + start; y < b.Min.Y+limit; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					r16, g16, b16, a16 := img.At(x, y).RGBA()
					if a16 != 0 {
						fr, fg, fb := unpremultiply(r16, a16), unpremultiply(g16, a16), unpremultiply(b16, a16)
						fr, fg, fb = t(fr, fg, fb)
						img.Set(x, y, &color.NRGBA64{R: f16i(fr), G: f16i(fg), B: f16i(fb)})
					}
				}
			}
		}
	default:
		d := image.NewNRGBA64(b)
		ans = d
		f = func(start, limit int) {
			for y := start; y < limit; y++ {
				row := d.Pix[d.Stride*y:]
				for x := range width {
					r16, g16, b16, a16 := img.At(x+b.Min.X, y+b.Min.Y).RGBA()
					if a16 != 0 {
						fr, fg, fb := unpremultiply(r16, a16), unpremultiply(g16, a16), unpremultiply(b16, a16)
						fr, fg, fb = t(fr, fg, fb)
						r, g, b := f16i(fr), f16i(fg), f16i(fb)
						s := row[0:8:8]
						row = row[8:]
						s[0], s[1] = uint8(r>>8), uint8(r)
						s[2], s[3] = uint8(g>>8), uint8(g)
						s[4], s[5] = uint8(b>>8), uint8(b)
						s[6] = uint8(a16 >> 8)
						s[7] = uint8(a16)
					}
				}
			}
		}
	}
	err = parallel.Run_in_parallel_over_range(0, f, 0, height)
	return
}
