package icc

import (
	"fmt"
	"strings"
	"sync"
)

type not_found struct {
	sig Signature
}

func (e *not_found) Error() string {
	return fmt.Sprintf("no tag for signature: %s found in this ICC profile", e.sig)
}

type unsupported struct {
	sig Signature
}

func (e *unsupported) Error() string {
	return fmt.Sprintf("the tag: %s (0x%x) is not supported", e.sig, uint32(e.sig))
}

type XYZType struct{ X, Y, Z unit_float }

func xyz_type(data []byte) XYZType {
	return XYZType{readS15Fixed16BE(data[:4]), readS15Fixed16BE(data[4:8]), readS15Fixed16BE(data[8:12])}
}

func decode_xyz(data []byte) (ans any, err error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("xyz tag too short")
	}
	a := xyz_type(data[8:])
	return &a, nil
}

func decode_array(data []byte) (ans any, err error) {
	data = data[8:]
	a := make([]unit_float, len(data)/4)
	for i := range a {
		a[i] = readS15Fixed16BE(data[:4:4])
		data = data[4:]
	}
	return a, nil
}

func parse_tag(sig Signature, data []byte, input_colorspace, output_colorspace ColorSpace) (result any, err error) {
	if len(data) == 0 {
		return nil, &not_found{sig}
	}
	if len(data) < 4 {
		return nil, &unsupported{sig}
	}
	s := signature(data)
	switch s {
	default:
		return nil, &unsupported{s}
	case DescSignature, DeviceManufacturerDescriptionSignature, DeviceModelDescriptionSignature, MultiLocalisedUnicodeSignature, TextTagSignature:
		return parse_text_tag(data)
	case SignateTagSignature:
		return sigDecoder(data)
	case MatrixElemTypeSignature:
		return matrixDecoder(data)
	case LutAtoBTypeSignature, LutBtoATypeSignature:
		return modularDecoder(data, input_colorspace, output_colorspace)
	case Lut16TypeSignature:
		return decode_mft16(data, input_colorspace, output_colorspace)
	case Lut8TypeSignature:
		return decode_mft8(data, input_colorspace, output_colorspace)
	case XYZTypeSignature:
		return decode_xyz(data)
	case S15Fixed16ArrayTypeSignature:
		return decode_array(data)
	case CurveTypeSignature, ParametricCurveTypeSignature:
		return curveDecoder(data)
	}
}

type parsed_tag struct {
	tag any
	err error
}

type raw_tag_entry struct {
	offset int
	data   []byte
}

type parse_cache_key struct {
	offset, size int
}

type TagTable struct {
	entries     map[Signature]raw_tag_entry
	lock        sync.Mutex
	parsed      map[Signature]parsed_tag
	parse_cache map[parse_cache_key]parsed_tag
}

func (t *TagTable) Has(sig Signature) bool {
	return t.entries[sig].data != nil
}

func (t *TagTable) add(sig Signature, offset int, data []byte) {
	t.entries[sig] = raw_tag_entry{offset, data}
}

func (t *TagTable) get_parsed(sig Signature, input_colorspace, output_colorspace ColorSpace) (ans any, err error) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if t.parsed == nil {
		t.parsed = make(map[Signature]parsed_tag)
		t.parse_cache = make(map[parse_cache_key]parsed_tag)
	}
	existing, found := t.parsed[sig]
	if found {
		return existing.tag, existing.err
	}
	var key parse_cache_key
	defer func() {
		t.parsed[sig] = parsed_tag{ans, err}
		t.parse_cache[key] = parsed_tag{ans, err}
	}()
	re := t.entries[sig]
	if re.data == nil {
		return nil, &not_found{sig}
	}
	key = parse_cache_key{re.offset, len(re.data)}
	if cached, ok := t.parse_cache[key]; ok {
		return cached.tag, cached.err
	}
	return parse_tag(sig, re.data, input_colorspace, output_colorspace)
}

func (t *TagTable) getDescription(s Signature) (string, error) {
	q, err := t.get_parsed(s, ColorSpaceRGB, ColorSpaceXYZ)
	if err != nil {
		return "", fmt.Errorf("could not get description for %s with error: %w", s, err)
	}
	if t, ok := q.(TextTag); ok {
		return t.BestGuessValue(), nil
	} else {
		return "", fmt.Errorf("tag for %s is not a text tag", s)
	}
}

func (t *TagTable) getProfileDescription() (string, error) {
	return t.getDescription(DescSignature)
}

func (t *TagTable) getDeviceManufacturerDescription() (string, error) {
	return t.getDescription(DeviceManufacturerDescriptionSignature)
}

func (t *TagTable) getDeviceModelDescription() (string, error) {
	return t.getDescription(DeviceModelDescriptionSignature)
}

func (t *TagTable) load_curve_tag(s Signature) (Curve1D, error) {
	r, err := t.get_parsed(s, ColorSpaceRGB, ColorSpaceXYZ)
	if err != nil {
		return nil, fmt.Errorf("could not load %s tag from profile with error: %w", s, err)
	}
	if ans, ok := r.(Curve1D); !ok {
		return nil, fmt.Errorf("could not load %s tag from profile as it is of unsupported type: %T", s, r)
	} else {
		return ans, nil
	}
}

func (t *TagTable) load_rgb_matrix() (*Matrix3, error) {
	r, err := t.get_parsed(RedMatrixColumnTagSignature, ColorSpaceRGB, ColorSpaceXYZ)
	if err != nil {
		return nil, err
	}
	g, err := t.get_parsed(GreenMatrixColumnTagSignature, ColorSpaceRGB, ColorSpaceXYZ)
	if err != nil {
		return nil, err
	}
	b, err := t.get_parsed(BlueMatrixColumnTagSignature, ColorSpaceRGB, ColorSpaceXYZ)
	if err != nil {
		return nil, err
	}
	rc, bc, gc := r.(*XYZType), g.(*XYZType), b.(*XYZType)
	var m Matrix3
	m[0][0], m[0][1], m[0][2] = rc.X, bc.X, gc.X
	m[1][0], m[1][1], m[1][2] = rc.Y, bc.Y, gc.Y
	m[2][0], m[2][1], m[2][2] = rc.Z, bc.Z, gc.Z
	return &m, nil
}

func array_to_matrix(a []unit_float) *Matrix3 {
	_ = a[8]
	m := Matrix3{}
	copy(m[0][:], a[:3])
	copy(m[1][:], a[3:6])
	copy(m[2][:], a[6:9])
	if is_identity_matrix(&m) {
		return nil
	}
	return &m
}

func (p *TagTable) get_chromatic_adaption() (*Matrix3, error) {
	x, err := p.get_parsed(ChromaticAdaptationTagSignature, ColorSpaceRGB, ColorSpaceXYZ)
	if err != nil {
		return nil, err
	}
	a, ok := x.([]unit_float)
	if !ok {
		return nil, fmt.Errorf("chad tag is not an ArrayType")
	}
	return array_to_matrix(a), nil
}

func emptyTagTable() TagTable {
	return TagTable{
		entries: make(map[Signature]raw_tag_entry),
	}
}

type Debug_callback = func(r, g, b, x, y, z unit_float, t ChannelTransformer)

func transform_debug(m ChannelTransformer, r, g, b unit_float, f Debug_callback) (unit_float, unit_float, unit_float) {
	x, y, z := m.Transform(r, g, b)
	f(r, g, b, x, y, z, m)
	return x, y, z
}

type ChannelTransformer interface {
	Transform(r, g, b unit_float) (unit_float, unit_float, unit_float)
	TransformDebug(r, g, b unit_float, callback Debug_callback) (unit_float, unit_float, unit_float)
	IsSuitableFor(num_input_channels int, num_output_channels int) bool
	String() string
}

type TwoTransformers struct {
	a, b         func(r, g, b unit_float) (unit_float, unit_float, unit_float)
	transformers []ChannelTransformer
}

func (t *TwoTransformers) IsSuitableFor(i, o int) bool {
	for _, x := range t.transformers {
		if !x.IsSuitableFor(i, o) {
			return false
		}
	}
	return true
}

func (t *TwoTransformers) Transform(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	r, g, b = t.a(r, g, b)
	r, g, b = t.b(r, g, b)
	return r, g, b
}

func (t *TwoTransformers) TransformDebug(r, g, b unit_float, f Debug_callback) (unit_float, unit_float, unit_float) {
	for _, x := range t.transformers {
		r, g, b = x.TransformDebug(r, g, b, f)
	}
	return r, g, b
}

func (t TwoTransformers) String() string {
	return fmt.Sprintf("TwoTransformers{ %v %v }", t.transformers[0], t.transformers[1])
}

type ThreeTransformers struct {
	a, b, c      func(r, g, b unit_float) (unit_float, unit_float, unit_float)
	transformers []ChannelTransformer
}

func (t *ThreeTransformers) IsSuitableFor(i, o int) bool {
	for _, x := range t.transformers {
		if !x.IsSuitableFor(i, o) {
			return false
		}
	}
	return true
}

func (t *ThreeTransformers) Transform(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	r, g, b = t.a(r, g, b)
	r, g, b = t.b(r, g, b)
	r, g, b = t.c(r, g, b)
	return r, g, b
}

func (t *ThreeTransformers) TransformDebug(r, g, b unit_float, f Debug_callback) (unit_float, unit_float, unit_float) {
	for _, x := range t.transformers {
		r, g, b = x.TransformDebug(r, g, b, f)
	}
	return r, g, b
}

func (t ThreeTransformers) String() string {
	return fmt.Sprintf("ThreeTransformers{ %v %v %v }", t.transformers[0], t.transformers[1], t.transformers[2])
}

type MultipleTransformers struct {
	transformers []ChannelTransformer
}

func (t *MultipleTransformers) IsSuitableFor(i, o int) bool {
	for _, x := range t.transformers {
		if !x.IsSuitableFor(i, o) {
			return false
		}
	}
	return true
}

func (t *MultipleTransformers) Transform(r, g, b unit_float) (unit_float, unit_float, unit_float) {
	for _, x := range t.transformers {
		r, g, b = x.Transform(r, g, b)
	}
	return r, g, b
}

func (t *MultipleTransformers) TransformDebug(r, g, b unit_float, f Debug_callback) (unit_float, unit_float, unit_float) {
	for _, x := range t.transformers {
		r, g, b = x.TransformDebug(r, g, b, f)
	}
	return r, g, b
}

func transformers_as_string(t ...ChannelTransformer) string {
	b := strings.Builder{}
	for _, x := range t {
		b.WriteString(fmt.Sprintf("%v ", x))
	}
	return b.String()
}

func (t MultipleTransformers) String() string {
	b := strings.Builder{}
	for _, x := range t.transformers {
		b.WriteString(fmt.Sprintf("%v ", x))
	}
	return fmt.Sprintf("MultipleTransformers{ %s }", transformers_as_string(t.transformers...))
}

func NewCombinedTransformer(t ...ChannelTransformer) ChannelTransformer {
	switch len(t) {
	case 0:
		m := IdentityMatrix(0)
		return &m
	case 1:
		return t[0]
	case 2:
		return &TwoTransformers{t[0].Transform, t[1].Transform, t}
	case 3:
		return &ThreeTransformers{t[0].Transform, t[1].Transform, t[2].Transform, t}
	default:
		return &MultipleTransformers{t}
	}
}
