package gifmeta

import (
	"fmt"
	"image/gif"
	"io"

	"github.com/kovidgoyal/imaging/prism/meta"
	"github.com/kovidgoyal/imaging/types"
)

var _ = fmt.Print

func ExtractMetadata(r io.Reader) (md *meta.Data, err error) {
	c, err := gif.DecodeConfig(r)
	if err != nil {
		return nil, err
	}
	md = &meta.Data{
		Format: types.GIF, PixelWidth: uint32(c.Width), PixelHeight: uint32(c.Height),
		BitsPerComponent: 8, HasFrames: true,
	}
	return md, nil
}
