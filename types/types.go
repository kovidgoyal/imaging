package types

import (
	"fmt"
)

var _ = fmt.Print

// Format is an image file format.
type Format int

// Image file formats.
const (
	UNKNOWN Format = iota
	JPEG
	PNG
	GIF
	TIFF
	WEBP
	BMP
	PBM
	PGM
	PPM
	PAM
)

var FormatExts = map[string]Format{
	"jpg":  JPEG,
	"jpeg": JPEG,
	"png":  PNG,
	"gif":  GIF,
	"tif":  TIFF,
	"tiff": TIFF,
	"webp": WEBP,
	"bmp":  BMP,
	"pbm":  PBM,
	"pgm":  PGM,
	"ppm":  PPM,
	"pam":  PAM,
}

var formatNames = map[Format]string{
	JPEG: "JPEG",
	PNG:  "PNG",
	GIF:  "GIF",
	TIFF: "TIFF",
	WEBP: "WEBP",
	BMP:  "BMP",
	PBM:  "PBM",
	PGM:  "PGM",
	PPM:  "PPM",
	PAM:  "PAM",
}

func (f Format) String() string {
	return formatNames[f]
}
