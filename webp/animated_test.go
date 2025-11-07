package webp

import (
	"bytes"
	"os"
	"testing"
)

func TestDecodeAnimatedLossy(t *testing.T) {
	type testCase struct {
		width     int
		height    int
		numFrames int
		file      string
	}
	testCases := []testCase{
		{
			width:     250,
			height:    260,
			numFrames: 4,
			file:      "../testdata/animated-webp-lossless.webp",
		},
		{
			width:     128,
			height:    128,
			numFrames: 28,
			file:      "../testdata/animated-webp-lossy.webp",
		},
	}
	for _, tc := range testCases {
		webpData, err := os.ReadFile(tc.file)
		if err != nil {
			t.Error(err.Error())
			t.FailNow()
		}

		anim, err := DecodeAnimated(bytes.NewReader(webpData))
		if err != nil {
			t.Errorf("%s got unexpected error: %s", tc.file, err.Error())
			t.FailNow()
		}
		if len(anim.Frames) != tc.numFrames {
			t.Errorf("%s expected %d frames, but got %d", tc.file, tc.numFrames, len(anim.Frames))
		}
		if anim.Config.Width != tc.width {
			t.Errorf("%s expected an image width of %d, but got %d", tc.file, tc.width, anim.Config.Width)
		}
		if anim.Config.Height != tc.height {
			t.Errorf("%s expected an image width of %d, but got %d", tc.file, tc.height, anim.Config.Height)
		}
	}
}
