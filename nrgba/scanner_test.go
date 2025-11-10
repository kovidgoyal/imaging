package nrgba

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

func TestReverse(t *testing.T) {
	testCases := []struct {
		pix  []uint8
		want []uint8
	}{
		{
			pix:  []uint8{},
			want: []uint8{},
		},
		{
			pix:  []uint8{1, 2, 3, 4},
			want: []uint8{1, 2, 3, 4},
		},
		{
			pix:  []uint8{1, 2, 3, 4, 5, 6, 7, 8},
			want: []uint8{5, 6, 7, 8, 1, 2, 3, 4},
		},
		{
			pix:  []uint8{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			want: []uint8{9, 10, 11, 12, 5, 6, 7, 8, 1, 2, 3, 4},
		},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			reverse4(tc.pix)
			require.Equal(t, tc.want, tc.pix)
		})
	}
}
