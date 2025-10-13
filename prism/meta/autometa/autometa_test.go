package autometa

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kovidgoyal/imaging/prism/meta/icc"
)

var _ = fmt.Println

func TestLoad(t *testing.T) {

	t.Run("returns original image data when format is unrecognised", func(t *testing.T) {
		randomBytes := make([]byte, 16)
		_, err := rand.Read(randomBytes)
		if err != nil {
			panic(err)
		}

		input := bytes.NewReader(randomBytes)

		md, stream, err := Load(input)

		if err == nil {
			t.Fatalf("Expected error but succeeded")
		}
		if expected, actual := "unrecognised image format", err.Error(); expected != actual {
			t.Errorf("Expected error '%s' but was '%s'", expected, actual)
		}

		if md != nil {
			t.Errorf("Expected no metadata to be returned but was %+v", md)
		}

		returnedBytes, err := io.ReadAll(stream)
		if err != nil {
			t.Errorf("Expected to be able to read %d bytes from returned stream but got error: %v", len(randomBytes), err)
		}
		if expected, actual := len(randomBytes), len(returnedBytes); expected != actual {
			t.Fatalf("Expected returned stream to contain %d bytes but found %d", expected, actual)
		}

		if !bytes.Equal(randomBytes, returnedBytes) {
			t.Errorf("Expected returned stream to contain original image data but was different.\n\nExpected:%v\nActual:%v\n", randomBytes, returnedBytes)
		}
	})
}

func TestProfileRecognition(t *testing.T) {
	for imgname, expected := range map[string]icc.WellKnownProfile{
		"pizza-rgb8-srgb.jpg":        icc.SRGBProfile,
		"pizza-rgb8-adobergb.jpg":    icc.AdobeRGBProfile,
		"pizza-rgb8-displayp3.jpg":   icc.DisplayP3Profile,
		"pizza-rgb8-prophotorgb.jpg": icc.PhotoProProfile,
	} {
		t.Run(imgname, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join("../../test-images", imgname)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			m, _, err := Load(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			p, err := m.ICCProfile()
			if err != nil {
				t.Fatal(err)
			}
			d, err := p.Description()
			if actual := p.WellKnownProfile(); actual != expected {
				t.Fatalf("Incorrect profile for img: %s, expected %s, got %s\nHeader: %s\nDescription: %s", imgname, expected, actual, p.Header, d)
			}
		})
	}
}
