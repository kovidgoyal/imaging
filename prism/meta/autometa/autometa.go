package autometa

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/kovidgoyal/imaging/prism/meta"
	"github.com/kovidgoyal/imaging/prism/meta/binary"
	"github.com/kovidgoyal/imaging/prism/meta/jpegmeta"
	"github.com/kovidgoyal/imaging/prism/meta/pngmeta"
	"github.com/kovidgoyal/imaging/prism/meta/webpmeta"
)

func load_with_seekable(r io.Reader, callback func(binary.Reader) error) (stream io.Reader, err error) {
	if s, ok := r.(io.ReadSeeker); ok {
		pos, err := s.Seek(0, io.SeekCurrent)
		if err == nil {
			defer func() {
				_, serr := s.Seek(pos, io.SeekStart)
				if err == nil {
					err = serr
				}
			}()
			err = callback(bufio.NewReader(s))
			return s, err
		}
	}
	rewindBuffer := &bytes.Buffer{}
	tee := io.TeeReader(r, rewindBuffer)
	err = callback(bufio.NewReader(tee))
	return io.MultiReader(rewindBuffer, r), err
}

// Load loads the metadata for an image stream, which may be one of the
// supported image formats.
//
// Only as much of the stream is consumed as necessary to extract the metadata;
// the returned stream contains a buffered copy of the consumed data such that
// reading from it will produce the same results as fully reading the input
// stream. This provides a convenient way to load the full image after loading
// the metadata.
//
// An error is returned if basic metadata could not be extracted. The returned
// stream still provides the full image data.
func Load(r io.Reader) (md *meta.Data, imgStream io.Reader, err error) {
	loaders := []func(binary.Reader) (*meta.Data, error){
		pngmeta.ExtractMetadata,
		jpegmeta.ExtractMetadata,
		webpmeta.ExtractMetadata,
	}
	for _, loader := range loaders {
		r, err = load_with_seekable(r, func(r binary.Reader) (err error) {
			md, err = loader(r)
			return
		})
		if err == nil {
			return md, r, nil
		}
	}
	return nil, r, fmt.Errorf("unrecognised image format")
}
