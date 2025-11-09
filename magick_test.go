package imaging

import (
	"fmt"
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
	var no_changes_cfg = NewDecodeConfig(ColorSpace(NO_CHANGE_OF_COLORSPACE), AutoOrientation(false))
	img, err := decode_all_magick(&types.Input{Path: o3}, nil, no_changes_cfg)
	require.NoError(t, err)
	require.Equal(t, len(img.Frames), 1)
}
