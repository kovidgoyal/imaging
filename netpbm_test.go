package imaging

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"testing"
)

var _ = fmt.Print

func eq(a, b uint32) bool {
	// we allow a difference of 1 to accomodate different rounding algorithms
	return a == b || a+1 == b || b+1 == a
}

func color_equal(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return eq(ar, br) && eq(ag, bg) && eq(ab, bb) && eq(aa, ba)
}

func ensure_images_are_equal(img1, img2 image.Image) error {
	b1 := img1.Bounds()
	b2 := img2.Bounds()
	if !b1.Eq(b2) {
		return fmt.Errorf("image sizes are not equal: %v != %v", b1, b2)
	}
	for y := b1.Min.Y; y < b1.Max.Y; y++ {
		for x := b1.Min.X; x < b1.Max.X; x++ {
			if a, b := img1.At(x, y), img2.At(x, y); !color_equal(a, b) {
				ar, ag, ab, aa := a.RGBA()
				br, bg, bb, ba := b.RGBA()
				return fmt.Errorf("image pixels at %dx%d are not equal: %T{%x, %x, %x, %x} != %T{%x, %x, %x, %x}",
					x, y, a, ar, ab, ag, aa, b, br, bb, bg, ba)
			}
		}
	}
	return nil
}

func open(path string) (image.Image, error) {
	file, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return DecodeNetPBM(file)
}

func png_open(path string) (image.Image, error) {
	file, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return png.Decode(file)
}

func open_config(path string) (image.Config, error) {
	file, err := fs.Open(path)
	if err != nil {
		return image.Config{}, err
	}
	defer file.Close()
	return DecodeNetPBMConfig(file)
}

func TestNetPBM(t *testing.T) {
	files := make([]string, 0, 16)
	for _, ext := range []string{"pbm", "pbm", "ppm", "pam"} {
		pbm, err := filepath.Glob("testdata/*" + ext)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, pbm...)
	}
	for _, f := range files {
		a, err := open(f)
		if err != nil {
			t.Fatal(fmt.Errorf("%s: %w", f, err))
		}
		c, err := open_config(f)
		if err != nil {
			t.Fatal(fmt.Errorf("%s: %w", f, err))
		}
		if c.Width != a.Bounds().Dx() || c.Height != a.Bounds().Dy() || c.ColorModel != a.ColorModel() {
			t.Fatalf("%s: DecodeConfig and Decode disagree: %v != %v %v", f, c, a.Bounds(), a.ColorModel())
		}
		b, err := png_open(f + ".png")
		if err != nil {
			t.Fatal(fmt.Errorf("%s.png: %w", f, err))
		}
		if err = ensure_images_are_equal(b, a); err != nil {
			t.Fatal(fmt.Errorf("%s: %w", f, err))
		}
	}
}
