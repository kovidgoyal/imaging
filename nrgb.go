package imaging

import (
	"fmt"
	"image"
	"image/color"
)

var _ = fmt.Print

type NRGBColor struct {
	R, G, B uint8
}

func (c NRGBColor) AsSharp() string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

func (c NRGBColor) String() string {
	return fmt.Sprintf("NRGBColor{%02X %02X %02X}", c.R, c.G, c.B)
}

func (c NRGBColor) RGBA() (r, g, b, a uint32) {
	r = uint32(c.R)
	r |= r << 8
	g = uint32(c.G)
	g |= g << 8
	b = uint32(c.B)
	b |= b << 8
	a = 65535 // (255 << 8 | 255)
	return
}

// NRGB is an in-memory image whose At method returns NRGBColor values.
type NRGB struct {
	// Pix holds the image's pixels, in R, G, B order. The pixel at
	// (x, y) starts at Pix[(y-Rect.Min.Y)*Stride + (x-Rect.Min.X)*3].
	Pix []uint8
	// Stride is the Pix stride (in bytes) between vertically adjacent pixels.
	Stride int
	// Rect is the image's bounds.
	Rect image.Rectangle
}

func nrgbModel(c color.Color) color.Color {
	if _, ok := c.(NRGBColor); ok {
		return c
	}
	r, g, b, a := c.RGBA()
	switch a {
	case 0xffff:
		return NRGBColor{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
	case 0:
		return NRGBColor{0, 0, 0}
	default:
		// Since Color.RGBA returns an alpha-premultiplied color, we should have r <= a && g <= a && b <= a.
		r = (r * 0xffff) / a
		g = (g * 0xffff) / a
		b = (b * 0xffff) / a
		return NRGBColor{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
	}
}

var NRGBModel color.Model = color.ModelFunc(nrgbModel)

func (p *NRGB) ColorModel() color.Model { return NRGBModel }

func (p *NRGB) Bounds() image.Rectangle { return p.Rect }

func (p *NRGB) At(x, y int) color.Color {
	return p.NRGBAt(x, y)
}

func (p *NRGB) NRGBAt(x, y int) NRGBColor {
	if !(image.Point{x, y}.In(p.Rect)) {
		return NRGBColor{}
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+3 : i+3] // Small cap improves performance, see https://golang.org/issue/27857
	return NRGBColor{s[0], s[1], s[2]}
}

// PixOffset returns the index of the first element of Pix that corresponds to
// the pixel at (x, y).
func (p *NRGB) PixOffset(x, y int) int {
	return (y-p.Rect.Min.Y)*p.Stride + (x-p.Rect.Min.X)*3
}

func (p *NRGB) Set(x, y int, c color.Color) {
	if !(image.Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	c1 := NRGBModel.Convert(c).(NRGBColor)
	s := p.Pix[i : i+3 : i+3] // Small cap improves performance, see https://golang.org/issue/27857
	s[0] = c1.R
	s[1] = c1.G
	s[2] = c1.B
}

func (p *NRGB) SetRGBA64(x, y int, c color.RGBA64) {
	if !(image.Point{x, y}.In(p.Rect)) {
		return
	}
	r, g, b, a := uint32(c.R), uint32(c.G), uint32(c.B), uint32(c.A)
	if (a != 0) && (a != 0xffff) {
		r = (r * 0xffff) / a
		g = (g * 0xffff) / a
		b = (b * 0xffff) / a
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+3 : i+3] // Small cap improves performance, see https://golang.org/issue/27857
	s[0] = uint8(r >> 8)
	s[1] = uint8(g >> 8)
	s[2] = uint8(b >> 8)
}

func (p *NRGB) SetNRGBA(x, y int, c color.NRGBA) {
	if !(image.Point{x, y}.In(p.Rect)) {
		return
	}
	i := p.PixOffset(x, y)
	s := p.Pix[i : i+3 : i+3] // Small cap improves performance, see https://golang.org/issue/27857
	s[0] = c.R
	s[1] = c.G
	s[2] = c.B
}

// SubImage returns an image representing the portion of the image p visible
// through r. The returned value shares pixels with the original image.
func (p *NRGB) SubImage(r image.Rectangle) image.Image {
	r = r.Intersect(p.Rect)
	// If r1 and r2 are Rectangles, r1.Intersect(r2) is not guaranteed to be inside
	// either r1 or r2 if the intersection is empty. Without explicitly checking for
	// this, the Pix[i:] expression below can panic.
	if r.Empty() {
		return &NRGB{}
	}
	i := p.PixOffset(r.Min.X, r.Min.Y)
	return &NRGB{
		Pix:    p.Pix[i:],
		Stride: p.Stride,
		Rect:   r,
	}
}

// Opaque scans the entire image and reports whether it is fully opaque.
func (p *NRGB) Opaque() bool { return true }

func NewNRGB(r image.Rectangle) *NRGB {
	return &NRGB{
		Pix:    make([]uint8, 3*r.Dx()*r.Dy()),
		Stride: 3 * r.Dx(),
		Rect:   r,
	}
}

func NewNRGBWithContiguousRGBPixels(p []byte, left, top, width, height int) (*NRGB, error) {
	const bpp = 3
	if expected := bpp * width * height; expected != len(p) {
		return nil, fmt.Errorf("the image width and height dont match the size of the specified pixel data: width=%d height=%d sz=%d != %d", width, height, len(p), expected)
	}
	return &NRGB{
		Pix:    p,
		Stride: bpp * width,
		Rect:   image.Rectangle{image.Point{left, top}, image.Point{left + width, top + height}},
	}, nil
}
