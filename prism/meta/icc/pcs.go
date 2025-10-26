package icc

import (
	"fmt"
)

var _ = fmt.Println

// A transformer to convert LAB colors to normalized [0,1] values
type NormalizeLAB struct {
	transform ChannelTransformer
	tfunc     func(r, g, b unit_float) (unit_float, unit_float, unit_float)
}

var _ ChannelTransformer = (*NormalizeLAB)(nil)

func (n NormalizeLAB) String() string              { return "NormalizeLAB" }
func (n NormalizeLAB) IsSuitableFor(int, int) bool { return true }
func (m *NormalizeLAB) Transform(l, a, b unit_float) (unit_float, unit_float, unit_float) {
	return m.tfunc(l/100, (a+128)/255, (b+128)/255)
}

func (m *NormalizeLAB) TransformDebug(l, a, b unit_float, callback Debug_callback) (unit_float, unit_float, unit_float) {
	x, y, z := l/100, (a+128)/255, (b+128)/255
	callback(l, a, b, x, y, z, m)
	return m.transform.TransformDebug(x, y, z, callback)
}

func NewNormalizeLAB(t ChannelTransformer) ChannelTransformer {
	return &NormalizeLAB{t, t.Transform}
}
