package autometa

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

var _ = fmt.Println

func TestLoad(t *testing.T) {

	t.Run("recognizes EXIF data in TIFF", func(t *testing.T) {
		data, err := os.ReadFile("orientation_2.tiff")
		require.NoError(t, err)
		input := bytes.NewReader(data)
		md, _, err := Load(input)
		require.NoError(t, err)
		require.NotNil(t, md)
	})

	t.Run("returns original image data when format is unrecognised", func(t *testing.T) {
		data := []byte("not an image format simply some plain text")
		input := bytes.NewReader(data)

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
		require.NoError(t, err)
		require.Equal(t, data, returnedBytes)
	})
}
