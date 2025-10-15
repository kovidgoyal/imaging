package icc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

type IdentityCurve int
type GammaCurve float64
type PointsCurve struct{ points []float64 }
type ConditionalZeroCurve struct{ g, a, b float64 }
type ConditionalCCurve struct{ g, a, b, c float64 }
type SplitCurve struct{ g, a, b, c, d float64 }
type ComplexCurve struct{ g, a, b, c, d, e, f float64 }

var _ ChannelTransformer = (*IdentityCurve)(nil)
var _ ChannelTransformer = (*GammaCurve)(nil)
var _ ChannelTransformer = (*PointsCurve)(nil)
var _ ChannelTransformer = (*ConditionalZeroCurve)(nil)
var _ ChannelTransformer = (*ConditionalCCurve)(nil)
var _ ChannelTransformer = (*SplitCurve)(nil)
var _ ChannelTransformer = (*ComplexCurve)(nil)

func (c IdentityCurve) WorkspaceSize() int                                             { return 0 }
func (c IdentityCurve) IsSuitableFor(num_input_channels, num_output_channels int) bool { return true }
func (c GammaCurve) WorkspaceSize() int                                                { return 0 }
func (c GammaCurve) IsSuitableFor(num_input_channels, num_output_channels int) bool    { return true }
func (c PointsCurve) WorkspaceSize() int                                               { return 0 }
func (c PointsCurve) IsSuitableFor(num_input_channels, num_output_channels int) bool   { return true }
func (c ConditionalZeroCurve) WorkspaceSize() int                                      { return 0 }
func (c ConditionalZeroCurve) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return true
}
func (c ConditionalCCurve) WorkspaceSize() int { return 0 }
func (c ConditionalCCurve) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return true
}
func (c SplitCurve) WorkspaceSize() int                                               { return 0 }
func (c SplitCurve) IsSuitableFor(num_input_channels, num_output_channels int) bool   { return true }
func (c ComplexCurve) WorkspaceSize() int                                             { return 0 }
func (c ComplexCurve) IsSuitableFor(num_input_channels, num_output_channels int) bool { return true }

type ParametricCurveFunction uint16

const (
	SimpleGammaFunction     ParametricCurveFunction = 0 // Y = X^g
	ConditionalZeroFunction ParametricCurveFunction = 1 // Y = (aX+b)^g for X >= d, else 0
	ConditionalCFunction    ParametricCurveFunction = 2 // Y = (aX+b)^g for X >= d, else c
	SplitFunction           ParametricCurveFunction = 3 // Two different functions split at d
	ComplexFunction         ParametricCurveFunction = 4 // More complex piecewise function
)

func curveDecoder(raw []byte) (any, error) {
	if len(raw) < 12 {
		return nil, errors.New("curv tag too short")
	}
	count := int(binary.BigEndian.Uint32(raw[8:12]))
	switch count {
	case 0:
		return IdentityCurve(0), nil
	case 1:
		if len(raw) < 14 {
			return nil, errors.New("curv tag missing gamma value")
		}
		// 8.8 fixed-point
		val := uint16(raw[12])<<8 | uint16(raw[13])
		return GammaCurve(float64(val) / 256), nil
	default:
		points := make([]uint16, count)
		_, err := binary.Decode(raw[12:], binary.BigEndian, points)
		if err != nil {
			return nil, errors.New("curv tag truncated")
		}
		fp := make([]float64, len(points))
		for i, p := range points {
			fp[i] = float64(p) / 65535
		}
		return &PointsCurve{fp}, nil
	}
}

func readS15Fixed16BE(raw []byte) float64 {
	msb := int16(raw[0])<<8 | int16(raw[1])
	lsb := uint16(raw[2])<<8 | uint16(raw[3])
	return float64(msb) + float64(lsb)/65536
}

func parametricCurveDecoder(raw []byte) (any, error) {
	if len(raw) < 14 {
		return nil, errors.New("para tag too short")
	}
	funcType := ParametricCurveFunction(binary.BigEndian.Uint16(raw[8:10]))
	raw = raw[10:]
	params := make([]float64, 0, len(raw)/4)
	for i := 0; i < len(raw); i += 4 {
		params = append(params, readS15Fixed16BE(raw[i:i+4]))
	}

	switch funcType {
	case SimpleGammaFunction:
		return GammaCurve(params[0]), nil
	case ConditionalZeroFunction:
		if len(params) < 3 {
			return nil, errors.New("para tag too short")
		}
		if params[1] == 0 {
			return nil, fmt.Errorf("conditional zero curve has a=0")
		}
		return &ConditionalZeroCurve{params[0], params[1], params[2]}, nil
	case ConditionalCFunction:
		if len(params) < 4 {
			return nil, errors.New("para tag too short")
		}
		if params[1] == 0 {
			return nil, fmt.Errorf("conditional C curve has a=0")
		}
		return &ConditionalCCurve{params[0], params[1], params[2], params[3]}, nil
	case SplitFunction:
		if len(params) < 5 {
			return nil, errors.New("para tag too short")
		}
		return &SplitCurve{params[0], params[1], params[2], params[3], params[4]}, nil
	case ComplexFunction:
		if len(params) < 7 {
			return nil, errors.New("para tag too short")
		}
		return &ComplexCurve{params[0], params[1], params[2], params[3], params[4], params[5], params[6]}, nil
	default:
		return nil, fmt.Errorf("unknown parametric function type: %d", funcType)
	}
}

func (c IdentityCurve) Transform(output, workspace []float64, inputs ...float64) error {
	copy(output, inputs)
	return nil
}

func (c GammaCurve) Transform(output, workspace []float64, inputs ...float64) error {
	for i, v := range inputs {
		output[i] = math.Pow(v, float64(c))
	}
	return nil
}

func (c PointsCurve) Transform(output, workspace []float64, inputs ...float64) error {
	for i, v := range inputs {
		idx := v * float64(len(c.points)-1)
		lo := int(math.Floor(idx))
		hi := int(math.Ceil(idx))
		if lo == hi {
			output[i] = c.points[lo]
		} else {
			p := idx - float64(lo)
			vlo := float64(c.points[lo])
			vhi := float64(c.points[hi])
			output[i] = vlo + p*(vhi-vlo)
		}
	}
	return nil
}

func (c ConditionalZeroCurve) Transform(output, workspace []float64, inputs ...float64) error {
	// Y = (aX+b)^g if X ≥ -b/a else 0
	for i, x := range inputs {
		if x >= -c.b/c.a {
			output[i] = math.Pow(c.a*x+c.b, c.g)
		} else {
			output[i] = 0
		}
	}
	return nil
}

func (c ConditionalCCurve) Transform(output, workspace []float64, inputs ...float64) error {
	// Y = (aX+b)^g + c if X ≥ -b/a else c
	for i, x := range inputs {
		if x >= -c.b/c.a {
			output[i] = math.Pow(c.a*x+c.b, c.g) + c.c
		} else {
			output[i] = c.c
		}
	}
	return nil
}

func (c SplitCurve) Transform(output, workspace []float64, inputs ...float64) error {
	for i, x := range inputs {
		// Y = (aX+b)^g if X ≥ d else cX
		if x >= c.d {
			output[i] = math.Pow(c.a*x+c.b, c.g)
		} else {
			output[i] = c.c * x
		}

	}
	return nil
}

func (c ComplexCurve) Transform(output, workspace []float64, inputs ...float64) error {
	// Y = (aX+b)^g + e if X ≥ d else cX+f
	for i, x := range inputs {
		if x >= c.d {
			output[i] = math.Pow(c.a*x+c.b, c.g) + c.e
		} else {
			output[i] = c.c*x + c.f
		}
	}
	return nil
}
