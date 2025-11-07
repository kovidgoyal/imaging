package imaging

import (
	"fmt"
	"image/gif"
	"time"

	"github.com/kettek/apng"
	"github.com/kovidgoyal/imaging/prism/meta/gifmeta"
)

var _ = fmt.Print

func (self *Image) populate_from_apng(p *apng.APNG) {
	self.LoopCount = p.LoopCount
	anchor_frame := uint(1)
	for _, f := range p.Frames {
		if f.IsDefault {
			self.DefaultImage = f.Image
			continue
		}
		frame := Frame{Number: uint(len(self.Frames) + 1), Image: NormalizeOrigin(f.Image), X: f.XOffset, Y: f.YOffset,
			Replace: f.BlendOp == apng.BLEND_OP_SOURCE,
			Delay:   time.Duration(float64(time.Second) * f.GetDelay())}
		dp := uint8(gif.DisposalNone)
		switch f.DisposeOp {
		case apng.DISPOSE_OP_BACKGROUND:
			dp = gif.DisposalBackground
		case apng.DISPOSE_OP_NONE:
			dp = gif.DisposalNone
		case apng.DISPOSE_OP_PREVIOUS:
			dp = gif.DisposalPrevious
		}
		anchor_frame, frame.ComposeOnto = gifmeta.SetGIFFrameDisposal(frame.Number, anchor_frame, dp)
		self.Frames = append(self.Frames, &frame)
	}
}

func (self *Image) populate_from_gif(g *gif.GIF) {
	min_gap := gifmeta.CalcMinimumGap(g.Delay)
	anchor_frame := uint(1)
	for i, img := range g.Image {
		b := img.Bounds()
		frame := Frame{Number: uint(len(self.Frames) + 1), Image: NormalizeOrigin(img), X: b.Min.X, Y: b.Min.Y,
			Delay: gifmeta.CalculateFrameDelay(g.Delay[i], min_gap)}
		anchor_frame, frame.ComposeOnto = gifmeta.SetGIFFrameDisposal(frame.Number, anchor_frame, g.Disposal[i])
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
