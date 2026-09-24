package imaging

import (
	"fmt"
	"image"

	"github.com/kovidgoyal/imaging/prism/meta/icc"
)

var _ = fmt.Print

// Convert to SRGB based on the supplied ICC color profile. The result
// may be either the original image unmodified if no color
// conversion was needed, the original image modified, or a new image (when the original image
// is not in a supported format).
func ConvertToSRGB(p *icc.Profile, intent icc.RenderingIntent, use_blackpoint_compensation bool, image_any image.Image) (ans image.Image, err error) {
	if p.IsSRGB() {
		return image_any, nil
	}
	num_channels := 3
	switch image_any.(type) {
	case *image.CMYK:
		num_channels = 4
	case *image.Gray:
		num_channels = 1
	}
	tr, err := p.CreateTransformerToSRGB(intent, use_blackpoint_compensation, num_channels, true, true, true)
	if err != nil {
		return nil, err
	}
	return convert(tr, image_any)
}
