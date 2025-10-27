package colorconv

import (
	"math"
)

// This package converts CIE L*a*b* colors defined relative to the D50 white point
// into sRGB values relative to D65. It performs chromatic
// adaptation (Bradford), fuses linear matrix transforms where possible for speed,
// and does a simple perceptually-minded gamut mapping by scaling chroma (a,b)
// down towards zero until the resulting sRGB is inside the [0,1] cube.
//
// Notes:
// - Input L,a,b are the usual CIELAB values (L in [0,100], a,b around -/+).
// - Returned sRGB values are in [0,1]. If gamut mapping fails (rare), values
//   will be clipped to [0,1] as a fallback.
//
// The code fuses the chromatic adaptation and XYZ->linear-sRGB matrices into a
// single 3x3 matrix so that the only linear operation after Lab->XYZ is a single
// matrix multiply.

type Vec3 [3]float64
type Mat3 [3][3]float64

// Standard reference whites (CIE XYZ) normalized so Y = 1.0
// Note that whiteD50 uses Z value from ICC spec rather that CIE spec.
var (
	whiteD50 = Vec3{0.96422, 1.00000, 0.82491}
	whiteD65 = Vec3{0.95047, 1.00000, 1.08883}
)

// Bradford transform matrices (forward and inverse)
var (
	bradford = Mat3{
		{0.8951, 0.2664, -0.1614},
		{-0.7502, 1.7135, 0.0367},
		{0.0389, -0.0685, 1.0296},
	}
	invBradford = Mat3{
		{0.9869929, -0.1470543, 0.1599627},
		{0.4323053, 0.5183603, 0.0492912},
		{-0.0085287, 0.0400428, 0.9684867},
	}
)

// sRGB (linear) transform matrix from CIE XYZ (D65)
var srgbFromXYZ = Mat3{
	{3.2406, -1.5372, -0.4986},
	{-0.9689, 1.8758, 0.0415},
	{0.0557, -0.2040, 1.0570},
}

// Precomputed combined matrix from XYZ(D50) directly to linear sRGB (D65).
// Combined = srgbFromXYZ * adaptMatrix (where adaptMatrix adapts XYZ D50 -> XYZ D65).
var combinedXYZD50ToLinearSRGB Mat3

func init() {
	// compute adaptation matrix once and fuse with srgbFromXYZ
	adapt := chromaticAdaptationMatrix(whiteD50, whiteD65)
	combinedXYZD50ToLinearSRGB = mulMat3(srgbFromXYZ, adapt)
}

// Public API

// LabToSRGB converts a Lab color (assumed D50) into sRGB (D65) with gamut mapping.
// Returned components are in [0,1].
func LabToSRGB(L, a, b float64) (r, g, bl float64) {
	// fast path: try direct conversion and only do gamut mapping if out of gamut
	r0, g0, b0 := labToSRGBNoGamutMap(L, a, b)
	if inGamut(r0, g0, b0) {
		return r0, g0, b0
	}
	// gamut map by scaling chroma (a,b) toward 0 while keeping L constant.
	rm, gm, bm := gamutMapChromaScale(L, a, b)
	return rm, gm, bm
}

// LabToLinearRGB converts Lab (D50) to linear RGB (not gamma-corrected), but still
// with chromatic adaptation to D65 fused into the matrix. Output is linear sRGB.
func LabToLinearRGB(L, a, b float64) (r, g, bl float64) {
	X, Y, Z := labToXYZ_D50(L, a, b)
	rv, gv, bv := mulMat3Vec(combinedXYZD50ToLinearSRGB, Vec3{X, Y, Z})
	return rv, gv, bv
}

// XYZToLinearRGB_D50 converts XYZ expressed relative to the D50 whitepoint
// directly to linear sRGB values (D65) using the precomputed fused matrix.
// The output is linear RGB and may be outside the [0,1] range.
func XYZToLinearRGB_D50(X, Y, Z float64) (r, g, b float64) {
	r, g, b = mulMat3Vec(combinedXYZD50ToLinearSRGB, Vec3{X, Y, Z})
	return
}

// XYZToSRGB_D50 converts XYZ expressed relative to the D50 whitepoint directly to
// gamma-corrected sRGB values (D65). The outputs are clamped to [0,1].
// This function re-uses the precomputed combined matrix and the existing companding function.
func XYZToSRGB_D50(X, Y, Z float64) (r, g, b float64) {
	rl, gl, bl := XYZToLinearRGB_D50(X, Y, Z)
	// Apply sRGB companding and clamp
	r = clamp01(linearToSRGBComp(rl))
	g = clamp01(linearToSRGBComp(gl))
	b = clamp01(linearToSRGBComp(bl))
	return
}

// If you need the non-clamped gamma-corrected values (for checking out-of-gamut)
// you can use this helper which only compands but doesn't clamp.
func XYZToSRGB_D50NoClamp(X, Y, Z float64) (r, g, b float64) {
	rl, gl, bl := XYZToLinearRGB_D50(X, Y, Z)
	r = linearToSRGBComp(rl)
	g = linearToSRGBComp(gl)
	b = linearToSRGBComp(bl)
	return
}

// XYZToSRGB_D50GamutMap converts XYZ (D50) to sRGB (D65) using the Lab-projection
// + chroma-scaling gamut mapping. It projects XYZ into CIELAB (D50), reuses the
// existing LabToSRGB (which performs chroma-scaling if needed), and returns final sRGB.
func XYZToSRGB_D50GamutMap(X, Y, Z float64) (r, g, b float64) {
	L, a, bb := XYZToLab_D50(X, Y, Z)
	return LabToSRGB(L, a, bb)
}

// Helpers: core conversions

// labToSRGBNoGamutMap converts Lab(D50) to sRGB(D65) without doing any gamut mapping.
// Values may be out of [0,1].
func labToSRGBNoGamutMap(L, a, b float64) (r, g, bl float64) {
	rLin, gLin, bLin := LabToLinearRGB(L, a, b)
	r = linearToSRGBComp(rLin)
	g = linearToSRGBComp(gLin)
	bl = linearToSRGBComp(bLin)
	return
}

func finv(t float64) float64 {
	const delta = 6.0 / 29.0
	if t > delta {
		return t * t * t
	}
	// when t <= delta: 3*delta^2*(t - 4/29)
	return 3 * delta * delta * (t - 4.0/29.0)
}

// labToXYZ_D50 converts Lab (D50) to CIE XYZ values relative to the D50 whitepoint (Y=1).
func labToXYZ_D50(L, a, b float64) (X, Y, Z float64) {
	// Inverse of the CIELAB f function
	var fy = (L + 16.0) / 116.0
	var fx = fy + (a / 500.0)
	var fz = fy - (b / 200.0)

	xr := finv(fx)
	yr := finv(fy)
	zr := finv(fz)

	X = xr * whiteD50[0]
	Y = yr * whiteD50[1]
	Z = zr * whiteD50[2]
	return
}

func ff(t float64) float64 {
	const delta = 6.0 / 29.0
	if t > delta*delta*delta {
		return math.Cbrt(t)
	}
	// t <= delta^3
	return t/(3*delta*delta) + 4.0/29.0
}

// XYZToLab_D50 converts XYZ (relative to D50, Y=1) into CIELAB (D50).
func XYZToLab_D50(X, Y, Z float64) (L, a, b float64) {
	// Normalize by D50 white
	xr := X / whiteD50[0]
	yr := Y / whiteD50[1]
	zr := Z / whiteD50[2]

	fx := ff(xr)
	fy := ff(yr)
	fz := ff(zr)

	L = 116.0*fy - 16.0
	a = 500.0 * (fx - fy)
	b = 200.0 * (fy - fz)
	return
}

// linearToSRGBComp applies the sRGB (gamma) companding function to a linear component.
func linearToSRGBComp(c float64) float64 {
	// clip small negative rounding noise at this stage for stability
	if c <= 0 {
		return 0.0
	}
	if c <= 0.0031308 {
		return 12.92 * c
	}
	return 1.055*math.Pow(c, 1.0/2.4) - 0.055
}

// inGamut checks whether r,g,b are all inside [0,1] (with a small epsilon)
func inGamut(r, g, b float64) bool {
	const eps = 1e-12
	return r >= -eps && g >= -eps && b >= -eps && r <= 1+eps && g <= 1+eps && b <= 1+eps
}

// gamutMapChromaScale reduces chroma (a,b) by scaling factor s in [0,1] to bring the
// color into gamut. Binary search is used to find the maximum s such that the color
// is in gamut. L is preserved.
func gamutMapChromaScale(L, a, b float64) (r, g, bl float64) {
	// If a==0 && b==0 we can't scale; just clip after conversion
	if a == 0 && b == 0 {
		r0, g0, b0 := labToSRGBNoGamutMap(L, a, b)
		return clamp01(r0), clamp01(g0), clamp01(b0)
	}
	// Binary search scale factor in [0,1]
	lo := 0.0
	hi := 1.0
	var mid float64
	var foundR, foundG, foundB float64
	// If even fully desaturated (a=b=0) is out of gamut, we'll clip
	for range 24 {
		mid = (lo + hi) / 2.0
		a2 := a * mid
		b2 := b * mid
		r0, g0, b0 := labToSRGBNoGamutMap(L, a2, b2)
		if inGamut(r0, g0, b0) {
			foundR, foundG, foundB = r0, g0, b0
			// can try to keep more chroma
			lo = mid
		} else {
			hi = mid
		}
	}
	// If we never found a valid in-gamut during binary search, try a= b =0
	if !(inGamut(foundR, foundG, foundB)) {
		r0, g0, b0 := labToSRGBNoGamutMap(L, 0, 0)
		// if still out-of-gamut (very unlikely), clip
		return clamp01(r0), clamp01(g0), clamp01(b0)
	}
	return clamp01(foundR), clamp01(foundG), clamp01(foundB)
}

// clamp01 clamps value to [0,1]
func clamp01(x float64) float64 {
	return max(0, min(x, 1))
}

// Matrix & vector utilities

func mulMat3(a, b Mat3) Mat3 {
	var out Mat3
	for i := range 3 {
		for j := range 3 {
			sum := 0.0
			for k := range 3 {
				sum += a[i][k] * b[k][j]
			}
			out[i][j] = sum
		}
	}
	return out
}

func mulMat3Vec(m Mat3, v Vec3) (x, y, z float64) {
	x = m[0][0]*v[0] + m[0][1]*v[1] + m[0][2]*v[2]
	y = m[1][0]*v[0] + m[1][1]*v[1] + m[1][2]*v[2]
	z = m[2][0]*v[0] + m[2][1]*v[1] + m[2][2]*v[2]
	return
}

// chromaticAdaptationMatrix constructs a 3x3 matrix that adapts XYZ values
// from sourceWhite to targetWhite using the Bradford method.
func chromaticAdaptationMatrix(sourceWhite, targetWhite Vec3) Mat3 {
	// Convert whites to LMS using Bradford
	srcL, srcM, srcS := mulMat3Vec(bradford, sourceWhite)
	tgtL, tgtM, tgtS := mulMat3Vec(bradford, targetWhite)
	// diag of ratios
	var ratios = Vec3{tgtL / srcL, tgtM / srcM, tgtS / srcS}
	// Build diag matrix in-between
	diag := Mat3{
		{ratios[0], 0, 0},
		{0, ratios[1], 0},
		{0, 0, ratios[2]},
	}
	// adapt = invBradford * diag * bradford
	tmp := mulMat3(diag, bradford)     // diag*B
	adapt := mulMat3(invBradford, tmp) // invB * (diag*B)
	return adapt
}

