package tiffmeta

import (
	"fmt"
	"image/color"
	"io"

	"github.com/kovidgoyal/imaging/prism/meta"
	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/tiff"
)

var _ = fmt.Print

func ExtractMetadata(r_ io.Reader) (md *meta.Data, err error) {
	r := r_.(io.ReadSeeker)
	pos, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	c, err := tiff.DecodeConfig(r)
	if err != nil {
		return nil, err
	}
	md = &meta.Data{
		Format: meta.ImageFormat("TIFF"), PixelWidth: uint32(c.Width), PixelHeight: uint32(c.Height),
	}
	switch c.ColorModel {
	case color.RGBAModel, color.NRGBAModel, color.YCbCrModel, color.CMYKModel:
		md.BitsPerComponent = 8
	case color.GrayModel:
		md.BitsPerComponent = 8
	case color.Gray16Model:
		md.BitsPerComponent = 16
	case color.AlphaModel:
		md.BitsPerComponent = 8
	case color.Alpha16Model:
		md.BitsPerComponent = 16
	default:
		// This handles paletted images and other custom color models.
		// For a palette, each color in the palette has its own depth.
		// We can check the bit depth by converting a color from the model to RGBA.
		// The `Convert` method is part of the color.Model interface.
		// A fully opaque red color is used for this check.
		r, g, b, a := c.ColorModel.Convert(color.RGBA{R: 255, A: 255}).RGBA()

		// The values returned by RGBA() are 16-bit alpha-premultiplied values (0-65535).
		// If the highest value is <= 255, it's an 8-bit model.
		if r|g|b|a <= 0xff {
			md.BitsPerComponent = 8
		} else {
			md.BitsPerComponent = 16
		}
	}
	if _, err = r.Seek(pos, io.SeekStart); err != nil {
		return nil, err
	}
	if e, err := exif.Decode(r); err == nil {
		md.SetExif(e)
	} else {
		md.SetExifError(err)
	}
	return md, nil
}
