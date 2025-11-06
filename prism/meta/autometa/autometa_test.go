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
			require.Nil(t, md)
		}

		returnedBytes, err := io.ReadAll(stream)
		require.NoError(t, err)
		require.Equal(t, data, returnedBytes)
	})
}
