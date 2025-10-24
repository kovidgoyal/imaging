package icc

import (
	"bytes"
	_ "embed"
	"fmt"
	"runtime"
	"sync"
)

var _ = fmt.Println

type WellKnownProfile int

const (
	UnknownProfile WellKnownProfile = iota
	SRGBProfile
	AdobeRGBProfile
	PhotoProProfile
	DisplayP3Profile
)

//go:embed test-profiles/sRGB2014.icc
var Srgb_xyz_profile_data []byte

//go:embed test-profiles/sRGB_ICC_v4_Appearance.icc
var Srgb_lab_profile_data []byte

var Srgb_xyz_profile = sync.OnceValue(func() *Profile {
	p, _ := NewProfileReader(bytes.NewReader(Srgb_xyz_profile_data)).ReadProfile()
	return p
})

var Srgb_lab_profile = sync.OnceValue(func() *Profile {
	p, _ := NewProfileReader(bytes.NewReader(Srgb_lab_profile_data)).ReadProfile()
	return p
})

func WellKnownProfileFromDescription(x string) WellKnownProfile {
	switch x {
	case "sRGB IEC61966-2.1", "sRGB_ICC_v4_Appearance.icc":
		return SRGBProfile
	case "Adobe RGB (1998)":
		return AdobeRGBProfile
	case "Display P3":
		return DisplayP3Profile
	case "ProPhoto RGB":
		return PhotoProProfile
	default:
		return UnknownProfile
	}
}

func (p WellKnownProfile) String() string {
	switch p {
	case SRGBProfile:
		return "sRGB IEC61966-2.1"
	case AdobeRGBProfile:
		return "Adobe RGB (1998)"
	case PhotoProProfile:
		return "ProPhoto RGB"
	case DisplayP3Profile:
		return "Display P3"
	default:
		return "Unknown Profile"
	}
}

type Profile struct {
	Header        Header
	TagTable      TagTable
	PCSIlluminant XYZType
}

func (p *Profile) Description() (string, error) {
	return p.TagTable.getProfileDescription()
}

func (p *Profile) DeviceManufacturerDescription() (string, error) {
	return p.TagTable.getDeviceManufacturerDescription()
}

func (p *Profile) DeviceModelDescription() (string, error) {
	return p.TagTable.getDeviceModelDescription()
}

func (p *Profile) WellKnownProfile() WellKnownProfile {
	model, err := p.DeviceModelDescription()
	if err == nil {
		switch model {
		case "IEC 61966-2-1 Default RGB Colour Space - sRGB":
			return SRGBProfile
		}
	}
	d, err := p.Description()
	if err == nil {
		if ans := WellKnownProfileFromDescription(d); ans != UnknownProfile {
			return ans
		}
	}
	switch p.Header.DeviceManufacturer {
	case IECManufacturerSignature:
		switch p.Header.DeviceModel {
		case SRGBModelSignature:
			return SRGBProfile
		}
	case AdobeManufacturerSignature:
		switch p.Header.DeviceModel {
		case AdobeRGBModelSignature:
			return AdobeRGBProfile
		case PhotoProModelSignature:
			return PhotoProProfile
		}
	case AppleManufacturerSignature, AppleUpperManufacturerSignature:
		switch p.Header.DeviceModel {
		case DisplayP3ModelSignature:
			return DisplayP3Profile
		}
	}
	return UnknownProfile
}

func (p *Profile) get_effective_chromatic_adaption() (*Matrix3, error) {
	pcs_whitepoint := p.Header.ParsedPCSIlluminant()
	x, err := p.TagTable.get_parsed(MediaWhitePointTagSignature)
	if err != nil {
		return nil, err
	}
	wtpt, ok := x.(*XYZType)
	if !ok {
		return nil, fmt.Errorf("wtpt tag is not of XYZType")
	}
	if pcs_whitepoint == *wtpt {
		return nil, nil
	}
	return p.TagTable.get_chromatic_adaption()
}

func (p *Profile) create_matrix_trc_transformer(forward bool, chromatic_adaptation *Matrix3) (ans ChannelTransformer, err error) {
	if p.Header.ProfileConnectionSpace != ColorSpaceXYZ {
		return nil, fmt.Errorf("matrix/TRC based profile using non XYZ PCS color space: %v", p.Header.ProfileConnectionSpace)
	}
	// See section F.3 of ICC.1-2202-5.pdf for how these transforms are composed
	var rc, gc, bc Curve1D
	if rc, err = p.TagTable.load_curve_tag(RedTRCTagSignature); err != nil {
		return nil, err
	}
	if gc, err = p.TagTable.load_curve_tag(GreenTRCTagSignature); err != nil {
		return nil, err
	}
	if bc, err = p.TagTable.load_curve_tag(BlueTRCTagSignature); err != nil {
		return nil, err
	}
	m, err := p.TagTable.load_rgb_matrix()
	if err != nil {
		return nil, err
	}
	if is_identity_matrix(m) {
		m = chromatic_adaptation
		chromatic_adaptation = nil
	}
	if forward {
		ct := NewCurveTransformer(rc, gc, bc)
		if chromatic_adaptation != nil {
			combined := chromatic_adaptation.Multiply(*m)
			m = &combined
		}
		if m == nil {
			return ct, nil
		}
		return NewCombinedTransformer(ct, m), nil
	} else {
		ct := NewInverseCurveTransformer(rc, gc, bc)
		inv, err := m.Inverted()
		if err != nil {
			return nil, fmt.Errorf("the colorspace conversion matrix is not invertible: %w", err)
		}
		m = &inv
		if chromatic_adaptation != nil {
			minv, err := chromatic_adaptation.Inverted()
			if err != nil {
				return nil, fmt.Errorf("the chromatic_adaptation matrix is not invertible: %w", err)
			}
			combined := m.Multiply(minv)
			m = &combined
		}
		return NewCombinedTransformer(m, ct), nil
	}
}

// See section 8.10.2 of ICC.1-2202-05.pdf for tag selection algorithm
func (p *Profile) find_conversion_tag(forward bool, rendering_intent RenderingIntent) (ans ChannelTransformer, err error) {
	var ans_sig Signature = UnknownSignature
	found_tag := false
	if forward {
		switch rendering_intent {
		case PerceptualRenderingIntent:
			ans_sig = AToB0TagSignature
		case RelativeColorimetricRenderingIntent:
			ans_sig = AToB1TagSignature
		case SaturationRenderingIntent:
			ans_sig = AToB2TagSignature
		case AbsoluteColorimetricRenderingIntent:
			ans_sig = AToB3TagSignature
		default:
			return nil, fmt.Errorf("unknown rendering intent: %v", rendering_intent)
		}
		found_tag = p.TagTable.Has(ans_sig)
		const fallback = AToB0TagSignature
		if !found_tag && p.TagTable.Has(fallback) {
			ans_sig = fallback
			found_tag = true
		}
	} else {
		switch rendering_intent {
		case PerceptualRenderingIntent:
			ans_sig = BToA0TagSignature
		case RelativeColorimetricRenderingIntent:
			ans_sig = BToA1TagSignature
		case SaturationRenderingIntent:
			ans_sig = BToA2TagSignature
		case AbsoluteColorimetricRenderingIntent:
			ans_sig = BToA3TagSignature
		default:
			return nil, fmt.Errorf("unknown rendering intent: %v", rendering_intent)
		}
		found_tag = p.TagTable.Has(ans_sig)
		const fallback = BToA0TagSignature
		if !found_tag && p.TagTable.Has(fallback) {
			ans_sig = fallback
			found_tag = true
		}
	}
	if !found_tag {
		return nil, nil
	}
	c, err := p.TagTable.get_parsed(ans_sig)
	if err != nil {
		return nil, err
	}
	ans, ok := c.(ChannelTransformer)
	if !ok {
		return nil, fmt.Errorf("%s tag is not a ChannelTransformer: %T", ans_sig, c)
	}
	return ans, nil
}

func (p *Profile) CreateTransformerToDevice(rendering_intent RenderingIntent) (ans ChannelTransformer, err error) {
	b2a, err := p.find_conversion_tag(false, rendering_intent)
	if err != nil {
		return nil, err
	}
	chromatic_adaptation, err := p.get_effective_chromatic_adaption()
	if err != nil {
		return nil, err
	}
	if b2a != nil {
		// TODO: handle chromatic_adaptation
		return b2a, nil
	}
	return p.create_matrix_trc_transformer(false, chromatic_adaptation)
}

// See section 8.10.2 of ICC.1-2202-05.pdf for tag selection algorithm
func (p *Profile) CreateTransformerToPCS(rendering_intent RenderingIntent) (ans ChannelTransformer, err error) {
	a2b, err := p.find_conversion_tag(false, rendering_intent)
	if err != nil {
		return nil, err
	}
	chromatic_adaptation, err := p.get_effective_chromatic_adaption()
	if err != nil {
		return nil, err
	}
	if a2b != nil {
		// TODO: handle chromatic_adaptation
		return a2b, nil
	}
	return p.create_matrix_trc_transformer(true, chromatic_adaptation)
}

func newProfile() *Profile {
	return &Profile{
		TagTable: emptyTagTable(),
	}
}

func MakeWorkspace(c ChannelTransformer) []unit_float {
	return make([]unit_float, c.WorkspaceSize())
}

var Points_for_transformer_comparison = sync.OnceValue(func() []XYZType {
	const num = 16
	ans := make([]XYZType, 0, num*num*num)
	m := 1 / unit_float(num-1)
	for a := range num {
		aa := unit_float(a) * m
		for b := range num {
			bb := unit_float(b) * m
			for c := range num {
				cc := unit_float(c) * m
				ans = append(ans, XYZType{aa, bb, cc})
			}
		}
	}
	return ans
})

func transformers_functionally_identical(a, b ChannelTransformer) bool {
	pts := Points_for_transformer_comparison()
	num := max(1, runtime.GOMAXPROCS(0))
	c := make(chan bool)
	defer func() { close(c) }()
	start, limit := 0, len(pts)
	chunk_sz := max(1, limit/num)
	for start < limit {
		end := min(start+chunk_sz, limit)
		go func(start, end int) {
			defer recover() // ignore panic on sending to closed channel
			workspace := make([]unit_float, max(a.WorkspaceSize(), b.WorkspaceSize()))
			for i := start; i < end; i++ {
				p := pts[i]
				ar, ag, ab := a.Transform(workspace, p.X, p.Y, p.Z)
				br, bg, bb := b.Transform(workspace, p.X, p.Y, p.Z)
				if abs(ar-br) > FLOAT_EQUALITY_THRESHOLD || abs(ag-bg) > FLOAT_EQUALITY_THRESHOLD || abs(ab-bb) > FLOAT_EQUALITY_THRESHOLD {
					c <- false
				}
			}
			c <- true
		}(start, end)
		start = end
	}
	for val := range c {
		if !val {
			return false
		}
	}
	return true
}
