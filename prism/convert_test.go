//go:build lcms2cgo

package prism

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/kovidgoyal/imaging/prism/meta/icc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func TestCGOConversion(t *testing.T) {
	xyz, err := CreateCMSProfile(icc.Srgb_xyz_profile_data)
	require.NoError(t, err)
	require.Equal(t, xyz.DeviceColorSpace, icc.RGBSignature)
	require.Equal(t, xyz.PCSColorSpace, icc.XYZSignature)
	defer xyz.Close()
	lab, err := CreateCMSProfile(icc.Srgb_lab_profile_data)
	require.Equal(t, lab.DeviceColorSpace, icc.RGBSignature)
	require.Equal(t, lab.PCSColorSpace, icc.LabSignature)
	require.NoError(t, err)
	defer lab.Close()
	_, err = CreateCMSProfile([]byte("invalid profile"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Got 15 bytes, block should be")

	pcs, err := xyz.TransformRGB8bitToPCS([]byte{128, 64, 255}, icc.RelativeColorimetricRenderingIntent)
	require.NoError(t, err)
	assert.InDeltaSlice(t, pcs, []float64{0.2569, 0.1454, 0.7221}, 0.001)
	pcs, err = lab.TransformRGB8bitToPCS([]byte{128, 64, 255}, icc.RelativeColorimetricRenderingIntent)
	require.NoError(t, err)
	assert.InDeltaSlice(t, pcs, []float64{45.2933, 58.3075, -85.6426}, 0.001)

	for _, p := range []*CMSProfile{xyz, lab} {
		inp := []byte{128, 64, 255}
		out, err := p.TransformRGB8(inp, p, icc.RelativeColorimetricRenderingIntent)
		require.NoError(t, err)
		assert.Equal(t, inp, out)
	}
}

func TestAgainstLCMS2(t *testing.T) {
	p, err := icc.NewProfileReader(bytes.NewReader(icc.Srgb_xyz_profile_data)).ReadProfile()
	if err != nil {
		t.Fatal(err)
	}
	println(p.Header.String())
	p.CreateTransformerToPCS(icc.PerceptualRenderingIntent)
	println(p.Header.String())
	p, err = icc.NewProfileReader(bytes.NewReader(icc.Srgb_lab_profile_data)).ReadProfile()
	if err != nil {
		t.Fatal(err)
	}
	p.CreateTransformerToPCS(icc.PerceptualRenderingIntent)
}
