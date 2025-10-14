package icc

import (
	"bytes"
	"encoding/binary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"unicode/utf16"
)

func TestDescDecoder(t *testing.T) {
	ascii := []byte("Test description\x00")
	unicode := []uint16{'T', 'e', 's', 't', 0xD834, 0xDD1E} // "Test𝄞"
	script := []byte("Latn")

	var buf bytes.Buffer
	buf.WriteString("desc")
	buf.Write([]byte{0, 0, 0, 0}) // reserved
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(ascii)))
	buf.Write(ascii)
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(unicode)))
	for _, r := range unicode {
		_ = binary.Write(&buf, binary.BigEndian, r)
	}
	buf.WriteByte(byte(len(script)))
	buf.Write(script)

	val, err := descDecoder(buf.Bytes())
	require.NoError(t, err)
	require.IsType(t, &DescriptionTag{}, val)
	desc := val.(*DescriptionTag)
	assert.Equal(t, "Test description", desc.ASCII)
	assert.Equal(t, "Test𝄞", desc.Unicode)
	assert.Equal(t, "Latn", desc.Script)
}

func TestDescDecoder_AsciiOnly(t *testing.T) {
	ascii := []byte("Test description\x00")

	var buf bytes.Buffer
	buf.WriteString("desc")
	buf.Write([]byte{0, 0, 0, 0}) // reserved
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(ascii)))
	buf.Write(ascii)

	val, err := descDecoder(buf.Bytes())
	require.NoError(t, err)
	require.IsType(t, &DescriptionTag{}, val)
	desc := val.(*DescriptionTag)
	assert.Equal(t, "Test description", desc.ASCII)
	assert.Equal(t, "", desc.Unicode)
	assert.Equal(t, "", desc.Script)
}

func TestDescDecoder_AsciiAndUnicodeOnly(t *testing.T) {
	ascii := []byte("Test description\x00")
	unicode := []uint16{'T', 'e', 's', 't', 0xD834, 0xDD1E} // "Test𝄞"

	var buf bytes.Buffer
	buf.WriteString("desc")
	buf.Write([]byte{0, 0, 0, 0}) // reserved
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(ascii)))
	buf.Write(ascii)
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(unicode)))
	for _, r := range unicode {
		_ = binary.Write(&buf, binary.BigEndian, r)
	}

	val, err := descDecoder(buf.Bytes())
	require.NoError(t, err)
	require.IsType(t, &DescriptionTag{}, val)
	desc := val.(*DescriptionTag)
	assert.Equal(t, "Test description", desc.ASCII)
	assert.Equal(t, "Test𝄞", desc.Unicode)
	assert.Equal(t, "", desc.Script)
}

func TestDescDecoder_Errors(t *testing.T) {
	t.Run("TooShortRaw", func(t *testing.T) {
		_, err := descDecoder([]byte("desc\x00\x00"))
		assert.ErrorContains(t, err, "desc tag too short")
	})
	t.Run("InvalidASCIILength", func(t *testing.T) {
		raw := append([]byte("desc\x00\x00\x00\x00"), []byte{0xFF, 0xFF, 0xFF, 0xFF}...) // absurd length
		_, err := descDecoder(raw)
		assert.ErrorContains(t, err, "invalid ASCII length")
	})
	t.Run("TruncatedUTF16", func(t *testing.T) {
		var buf bytes.Buffer
		buf.WriteString("desc")
		buf.Write([]byte{0, 0, 0, 0})
		ascii := []byte("Short\x00")
		_ = binary.Write(&buf, binary.BigEndian, uint32(len(ascii)))
		buf.Write(ascii)
		_ = binary.Write(&buf, binary.BigEndian, uint32(2)) // claims 2 UTF-16 code units
		buf.Write([]byte{0x00, 0x41})                       // only one byte pair instead of two
		_, err := descDecoder(buf.Bytes())
		assert.ErrorContains(t, err, "desc tag truncated: missing UTF-16 data")
	})
	t.Run("TruncatedScript", func(t *testing.T) {
		var buf bytes.Buffer
		buf.WriteString("desc")
		buf.Write([]byte{0, 0, 0, 0})
		ascii := []byte("Just ASCII\x00")
		_ = binary.Write(&buf, binary.BigEndian, uint32(len(ascii)))
		buf.Write(ascii)
		_ = binary.Write(&buf, binary.BigEndian, uint32(1)) // one code unit
		buf.Write([]byte{0x00, 0x61})                       // 'a'
		buf.WriteByte(4)                                    // says 4 bytes of script
		buf.Write([]byte("La"))                             // only 2 bytes provided
		_, err := descDecoder(buf.Bytes())
		assert.ErrorContains(t, err, "desc tag truncated: missing ScriptCode data")
	})
}

func TestTextDecoder(t *testing.T) {
	t.Run("full text", func(t *testing.T) {
		str, err := textDecoder([]byte("\x00\x00\x00\x00\x00\x00\x00\x00foo"))
		require.NoError(t, err)
		require.Equal(t, "foo", str.(*PlainText).val)
	})
	t.Run("zero trimmed text", func(t *testing.T) {
		str, err := textDecoder([]byte("\x00\x00\x00\x00\x00\x00\x00\x00foo\x00\x00\x00"))
		require.NoError(t, err)
		require.Equal(t, "foo", str.(*PlainText).val)
	})
}

func TestTextDecoder_Errors(t *testing.T) {
	_, err := textDecoder([]byte("1234567"))
	require.Error(t, err)
}

func TestSigDecoder(t *testing.T) {
	t.Run("full text", func(t *testing.T) {
		str, err := sigDecoder([]byte("\x00\x00\x00\x00\x00\x00\x00\x00foob"))
		require.NoError(t, err)
		require.Equal(t, SignatureFromString("foob"), str)
	})
	t.Run("zero trimmed text", func(t *testing.T) {
		str, err := sigDecoder([]byte("\x00\x00\x00\x00\x00\x00\x00\x00foo \x00\x00"))
		require.NoError(t, err)
		require.Equal(t, SignatureFromString("foo"), str)
	})
}

func TestSigDecoder_Errors(t *testing.T) {
	_, err := sigDecoder([]byte("1234567"))
	require.Error(t, err)
}

func TestMLUCDecoder_SingleEntry(t *testing.T) {
	var buf bytes.Buffer
	// Header: signature + reserved
	buf.WriteString("mluc")
	buf.Write([]byte{0, 0, 0, 0})
	// Count: 1 entry, record size: 12
	_ = binary.Write(&buf, binary.BigEndian, uint32(1))
	_ = binary.Write(&buf, binary.BigEndian, uint32(12))
	// Record: lang="en", country="US", len=12, offset=28
	buf.Write([]byte("enUS"))
	_ = binary.Write(&buf, binary.BigEndian, uint32(12)) // length
	_ = binary.Write(&buf, binary.BigEndian, uint32(28)) // offset
	// Pad up to offset 28
	for buf.Len() < 28 {
		buf.WriteByte(0)
	}
	// UTF-16BE for "Hello"
	buf.Write([]byte{
		0x00, 'H', 0x00, 'e', 0x00, 'l', 0x00, 'l', 0x00, 'o', 0x00, '!',
	})
	val, err := mlucDecoder(buf.Bytes())
	require.NoError(t, err)
	require.IsType(t, &MultiLocalizedTag{}, val)
	mluc := val.(*MultiLocalizedTag)
	assert.Len(t, mluc.Strings, 1)
	assert.Equal(t, "en", mluc.Strings[0].Language)
	assert.Equal(t, "US", mluc.Strings[0].Country)
	assert.Equal(t, "Hello!", mluc.Strings[0].Value)
}

func TestMLUCDecoder_Errors(t *testing.T) {
	t.Run("TooShort", func(t *testing.T) {
		_, err := mlucDecoder([]byte("mluc"))
		assert.ErrorContains(t, err, "mluc tag too short")
	})
	t.Run("InvalidRecordSize", func(t *testing.T) {
		buf := append([]byte("mluc\x00\x00\x00\x00"), []byte{
			0x00, 0x00, 0x00, 0x01, // count
			0x00, 0x00, 0x00, 0x10, // record size (not 12)
		}...)
		_, err := mlucDecoder(buf)
		require.Error(t, err)
		assert.ErrorContains(t, err, "unexpected mluc record size")
	})
	t.Run("TruncatedRecords", func(t *testing.T) {
		buf := append([]byte("mluc\x00\x00\x00\x00"), []byte{
			0x00, 0x00, 0x00, 0x02, // count = 2
			0x00, 0x00, 0x00, 0x0C, // record size = 12
		}...)
		// Only 1 record provided (24 bytes needed after header)
		buf = append(buf, make([]byte, 12)...)
		_, err := mlucDecoder(buf)
		require.Error(t, err)
		assert.ErrorContains(t, err, "mluc tag too small for 2 records")
	})
	t.Run("InvalidOffsetOrLength", func(t *testing.T) {
		var buf bytes.Buffer
		buf.WriteString("mluc")
		buf.Write([]byte{0, 0, 0, 0})
		_ = binary.Write(&buf, binary.BigEndian, uint32(1))
		_ = binary.Write(&buf, binary.BigEndian, uint32(12))
		buf.Write([]byte("enUS"))
		_ = binary.Write(&buf, binary.BigEndian, uint32(13)) // odd length
		_ = binary.Write(&buf, binary.BigEndian, uint32(64)) // way out of bounds
		_, err := mlucDecoder(buf.Bytes())
		require.Error(t, err)
		assert.ErrorContains(t, err, "invalid string offset/length")
	})
	t.Run("InvalidUTF16", func(t *testing.T) {
		var buf bytes.Buffer
		buf.WriteString("mluc")
		buf.Write([]byte{0, 0, 0, 0})                        // reserved
		_ = binary.Write(&buf, binary.BigEndian, uint32(1))  // count
		_ = binary.Write(&buf, binary.BigEndian, uint32(12)) // record size
		buf.Write([]byte("enUS"))                            // lang + country
		_ = binary.Write(&buf, binary.BigEndian, uint32(5))  // invalid (odd) length
		_ = binary.Write(&buf, binary.BigEndian, uint32(32)) // offset
		// pad to 32
		for buf.Len() < 32 {
			buf.WriteByte(0)
		}
		buf.Write([]byte{0x00, 'B', 0x00, 'a', 0x00}) // 5 bytes, odd!
		_, err := mlucDecoder(buf.Bytes())
		require.Error(t, err)
		assert.ErrorContains(t, err, "invalid string offset/length in mluc record")
	})
}

func TestDecodeUTF16BE(t *testing.T) {
	t.Run("BasicASCII", func(t *testing.T) {
		// UTF-16BE for "Go!"
		utf16be := []byte{0x00, 'G', 0x00, 'o', 0x00, '!'}
		str := decodeUTF16BE(utf16be)
		assert.Equal(t, "Go!", str)
	})
	t.Run("WithSurrogatePair", func(t *testing.T) {
		// "Test𝄞" (𝄞 = U+1D11E = surrogate pair)
		runes := []rune("Test𝄞")
		codeUnits := utf16.Encode(runes)
		buf := make([]byte, len(codeUnits)*2)
		for i, cu := range codeUnits {
			binary.BigEndian.PutUint16(buf[i*2:], cu)
		}
		str := decodeUTF16BE(buf)
		assert.Equal(t, "Test𝄞", str)
	})
}
