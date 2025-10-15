package prism

import (
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
	defer xyz.Close()
	lab, err := CreateCMSProfile(icc.Srgb_lab_profile_data)
	require.NoError(t, err)
	defer lab.Close()
	_, err = CreateCMSProfile([]byte("invalid profile"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Got 15 bytes, block should be")
}
