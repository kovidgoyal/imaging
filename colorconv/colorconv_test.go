package colorconv

import (
	"math"
	"testing"
)

func nearlyEqual(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}

var tableCases = []struct {
	name    string
	L       float64
	a       float64
	b       float64
	R, G, B float64 // used for regression testing
}{
	{"neutral gray", 50, 0, 0, 0.466327138113, 0.466343141295, 0.466290642380},
	{"vivid warm", 60, 80, 60, 0.999999988221, 0.309592440025, 0.239353320225},
	{"vivid cyan-ish", 75, -70, 70, 0.000000000000, 0.840464140658, 0.040384236879},
	{"light slightly red", 90, 30, 0, 0.999999987494, 0.848384781219, 0.890299422531},
	{"dark saturated green-blue", 20, 80, -60, 0.454179083596, 0.000000000000, 0.550682636720},
	{"intentionally out-of-gamut 1", 50, 120, 120, 0.999999968322, 0.000000000000, 0.000000000000},
	{"intentionally out-of-gamut 2", 80, -150, 50, 0.000000000000, 0.978934883152, 0.355515659030},
	{"very dark saturated", 5, 60, -40, 0.250536419368, 0.000000000000, 0.282271881291},
	{"very bright saturated", 99, -80, 90, 0.963161171684, 0.999999995817, 0.944239594830},
}

func TestPathConsistency_TableDriven(t *testing.T) {
	// Verify consistency between Lab->SRGB and XYZ->SRGB paths (both no-gamut and with gamut mapping)
	eps := 1e-9

	for _, tc := range tableCases {
		t.Run(tc.name+"/NoGamut_NoClamp", func(t *testing.T) {
			X, Y, Z := labToXYZ_D50(tc.L, tc.a, tc.b)

			// via Lab path (no gamut map, companded, no clamp)
			rLab, gLab, bLab := labToSRGBNoGamutMap(tc.L, tc.a, tc.b)

			// via XYZ direct (no clamp)
			rXYZ, gXYZ, bXYZ := XYZToSRGB_D50NoClamp(X, Y, Z)

			if !nearlyEqual(rLab, rXYZ, eps) || !nearlyEqual(gLab, gXYZ, eps) || !nearlyEqual(bLab, bXYZ, eps) {
				t.Fatalf("No-gamut mismatch for %s: labPath=(%.12f,%.12f,%.12f) xyzPath=(%.12f,%.12f,%.12f)",
					tc.name, rLab, gLab, bLab, rXYZ, gXYZ, bXYZ)
			}
		})

		t.Run(tc.name+"/GamutMapped", func(t *testing.T) {
			X, Y, Z := labToXYZ_D50(tc.L, tc.a, tc.b)

			// via LabToSRGB (performs chroma-scaling gamut mapping if needed)
			rLab, gLab, bLab := LabToSRGB(tc.L, tc.a, tc.b)

			// via XYZ path projecting to Lab and reusing LabToSRGB
			rXYZ, gXYZ, bXYZ := XYZToSRGB_D50GamutMap(X, Y, Z)

			if !nearlyEqual(rLab, rXYZ, eps) || !nearlyEqual(gLab, gXYZ, eps) || !nearlyEqual(bLab, bXYZ, eps) {
				t.Fatalf("Gamut-mapped mismatch for %s: labPath=(%.12f,%.12f,%.12f) xyzPath=(%.12f,%.12f,%.12f)",
					tc.name, rLab, gLab, bLab, rXYZ, gXYZ, bXYZ)
			}
		})
	}
}

func TestLabXYZ_Roundtrip_TableDriven(t *testing.T) {
	epsL := 1e-9
	epsAB := 1e-8 // a,b can be slightly more sensitive

	for _, tc := range tableCases {
		t.Run(tc.name+"/Lab->XYZ->Lab", func(t *testing.T) {
			X, Y, Z := labToXYZ_D50(tc.L, tc.a, tc.b)
			L2, a2, b2 := XYZToLab_D50(X, Y, Z)

			if !nearlyEqual(tc.L, L2, epsL) || !nearlyEqual(tc.a, a2, epsAB) || !nearlyEqual(tc.b, b2, epsAB) {
				t.Fatalf("Roundtrip mismatch for %s: in Lab=(%.9f,%.9f,%.9f) out Lab=(%.9f,%.9f,%.9f)",
					tc.name, tc.L, tc.a, tc.b, L2, a2, b2)
			}
		})
	}
}

func TestGamutMapping_Ensures_InGamut_TableDriven(t *testing.T) {
	// Ensure that gamut mapping (LabToSRGB) always returns values inside [0,1]
	for _, tc := range tableCases {
		t.Run(tc.name+"/InGamutAfterMapping", func(t *testing.T) {
			r, g, b := LabToSRGB(tc.L, tc.a, tc.b)
			if !inGamut(r, g, b) {
				t.Fatalf("Gamut mapping failed to produce in-gamut RGB for %s: got (%.12f,%.12f,%.12f)", tc.name, r, g, b)
			}
		})
	}
}

func TestAdaptedWhite_IsNearOne(t *testing.T) {
	// Sanity: for neutral D50 white, after adaptation+matrix multiply and companding we should be near sRGB white.
	rl, gl, bl := XYZToLinearRGB_D50(whiteD50[0], whiteD50[1], whiteD50[2])
	rg := linearToSRGBComp(rl)
	gg := linearToSRGBComp(gl)
	bg := linearToSRGBComp(bl)

	if !(rg > 0.99 && gg > 0.99 && bg > 0.99) {
		t.Fatalf("Adapted white not near 1: got (%.6f, %.6f, %.6f)", rg, gg, bg)
	}
}

func TestGoldenRegression(t *testing.T) {
	// Use a tight epsilon for regression comparisons.
	const eps = 1e-9

	for _, tc := range tableCases {
		// Compute current outputs via LabToSRGB (which applies gamut mapping)
		gotR, gotG, gotB := LabToSRGB(tc.L, tc.a, tc.b)

		if !nearlyEqual(tc.R, gotR, eps) || !nearlyEqual(tc.G, gotG, eps) || !nearlyEqual(tc.B, gotB, eps) {
			t.Fatalf("golden mismatch for %s:\n  expected R,G,B = (%.12f, %.12f, %.12f)\n  got      R,G,B = (%.12f, %.12f, %.12f)\n\nIf this change is intentional, update the table of test cases",
				tc.name, tc.R, tc.G, tc.B, gotR, gotG, gotB)
		}
	}
}

