package imaging

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func test_image(t *testing.T, img draw.Image) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	t.Run(fmt.Sprintf("%T", img), func(t *testing.T) {
		t.Parallel()
		for y := range h {
			for x := range w {
				img.Set(x, y, color.Opaque)
			}
		}
		require.True(t, IsOpaque(img))
		for y := range h {
			for x := range w {
				img.Set(x, y, color.Transparent)
				require.False(t, IsOpaque(img))
				img.Set(x, y, color.Opaque)
			}
		}
	})
}

func TestIsOpaque(t *testing.T) {
	r := image.Rect(0, 0, 3, 13*runtime.GOMAXPROCS(0))
	w, h := r.Dx(), r.Dy()
	test_image(t, image.NewNRGBA(r))
	test_image(t, image.NewNRGBA64(r))
	test_image(t, image.NewRGBA(r))
	test_image(t, image.NewRGBA64(r))
	t.Run("NYCbCrA", func(t *testing.T) {
		t.Parallel()
		img := image.NewNYCbCrA(r, image.YCbCrSubsampleRatio444)
		for i := range img.A {
			img.A[i] = 0xff
		}
		require.True(t, IsOpaque(img))
		for y := range h {
			for x := range w {
				img.A[y*img.AStride+x] = 13
				require.False(t, IsOpaque(img))
				img.A[y*img.AStride+x] = 0xff
			}
		}

	})

}
