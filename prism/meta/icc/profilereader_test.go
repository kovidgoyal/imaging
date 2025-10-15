package icc

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path"
	"strings"
	"testing"
)

func TestProfileReader(t *testing.T) {
	var profileSize uint32
	var profileID [16]byte
	var reservedBytes [28]byte

	loadTestProfile := func(profileFileName string) (*Profile, error) {
		profileFile, err := os.Open(path.Join("test-profiles", profileFileName))
		if err != nil {
			return nil, fmt.Errorf("error opening '%s': %w", profileFileName, err)
		}

		defer profileFile.Close()

		reader := NewProfileReader(bufio.NewReader(profileFile))
		return reader.ReadProfile()
	}

	writeHeader := func(w io.Writer, profileSig [4]byte) {
		profileSize = uint32(rand.Int31())
		binary.Write(w, binary.BigEndian, profileSize)

		_, _ = w.Write([]byte{'t', 'e', 's', 't'})                 // Preferred CMM
		_, _ = w.Write([]byte{4, 0, 0, 0})                         // Version
		_, _ = w.Write([]byte{'t', 'e', 's', 't'})                 // Device class
		_, _ = w.Write([]byte{'R', 'G', 'B', ' '})                 // Data colour space
		_, _ = w.Write([]byte{'X', 'Y', 'Z', ' '})                 // Profile connection space
		_, _ = w.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) // Creation date/time
		_, _ = w.Write(profileSig[:])                              // Profile signature
		_, _ = w.Write([]byte{'t', 'e', 's', 't'})                 // Primary platform
		_, _ = w.Write([]byte{0, 0, 0, 0})                         // Profile flags
		_, _ = w.Write([]byte{0, 0, 0, 0})                         // Device manufacturer
		_, _ = w.Write([]byte{0, 0, 0, 0})                         // Device model
		_, _ = w.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0})             // Device attributes
		_, _ = w.Write([]byte{0, 0, 0, 0})                         // Rendering intent
		_, _ = w.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) // PCS illuminant
		_, _ = w.Write([]byte{0, 0, 0, 0})                         // Profile creator
		_, _ = w.Write(profileID[:])
		_, _ = w.Write(reservedBytes[:])
	}

	writeTagTable := func(w io.Writer, tags map[[4]byte][]byte) {
		_ = binary.Write(w, binary.BigEndian, uint32(len(tags)))

		offset := 128 + 4 + len(tags)*12

		tagTableData := &bytes.Buffer{}

		for tagSig, tagData := range tags {
			_, _ = w.Write(tagSig[:])
			_ = binary.Write(w, binary.BigEndian, uint32(offset))
			_ = binary.Write(w, binary.BigEndian, uint32(len(tagData)))
			offset += len(tagData)

			_, _ = tagTableData.Write(tagData)
		}

		_, _ = w.Write(tagTableData.Bytes())
	}

	t.Run("readHeader()", func(t *testing.T) {

		t.Run("parses valid header successfully", func(t *testing.T) {
			headerData := &bytes.Buffer{}
			writeHeader(headerData, [4]byte{'a', 'c', 's', 'p'})
			pr := NewProfileReader(headerData)

			header := Header{}
			err := pr.readHeader(&header)
			if err != nil {
				t.Fatalf("Expected success but got error: %v", err)
			}

			if expected, actual := profileSize, header.ProfileSize; expected != actual {
				t.Errorf("Expected profile size of %d but got %d", expected, actual)
			}
			if expected, actual := profileID, header.ProfileID; expected != actual {
				t.Errorf("Expected profile ID %v but got %v", expected, actual)
			}
		})

		t.Run("returns error with invalid profile signature", func(t *testing.T) {
			headerData := &bytes.Buffer{}
			writeHeader(headerData, [4]byte{'b', 'a', 'd', '!'})
			pr := NewProfileReader(headerData)

			header := Header{}
			err := pr.readHeader(&header)
			if err == nil {
				t.Errorf("Expected an error but succeeded")
			} else if actual := err.Error(); !strings.Contains(actual, "'bad!'") {
				t.Errorf("Expected error bad signature error but got '%s'", actual)
			}
		})
	})

	t.Run("ReadProfile()", func(t *testing.T) {

		t.Run("returns an error when header parsing fails", func(t *testing.T) {
			profileData := &bytes.Buffer{}
			writeHeader(profileData, [4]byte{'b', 'a', 'd', '!'})
			writeTagTable(profileData, map[[4]byte][]byte{
				{'t', 'e', 's', 't'}: {},
			})

			reader := NewProfileReader(profileData)
			_, err := reader.ReadProfile()

			if err == nil {
				t.Errorf("Expected error but operation succeeded")
			} else if actual := err.Error(); !strings.Contains(actual, "'bad!'") {
				t.Errorf("Expected error bad signature but got '%s'", actual)
			}
		})

		t.Run("returns an error when tag table parsing fails", func(t *testing.T) {
			profileData := &bytes.Buffer{}
			writeHeader(profileData, [4]byte{'a', 'c', 's', 'p'})

			_, _ = profileData.Write([]byte{
				0x00, 0x00, 0x00, 0x01, // Tag count
			})

			reader := NewProfileReader(profileData)
			_, err := reader.ReadProfile()

			if err == nil {
				t.Errorf("Expected error but operation succeeded")
			} else if actual := err.Error(); !strings.Contains(actual, "EOF") {
				t.Errorf("Expected error EOF but got '%s'", actual)
			}
		})

		t.Run("successfully reads profile descriptions", func(t *testing.T) {
			cases := []struct {
				ProfileFileName     string
				ExpectedDescription string
			}{
				{ProfileFileName: "display-p3-v4-with-v2-desc.icc", ExpectedDescription: "Display P3"},
			}

			for _, c := range cases {
				profile, err := loadTestProfile(c.ProfileFileName)
				if err != nil {
					t.Errorf("Error reading profile '%s', %v", c.ProfileFileName, err)
					continue
				}

				desc, err := profile.Description()
				if err != nil {
					t.Errorf("Error reading profile description from '%s': %v", c.ProfileFileName, err)
					continue
				}

				if desc != c.ExpectedDescription {
					t.Errorf("Expected description '%s' for profile '%s' but got '%s'", c.ExpectedDescription, c.ProfileFileName, desc)
				}
			}
		})

		t.Run("recognises well known profiles", func(t *testing.T) {
			for fname, expected := range map[string]WellKnownProfile{
				"sRGB2014.icc":                   SRGBProfile,
				"sRGB_ICC_v4_Appearance.icc":     SRGBProfile,
				"display-p3-v4-with-v2-desc.icc": DisplayP3Profile,
			} {
				p, err := loadTestProfile(fname)
				if err != nil {
					t.Fatalf("failed reading profile: %s with error: %s", fname, err)
				}
				d, err := p.Description()
				man, err := p.DeviceManufacturerDescription()
				model, err := p.DeviceModelDescription()
				if actual := p.WellKnownProfile(); actual != expected {
					t.Fatalf("Incorrect profile for img: %s, expected %s, got %s\nHeader: %s\nDescription: %s\nDeviceManufacturer: %s\nDevice Model: %s", fname, expected, actual, p.Header, d, man, model)
				}
			}
		})
	})
}
