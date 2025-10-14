package icc

import (
	"encoding/binary"
	"fmt"
)

type TagTable struct {
	entries map[Signature][]byte
}

func (t *TagTable) add(sig Signature, data []byte) {
	t.entries[sig] = data
}

func (t *TagTable) getDescription(s Signature) (string, error) {
	data, ok := t.entries[s]
	if !ok {
		return "", fmt.Errorf("no description tag in ICC profile")
	}
	var sig uint32

	if _, err := binary.Decode(data, binary.BigEndian, &sig); err != nil {
		return "", err
	}

	switch Signature(sig) {

	case DescSignature:
		desc, err := parseTextDescription(data)
		if err != nil {
			return "", err
		}
		return desc.ASCII, nil

	case MultiLocalisedUnicodeSignature:
		mluc, err := parseMultiLocalisedUnicode(data)
		if err != nil {
			return "", err
		}
		if enUS := mluc.getStringForLanguage([2]byte{'e', 'n'}); enUS != "" {
			return enUS, nil
		}
		return mluc.getAnyString(), nil

	default:
		return "", fmt.Errorf("unknown profile description type (%v)", Signature(sig))
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

func emptyTagTable() TagTable {
	return TagTable{
		entries: make(map[Signature][]byte),
	}
}
