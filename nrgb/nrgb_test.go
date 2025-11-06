package nrgb

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func TestNRGB(t *testing.T) {
	testCases := []struct {
		pix  []uint8
		want []uint8
	}{
		{
			pix:  []uint8{},
			want: []uint8{},
		},
		{
			pix:  []uint8{1, 2, 3},
			want: []uint8{1, 2, 3},
		},
		{
			pix:  []uint8{1, 2, 3, 5, 6, 7},
			want: []uint8{5, 6, 7, 1, 2, 3},
		},
		{
			pix:  []uint8{1, 2, 3, 5, 6, 7, 9, 10, 11},
			want: []uint8{9, 10, 11, 5, 6, 7, 1, 2, 3},
		},
	}
	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			reverse3(tc.pix)
			require.Equal(t, tc.want, tc.pix)
		})
	}
}
