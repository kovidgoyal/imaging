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

func (t *TagTable) load_rgb_matrix() (ChannelTransformer, ChannelTransformer, error) {
	r, err := t.get_parsed(RedMatrixColumnTagSignature)
	if err != nil {
		return nil, nil, err
	}
	g, err := t.get_parsed(GreenMatrixColumnTagSignature)
	if err != nil {
		return nil, nil, err
	}
	b, err := t.get_parsed(BlueMatrixColumnTagSignature)
	if err != nil {
		return nil, nil, err
	}
	rc, bc, gc := r.(*XYZType), g.(*XYZType), b.(*XYZType)
	var m Matrix3
	m[0][0], m[0][1], m[0][2] = rc.x, bc.x, gc.x
	m[1][0], m[1][1], m[1][2] = rc.y, bc.y, gc.y
	m[2][0], m[2][1], m[2][2] = rc.z, bc.z, gc.z
	if is_identity_matrix(&m) {
		im := IdentityMatrix(0)
		return &im, &im, nil
	}
	im, err := m.Inverted()
	return &m, &im, err
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
