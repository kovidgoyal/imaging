package icc

import (
	"fmt"
)

var _ = fmt.Println
var D50 = XYZType{0.9642, 1.0, 0.82491}

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

func NewBlackPointCorrection(in_blackpoint, out_blackpoint XYZType) *BlackPointCorrection {
	tx := in_blackpoint.X - D50.X
	ty := in_blackpoint.Y - D50.Y
	tz := in_blackpoint.Z - D50.Z
	ans := BlackPointCorrection{}

	ans.scale.X = (out_blackpoint.X - D50.X) / tx
	ans.scale.Y = (out_blackpoint.Y - D50.Y) / ty
	ans.scale.Z = (out_blackpoint.Z - D50.Z) / tz

	ans.offset.X = -D50.X * (out_blackpoint.X - in_blackpoint.X) / tx
	ans.offset.Y = -D50.Y * (out_blackpoint.Y - in_blackpoint.Y) / ty
	ans.offset.Z = -D50.Z * (out_blackpoint.Z - in_blackpoint.Z) / tz

	return &ans
}

func (c *BlackPointCorrection) String() string {
	return fmt.Sprintf("BlackPointCorrection{scale: %v offset: %v}", c.scale, c.offset)
}

func (c *BlackPointCorrection) Transform(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	return c.scale.X*r + c.offset.X, c.scale.Y*g + c.offset.Y, c.scale.Z*b + c.offset.Z
}

type LabToXYZ XYZType

func NewLabToXYZ(illuminant XYZType) *LabToXYZ {
	x := LabToXYZ(illuminant)
	return &x
}

func NewLabToXYZStandard() *LabToXYZ {
	x := LabToXYZ(D50)
	return &x
}

func (x LabToXYZ) String() string                        { return fmt.Sprintf("LabToXYZ{ %v }", XYZType(x)) }
func (x LabToXYZ) IOSig() (int, int)                     { return 3, 3 }
func (x *LabToXYZ) Iter(f func(ChannelTransformer) bool) { f(x) }
func (wt *LabToXYZ) Transform(l, a, b unit_float) (x, y, z unit_float) {
	return Lab_to_XYZ(wt.X, wt.Y, wt.Z, l, a, b)
}

func f_1(t unit_float) unit_float {
	const limit = (24.0 / 116.0)
	if t <= limit {
		return (108.0 / 841.0) * (t - (16.0 / 116.0))
	}
	return t * t * t
}

// Standard Lab to XYZ. It can return negative XYZ in some cases
func Lab_to_XYZ(wt_x, wt_y, wt_z, l, a, b unit_float) (x, y, z unit_float) {
	y = (l + 16.0) / 116.0
	x = y + 0.002*a
	z = y - 0.005*b

	x = f_1(x) * wt_x
	y = f_1(y) * wt_y
	z = f_1(z) * wt_z
	return
}
