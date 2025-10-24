package icc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

// ModularTag represents a modular tag section 10.12 and 10.13 of ICC.1-2202-05.pdf
type ModularTag struct {
	num_input_channels, num_output_channels int
	a_curves, m_curves, b_curves            []Curve1D
	clut, matrix                            ChannelTransformer
	transforms                              []func(workspace []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float)
	transform_objects                       []ChannelTransformer
	workspace_size                          int
	is_a_to_b                               bool
}

func (m ModularTag) String() string {
	return fmt.Sprintf("%s{ %s }", IfElse(m.is_a_to_b, "mAB", "mBA"), transformers_as_string(m.transform_objects...))
}

var _ ChannelTransformer = (*ModularTag)(nil)

func (m *ModularTag) AddTransform(c ChannelTransformer, prepend bool) {
	m.workspace_size = max(c.WorkspaceSize(), m.workspace_size)
	if len(m.transforms) == 0 {
		m.transform_objects = append(m.transform_objects, c)
		m.transforms = append(m.transforms, c.Transform)
		return
	}
	if cm, ok := c.(*Matrix3); ok {
		idx := IfElse(prepend, 0, len(m.transform_objects)-1)
		q := m.transform_objects[idx]
		if mat, ok := q.(*Matrix3); ok {
			var combined Matrix3
			if prepend {
				combined = mat.Multiply(*cm)
			} else {
				combined = cm.Multiply(*mat)
			}
			m.transform_objects[idx] = &combined
			m.transforms[idx] = combined.Transform
			return
		}
	}
	if prepend {
		slices.Insert(m.transform_objects, 0, c)
		slices.Insert(m.transforms, 0, c.Transform)
	} else {
		m.transform_objects = append(m.transform_objects, c)
		m.transforms = append(m.transforms, c.Transform)
	}
}

func (m *ModularTag) WorkspaceSize() int { return m.workspace_size }
func (m *ModularTag) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return m.num_input_channels == num_input_channels && m.num_output_channels == num_output_channels
}
func (m *ModularTag) Transform(workspace []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float) {
	for i, t := range m.transforms {
		fmt.Println(11111111, r, g, b, m.transform_objects[i].String())
		r, g, b = t(workspace, r, g, b)
	}
	fmt.Println(222222222, r, g, b)
	return r, g, b
}

func IfElse[T any](condition bool, if_val T, else_val T) T {
	if condition {
		return if_val
	}
	return else_val
}

func modularDecoder(raw []byte) (ans any, err error) {
	if len(raw) < 40 {
		return nil, errors.New("modular (mAB/mBA) tag too short")
	}
	var s Signature
	_, _ = binary.Decode(raw[:4], binary.BigEndian, &s)
	is_a_to_b := false
	switch s {
	case LutAtoBTypeSignature:
		is_a_to_b = true
	case LutBtoATypeSignature:
		is_a_to_b = false
	default:
		return nil, fmt.Errorf("modular tag has unknown signature: %s", s)
	}
	inputCh, outputCh := int(raw[8]), int(raw[9])
	var offsets [5]uint32
	if _, err := binary.Decode(raw[12:], binary.BigEndian, offsets[:]); err != nil {
		return nil, err
	}
	b, matrix, m, clut, a := offsets[0], offsets[1], offsets[2], offsets[3], offsets[4]
	mt := &ModularTag{num_input_channels: inputCh, num_output_channels: outputCh, is_a_to_b: is_a_to_b}
	read_curves := func(offset uint32, num_curves_reqd int) (ans []Curve1D, err error) {
		if offset == 0 {
			return nil, nil
		}
		if int(offset)+8 > len(raw) {
			return nil, errors.New("modular (mAB/mBA) tag too short")
		}
		block := raw[offset:]
		var c any
		var consumed int
		for range inputCh {
			if len(block) < 4 {
				return nil, errors.New("modular (mAB/mBA) tag too short")
			}
			sig := Signature(binary.BigEndian.Uint32(block[:4]))
			switch sig {
			case CurveTypeSignature:
				c, consumed, err = embeddedCurveDecoder(block)
			case ParametricCurveTypeSignature:
				c, consumed, err = embeddedParametricCurveDecoder(block)
			default:
				return nil, fmt.Errorf("unknown curve type: %s in modularDecoder", sig)
			}
			if err != nil {
				return nil, err
			}
			block = block[consumed:]
			ans = append(ans, c.(Curve1D))
		}
		if len(ans) != num_curves_reqd {
			return nil, fmt.Errorf("number of curves in modular tag: %d does not match the number of channels: %d", len(ans), num_curves_reqd)
		}
		return
	}
	if mt.b_curves, err = read_curves(b, IfElse(is_a_to_b, outputCh, inputCh)); err != nil {
		return nil, err
	}
	if mt.a_curves, err = read_curves(a, IfElse(is_a_to_b, inputCh, outputCh)); err != nil {
		return nil, err
	}
	if mt.m_curves, err = read_curves(m, outputCh); err != nil {
		return nil, err
	}
	var temp any
	if clut > 0 {
		if temp, err = embeddedClutDecoder(raw[clut:], inputCh, outputCh); err != nil {
			return nil, err
		}
		mt.clut = temp.(ChannelTransformer)
		mt.workspace_size = max(mt.workspace_size, mt.clut.WorkspaceSize())
	}
	if matrix > 0 {
		if temp, err = embeddedMatrixDecoder(raw[clut:]); err != nil {
			return nil, err
		}
		if _, is_identity_matrix := temp.(*IdentityMatrix); !is_identity_matrix {
			mt.matrix = temp.(ChannelTransformer)
			mt.workspace_size = max(mt.workspace_size, mt.matrix.WorkspaceSize())
		}
	}
	ans = mt
	add_curves := func(c []Curve1D) {
		if len(c) > 0 {
			has_non_identity := false
			for _, x := range c {
				if _, ok := x.(*IdentityCurve); !ok {
					has_non_identity = true
					break
				}
			}
			if has_non_identity {
				nc := NewCurveTransformer(mt.a_curves...)
				mt.transforms = append(mt.transforms, nc.Transform)
				mt.transform_objects = append(mt.transform_objects, nc)
			}
		}
	}
	add_curves(mt.a_curves)
	if mt.clut != nil {
		mt.transforms = append(mt.transforms, mt.clut.Transform)
		mt.transform_objects = append(mt.transform_objects, mt.clut)
	}
	add_curves(mt.m_curves)
	if mt.matrix != nil {
		mt.transforms = append(mt.transforms, mt.matrix.Transform)
		mt.transform_objects = append(mt.transform_objects, mt.matrix)
	}
	add_curves(mt.b_curves)
	if !is_a_to_b {
		slices.Reverse(mt.transforms)
		slices.Reverse(mt.transform_objects)
	}
	return
}
