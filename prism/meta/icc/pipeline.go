package icc

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

var _ = fmt.Print

type Pipeline struct {
	coalesce_matrices bool
	transformers      []ChannelTransformer
	tfuncs            []func(r, g, b unit_float) (unit_float, unit_float, unit_float)
}

type AsMatrix3 interface {
	AsMatrix3() *Matrix3
}

// check for interface being nil or the dynamic value it points to being nil
func is_nil(i any) bool {
	if i == nil {
		return true // interface itself is nil
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (p *Pipeline) insert(idx int, c ChannelTransformer) {
	if is_nil(c) {
		return
	}
	switch c.(type) {
	case *IdentityMatrix:
		return
	}
	if len(p.transformers) == 0 {
		p.transformers = append(p.transformers, c)
		p.tfuncs = append(p.tfuncs, c.Transform)
		return
	}
	if idx >= len(p.transformers) {
		panic(fmt.Sprintf("cannot insert at idx: %d in pipeline of length: %d", idx, len(p.transformers)))
	}
	prepend := idx > -1
	if cmm, ok := c.(AsMatrix3); p.coalesce_matrices && ok {
		q := p.transformers[IfElse(prepend, idx, len(p.transformers)-1)]
		if cmat, ok := q.(AsMatrix3); ok {
			mat := cmat.AsMatrix3()
			cm := cmm.AsMatrix3()
			var combined Matrix3
			if prepend {
				combined = mat.Multiply(*cm)
			} else {
				combined = cm.Multiply(*mat)
				idx = len(p.transformers) - 1
			}
			p.transformers[idx] = &combined
			p.tfuncs[idx] = combined.Transform
			return
		}
	}
	if prepend {
		p.transformers = slices.Insert(p.transformers, 0, c)
		p.tfuncs = slices.Insert(p.tfuncs, 0, c.Transform)
	} else {
		p.transformers = append(p.transformers, c)
		p.tfuncs = append(p.tfuncs, c.Transform)
	}
}

func (p *Pipeline) Insert(idx int, c ChannelTransformer) {
	s := slices.Collect(c.Iter)
	if idx > -1 {
		slices.Reverse(s)
	}
	for _, x := range s {
		p.insert(idx, x)
	}
}

func (p *Pipeline) Append(c ...ChannelTransformer) {
	for _, x := range c {
		p.Insert(-1, x)
	}
}

func (p *Pipeline) RemoveLastMatrix3() *Matrix3 {
	if len(p.transformers) > 0 {
		if q, ok := p.transformers[len(p.transformers)-1].(AsMatrix3); ok {
			p.transformers = p.transformers[:len(p.transformers)-1]
			p.tfuncs = p.tfuncs[:len(p.tfuncs)-1]
			return q.AsMatrix3()
		}
	}
	return nil
}

func (p *Pipeline) Transform(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	for _, t := range p.tfuncs {
		r, g, b = t(r, g, b)
	}
	return r, g, b
}

func (p *Pipeline) TransformDebug(r, g, b unit_float, f Debug_callback) (unit_float, unit_float, unit_float) {
	for _, t := range p.transformers {
		x, y, z := t.Transform(r, g, b)
		f(r, g, b, x, y, z, t)
		r, g, b = x, y, z
	}
	return r, g, b
}

func (p *Pipeline) TransformGeneral(out, in []unit_float) {
	for _, t := range p.transformers {
		t.TransformGeneral(out, in)
	}
}

func (p *Pipeline) Len() int { return len(p.transformers) }

func transformers_as_string(t ...ChannelTransformer) string {
	items := make([]string, len(t))
	for i, t := range t {
		items[i] = t.String()
	}
	return strings.Join(items, " → ")
}

func (p *Pipeline) String() string {
	return transformers_as_string(p.transformers...)
}

func (p *Pipeline) IOSig() (i int, o int) {
	if len(p.transformers) == 0 {
		return -1, -1
	}
	i, _ = p.transformers[0].IOSig()
	_, o = p.transformers[0].IOSig()
	return
}

func (p *Pipeline) IsSuitableFor(i, o int) bool {
	for _, t := range p.transformers {
		qi, qo := t.IOSig()
		if qi != i {
			return false
		}
		i = qo
	}
	return i == o
}

func (p *Pipeline) UseTrilinearInsteadOfTetrahedral() {
	for i, q := range p.transformers {
		if x, ok := q.(*TetrahedralInterpolate); ok {
			p.transformers[i] = &TrilinearInterpolate{x.d, x.legacy}
			p.tfuncs[i] = p.transformers[i].Transform
		}
	}
}

func (p *Pipeline) IsXYZSRGB() bool {
	if p.Len() == 2 {
		if c, ok := p.transformers[0].(Curves); ok {
			is_srgb := true
			for _, cc := range c.Curves() {
				if q, ok := cc.(IsSRGB); ok {
					is_srgb = q.IsSRGB()
				} else {
					is_srgb = false
				}
				if !is_srgb {
					break
				}
			}
			if is_srgb {
				if c, ok := p.transformers[1].(AsMatrix3); ok {
					q := c.AsMatrix3()
					var expected_matrix = Matrix3{{0.218036, 0.192576, 0.0715343}, {0.111246, 0.358442, 0.0303044}, {0.00695811, 0.0485389, 0.357053}}
					// unfortunately there exist profiles in the wild that
					// deviate from the expected matrix by more than FLOAT_EQUALITY_THRESHOLD
					if q.Equals(&expected_matrix, 7*FLOAT_EQUALITY_THRESHOLD) {
						return true
					}
				}
			}
		}
	}
	return false
}
