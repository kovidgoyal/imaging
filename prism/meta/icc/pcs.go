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

func tg33(t func(r, g, b unit_float) (x, y, z unit_float), o, i []unit_float) {
	_ = o[len(i)-1]
	limit := len(i) / 3
	for range limit {
		o[0], o[1], o[2] = t(i[0], i[1], i[2])
		i, o = i[3:], o[3:]
	}
}

func (m *NormalizeLAB) TransformGeneral(o, i []unit_float) { tg33(m.Transform, o, i) }

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
func (m *BlackPointCorrection) TransformGeneral(o, i []unit_float) { tg33(m.Transform, o, i) }

type LABtosRGB struct {
	c *colorconv.ConvertColor
	t func(l, a, b unit_float) (x, y, z unit_float)
}

func NewLABtosRGB(whitepoint XYZType) LABtosRGB {
	c := colorconv.NewConvertColor(whitepoint.X, whitepoint.Y, whitepoint.Z)
	return LABtosRGB{c, c.LabToSRGB}
}

func (c LABtosRGB) Transform(l, a, b unit_float) (unit_float, unit_float, unit_float) {
	return c.t(l, a, b)
}
func (m LABtosRGB) TransformGeneral(o, i []unit_float)   { tg33(m.Transform, o, i) }
func (n LABtosRGB) IOSig() (int, int)                    { return 3, 3 }
func (n LABtosRGB) String() string                       { return fmt.Sprintf("%T%s", n, n.c.String()) }
func (n LABtosRGB) Iter(f func(ChannelTransformer) bool) { f(n) }

type XYZtosRGB struct {
	c *colorconv.ConvertColor
	t func(l, a, b unit_float) (x, y, z unit_float)
}

func NewXYZtosRGB(whitepoint XYZType) XYZtosRGB {
	c := colorconv.NewConvertColor(whitepoint.X, whitepoint.Y, whitepoint.Z)
	return XYZtosRGB{c, c.XYZToSRGB}
}

func (c XYZtosRGB) Transform(l, a, b unit_float) (unit_float, unit_float, unit_float) {
	return c.t(l, a, b)
}
func (m XYZtosRGB) TransformGeneral(o, i []unit_float)   { tg33(m.Transform, o, i) }
func (n XYZtosRGB) IOSig() (int, int)                    { return 3, 3 }
func (n XYZtosRGB) String() string                       { return fmt.Sprintf("%T%s", n, n.c.String()) }
func (n XYZtosRGB) Iter(f func(ChannelTransformer) bool) { f(n) }

type LABtoXYZ struct {
	c *colorconv.ConvertColor
	t func(l, a, b unit_float) (x, y, z unit_float)
}

func NewLABtoXYZ(whitepoint XYZType) LABtoXYZ {
	c := colorconv.NewConvertColor(whitepoint.X, whitepoint.Y, whitepoint.Z)
	return LABtoXYZ{c, c.LabToXYZ}
}

func (c LABtoXYZ) Transform(l, a, b unit_float) (unit_float, unit_float, unit_float) {
	return c.t(l, a, b)
}
func (m LABtoXYZ) TransformGeneral(o, i []unit_float)   { tg33(m.Transform, o, i) }
func (n LABtoXYZ) IOSig() (int, int)                    { return 3, 3 }
func (n LABtoXYZ) String() string                       { return fmt.Sprintf("%T%s", n, n.c.String()) }
func (n LABtoXYZ) Iter(f func(ChannelTransformer) bool) { f(n) }

type XYZtoLAB struct {
	c *colorconv.ConvertColor
	t func(l, a, b unit_float) (x, y, z unit_float)
}

func NewXYZtoLAB(whitepoint XYZType) XYZtoLAB {
	c := colorconv.NewConvertColor(whitepoint.X, whitepoint.Y, whitepoint.Z)
	return XYZtoLAB{c, c.XYZToLab}
}

func (c XYZtoLAB) Transform(l, a, b unit_float) (unit_float, unit_float, unit_float) {
	return c.t(l, a, b)
}
func (m XYZtoLAB) TransformGeneral(o, i []unit_float)   { tg33(m.Transform, o, i) }
func (n XYZtoLAB) IOSig() (int, int)                    { return 3, 3 }
func (n XYZtoLAB) String() string                       { return fmt.Sprintf("%T%s", n, n.c.String()) }
func (n XYZtoLAB) Iter(f func(ChannelTransformer) bool) { f(n) }
