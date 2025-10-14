package icc

type Signature uint32

const (
	ProfileFileSignature Signature = 0x61637370 // 'acsp'
	TextTagSignature     Signature = 0x74657874 // 'text'
	SignateTagSignature  Signature = 0x73696720 // 'sig '

	DescSignature                          Signature = 0x64657363 // 'desc'
	MultiLocalisedUnicodeSignature         Signature = 0x6D6C7563 // 'mluc'
	DeviceManufacturerDescriptionSignature Signature = 0x646d6e64 // 'dmnd'
	DeviceModelDescriptionSignature        Signature = 0x646d6464 // 'dmdd'

	AdobeManufacturerSignature      Signature = 0x41444245 // 'ADBE'
	AppleManufacturerSignature      Signature = 0x6170706c // 'appl'
	AppleUpperManufacturerSignature Signature = 0x4150504c // 'APPL'
	IECManufacturerSignature        Signature = 0x49454320 // 'IEC '

	AdobeRGBModelSignature  Signature = 0x52474220 // 'RGB '
	SRGBModelSignature      Signature = 0x73524742 // 'sRGB'
	PhotoProModelSignature  Signature = 0x50525452 // 'PTPR'
	DisplayP3ModelSignature Signature = 0x70332020 // 'p3  '
)

func maskNull(b byte) byte {
	switch b {
	case 0:
		return ' '
	default:
		return b
	}
}

func signature(b []byte) Signature {
	return Signature(uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]))
}

func SignatureFromString(sig string) Signature {
	var b []byte = []byte{0x20, 0x20, 0x20, 0x20}
	copy(b, sig)
	return signature(b)
}

func (s Signature) String() string {
	v := []byte{
		(maskNull(byte((s >> 24) & 0xff))),
		(maskNull(byte((s >> 16) & 0xff))),
		(maskNull(byte((s >> 8) & 0xff))),
		(maskNull(byte(s & 0xff))),
	}
	return "'" + string(v) + "'"
}
