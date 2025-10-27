package icc

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

var _ = fmt.Print

type Pipeline struct {
	transformers []ChannelTransformer
	tfuncs       []func(r, g, b unit_float) (unit_float, unit_float, unit_float)
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
	if cm, ok := c.(*Matrix3); ok {
		q := p.transformers[IfElse(prepend, idx, len(p.transformers)-1)]
		if mat, ok := q.(*Matrix3); ok {
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
