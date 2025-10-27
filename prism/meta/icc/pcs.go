package icc

import (
	"fmt"

	"github.com/kovidgoyal/imaging/colorconv"
)

var _ = fmt.Println

// A transformer to convert LAB colors to normalized [0,1] values
type NormalizeLAB int

var _ ChannelTransformer = (*NormalizeLAB)(nil)
var _ ChannelTransformer = (*BlackPointCorrection)(nil)

func (n NormalizeLAB) String() string                        { return "NormalizeLAB" }
func (n NormalizeLAB) IOSig() (int, int)                     { return 3, 3 }
func (n *NormalizeLAB) Iter(f func(ChannelTransformer) bool) { f(n) }
func (m *NormalizeLAB) Transform(l, a, b unit_float) (unit_float, unit_float, unit_float) {
	return l / 100, (a + 128) / 255, (b + 128) / 255
}

func NewNormalizeLAB() *NormalizeLAB {
	x := NormalizeLAB(0)
	return &x
}

type BlackPointCorrection struct {
	scale, offset XYZType
}

func (n BlackPointCorrection) IOSig() (int, int)                     { return 3, 3 }
func (n *BlackPointCorrection) Iter(f func(ChannelTransformer) bool) { f(n) }

func NewBlackPointCorrection(in_whitepoint, in_blackpoint, out_blackpoint XYZType) *BlackPointCorrection {
	tx := in_blackpoint.X - in_whitepoint.X
	ty := in_blackpoint.Y - in_whitepoint.Y
	tz := in_blackpoint.Z - in_whitepoint.Z
	ans := BlackPointCorrection{}

	ans.scale.X = (out_blackpoint.X - in_whitepoint.X) / tx
	ans.scale.Y = (out_blackpoint.Y - in_whitepoint.Y) / ty
	ans.scale.Z = (out_blackpoint.Z - in_whitepoint.Z) / tz

	ans.offset.X = -in_whitepoint.X * (out_blackpoint.X - in_blackpoint.X) / tx
	ans.offset.Y = -in_whitepoint.Y * (out_blackpoint.Y - in_blackpoint.Y) / ty
	ans.offset.Z = -in_whitepoint.Z * (out_blackpoint.Z - in_blackpoint.Z) / tz

	return &ans
}

func (c *BlackPointCorrection) String() string {
	return fmt.Sprintf("BlackPointCorrection{scale: %v offset: %v}", c.scale, c.offset)
}

func (c *BlackPointCorrection) Transform(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	return c.scale.X*r + c.offset.X, c.scale.Y*g + c.offset.Y, c.scale.Z*b + c.offset.Z
}

type LABtosRGB struct{ c *colorconv.ConvertColor }

func NewLABtosRGB(whitepoint XYZType) LABtosRGB {
	return LABtosRGB{colorconv.NewConvertColor(whitepoint.X, whitepoint.Y, whitepoint.Z)}
}

func (c LABtosRGB) Transform(l, a, b unit_float) (unit_float, unit_float, unit_float) {
	return c.c.LabToSRGB(l, a, b)
}
func (n LABtosRGB) IOSig() (int, int)                     { return 3, 3 }
func (n LABtosRGB) String() string                        { return "LABtosRGB" }
func (n *LABtosRGB) Iter(f func(ChannelTransformer) bool) { f(n) }
