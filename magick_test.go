package imaging

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/kovidgoyal/imaging/magick"
	"github.com/kovidgoyal/imaging/types"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func TestMagick(t *testing.T) {
	if !magick.HasMagick() {
		t.Skip("ImageMagick not available, skipping test")
	}
	const o3 = "testdata/orientation_3.jpg"
	const cmykpath = "prism/test-images/cmyk.jpg"
	var no_changes_cfg = NewDecodeConfig(ColorSpace(NO_CHANGE_OF_COLORSPACE), AutoOrientation(false))
	img, err := decode_all_magick(&types.Input{Path: o3}, nil, no_changes_cfg)
	require.NoError(t, err)
	require.Equal(t, len(img.Frames), 1)
	f, err := os.Open(o3)
	require.NoError(t, err)
	defer f.Close()
	img2, err := decode_all_magick(&types.Input{Reader: f}, nil, no_changes_cfg)
	require.NoError(t, err)
	require.Equal(t, len(img2.Frames), 1)
	data, err := os.ReadFile(o3)
	require.NoError(t, err)
	img2, err = decode_all_magick(&types.Input{Reader: bytes.NewReader(data)}, nil, no_changes_cfg)
	require.NoError(t, err)
	require.Equal(t, len(img2.Frames), 1)

	cmyk, err := decode_all_magick(&types.Input{Path: cmykpath}, nil, NewDecodeConfig())
	require.NoError(t, err)
	require.Equal(t, len(cmyk.Frames), 1)
}
