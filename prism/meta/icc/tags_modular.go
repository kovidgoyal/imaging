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
	transforms                              []ChannelTransformer
	workspace_size                          int
	is_a_to_b                               bool
}

var _ ChannelTransformer = (*ModularTag)(nil)

func (m *ModularTag) WorkspaceSize() int { return m.workspace_size }
func (m *ModularTag) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return m.num_input_channels == num_input_channels && m.num_output_channels == num_output_channels
}
func (m *ModularTag) Transform(output, workspace []float64, inputs ...float64) (err error) {
	for _, t := range m.transforms {
		if err = t.Transform(output, workspace, inputs...); err != nil {
			return err
		}
	}
	return nil
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
		if !is_identity_matrix(temp.(*MatrixTag).Matrix) {
			mt.matrix = temp.(ChannelTransformer)
			mt.workspace_size = max(mt.workspace_size, mt.matrix.WorkspaceSize())
		}
	}
	ans = mt
	if mt.a_curves != nil {
		mt.transforms = append(mt.transforms, NewCurveTransformer(mt.a_curves))
	}
	if mt.clut != nil {
		mt.transforms = append(mt.transforms, mt.clut)
	}
	if mt.m_curves != nil {
		mt.transforms = append(mt.transforms, NewCurveTransformer(mt.m_curves))
	}
	if mt.matrix != nil {
		mt.transforms = append(mt.transforms, mt.matrix)
	}
	if mt.b_curves != nil {
		mt.transforms = append(mt.transforms, NewCurveTransformer(mt.b_curves))
	}
	if !is_a_to_b {
		slices.Reverse(mt.transforms)
	}
	return
}
