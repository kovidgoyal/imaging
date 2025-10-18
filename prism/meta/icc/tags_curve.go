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
type Curve1D interface{ Transform(x float64) float64 }

var _ Curve1D = (*IdentityCurve)(nil)
var _ Curve1D = (*GammaCurve)(nil)
var _ Curve1D = (*PointsCurve)(nil)
var _ Curve1D = (*ConditionalZeroCurve)(nil)
var _ Curve1D = (*ConditionalCCurve)(nil)
var _ Curve1D = (*SplitCurve)(nil)
var _ Curve1D = (*ComplexCurve)(nil)

type CurveTransformer struct {
	curves []Curve1D
}

func (c CurveTransformer) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return len(c.curves) == num_input_channels && len(c.curves) == num_output_channels
}
func (c CurveTransformer) WorkspaceSize() int { return 0 }
func (c CurveTransformer) Transform(output, workspace []float64, inputs ...float64) error {
	for i, x := range inputs {
		output[i] = c.curves[i].Transform(x)
	}
	return nil
}

type CurveTransformer3 struct {
	r, g, b Curve1D
}

func (c CurveTransformer3) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return 3 == num_input_channels && 3 == num_output_channels
}
func (c CurveTransformer3) WorkspaceSize() int { return 0 }
func (c CurveTransformer3) Transform(output, workspace []float64, inputs ...float64) error {
	output[0] = c.r.Transform(inputs[0])
	output[1] = c.g.Transform(inputs[1])
	output[2] = c.b.Transform(inputs[2])
	return nil
}

func NewCurveTransformer(curves []Curve1D) ChannelTransformer {
	switch len(curves) {
	case 3:
		return &CurveTransformer3{curves[0], curves[1], curves[2]}
	default:
		return &CurveTransformer{curves}
	}
}

type ParametricCurveFunction uint16

const (
	SimpleGammaFunction     ParametricCurveFunction = 0 // Y = X^g
	ConditionalZeroFunction ParametricCurveFunction = 1 // Y = (aX+b)^g for X >= d, else 0
	ConditionalCFunction    ParametricCurveFunction = 2 // Y = (aX+b)^g for X >= d, else c
	SplitFunction           ParametricCurveFunction = 3 // Two different functions split at d
	ComplexFunction         ParametricCurveFunction = 4 // More complex piecewise function
)

func align_to_4(x int) int {
	if extra := x % 4; extra > 0 {
		x += 4 - extra
	}
	return x
}

func fixed88ToFloat(raw []byte) float64 {
	return float64(uint16(raw[0])<<8|uint16(raw[1])) / 256
}

func embeddedCurveDecoder(raw []byte) (any, int, error) {
	if len(raw) < 12 {
		return nil, 0, errors.New("curv tag too short")
	}
	count := int(binary.BigEndian.Uint32(raw[8:12]))
	consumed := align_to_4(12 + count*2)
	switch count {
	case 0:
		return IdentityCurve(0), consumed, nil
	case 1:
		if len(raw) < 14 {
			return nil, 0, errors.New("curv tag missing gamma value")
		}
		// 8.8 fixed-point
		val := uint16(raw[12])<<8 | uint16(raw[13])
		return GammaCurve(float64(val) / 256), consumed, nil
	default:
		points := make([]uint16, count)
		_, err := binary.Decode(raw[12:], binary.BigEndian, points)
		if err != nil {
			return nil, 0, errors.New("curv tag truncated")
		}
		fp := make([]float64, len(points))
		for i, p := range points {
			fp[i] = float64(p) / 65535
		}
		return &PointsCurve{fp}, consumed, nil
	}
}

func curveDecoder(raw []byte) (any, error) {
	ans, _, err := embeddedCurveDecoder(raw)
	return ans, err
}

func readS15Fixed16BE(raw []byte) float64 {
	msb := int16(raw[0])<<8 | int16(raw[1])
	lsb := uint16(raw[2])<<8 | uint16(raw[3])
	return float64(msb) + float64(lsb)/65536
}

func embeddedParametricCurveDecoder(raw []byte) (ans any, consumed int, err error) {
	block_len := len(raw)
	if block_len < 16 {
		return nil, 0, errors.New("para tag too short")
	}
	funcType := ParametricCurveFunction(binary.BigEndian.Uint16(raw[8:10]))
	const header_len = 12
	raw = raw[header_len:]
	p := func() float64 {
		ans := readS15Fixed16BE(raw[:4])
		raw = raw[4:]
		return ans
	}
	defer func() { consumed = align_to_4(consumed) }()

	switch funcType {
	case SimpleGammaFunction:
		if consumed = header_len + 4; block_len < consumed {
			return nil, 0, errors.New("para tag too short")
		}
		return GammaCurve(p()), 14, nil
	case ConditionalZeroFunction:
		if consumed = header_len + 3*4; block_len < consumed {
			return nil, 0, errors.New("para tag too short")
		}
		c := &ConditionalZeroCurve{p(), p(), p()}
		if c.a == 0 {
			return nil, 0, fmt.Errorf("conditional zero curve has a=0")
		}
		return c, consumed, nil
	case ConditionalCFunction:
		if consumed = header_len + 4*4; block_len < consumed {
			return nil, 0, errors.New("para tag too short")
		}
		c := &ConditionalCCurve{p(), p(), p(), p()}
		if c.a == 0 {
			return nil, 0, fmt.Errorf("conditional C curve has a=0")
		}
		return c, consumed, nil
	case SplitFunction:
		if consumed = header_len + 5*4; block_len < consumed {
			return nil, 0, errors.New("para tag too short")
		}
		return &SplitCurve{p(), p(), p(), p(), p()}, consumed, nil
	case ComplexFunction:
		if consumed = header_len + 7*4; block_len < consumed {
			return nil, 0, errors.New("para tag too short")
		}
		return &ComplexCurve{p(), p(), p(), p(), p(), p(), p()}, consumed, nil
	default:
		return nil, 0, fmt.Errorf("unknown parametric function type: %d", funcType)
	}
}

func parametricCurveDecoder(raw []byte) (any, error) {
	ans, _, err := embeddedParametricCurveDecoder(raw)
	return ans, err
}

func (c IdentityCurve) Transform(x float64) float64 {
	return x
}

func (c GammaCurve) Transform(x float64) float64 {
	return math.Pow(x, float64(c))
}

func (c PointsCurve) Transform(v float64) float64 {
	idx := v * float64(len(c.points)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return c.points[lo]
	}
	p := idx - float64(lo)
	vlo := float64(c.points[lo])
	vhi := float64(c.points[hi])
	return vlo + p*(vhi-vlo)
}

func (c ConditionalZeroCurve) Transform(x float64) float64 {
	// Y = (aX+b)^g if X ≥ -b/a else 0
	if x >= -c.b/c.a {
		return math.Pow(c.a*x+c.b, c.g)
	}
	return 0
}

func (c ConditionalCCurve) Transform(x float64) float64 {
	// Y = (aX+b)^g + c if X ≥ -b/a else c
	if x >= -c.b/c.a {
		return math.Pow(c.a*x+c.b, c.g) + c.c
	}
	return c.c
}

func (c SplitCurve) Transform(x float64) float64 {
	// Y = (aX+b)^g if X ≥ d else cX
	if x >= c.d {
		return math.Pow(c.a*x+c.b, c.g)
	}
	return c.c * x
}

func (c ComplexCurve) Transform(x float64) float64 {
	// Y = (aX+b)^g + e if X ≥ d else cX+f
	if x >= c.d {
		return math.Pow(c.a*x+c.b, c.g) + c.e
	}
	return c.c*x + c.f
}
