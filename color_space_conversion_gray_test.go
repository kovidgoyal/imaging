package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
	"testing"

	"github.com/kovidgoyal/imaging/nrgb"
	"github.com/kovidgoyal/imaging/prism/meta/icc"
	"github.com/stretchr/testify/require"
)

// encodeS15Fixed16BEForTest encodes f as an ICC s15Fixed16Number.
func encodeS15Fixed16BEForTest(f float64) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(int32(f*65536)))
	return b
}

// buildGrayICCProfile builds a minimal, valid monochrome (Gray) ICC profile
// with a simple gamma kTRC curve and a D50-ish media white point, for
// testing the Gray colour-space conversion path end to end.
func buildGrayICCProfile(t *testing.T) []byte {
	t.Helper()

	para := func(gamma float64) []byte {
		b := bytes.NewBuffer([]byte("para\x00\x00\x00\x00"))
		binary.Write(b, binary.BigEndian, uint16(0)) // function type 0: Y = X^g
		b.WriteString("\x00\x00")
		b.Write(encodeS15Fixed16BEForTest(gamma))
		if extra := b.Len() % 4; extra != 0 {
			b.Write(bytes.Repeat([]byte{0}, 4-extra))
		}
		return b.Bytes()
	}
	xyz := func(x, y, z float64) []byte {
		b := bytes.NewBuffer([]byte("XYZ \x00\x00\x00\x00"))
		b.Write(encodeS15Fixed16BEForTest(x))
		b.Write(encodeS15Fixed16BEForTest(y))
		b.Write(encodeS15Fixed16BEForTest(z))
		return b.Bytes()
	}

	header := &bytes.Buffer{}
	binary.Write(header, binary.BigEndian, uint32(0)) // Profile size
	header.Write([]byte{'t', 'e', 's', 't'})          // Preferred CMM
	header.Write([]byte{4, 0, 0, 0})                  // Version
	header.Write([]byte{'m', 'n', 't', 'r'})          // Device class
	header.Write([]byte{'G', 'R', 'A', 'Y'})          // Data colour space
	header.Write([]byte{'X', 'Y', 'Z', ' '})          // Profile connection space
	header.Write(make([]byte, 12))                    // Creation date/time
	header.Write([]byte{'a', 'c', 's', 'p'})          // Profile signature
	header.Write([]byte{'t', 'e', 's', 't'})          // Primary platform
	header.Write(make([]byte, 4))                     // Profile flags
	header.Write(make([]byte, 4))                     // Device manufacturer
	header.Write(make([]byte, 4))                     // Device model
	header.Write(make([]byte, 8))                     // Device attributes
	header.Write([]byte{0, 0, 0, 0})                  // Rendering intent (Perceptual)
	header.Write(make([]byte, 12))                    // PCS illuminant
	header.Write(make([]byte, 4))                     // Profile creator
	header.Write(make([]byte, 16))                    // Profile ID
	header.Write(make([]byte, 28))                    // Reserved

	tags := map[[4]byte][]byte{
		{'k', 'T', 'R', 'C'}: para(2.2),
		{'w', 't', 'p', 't'}: xyz(0.9642, 1.0, 0.8249),
	}
	tagTable := &bytes.Buffer{}
	binary.Write(tagTable, binary.BigEndian, uint32(len(tags)))
	offset := 128 + 4 + len(tags)*12
	data := &bytes.Buffer{}
	for sig, tagData := range tags {
		tagTable.Write(sig[:])
		binary.Write(tagTable, binary.BigEndian, uint32(offset))
		binary.Write(tagTable, binary.BigEndian, uint32(len(tagData)))
		offset += len(tagData)
		data.Write(tagData)
	}
	tagTable.Write(data.Bytes())

	return append(header.Bytes(), tagTable.Bytes()...)
}

func TestConvertGrayToSRGB(t *testing.T) {
	data := buildGrayICCProfile(t)
	p, err := icc.DecodeProfile(bytes.NewReader(data))
	require.NoError(t, err)
	require.False(t, p.IsSRGB())

	img := image.NewGray(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 15)
	}

	out, err := ConvertToSRGB(p, icc.PerceptualRenderingIntent, false, img)
	require.NoError(t, err)

	nimg, ok := out.(*nrgb.Image)
	require.True(t, ok, "expected *nrgb.Image, got %T", out)
	require.Equal(t, img.Bounds(), nimg.Bounds())

	// Brighter input gray levels should map to brighter (or equal) output
	// luminance: the conversion should be monotonic, not garbage.
	var prevLuma int
	for i := 0; i < len(img.Pix); i++ {
		r, g, b := nimg.Pix[3*i], nimg.Pix[3*i+1], nimg.Pix[3*i+2]
		luma := int(r) + int(g) + int(b)
		require.GreaterOrEqual(t, luma, prevLuma, "pixel %d should not be darker than the previous one", i)
		prevLuma = luma
	}
}
