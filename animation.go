package imaging

import (
	"fmt"
	"image"
	"image/gif"
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
