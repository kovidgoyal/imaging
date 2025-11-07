package imaging

import (
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/png"
	"io"
	"math"
	"time"

	"github.com/kettek/apng"
	"github.com/kovidgoyal/imaging/prism/meta"
	"github.com/kovidgoyal/imaging/prism/meta/gifmeta"
)

var _ = fmt.Print

type Frame struct {
	Number      uint
	X, Y        int
	Image       image.Image `json:"-"`
	Delay       time.Duration
	ComposeOnto uint
	Replace     bool // Do a simple pixel replacement rather than a full alpha blend when compositing this frame
}

type Image struct {
	Frames       []*Frame
	Metadata     *meta.Data
	LoopCount    uint        // 0 means loop forever, 1 means loop once, ...
	DefaultImage image.Image `json:"-"` // a "default image" for an animation that is not part of the actual animation
}

func (self *Image) populate_from_apng(p *apng.APNG) {
	self.LoopCount = p.LoopCount
	prev_disposal := apng.DISPOSE_OP_BACKGROUND
	var prev_compose_onto uint
	for _, f := range p.Frames {
		if f.IsDefault {
			self.DefaultImage = f.Image
			continue
		}
		frame := Frame{Number: uint(len(self.Frames) + 1), Image: NormalizeOrigin(f.Image), X: f.XOffset, Y: f.YOffset,
			Replace: f.BlendOp == apng.BLEND_OP_SOURCE,
			Delay:   time.Duration(float64(time.Second) * f.GetDelay())}
		switch prev_disposal {
		case apng.DISPOSE_OP_NONE:
			frame.ComposeOnto = frame.Number - 1
		case apng.DISPOSE_OP_PREVIOUS:
			frame.ComposeOnto = uint(prev_compose_onto)
		}
		prev_disposal, prev_compose_onto = int(f.DisposeOp), frame.ComposeOnto
		self.Frames = append(self.Frames, &frame)
	}
}

func (self *Image) populate_from_gif(g *gif.GIF) {
	min_gap := gifmeta.CalcMinimumGap(g.Delay)
	prev_disposal := uint8(gif.DisposalBackground)
	var prev_compose_onto uint
	for i, img := range g.Image {
		b := img.Bounds()
		frame := Frame{
			Number: uint(len(self.Frames) + 1), Image: NormalizeOrigin(img), X: b.Min.X, Y: b.Min.Y,
			Delay: gifmeta.CalculateFrameDelay(g.Delay[i], min_gap),
		}
		switch prev_disposal {
		case gif.DisposalNone:
			frame.ComposeOnto = frame.Number - 1
		case gif.DisposalPrevious:
			frame.ComposeOnto = prev_compose_onto
		case gif.DisposalBackground:
			// this is in contravention of the GIF spec but browsers and
			// gif2apng both do this, so follow them. Test image for this
			// is apple.gif
			frame.ComposeOnto = frame.Number - 1
		}
		prev_disposal, prev_compose_onto = g.Disposal[i], frame.ComposeOnto
		self.Frames = append(self.Frames, &frame)
	}
	switch {
	case g.LoopCount == 0:
		self.LoopCount = 0
	case g.LoopCount < 0:
		self.LoopCount = 1
	default:
		self.LoopCount = uint(g.LoopCount) + 1
	}
}

func (self *Image) Clone() *Image {
	ans := *self
	if ans.DefaultImage != nil {
		ans.DefaultImage = ClonePreservingType(ans.DefaultImage)
	}
	for i, f := range self.Frames {
		nf := *f
		nf.Image = ClonePreservingType(f.Image)
		ans.Frames[i] = &nf
	}
	return &ans
}

// Coalesce all animation frames so that each frame is a snapshot of the
// animation at that instant.
func (self *Image) Coalesce() {
	if len(self.Frames) == 1 {
		return
	}
	var canvas *image.RGBA64
	for _, f := range self.Frames {
		b := f.Image.Bounds()
		if f.ComposeOnto == 0 {
			canvas = image.NewRGBA64(image.Rect(0, 0, int(self.Metadata.PixelWidth), int(self.Metadata.PixelHeight)))
		} else {
			canvas = ClonePreservingType(self.Frames[f.ComposeOnto-1].Image).(*image.RGBA64)
		}
		op := draw.Over
		if f.Replace {
			op = draw.Src
		}
		draw.Draw(canvas, image.Rect(f.X, f.Y, f.X+b.Dx(), f.Y+b.Dy()), f.Image, image.Point{}, op)
		f.Image = canvas
		f.X = 0
		f.Y = 0
		f.ComposeOnto = 0
		f.Replace = true
	}
}

// converts a time.Duration to a numerator and denominator of type uint16.
// It finds the best rational approximation of the duration in seconds.
func as_fraction(d time.Duration) (num, den uint16) {
	if d <= 0 {
		return 0, 1
	}

	// Convert duration to seconds as a float64
	val := d.Seconds()

	// Use continued fractions to find the best rational approximation.
	// We look for the convergent that is closest to the original value
	// while keeping the numerator and denominator within uint16 bounds.

	bestNum, bestDen := uint16(0), uint16(1)
	bestError := math.Abs(val)

	var h, k [3]int64
	h[0], k[0] = 0, 1
	h[1], k[1] = 1, 0

	f := val

	for i := 2; i < 100; i++ { // Limit iterations to prevent infinite loops
		a := int64(f)

		// Calculate next convergent
		h[2] = a*h[1] + h[0]
		k[2] = a*k[1] + k[0]

		if h[2] > math.MaxUint16 || k[2] > math.MaxUint16 {
			// This convergent is out of bounds, so the previous one was the best we could do.
			break
		}

		numConv := uint16(h[2])
		denConv := uint16(k[2])

		currentVal := float64(numConv) / float64(denConv)
		currentError := math.Abs(val - currentVal)

		if currentError < bestError {
			bestError = currentError
			bestNum = numConv
			bestDen = denConv
		}

		// Check if we have a perfect approximation
		if f-float64(a) == 0.0 {
			break
		}

		f = 1.0 / (f - float64(a))

		h[0], h[1] = h[1], h[2]
		k[0], k[1] = k[1], k[2]
	}

	return bestNum, bestDen
}

func (self *Image) as_apng() (ans apng.APNG) {
	ans.LoopCount = self.LoopCount
	if self.DefaultImage != nil {
		ans.Frames = append(ans.Frames, apng.Frame{Image: self.DefaultImage, IsDefault: true})
	}
	for i, f := range self.Frames {
		d := apng.Frame{
			DisposeOp: apng.DISPOSE_OP_BACKGROUND, BlendOp: apng.BLEND_OP_OVER, XOffset: f.X, YOffset: f.Y, Image: f.Image,
		}
		if !f.Replace {
			d.BlendOp = apng.BLEND_OP_SOURCE
		}
		d.DelayNumerator, d.DelayDenominator = as_fraction(f.Delay)
		if i+1 < len(self.Frames) {
			nf := self.Frames[i+1]
			switch nf.ComposeOnto {
			case f.Number:
				d.DisposeOp = apng.DISPOSE_OP_NONE
			case 0:
				d.DisposeOp = apng.DISPOSE_OP_BACKGROUND
			case f.ComposeOnto:
				d.DisposeOp = apng.DISPOSE_OP_PREVIOUS
			}
		}
		ans.Frames = append(ans.Frames, d)
	}
	return
}

func (self *Image) EncodeAsPNG(w io.Writer) error {
	if len(self.Frames) < 2 {
		img := self.DefaultImage
		if img == nil {
			img = self.Frames[0].Image
		}
		return png.Encode(w, img)
	}
	// Unfortunately apng.Encode() is buggy or I am getting my dispose op
	// mapping wrong, so coalesce first
	img := self.Clone()
	img.Coalesce()
	return apng.Encode(w, img.as_apng())
}
