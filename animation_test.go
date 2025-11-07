package imaging

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func disposal(img *Image) []uint {
	actual := make([]uint, len(img.Frames))
	for i, f := range img.Frames {
		actual[i] = f.ComposeOnto
	}
	return actual
}

func assert_disposal(t *testing.T, img *Image, onto ...uint) {
	t.Helper()
	actual := make([]uint, len(img.Frames))
	for i, f := range img.Frames {
		actual[i] = f.ComposeOnto
	}
	require.Equal(t, onto, disposal(img))
}

func TestAnimation(t *testing.T) {
	gif, err := OpenAll("testdata/animated.gif")
	require.NoError(t, err)
	png, err := OpenAll("testdata/animated.apng")
	require.NoError(t, err)
	require.Equal(t, len(gif.Frames), len(png.Frames))
	require.Equal(t, (gif.LoopCount), (png.LoopCount))
	for i, gf := range gif.Frames {
		pf := png.Frames[i]
		require.Equal(t, gf.Delay, pf.Delay)
		require.Equal(t, gf.Number, pf.Number)
		if i > 0 {
			require.Equal(t, gf.Replace, pf.Replace)
		}
		require.Equal(t, gf.Image.Bounds().Min, pf.Image.Bounds().Min, fmt.Sprintf("frame number: %d", gf.Number))
	}

	assert_disposal(t, png, 0x0, 0x1, 0x2, 0x3, 0x4, 0x5, 0x5, 0x7, 0x8, 0x9, 0x9, 0xb)
	assert_disposal(t, gif, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11)
	gif, err = OpenAll("testdata/apple.gif")
	require.NoError(t, err)
	// this tests the background == none disposal behavior
	assert_disposal(t, gif, 0, 1, 2, 3, 4, 5, 6, 7)

	apng := gif.as_apng()
	r := Image{}
	r.populate_from_apng(&apng)
	require.Equal(t, disposal(gif), disposal(&r))
}
