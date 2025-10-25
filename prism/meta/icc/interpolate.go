package icc

import (
	"fmt"
	"math"
)

var _ = fmt.Print

func sampled_value(samples []unit_float, max_idx unit_float, x unit_float) unit_float {
	idx := clamp01(x) * max_idx
	lof := unit_float(math.Trunc(float64(idx)))
	lo := int(lof)
	if lof == idx {
		return samples[lo]
	}
	p := idx - unit_float(lo)
	vhi := unit_float(samples[lo+1])
	vlo := unit_float(samples[lo])
	return vlo + p*(vhi-vlo)
}

// Performs an n-linear interpolation on the CLUT values for the given input color using an iterative method.
// Input values should be normalized between 0.0 and 1.0. Output MUST be zero initialized.
func trilinear_interpolate(input, values, output []unit_float, input_channels, output_channels int, grid_points []int) {
	// Pre-allocate slices for indices and weights
	var buf [4]int
	var wbuf [4]unit_float
	indices := buf[:input_channels]
	weights := wbuf[:input_channels]
	input = input[:input_channels]
	output = output[:output_channels]

	// Calculate the base indices and interpolation weights for each dimension.
	for i, val := range input {
		gp := grid_points[i]
		val = clamp01(val)
		// Scale the value to the grid dimensions
		pos := val * unit_float(gp-1)
		// The base index is the floor of the position.
		idx := int(pos)
		// The weight is the fractional part of the position.
		weight := pos - unit_float(idx)
		// Clamp index to be at most the second to last grid point.
		if idx >= gp-1 {
			idx = gp - 2
			weight = 1 // set weight to 1 for border index
		}
		indices[i] = idx
		weights[i] = weight
	}
	// Iterate through all 2^InputChannels corners of the n-dimensional hypercube
	for i := range 1 << input_channels {
		// Calculate the combined weight for this corner
		cornerWeight := unit_float(1)
		// Calculate the N-dimensional index to look up in the table
		tableIndex := 0
		multiplier := unit_float(1)

		for j := input_channels - 1; j >= 0; j-- {
			// Check the j-th bit of i to decide if we are at the lower or upper bound for this dimension
			if (i>>j)&1 == 1 {
				// Upper bound for this dimension
				cornerWeight *= weights[j]
				tableIndex += int(unit_float(indices[j]+1) * multiplier)
			} else {
				// Lower bound for this dimension
				cornerWeight *= (1.0 - weights[j])
				tableIndex += int(unit_float(indices[j]) * multiplier)
			}
			multiplier *= unit_float(grid_points[j])
		}
		// Get the color value from the table for the current corner
		offset := tableIndex * output_channels
		// Add the weighted corner color to the output
		for k, v := range values[offset : offset+output_channels] {
			output[k] += v * cornerWeight
		}
	}
}
