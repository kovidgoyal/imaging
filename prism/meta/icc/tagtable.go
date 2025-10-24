package icc

import (
	"fmt"
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

type XYZType struct{ x, y, z unit_float }

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

func parse_tag(sig Signature, data []byte) (result any, err error) {
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
		return modularDecoder(data)
	case Lut16TypeSignature:
		return decode_mft16(data)
	case Lut8TypeSignature:
		return decode_mft8(data)
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

func (t *TagTable) get_parsed(sig Signature) (ans any, err error) {
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
	return parse_tag(sig, re.data)
}

func (t *TagTable) getDescription(s Signature) (string, error) {
	q, err := t.get_parsed(s)
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
	r, err := t.get_parsed(s)
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
	r, err := t.get_parsed(RedMatrixColumnTagSignature)
	if err != nil {
		return nil, err
	}
	g, err := t.get_parsed(GreenMatrixColumnTagSignature)
	if err != nil {
		return nil, err
	}
	b, err := t.get_parsed(BlueMatrixColumnTagSignature)
	if err != nil {
		return nil, err
	}
	rc, bc, gc := r.(*XYZType), g.(*XYZType), b.(*XYZType)
	var m Matrix3
	m[0][0], m[0][1], m[0][2] = rc.x, bc.x, gc.x
	m[1][0], m[1][1], m[1][2] = rc.y, bc.y, gc.y
	m[2][0], m[2][1], m[2][2] = rc.z, bc.z, gc.z
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
	x, err := p.get_parsed(ChromaticAdaptationTagSignature)
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

type ChannelTransformer interface {
	Transform(workspace []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float)
	IsSuitableFor(num_input_channels int, num_output_channels int) bool
	WorkspaceSize() int
}

type TwoTransformers struct {
	a, b         func(ws []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float)
	ws           int
	transformers []ChannelTransformer
}

func (t *TwoTransformers) WorkspaceSize() int {
	return t.ws
}

func (t *TwoTransformers) IsSuitableFor(i, o int) bool {
	for _, x := range t.transformers {
		if !x.IsSuitableFor(i, o) {
			return false
		}
	}
	return true
}

func (t *TwoTransformers) Transform(ws []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float) {
	r, g, b = t.a(ws, r, g, b)
	r, g, b = t.b(ws, r, g, b)
	return r, g, b
}

type ThreeTransformers struct {
	a, b, c      func(ws []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float)
	ws           int
	transformers []ChannelTransformer
}

func (t *ThreeTransformers) WorkspaceSize() int {
	return t.ws
}

func (t *ThreeTransformers) IsSuitableFor(i, o int) bool {
	for _, x := range t.transformers {
		if !x.IsSuitableFor(i, o) {
			return false
		}
	}
	return true
}

func (t *ThreeTransformers) Transform(ws []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float) {
	r, g, b = t.a(ws, r, g, b)
	r, g, b = t.b(ws, r, g, b)
	r, g, b = t.c(ws, r, g, b)
	return r, g, b
}

type MultipleTransformers struct {
	ws           int
	transformers []ChannelTransformer
}

func (t *MultipleTransformers) WorkspaceSize() int {
	return t.ws
}

func (t *MultipleTransformers) IsSuitableFor(i, o int) bool {
	for _, x := range t.transformers {
		if !x.IsSuitableFor(i, o) {
			return false
		}
	}
	return true
}

func (t *MultipleTransformers) Transform(ws []unit_float, r, g, b unit_float) (unit_float, unit_float, unit_float) {
	for _, x := range t.transformers {
		r, g, b = x.Transform(ws, r, g, b)
	}
	return r, g, b
}

func NewCombinedTransformer(t ...ChannelTransformer) ChannelTransformer {
	ws := 0
	for _, x := range t {
		ws = max(ws, x.WorkspaceSize())
	}
	switch len(t) {
	case 0:
		m := IdentityMatrix(0)
		return &m
	case 1:
		return t[0]
	case 2:
		return &TwoTransformers{t[0].Transform, t[1].Transform, ws, t}
	case 3:
		return &ThreeTransformers{t[0].Transform, t[1].Transform, t[2].Transform, ws, t}
	default:
		return &MultipleTransformers{ws, t}
	}
}
