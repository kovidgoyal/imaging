/*
Package imaging provides basic image processing functions (resize, rotate, crop, brightness/contrast adjustments, etc.).

All the image processing functions provided by the package accept any image type that implements image.Image interface
as an input, and return a new image of *image.NRGBA type (32bit RGBA colors, non-premultiplied alpha).
*/
package imaging

import "fmt"

type ImagingVersion struct {
	Major, Minor, Patch uint
}

func (v ImagingVersion) String(o ImagingVersion) string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v ImagingVersion) Equal(o ImagingVersion) bool {
	return v.Major == o.Major && v.Minor == o.Minor && v.Patch == o.Patch
}

func (v ImagingVersion) After(o ImagingVersion) bool {
	switch {
	case v.Major == o.Major:
		switch {
		case v.Minor == o.Minor:
			return v.Patch > o.Patch
		case v.Minor > o.Minor:
			return true
		case v.Minor < o.Minor:
			return false
		}
	case v.Major > o.Major:
		return true
	case v.Major < o.Major:
		return false
	}
	return false
}

func (v ImagingVersion) Before(o ImagingVersion) bool {
	return !v.Equal(o) && !v.After(o)
}

var Version = ImagingVersion{1, 7, 1}
