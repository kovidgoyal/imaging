package icc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Determinant lower than that are assumed zero (used on matrix invert)
const MATRIX_DET_TOLERANCE = 0.0001

type IdentityCurve int
type GammaCurve struct {
	gamma, inv_gamma float64
	is_one           bool
}
type PointsCurve struct {
	points, reverse_lookup []float64
	max_idx                float64
}
type ConditionalZeroCurve struct{ g, a, b, threshold, inv_gamma, inv_a float64 }
type ConditionalCCurve struct{ g, a, b, c, threshold, inv_gamma, inv_a float64 }
type SplitCurve struct{ g, a, b, c, d, inv_g, inv_a, inv_c, threshold float64 }
type ComplexCurve struct{ g, a, b, c, d, e, f, inv_g, inv_a, inv_c, threshold float64 }
type Curve1D interface {
	Transform(x float64) float64
	InverseTransform(x float64) float64
	Prepare() error
	String() string
}

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
type InverseCurveTransformer struct{ curves []Curve1D }

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

func (c InverseCurveTransformer) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return len(c.curves) == num_input_channels && len(c.curves) == num_output_channels
}
func (c InverseCurveTransformer) WorkspaceSize() int { return 0 }
func (c InverseCurveTransformer) Transform(output, workspace []float64, inputs ...float64) error {
	for i, x := range inputs {
		output[i] = c.curves[i].InverseTransform(x)
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

type InverseCurveTransformer3 struct{ r, g, b Curve1D }

func (c CurveTransformer3) String() string        { return c.r.String() }
func (c CurveTransformer) String() string         { return c.curves[0].String() }
func (c InverseCurveTransformer3) String() string { return c.r.String() }
func (c InverseCurveTransformer) String() string  { return c.curves[0].String() }

func (c InverseCurveTransformer3) IsSuitableFor(num_input_channels, num_output_channels int) bool {
	return 3 == num_input_channels && 3 == num_output_channels
}
func (c InverseCurveTransformer3) WorkspaceSize() int { return 0 }
func (c InverseCurveTransformer3) Transform(output, workspace []float64, inputs ...float64) error {
	output[0] = c.r.InverseTransform(inputs[0])
	output[1] = c.g.InverseTransform(inputs[1])
	output[2] = c.b.InverseTransform(inputs[2])
	return nil
}

func NewCurveTransformer(curves ...Curve1D) ChannelTransformer {
	switch len(curves) {
	case 3:
		return &CurveTransformer3{curves[0], curves[1], curves[2]}
	default:
		return &CurveTransformer{curves}
	}
}
func NewInverseCurveTransformer(curves ...Curve1D) ChannelTransformer {
	switch len(curves) {
	case 3:
		return &InverseCurveTransformer3{curves[0], curves[1], curves[2]}
	default:
		return &InverseCurveTransformer{curves}
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
		c := IdentityCurve(0)
		return &c, consumed, nil
	case 1:
		if len(raw) < 14 {
			return nil, 0, errors.New("curv tag missing gamma value")
		}
		// 8.8 fixed-point
		val := uint16(raw[12])<<8 | uint16(raw[13])
		c := &GammaCurve{gamma: float64(val) / 256}
		if err := c.Prepare(); err != nil {
			return nil, 0, err
		}
		return c, consumed, nil
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
		c := &PointsCurve{points: fp}
		if err := c.Prepare(); err != nil {
			return nil, 0, err
		}
		return c, consumed, nil
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
	var c Curve1D

	switch funcType {
	case SimpleGammaFunction:
		if consumed = header_len + 4; block_len < consumed {
			return nil, 0, errors.New("para tag too short")
		}
		c = &GammaCurve{gamma: p()}
	case ConditionalZeroFunction:
		if consumed = header_len + 3*4; block_len < consumed {
			return nil, 0, errors.New("para tag too short")
		}
		c = &ConditionalZeroCurve{g: p(), a: p(), b: p()}
	case ConditionalCFunction:
		if consumed = header_len + 4*4; block_len < consumed {
			return nil, 0, errors.New("para tag too short")
		}
		c = &ConditionalCCurve{g: p(), a: p(), b: p(), c: p()}
	case SplitFunction:
		if consumed = header_len + 5*4; block_len < consumed {
			return nil, 0, errors.New("para tag too short")
		}
		c = &SplitCurve{g: p(), a: p(), b: p(), c: p(), d: p()}
	case ComplexFunction:
		if consumed = header_len + 7*4; block_len < consumed {
			return nil, 0, errors.New("para tag too short")
		}
		c = &ComplexCurve{g: p(), a: p(), b: p(), c: p(), d: p(), e: p(), f: p()}
	default:
		return nil, 0, fmt.Errorf("unknown parametric function type: %d", funcType)
	}
	if err = c.Prepare(); err != nil {
		return nil, 0, err
	}
	return c, consumed, nil

}

func parametricCurveDecoder(raw []byte) (any, error) {
	ans, _, err := embeddedParametricCurveDecoder(raw)
	return ans, err
}

func (c IdentityCurve) Transform(x float64) float64 {
	return x
}

func (c IdentityCurve) InverseTransform(x float64) float64 {
	return x
}

func (c IdentityCurve) Prepare() error { return nil }
func (c IdentityCurve) String() string { return "IdentityCurve{}" }

func (c GammaCurve) Transform(x float64) float64 {
	if x < 0 {
		if c.is_one {
			return x
		}
		return 0
	}
	return math.Pow(x, c.gamma)
}

func (c GammaCurve) InverseTransform(x float64) float64 {
	if x < 0 {
		if c.is_one {
			return x
		}
		return 0
	}
	return math.Pow(x, c.inv_gamma)
}

func (c *GammaCurve) Prepare() error {
	if c.gamma == 0 {
		return fmt.Errorf("gamma curve has zero gamma value")
	}
	c.inv_gamma = 1 / c.gamma
	c.is_one = math.Abs(c.gamma-1) < MATRIX_DET_TOLERANCE
	return nil
}
func (c GammaCurve) String() string { return fmt.Sprintf("GammaCurve{%f}", c.gamma) }

func sampled_value(samples []float64, max_idx float64, x float64) float64 {
	idx := x * max_idx
	lof := math.Trunc(idx)
	lo := int(lof)
	if lof == idx {
		return samples[lo]
	}
	p := idx - float64(lo)
	vhi := float64(samples[lo+1])
	vlo := float64(samples[lo])
	return vlo + p*(vhi-vlo)
}

func (c *PointsCurve) Prepare() error {
	c.max_idx = float64(len(c.points) - 1)
	reverse_lookup := make([]float64, len(c.points))
	for i := range len(reverse_lookup) {
		y := float64(i) / float64(len(reverse_lookup)-1)
		idx := get_interval(c.points, y)
		if idx < 0 {
			reverse_lookup[i] = 0
		} else {
			y1, y2 := c.points[idx], c.points[idx+1]
			if y2 < y1 {
				y1, y2 = y2, y1
			}
			x1, x2 := float64(idx)/c.max_idx, float64(idx+1)/c.max_idx
			frac := (y - y1) / (y2 - y1)
			reverse_lookup[i] = x1 + frac*(x2-x1)
		}
	}
	c.reverse_lookup = reverse_lookup
	return nil
}

func (c PointsCurve) Transform(v float64) float64 {
	return sampled_value(c.points, c.max_idx, v)
}

func (c PointsCurve) InverseTransform(v float64) float64 {
	return sampled_value(c.reverse_lookup, c.max_idx, v)
}
func (c PointsCurve) String() string { return fmt.Sprintf("PointsCurve{%d}", len(c.points)) }

func get_interval(lookup []float64, y float64) int {
	if len(lookup) < 2 {
		return -1
	}
	for i := range len(lookup) - 1 {
		y0, y1 := lookup[i], lookup[i+1]
		if y1 < y0 {
			y0, y1 = y1, y0
		}
		if y0 <= y && y <= y1 {
			return i
		}
	}
	return -1
}

func (c *ConditionalZeroCurve) Prepare() error {
	if c.a == 0 || c.g == 0 {
		return fmt.Errorf("conditional zero curve as zero parameter value: a=%f or g=%f", c.a, c.g)
	}
	c.threshold, c.inv_gamma, c.inv_a = -c.b/c.a, 1/c.g, 1/c.a
	return nil
}

func (c *ConditionalZeroCurve) String() string {
	return fmt.Sprintf("ConditionalZeroCurve{a: %v b: %v g: %v}", c.a, c.b, c.g)
}

func (c *ConditionalZeroCurve) Transform(x float64) float64 {
	// Y = (aX+b)^g if X ≥ -b/a else 0
	if x >= c.threshold {
		if e := c.a*x + c.b; e > 0 {
			return math.Pow(e, c.g)
		}
	}
	return 0
}

func (c *ConditionalZeroCurve) InverseTransform(y float64) float64 {
	// X = (Y^(1/g) - b) / a if Y >= 0 else X = -b/a
	// the below doesnt match the actual spec but matches lcms2 implementation
	return max(0, (math.Pow(y, c.inv_gamma)-c.b)*c.inv_a)
}

func (c *ConditionalCCurve) Prepare() error {
	if c.a == 0 || c.g == 0 {
		return fmt.Errorf("conditional C curve as zero parameter value: a=%f or g=%f", c.a, c.g)
	}
	c.threshold, c.inv_gamma, c.inv_a = -c.b/c.a, 1/c.g, 1/c.a
	return nil
}

func (c *ConditionalCCurve) String() string {
	return fmt.Sprintf("ConditionalCCurve{a: %v b: %v c: %v g: %v}", c.a, c.b, c.c, c.g)
}

func (c *ConditionalCCurve) Transform(x float64) float64 {
	// Y = (aX+b)^g + c if X ≥ -b/a else c
	if x >= c.threshold {
		if e := c.a*x + c.b; e > 0 {
			return math.Pow(e, c.g) + c.c
		}
		return 0
	}
	return c.c
}

func (c *ConditionalCCurve) InverseTransform(y float64) float64 {
	// X = ((Y-c)^(1/g) - b) / a if Y >= c else X = -b/a
	if e := y - c.c; e >= 0 {
		if e == 0 {
			return 0
		}
		return (math.Pow(e, c.inv_gamma) - c.b) * c.inv_a
	}
	return c.threshold
}

func (c *SplitCurve) Prepare() error {
	if c.a == 0 || c.g == 0 || c.c == 0 {
		return fmt.Errorf("conditional C curve as zero parameter value: a=%f or g=%f or c=%f", c.a, c.g, c.c)
	}
	c.threshold, c.inv_g, c.inv_a, c.inv_c = math.Pow(c.a*c.d+c.b, c.g), 1/c.g, 1/c.a, 1/c.c
	return nil
}

func (c *SplitCurve) String() string {
	return fmt.Sprintf("SplitCurve{a: %v b: %v c: %v d: %v g: %v}", c.a, c.b, c.c, c.d, c.g)
}

func (c *SplitCurve) Transform(x float64) float64 {
	// Y = (aX+b)^g if X ≥ d else cX
	if x >= c.d {
		if e := c.a*x + c.b; e > 0 {
			return math.Pow(e, c.g)
		}
		return 0
	}
	return c.c * x
}

func (c *SplitCurve) InverseTransform(y float64) float64 {
	// X=((Y^1/g-b)/a)    | Y >= (ad+b)^g
	// X=Y/c              | Y< (ad+b)^g
	if y < c.threshold {
		return y * c.inv_c
	}
	return (math.Pow(y, c.inv_g) - c.b) * c.inv_a
}

func (c *ComplexCurve) Prepare() error {
	if c.a == 0 || c.g == 0 || c.c == 0 {
		return fmt.Errorf("conditional C curve as zero parameter value: a=%f or g=%f or c=%f", c.a, c.g, c.c)
	}
	c.threshold, c.inv_g, c.inv_a, c.inv_c = math.Pow(c.a*c.d+c.b, c.g)+c.e, 1/c.g, 1/c.a, 1/c.c
	return nil
}

func (c *ComplexCurve) String() string {
	return fmt.Sprintf("ComplexCurve{a: %v b: %v c: %v d: %v e: %v f: %v g: %v}", c.a, c.b, c.c, c.d, c.e, c.f, c.g)
}

func (c *ComplexCurve) Transform(x float64) float64 {
	// Y = (aX+b)^g + e if X ≥ d else cX+f
	if x >= c.d {
		if e := c.a*x + c.b; e > 0 {
			return math.Pow(e, c.g) + c.e
		}
		return c.e
	}
	return c.c*x + c.f
}

func (c *ComplexCurve) InverseTransform(y float64) float64 {
	// X=((Y-e)1/g-b)/a   | Y >=(ad+b)^g+e), cd+f
	// X=(Y-f)/c          | else
	if y < c.threshold {
		return (y - c.f) * c.inv_c
	}
	if e := y - c.e; e > 0 {
		return (math.Pow(y-c.e, c.inv_g) - c.b) * c.inv_a
	}
	return 0
}
