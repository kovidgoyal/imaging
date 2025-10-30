package icc

import (
	"fmt"

	"github.com/kovidgoyal/imaging/colorconv"
)

var _ = fmt.Println

func tg33(t func(r, g, b unit_float) (x, y, z unit_float), o, i []unit_float) {
	_ = o[len(i)-1]
	limit := len(i) / 3
	for range limit {
		o[0], o[1], o[2] = t(i[0], i[1], i[2])
		i, o = i[3:], o[3:]
	}
}

// A transformer to convert normalized [0,1] values to the [0,1.99997]
// (u1Fixed15Number) values used by ICC XYZ PCS space
type NormalizedToXYZ int

const MAX_ENCODEABLE_XYZ = 1.0 + 32767.0/32768.0
const MAX_ENCODEABLE_XYZ_INVERSE = 1 / (MAX_ENCODEABLE_XYZ)

func (n NormalizedToXYZ) String() string                        { return "NormalizedToXYZ" }
func (n NormalizedToXYZ) IOSig() (int, int)                     { return 3, 3 }
func (n *NormalizedToXYZ) Iter(f func(ChannelTransformer) bool) { f(n) }
func (m *NormalizedToXYZ) Transform(x, y, z unit_float) (unit_float, unit_float, unit_float) {
	return x * MAX_ENCODEABLE_XYZ, y * MAX_ENCODEABLE_XYZ, z * MAX_ENCODEABLE_XYZ
}
func (m *NormalizedToXYZ) AsMatrix3() *Matrix3 { return NewScalingMatrix3(MAX_ENCODEABLE_XYZ) }

func (m *NormalizedToXYZ) TransformGeneral(o, i []unit_float) { tg33(m.Transform, o, i) }

func NewNormalizedToXYZ() *NormalizedToXYZ {
	x := NormalizedToXYZ(0)
	return &x
}

type XYZToNormalized int

func (n XYZToNormalized) String() string                        { return "XYZToNormalized" }
func (n XYZToNormalized) IOSig() (int, int)                     { return 3, 3 }
func (n *XYZToNormalized) Iter(f func(ChannelTransformer) bool) { f(n) }
func (m *XYZToNormalized) Transform(x, y, z unit_float) (unit_float, unit_float, unit_float) {
	return x * MAX_ENCODEABLE_XYZ_INVERSE, y * MAX_ENCODEABLE_XYZ_INVERSE, z * MAX_ENCODEABLE_XYZ_INVERSE
}
func (m *XYZToNormalized) AsMatrix3() *Matrix3 { return NewScalingMatrix3(MAX_ENCODEABLE_XYZ_INVERSE) }

func (m *XYZToNormalized) TransformGeneral(o, i []unit_float) { tg33(m.Transform, o, i) }

func NewXYZToNormalized() *XYZToNormalized {
	x := XYZToNormalized(0)
	return &x
}

// A transformer to convert normalized [0,1] to the LAB co-ordinate system
// used by ICC PCS LAB profiles [0-100], [-128, 127]
type NormalizedToLAB int

func (n NormalizedToLAB) String() string                        { return "NormalizedToLAB" }
func (n NormalizedToLAB) IOSig() (int, int)                     { return 3, 3 }
func (n *NormalizedToLAB) Iter(f func(ChannelTransformer) bool) { f(n) }
func (m *NormalizedToLAB) Transform(x, y, z unit_float) (unit_float, unit_float, unit_float) {
	// See PackLabDoubleFromFloat in lcms source code
	return x * 100, (y*255 - 128), (z*255 - 128)
}

func (m *NormalizedToLAB) TransformGeneral(o, i []unit_float) { tg33(m.Transform, o, i) }

func NewNormalizedToLAB() *NormalizedToLAB {
	x := NormalizedToLAB(0)
	return &x
}

type LABToNormalized int

func (n LABToNormalized) String() string                        { return "LABToNormalized" }
func (n LABToNormalized) IOSig() (int, int)                     { return 3, 3 }
func (n *LABToNormalized) Iter(f func(ChannelTransformer) bool) { f(n) }
func (m *LABToNormalized) Transform(x, y, z unit_float) (unit_float, unit_float, unit_float) {
	// See PackLabDoubleFromFloat in lcms source code
	return x * (1. / 100), (y*(1./255) + 128./255), (z*(1./255) + 128./255)
}

func (m *LABToNormalized) TransformGeneral(o, i []unit_float) { tg33(m.Transform, o, i) }

func NewLABToNormalized() *LABToNormalized {
	x := LABToNormalized(0)
	return &x
}

type BlackPointCorrection struct {
	scale, offset XYZType
}

func (n BlackPointCorrection) IOSig() (int, int) { return 3, 3 }
func (n *BlackPointCorrection) Iter(f func(ChannelTransformer) bool) {
	m := &Matrix3{{n.scale.X, 0, 0}, {0, n.scale.Y, 0}, {0, 0, n.scale.Z}}
	if !is_identity_matrix(m) {
		if !f(m) {
			return
		}
	}
	t := &Translation{n.offset.X, n.offset.Y, n.offset.Z}
	if !t.Empty() {
		f(t)
	}
}

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
	ans.offset.X *= MAX_ENCODEABLE_XYZ_INVERSE
	ans.offset.Y *= MAX_ENCODEABLE_XYZ_INVERSE
	ans.offset.Z *= MAX_ENCODEABLE_XYZ_INVERSE

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
	c := colorconv.NewConvertColor(whitepoint.X, whitepoint.Y, whitepoint.Z, 1)
	return LABtosRGB{c, c.LabToSRGB}
}

func (c LABtosRGB) Transform(l, a, b unit_float) (unit_float, unit_float, unit_float) {
	return c.t(l, a, b)
}
func (m LABtosRGB) TransformGeneral(o, i []unit_float)   { tg33(m.Transform, o, i) }
func (n LABtosRGB) IOSig() (int, int)                    { return 3, 3 }
func (n LABtosRGB) String() string                       { return fmt.Sprintf("%T%s", n, n.c.String()) }
func (n LABtosRGB) Iter(f func(ChannelTransformer) bool) { f(n) }

type UniformFunctionTransformer struct {
	name string
	f    func(unit_float) unit_float
}

func (n UniformFunctionTransformer) IOSig() (int, int)                     { return 3, 3 }
func (n UniformFunctionTransformer) String() string                        { return n.name }
func (n *UniformFunctionTransformer) Iter(f func(ChannelTransformer) bool) { f(n) }
func (c *UniformFunctionTransformer) Transform(x, y, z unit_float) (unit_float, unit_float, unit_float) {
	return c.f(x), c.f(y), c.f(z)
}
func (c *UniformFunctionTransformer) TransformGeneral(o, i []unit_float) {
	for k, x := range i {
		o[k] = c.f(x)
	}
}

type XYZtosRGB struct {
	c *colorconv.ConvertColor
	t func(l, a, b unit_float) (x, y, z unit_float)
}

func NewXYZtosRGB(whitepoint XYZType, clamp, map_gamut bool) XYZtosRGB {
	c := colorconv.NewConvertColor(whitepoint.X, whitepoint.Y, whitepoint.Z, 1)
	if clamp {
		if map_gamut {
			return XYZtosRGB{c, c.XYZToSRGB}
		}
		return XYZtosRGB{c, c.XYZToSRGBNoGamutMap}
	}
	return XYZtosRGB{c, c.XYZToSRGBNoClamp}
}

func (n *XYZtosRGB) AddPreviousMatrix(m Matrix3) {
	n.c.AddPreviousMatrix(m[0], m[1], m[2])
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
	c := colorconv.NewConvertColor(whitepoint.X, whitepoint.Y, whitepoint.Z, 1)
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
	c := colorconv.NewConvertColor(whitepoint.X, whitepoint.Y, whitepoint.Z, 1)
	return XYZtoLAB{c, c.XYZToLab}
}

func (c XYZtoLAB) Transform(l, a, b unit_float) (unit_float, unit_float, unit_float) {
	return c.t(l, a, b)
}
func (m XYZtoLAB) TransformGeneral(o, i []unit_float)   { tg33(m.Transform, o, i) }
func (n XYZtoLAB) IOSig() (int, int)                    { return 3, 3 }
func (n XYZtoLAB) String() string                       { return fmt.Sprintf("%T%s", n, n.c.String()) }
func (n XYZtoLAB) Iter(f func(ChannelTransformer) bool) { f(n) }
