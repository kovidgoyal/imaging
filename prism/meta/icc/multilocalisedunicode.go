package icc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf16"
)

type MultiLocalisedUnicode struct {
	entriesByLanguageCountry map[[2]byte]map[[2]byte]string
}

func (mluc *MultiLocalisedUnicode) getAnyString() string {
	for _, country := range mluc.entriesByLanguageCountry {
		for _, s := range country {
			return s
		}
	}
	return ""
}

func (mluc *MultiLocalisedUnicode) getString(language [2]byte, country [2]byte) string {
	countries, ok := mluc.entriesByLanguageCountry[language]
	if !ok {
		return ""
	}

	return countries[country]
}

func (mluc *MultiLocalisedUnicode) getStringForLanguage(language [2]byte) string {
	for _, s := range mluc.entriesByLanguageCountry[language] {
		return s
	}
	return ""
}

func (mluc *MultiLocalisedUnicode) setString(language [2]byte, country [2]byte, text string) {
	countries, ok := mluc.entriesByLanguageCountry[language]
	if !ok {
		countries = map[[2]byte]string{
			country: text,
		}
		mluc.entriesByLanguageCountry[language] = countries

	} else {
		countries[country] = text
	}
}

func parseMultiLocalisedUnicode(data []byte) (MultiLocalisedUnicode, error) {
	result := MultiLocalisedUnicode{
		entriesByLanguageCountry: make(map[[2]byte]map[[2]byte]string),
	}

	reader := bytes.NewReader(data)
	type Header struct {
		Sig, Reserved, RecordCount, RecordSize uint32
	}
	var h Header
	if err := binary.Read(reader, binary.BigEndian, &h); err != nil {
		return result, err
	}

	if s := Signature(h.Sig); s != MultiLocalisedUnicodeSignature {
		return result, fmt.Errorf("expected %v but got %v", MultiLocalisedUnicodeSignature, s)
	}
	recordCount, recordSize := h.RecordCount, h.RecordSize
	type RecordHeader struct {
		Language, Country          [2]byte
		StringLength, StringOffset uint32
	}
	var rh RecordHeader

	for range recordCount {
		if err := binary.Read(reader, binary.BigEndian, &rh); err != nil {
			return result, fmt.Errorf("failed to read multilang record header: %w", err)
		}
		if uint64(rh.StringOffset)+uint64(rh.StringLength) > uint64(len(data)) {
			return result, fmt.Errorf("record exceeds tag data length")
		}
		recordStringBytes := data[rh.StringOffset : rh.StringOffset+rh.StringLength]
		recordStringUTF16 := make([]uint16, len(recordStringBytes)/2)
		if err := binary.Read(reader, binary.BigEndian, recordStringUTF16); err != nil {
			return result, err
		}
		result.setString(rh.Language, rh.Country, string(utf16.Decode(recordStringUTF16)))

		// Skip to next record
		if recordSize > 12 {
			skip := make([]byte, recordSize-12)
			if _, err := io.ReadFull(reader, skip); err != nil {
				return result, err
			}
		}
	}

	return result, nil
}

type languageCountry struct {
	language [2]byte
	country  [2]byte
}

func (lc languageCountry) String() string {
	return fmt.Sprintf("%c%c_%c%c", lc.language[0], lc.language[1], lc.country[0], lc.country[1])
}
