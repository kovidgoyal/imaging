package imaging

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/kovidgoyal/imaging/netpbm"
	"github.com/kovidgoyal/imaging/prism/meta"
	"github.com/kovidgoyal/imaging/prism/meta/autometa"
	"github.com/kovidgoyal/imaging/prism/meta/gifmeta"
	"github.com/kovidgoyal/imaging/prism/meta/tiffmeta"
	"github.com/kovidgoyal/imaging/streams"
	"github.com/kovidgoyal/imaging/types"

	"github.com/kettek/apng"
	"github.com/rwcarlsen/goexif/exif"
	exif_tiff "github.com/rwcarlsen/goexif/tiff"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

type fileSystem interface {
	Create(string) (io.WriteCloser, error)
	Open(string) (io.ReadCloser, error)
}

type localFS struct{}

func (localFS) Create(name string) (io.WriteCloser, error) { return os.Create(name) }
func (localFS) Open(name string) (io.ReadCloser, error)    { return os.Open(name) }

var fs fileSystem = localFS{}

type ColorSpaceType int

const (
	NO_CHANGE_OF_COLORSPACE ColorSpaceType = iota
	SRGB_COLORSPACE
)

type decodeConfig struct {
	autoOrientation  bool
	outputColorspace ColorSpaceType
}

var defaultDecodeConfig = decodeConfig{
	autoOrientation:  true,
	outputColorspace: SRGB_COLORSPACE,
}

// DecodeOption sets an optional parameter for the Decode and Open functions.
type DecodeOption func(*decodeConfig)

// AutoOrientation returns a DecodeOption that sets the auto-orientation mode.
// If auto-orientation is enabled, the image will be transformed after decoding
// according to the EXIF orientation tag (if present). By default it's enabled.
func AutoOrientation(enabled bool) DecodeOption {
	return func(c *decodeConfig) {
		c.autoOrientation = enabled
	}
}

// ColorSpace returns a DecodeOption that sets the colorspace that the
// opened image will be in. Defaults to sRGB. If the image has an embedded ICC
// color profile it is automatically used to convert colors to sRGB if needed.
func ColorSpace(cs ColorSpaceType) DecodeOption {
	return func(c *decodeConfig) {
		c.outputColorspace = cs
	}
}

func fix_colors(images []*Frame, md *meta.Data, cfg *decodeConfig) error {
	var err error
	if md == nil || cfg.outputColorspace != SRGB_COLORSPACE {
		return nil
	}
	if md.CICP.IsSet && !md.CICP.IsSRGB() {
		p := md.CICP.PipelineToSRGB()
		if p == nil {
			return fmt.Errorf("cannot convert colorspace, unknown %s", md.CICP)
		}
		for _, f := range images {
			if f.Image, err = convert(p, f.Image); err != nil {
				return err
			}
		}
		return nil
	}
	profile, err := md.ICCProfile()
	if err != nil {
		return err
	}
	if profile != nil {
		for _, f := range images {
			if f.Image, err = ConvertToSRGB(profile, f.Image); err != nil {
				return err
			}
		}
	}
	return nil
}

func fix_orientation(ans *Image, md *meta.Data, cfg *decodeConfig) error {
	if md == nil || !cfg.autoOrientation {
		return nil
	}
	exif_data, err := md.Exif()
	if err != nil {
		return err
	}
	var oval orientation = orientationUnspecified
	if exif_data != nil {
		orient, err := exif_data.Get(exif.Orientation)
		if err == nil && orient != nil && orient.Format() == exif_tiff.IntVal {
			if x, err := orient.Int(0); err == nil && x > 0 && x < 9 {
				oval = orientation(x)
			}
		}
	}
	if oval != orientationUnspecified {
		for _, img := range ans.Frames {
			img.Image = fixOrientation(img.Image, ans.Metadata, oval)
		}
	}
	return nil

}

type Frame struct {
	Number      uint
	X, Y        int
	Image       image.Image `json:"-"`
	Delay       time.Duration
	ComposeOnto uint
	Replace     bool // Do a simple pixel replacement rather than a full alpha blend when compositing this frame
}

type Image struct {
	Frames       []*Frame
	Metadata     *meta.Data
	LoopCount    uint        // 0 means loop forever, 1 means loop once, ...
	DefaultImage image.Image `json:"-"` // a "default image" for an animation that is not part of the actual animation
}

func format_from_decode_result(x string) Format {
	switch x {
	case "BMP":
		return BMP
	case "TIFF", "TIF":
		return TIFF
	}
	return UNKNOWN
}

func (self *Image) populate_from_apng(p *apng.APNG) {
	self.LoopCount = p.LoopCount
	anchor_frame := uint(1)
	for _, f := range p.Frames {
		if f.IsDefault {
			self.DefaultImage = f.Image
			continue
		}
		n, d := f.DelayNumerator, f.DelayDenominator
		if d <= 0 {
			d = 100
		}
		n = max(0, n)
		frame := Frame{Number: uint(len(self.Frames) + 1), Image: f.Image, X: f.XOffset, Y: f.YOffset,
			Replace: f.BlendOp == apng.BLEND_OP_SOURCE,
			Delay:   time.Duration(float64(time.Second) * float64(n) / float64(d))}
		dp := uint8(gif.DisposalNone)
		switch f.DisposeOp {
		case apng.DISPOSE_OP_BACKGROUND:
			dp = gif.DisposalBackground
		case apng.DISPOSE_OP_NONE:
			dp = gif.DisposalNone
		case apng.DISPOSE_OP_PREVIOUS:
			dp = gif.DisposalPrevious
		}
		anchor_frame, frame.ComposeOnto = gifmeta.SetGIFFrameDisposal(frame.Number, anchor_frame, dp)
		self.Frames = append(self.Frames, &frame)
	}
}

func (self *Image) populate_from_gif(g *gif.GIF) {
	min_gap := gifmeta.CalcMinimumGap(g.Delay)
	anchor_frame := uint(1)
	for i, img := range g.Image {
		frame := Frame{Number: uint(len(self.Frames) + 1), Image: img, X: img.Bounds().Min.X, Y: img.Bounds().Min.Y,
			Delay: gifmeta.CalculateFrameDelay(g.Delay[i], min_gap)}
		anchor_frame, frame.ComposeOnto = gifmeta.SetGIFFrameDisposal(frame.Number, anchor_frame, g.Disposal[i])
		self.Frames = append(self.Frames, &frame)
	}
	switch {
	case g.LoopCount == 0:
		self.LoopCount = 0
	case g.LoopCount < 0:
		self.LoopCount = 1
	default:
		self.LoopCount = uint(g.LoopCount) + 1
	}
}

func decode_all(r io.Reader, opts []DecodeOption) (ans *Image, err error) {
	cfg := defaultDecodeConfig
	for _, option := range opts {
		option(&cfg)
	}

	defer func() {
		if ans != nil && err == nil && ans.Metadata != nil && (cfg.autoOrientation || cfg.outputColorspace != NO_CHANGE_OF_COLORSPACE) {
			if err = fix_colors(ans.Frames, ans.Metadata, &cfg); err != nil {
				return
			}
			if err = fix_orientation(ans, ans.Metadata, &cfg); err != nil {
				return
			}
		}
	}()
	md, r, err := autometa.Load(r)
	if md == nil {
		if err != nil {
			return nil, err
		}
		img, imgf, err := image.Decode(r)
		if err != nil {
			return nil, err
		}
		m := meta.Data{
			PixelWidth:       uint32(img.Bounds().Dx()),
			PixelHeight:      uint32(img.Bounds().Dy()),
			Format:           format_from_decode_result(imgf),
			BitsPerComponent: tiffmeta.BitsPerComponent(img.ColorModel()),
		}
		f := Frame{Image: img}
		return &Image{Metadata: &m, Frames: []*Frame{&f}}, nil
	}
	ans = &Image{Metadata: md}
	if md.HasFrames {
		switch md.Format {
		case GIF:
			g, err := gif.DecodeAll(r)
			if err != nil {
				return nil, err
			}
			ans.populate_from_gif(g)
		case PNG:
			png, err := apng.DecodeAll(r)
			if err != nil {
				return nil, err
			}
			ans.populate_from_apng(&png)
		case WEBP:
			return nil, nil // animated WEBP not currently supported
		}
	} else {
		img, _, err := image.Decode(r)
		if err != nil {
			return nil, err
		}
		ans.Metadata.PixelWidth = uint32(img.Bounds().Dx())
		ans.Metadata.PixelHeight = uint32(img.Bounds().Dy())
		ans.Frames = append(ans.Frames, &Frame{Image: img})
	}
	return
}

// Decode image from r including all animation frames if its an animated image.
// Returns nil with no error when no supported image is found in r.
// Also returns a reader that will yield all bytes from r so that this API does
// not exhaust r.
func DecodeAll(r io.Reader, opts ...DecodeOption) (ans *Image, s io.Reader, err error) {
	s, err = streams.CallbackWithSeekable(r, func(r io.Reader) (err error) {
		ans, err = decode_all(r, opts)
		return
	})
	return
}

// Decode reads an image from r.
func Decode(r io.Reader, opts ...DecodeOption) (image.Image, error) {
	cfg := defaultDecodeConfig

	for _, option := range opts {
		option(&cfg)
	}

	if !cfg.autoOrientation && cfg.outputColorspace == NO_CHANGE_OF_COLORSPACE {
		img, _, err := image.Decode(r)
		return img, err
	}
	ans, err := decode_all(r, opts)
	if err != nil {
		return nil, err
	}
	if ans == nil {
		return nil, fmt.Errorf("unrecognised image format")
	}
	if ans.DefaultImage != nil {
		return ans.DefaultImage, nil
	}
	return ans.Frames[0].Image, nil
}

// Open loads an image from file.
//
// Examples:
//
//	// Load an image from file.
//	img, err := imaging.Open("test.jpg")
func Open(filename string, opts ...DecodeOption) (image.Image, error) {
	file, err := fs.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return Decode(file, opts...)
}

func OpenAll(filename string, opts ...DecodeOption) (*Image, error) {
	file, err := fs.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	ans, _, err := DecodeAll(file, opts...)
	return ans, err
}

func OpenConfig(filename string) (ans image.Config, format_name string, err error) {
	file, err := fs.Open(filename)
	if err != nil {
		return ans, "", err
	}
	defer file.Close()
	return image.DecodeConfig(file)
}

type Format = types.Format

const (
	UNKNOWN = types.UNKNOWN
	JPEG    = types.JPEG
	PNG     = types.PNG
	GIF     = types.GIF
	TIFF    = types.TIFF
	WEBP    = types.WEBP
	BMP     = types.BMP
	PBM     = types.PBM
	PGM     = types.PGM
	PPM     = types.PPM
	PAM     = types.PAM
)

// ErrUnsupportedFormat means the given image format is not supported.
var ErrUnsupportedFormat = errors.New("imaging: unsupported image format")

// FormatFromExtension parses image format from filename extension:
// "jpg" (or "jpeg"), "png", "gif", "tif" (or "tiff") and "bmp" are supported.
func FormatFromExtension(ext string) (Format, error) {
	if f, ok := types.FormatExts[strings.ToLower(strings.TrimPrefix(ext, "."))]; ok {
		return f, nil
	}
	return -1, ErrUnsupportedFormat
}

// FormatFromFilename parses image format from filename:
// "jpg" (or "jpeg"), "png", "gif", "tif" (or "tiff") and "bmp" are supported.
func FormatFromFilename(filename string) (Format, error) {
	ext := filepath.Ext(filename)
	return FormatFromExtension(ext)
}

type encodeConfig struct {
	jpegQuality         int
	gifNumColors        int
	gifQuantizer        draw.Quantizer
	gifDrawer           draw.Drawer
	pngCompressionLevel png.CompressionLevel
}

var defaultEncodeConfig = encodeConfig{
	jpegQuality:         95,
	gifNumColors:        256,
	gifQuantizer:        nil,
	gifDrawer:           nil,
	pngCompressionLevel: png.DefaultCompression,
}

// EncodeOption sets an optional parameter for the Encode and Save functions.
type EncodeOption func(*encodeConfig)

// JPEGQuality returns an EncodeOption that sets the output JPEG quality.
// Quality ranges from 1 to 100 inclusive, higher is better. Default is 95.
func JPEGQuality(quality int) EncodeOption {
	return func(c *encodeConfig) {
		c.jpegQuality = quality
	}
}

// GIFNumColors returns an EncodeOption that sets the maximum number of colors
// used in the GIF-encoded image. It ranges from 1 to 256.  Default is 256.
func GIFNumColors(numColors int) EncodeOption {
	return func(c *encodeConfig) {
		c.gifNumColors = numColors
	}
}

// GIFQuantizer returns an EncodeOption that sets the quantizer that is used to produce
// a palette of the GIF-encoded image.
func GIFQuantizer(quantizer draw.Quantizer) EncodeOption {
	return func(c *encodeConfig) {
		c.gifQuantizer = quantizer
	}
}

// GIFDrawer returns an EncodeOption that sets the drawer that is used to convert
// the source image to the desired palette of the GIF-encoded image.
func GIFDrawer(drawer draw.Drawer) EncodeOption {
	return func(c *encodeConfig) {
		c.gifDrawer = drawer
	}
}

// PNGCompressionLevel returns an EncodeOption that sets the compression level
// of the PNG-encoded image. Default is png.DefaultCompression.
func PNGCompressionLevel(level png.CompressionLevel) EncodeOption {
	return func(c *encodeConfig) {
		c.pngCompressionLevel = level
	}
}

// Encode writes the image img to w in the specified format (JPEG, PNG, GIF, TIFF or BMP).
func Encode(w io.Writer, img image.Image, format Format, opts ...EncodeOption) error {
	cfg := defaultEncodeConfig
	for _, option := range opts {
		option(&cfg)
	}

	switch format {
	case JPEG:
		if nrgba, ok := img.(*image.NRGBA); ok && nrgba.Opaque() {
			rgba := &image.RGBA{
				Pix:    nrgba.Pix,
				Stride: nrgba.Stride,
				Rect:   nrgba.Rect,
			}
			return jpeg.Encode(w, rgba, &jpeg.Options{Quality: cfg.jpegQuality})
		}
		return jpeg.Encode(w, img, &jpeg.Options{Quality: cfg.jpegQuality})

	case PNG:
		encoder := png.Encoder{CompressionLevel: cfg.pngCompressionLevel}
		return encoder.Encode(w, img)

	case GIF:
		return gif.Encode(w, img, &gif.Options{
			NumColors: cfg.gifNumColors,
			Quantizer: cfg.gifQuantizer,
			Drawer:    cfg.gifDrawer,
		})

	case TIFF:
		return tiff.Encode(w, img, &tiff.Options{Compression: tiff.Deflate, Predictor: true})

	case BMP:
		return bmp.Encode(w, img)
	}

	return ErrUnsupportedFormat
}

// Save saves the image to file with the specified filename.
// The format is determined from the filename extension:
// "jpg" (or "jpeg"), "png", "gif", "tif" (or "tiff") and "bmp" are supported.
//
// Examples:
//
//	// Save the image as PNG.
//	err := imaging.Save(img, "out.png")
//
//	// Save the image as JPEG with optional quality parameter set to 80.
//	err := imaging.Save(img, "out.jpg", imaging.JPEGQuality(80))
func Save(img image.Image, filename string, opts ...EncodeOption) (err error) {
	f, err := FormatFromFilename(filename)
	if err != nil {
		return err
	}
	file, err := fs.Create(filename)
	if err != nil {
		return err
	}
	err = Encode(file, img, f, opts...)
	errc := file.Close()
	if err == nil {
		err = errc
	}
	return err
}

// orientation is an EXIF flag that specifies the transformation
// that should be applied to image to display it correctly.
type orientation int

const (
	orientationUnspecified = 0
	orientationNormal      = 1
	orientationFlipH       = 2
	orientationRotate180   = 3
	orientationFlipV       = 4
	orientationTranspose   = 5
	orientationRotate270   = 6
	orientationTransverse  = 7
	orientationRotate90    = 8
)

// fixOrientation applies a transform to img corresponding to the given orientation flag.
func fixOrientation(img image.Image, md *meta.Data, o orientation) image.Image {
	switch o {
	case orientationNormal:
	case orientationFlipH:
		img = FlipH(img)
	case orientationFlipV:
		img = FlipV(img)
	case orientationRotate90:
		img = Rotate90(img)
		if md != nil {
			md.PixelWidth, md.PixelHeight = md.PixelHeight, md.PixelWidth
		}
	case orientationRotate180:
		img = Rotate180(img)
	case orientationRotate270:
		img = Rotate270(img)
		if md != nil {
			md.PixelWidth, md.PixelHeight = md.PixelHeight, md.PixelWidth
		}
	case orientationTranspose:
		img = Transpose(img)
		if md != nil {
			md.PixelWidth, md.PixelHeight = md.PixelHeight, md.PixelWidth
		}
	case orientationTransverse:
		img = Transverse(img)
		if md != nil {
			md.PixelWidth, md.PixelHeight = md.PixelHeight, md.PixelWidth
		}
	}
	return img
}
