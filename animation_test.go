package imaging

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

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
		require.Equal(t, gf.ComposeOnto, pf.ComposeOnto, fmt.Sprintf("frame number: %d", gf.Number))
	}

}
