package icc

import (
	"bytes"
	_ "embed"
	"fmt"
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

func (p *Profile) CreateTransformerToPCS(rendering_intent RenderingIntent) (ans ChannelTransformer, err error) {
	a2b := UnknownSignature
	switch rendering_intent {
	case PerceptualRenderingIntent:
		a2b = AToB0TagSignature
	case RelativeColorimetricRenderingIntent:
		a2b = AToB1TagSignature
	case SaturationRenderingIntent:
		a2b = AToB2TagSignature
	case AbsoluteColorimetricRenderingIntent:
		a2b = AToB3TagSignature
	}
	found := p.TagTable.Has(a2b)
	if !found && a2b == AToB3TagSignature && p.TagTable.Has(AToB0TagSignature) {
		a2b = AToB0TagSignature
		found = true
	}
	if found {
		fmt.Println(3333333333, string(p.TagTable.entries[a2b].data[:8]), len(p.TagTable.entries[a2b].data))
	} else {
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
		ct := NewInverseCurveTransformer(rc, gc, bc)
		m, _, err := p.TagTable.load_rgb_matrix()
		if err != nil {
			return nil, err
		}
		_, _ = ct, m
	}
	for sig := range p.TagTable.entries {
		fmt.Println(11111111, sig.String())
	}
	return nil, nil
}

func newProfile() *Profile {
	return &Profile{
		TagTable: emptyTagTable(),
	}
}
