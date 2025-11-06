package imaging

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	errCreate = errors.New("failed to create file")
	errClose  = errors.New("failed to close file")
	errOpen   = errors.New("failed to open file")
)

type badFS struct{}

func (badFS) Create(name string) (io.WriteCloser, error) {
	if name == "badFile.jpg" {
		return badFile{io.Discard}, nil
	}
	return nil, errCreate
}

func (badFS) Open(name string) (io.ReadCloser, error) {
	return nil, errOpen
}

type badFile struct {
	io.Writer
}

func (badFile) Close() error {
	return errClose
}

type quantizer struct {
	palette []color.Color
}

func (q quantizer) Quantize(p color.Palette, m image.Image) color.Palette {
	pal := make([]color.Color, len(p), cap(p))
	copy(pal, p)
	n := min(cap(p)-len(p), len(q.palette))
	for i := range n {
		pal = append(pal, q.palette[i])
	}
	return pal
}

func TestOpenSave(t *testing.T) {
	imgWithoutAlpha := image.NewNRGBA(image.Rect(0, 0, 4, 6))
	imgWithoutAlpha.Pix = []uint8{
		0x00, 0x00, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0x00, 0x00, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0x00, 0x00, 0xff, 0xff, 0x00, 0x00, 0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0xff,
		0xff, 0x00, 0x00, 0xff, 0xff, 0x00, 0x00, 0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0xff,
		0x00, 0x00, 0xff, 0xff, 0x00, 0x00, 0xff, 0xff, 0x88, 0x88, 0x88, 0xff, 0x88, 0x88, 0x88, 0xff,
		0x00, 0x00, 0xff, 0xff, 0x00, 0x00, 0xff, 0xff, 0x88, 0x88, 0x88, 0xff, 0x88, 0x88, 0x88, 0xff,
	}
	imgWithAlpha := image.NewNRGBA(image.Rect(0, 0, 4, 6))
	imgWithAlpha.Pix = []uint8{
		0x00, 0x00, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0x00, 0x00, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0x00, 0x00, 0x80, 0xff, 0x00, 0x00, 0x80, 0x00, 0xff, 0x00, 0x80, 0x00, 0xff, 0x00, 0x80,
		0xff, 0x00, 0x00, 0x80, 0xff, 0x00, 0x00, 0x80, 0x00, 0xff, 0x00, 0x80, 0x00, 0xff, 0x00, 0x80,
		0x00, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0x00, 0x88, 0x88, 0x88, 0x00, 0x88, 0x88, 0x88, 0x00,
		0x00, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0x00, 0x88, 0x88, 0x88, 0x00, 0x88, 0x88, 0x88, 0x00,
	}

	options := [][]EncodeOption{
		{
			JPEGQuality(100),
		},
		{
			JPEGQuality(99),
			GIFDrawer(draw.FloydSteinberg),
			GIFNumColors(256),
			GIFQuantizer(quantizer{palette.Plan9}),
			PNGCompressionLevel(png.BestSpeed),
		},
	}

	dir, err := os.MkdirTemp("", "imaging")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(dir)

	for _, ext := range []string{"jpg", "jpeg", "png", "gif", "bmp", "tif", "tiff"} {
		filename := filepath.Join(dir, "test."+ext)

		img := imgWithoutAlpha
		if ext == "png" {
			img = imgWithAlpha
		}

		for _, opts := range options {
			err := Save(img, filename, opts...)
			if err != nil {
				t.Fatalf("failed to save image (%q): %v", filename, err)
			}

			img2, err := Open(filename)
			if err != nil {
				t.Fatalf("failed to open image (%q): %v", filename, err)
			}
			got := Clone(img2)

			delta := 0
			if ext == "jpg" || ext == "jpeg" || ext == "gif" {
				delta = 3
			}

			if !compareNRGBA(got, img, delta) {
				t.Fatalf("bad encode-decode result (ext=%q): got %#v want %#v", ext, got, img)
			}
		}
	}

	buf := &bytes.Buffer{}
	err = Encode(buf, imgWithAlpha, JPEG)
	if err != nil {
		t.Fatalf("failed to encode alpha to JPEG: %v", err)
	}

	buf = &bytes.Buffer{}
	err = Encode(buf, imgWithAlpha, Format(100))
	if err != ErrUnsupportedFormat {
		t.Fatalf("got %v want ErrUnsupportedFormat", err)
	}

	buf = bytes.NewBuffer([]byte("bad data"))
	_, err = Decode(buf)
	if err == nil {
		t.Fatalf("decoding bad data: expected error got nil")
	}

	err = Save(imgWithAlpha, filepath.Join(dir, "test.unknown"))
	if err != ErrUnsupportedFormat {
		t.Fatalf("got %v want ErrUnsupportedFormat", err)
	}

	prevFS := fs
	fs = badFS{}
	defer func() { fs = prevFS }()

	err = Save(imgWithAlpha, "test.jpg")
	if err != errCreate {
		t.Fatalf("got error %v want errCreate", err)
	}

	err = Save(imgWithAlpha, "badFile.jpg")
	if err != errClose {
		t.Fatalf("got error %v want errClose", err)
	}

	_, err = Open("test.jpg")
	if err != errOpen {
		t.Fatalf("got error %v want errOpen", err)
	}
}

func TestFormats(t *testing.T) {
	formatNames := map[Format]string{
		JPEG:       "JPEG",
		PNG:        "PNG",
		GIF:        "GIF",
		BMP:        "BMP",
		TIFF:       "TIFF",
		Format(-1): "",
	}
	for format, name := range formatNames {
		got := format.String()
		if got != name {
			t.Fatalf("got format name %q want %q", got, name)
		}
	}
}

func TestFormatFromExtension(t *testing.T) {
	testCases := []struct {
		name string
		ext  string
		want Format
		err  error
	}{
		{
			name: "jpg without leading dot",
			ext:  "jpg",
			want: JPEG,
		},
		{
			name: "jpg with leading dot",
			ext:  ".jpg",
			want: JPEG,
		},
		{
			name: "jpg uppercase",
			ext:  ".JPG",
			want: JPEG,
		},
		{
			name: "unsupported",
			ext:  ".unsupportedextension",
			want: -1,
			err:  ErrUnsupportedFormat,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FormatFromExtension(tc.ext)
			if err != tc.err {
				t.Errorf("got error %#v want %#v", err, tc.err)
			}
			if got != tc.want {
				t.Errorf("got result %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestAutoOrientation(t *testing.T) {
	toBW := func(img image.Image) []byte {
		b := img.Bounds()
		data := make([]byte, 0, b.Dx()*b.Dy())
		for x := b.Min.X; x < b.Max.X; x++ {
			for y := b.Min.Y; y < b.Max.Y; y++ {
				c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
				if c.Y < 128 {
					data = append(data, 1)
				} else {
					data = append(data, 0)
				}
			}
		}
		return data
	}

	f, err := os.Open("testdata/orientation_0.jpg")
	if err != nil {
		t.Fatalf("os.Open(%q): %v", "testdata/orientation_0.jpg", err)
	}
	orig, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("image.Decode(%q): %v", "testdata/orientation_0.jpg", err)
	}
	origBW := toBW(orig)

	testCases := []struct {
		path string
	}{
		{"testdata/orientation_0.jpg"},
		{"testdata/orientation_1.jpg"},
		{"testdata/orientation_2.jpg"},
		{"testdata/orientation_3.jpg"},
		{"testdata/orientation_4.jpg"},
		{"testdata/orientation_5.jpg"},
		{"testdata/orientation_6.jpg"},
		{"testdata/orientation_7.jpg"},
		{"testdata/orientation_8.jpg"},
	}
	for _, tc := range testCases {
		img, err := OpenAll(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if img.Frames[0].Image.Bounds() != orig.Bounds() {
			t.Fatalf("%s: got bounds %v want %v", tc.path, img.Frames[0].Image.Bounds(), orig.Bounds())
		}
		imgBW := toBW(img.Frames[0].Image)
		if !bytes.Equal(imgBW, origBW) {
			t.Fatalf("%s: got bw data %v want %v", tc.path, imgBW, origBW)
		}
		require.Equal(t, orig.Bounds().Dx(), int(img.Metadata.PixelWidth))
		require.Equal(t, orig.Bounds().Dy(), int(img.Metadata.PixelHeight))
	}

	if _, err := Decode(strings.NewReader("invalid data"), AutoOrientation(true)); err == nil {
		t.Fatal("expected error got nil")
	}
}
