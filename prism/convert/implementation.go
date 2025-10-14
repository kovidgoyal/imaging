package convert

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/kovidgoyal/go-parallel"
	"github.com/kovidgoyal/imaging"
)

var _ = fmt.Print

func premultiply8(r, a uint8) uint8 {
	return uint8((uint16(r) * uint16(a)) / uint16(0xff))
}

func unpremultiply8(r, a uint8) uint8 {
	return uint8((uint16(r) * 0xff) / uint16(a))
}

func unpremultiply(r, a uint32) uint16 {
	return uint16((r * 0xffff) / a)
}

func premultiply(r, a uint32) uint16 {
	return uint16((r * a) / 0xffff)
}

func convert(image_any image.Image, convert8 Convert8, convert16 Convert16) (ans image.Image, err error) {
	b := image_any.Bounds()
	width, height := b.Dx(), b.Dy()
	ans = image_any
	buf := []uint16{0, 0, 0}
	sl := buf[0:3:3]
	var f func(start, limit int)
	switch img := image_any.(type) {
	case *imaging.NRGB:
		f = func(start, limit int) {
			for y := start; y < limit; y++ {
				row := img.Pix[img.Stride*y:]
				_ = row[3*(width-1)]
				for range width {
					convert8(row[0:3:3])
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
					convert8(row[0:3:3])
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
					sl[0] = uint16(s[0])<<8 | uint16(s[1])
					sl[1] = uint16(s[2])<<8 | uint16(s[3])
					sl[2] = uint16(s[4])<<8 | uint16(s[5])
					convert16(sl)
					s[0] = uint8(sl[0] >> 8)
					s[1] = uint8(sl[0])
					s[2] = uint8(sl[1] >> 8)
					s[3] = uint8(sl[1])
					s[4] = uint8(sl[2] >> 8)
					s[5] = uint8(sl[2])
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
						r[0], r[1], r[2] = unpremultiply8(r[0], a), unpremultiply8(r[1], a), unpremultiply8(r[2], a)
						convert8(r)
						r[0], r[1], r[2] = premultiply8(r[0], a), premultiply8(r[1], a), premultiply8(r[2], a)
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
						sl[0] = unpremultiply(uint32(uint16(s[0])<<8|uint16(s[1])), a)
						sl[1] = unpremultiply(uint32(uint16(s[2])<<8|uint16(s[3])), a)
						sl[2] = unpremultiply(uint32(uint16(s[4])<<8|uint16(s[5])), a)
						convert16(sl)
						sl[0] = premultiply(uint32(sl[0]), a)
						sl[1] = premultiply(uint32(sl[0]), a)
						sl[2] = premultiply(uint32(sl[0]), a)
						s[0] = uint8(sl[0] >> 8)
						s[1] = uint8(sl[0])
						s[2] = uint8(sl[1] >> 8)
						s[3] = uint8(sl[1])
						s[4] = uint8(sl[2] >> 8)
						s[5] = uint8(sl[2])
					}
					row = row[8:]
				}
			}
		}
	case *image.Paletted:
		for i, c := range img.Palette {
			r, g, b, a := c.RGBA()
			if a != 0 {
				sl[0], sl[1], sl[2] = unpremultiply(r, a), unpremultiply(g, a), unpremultiply(b, a)
				convert16(sl)
				img.Palette[i] = &color.NRGBA64{R: sl[0], G: sl[1], B: sl[2]}
			}
		}
		return
	case *image.Gray:
		d := imaging.NewNRGB(b)
		ans = d
		f = func(start, limit int) {
			sl := []uint8{0, 0, 0}
			for y := start; y < limit; y++ {
				row := img.Pix[img.Stride*y:]
				_ = row[width-1]
				drow := d.Pix[d.Stride*y:]
				_ = drow[3*(width-1)]
				for _, gray := range row {
					sl[0], sl[1], sl[2] = gray, gray, gray
					convert8(sl)
					drow[0], drow[1], drow[2] = sl[0], sl[1], sl[2]
					drow = drow[3:]
				}
			}
		}
	case *image.Gray16:
		d := image.NewNRGBA64(b)
		ans = d
		f = func(start, limit int) {
			for y := start; y < limit; y++ {
				row := img.Pix[img.Stride*y:]
				_ = row[2*(width-1)]
				drow := d.Pix[d.Stride*y:]
				_ = drow[8*(width-1)]
				for range width {
					gray := uint16(row[0])<<8 | uint16(row[1])
					sl[0], sl[1], sl[2] = gray, gray, gray
					convert16(sl)
					s := drow[0:8:8]
					s[0] = uint8(sl[0] >> 8)
					s[1] = uint8(sl[0])
					s[2] = uint8(sl[1] >> 8)
					s[3] = uint8(sl[1])
					s[4] = uint8(sl[2] >> 8)
					s[5] = uint8(sl[2])
					s[6] = 0xff
					s[7] = 0xff
					row = row[2:]
					drow = drow[8:]
				}
			}
		}

	case draw.Image:
		f = func(start, limit int) {
			for y := b.Min.Y + start; y < b.Min.Y+limit; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					r16, g16, b16, a16 := img.At(x, y).RGBA()
					if a16 != 0 {
						sl[0], sl[1], sl[2] = unpremultiply(r16, a16), unpremultiply(g16, a16), unpremultiply(b16, a16)
						convert16(sl)
						img.Set(x, y, &color.NRGBA64{R: sl[0], G: sl[1], B: sl[2]})
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
				for x := 0; x < width; x++ {
					r16, g16, b16, a16 := img.At(x+b.Min.X, y+b.Min.Y).RGBA()
					if a16 != 0 {
						sl[0], sl[1], sl[2] = unpremultiply(r16, a16), unpremultiply(g16, a16), unpremultiply(b16, a16)
						convert16(sl)
						s := row[0:8:8]
						s[0] = uint8(sl[0] >> 8)
						s[1] = uint8(sl[0])
						s[2] = uint8(sl[1] >> 8)
						s[3] = uint8(sl[1])
						s[4] = uint8(sl[2] >> 8)
						s[5] = uint8(sl[2])
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
