//go:build lcms2cgo

package prism

/*
#cgo pkg-config: lcms2
#include <lcms2.h>
#include <stdlib.h>

// Forward declaration for Go error handler callback
extern void go_lcms2_error_handler(void*, int, char *);

// Bridge to call Go error handler from C
static void lcms2_error_handler(cmsContext ctx, cmsUInt32Number code, const char *text) {
    go_lcms2_error_handler(cmsGetContextUserData(ctx), code, (char*)text);
}

// Wrapper to set error handler
static void set_lcms2_error_handler(cmsContext ctx) {
    cmsSetLogErrorHandlerTHR(ctx, lcms2_error_handler);
}
*/
import "C"

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"unsafe"

	"github.com/kovidgoyal/imaging/prism/meta/icc"
)

var _ = fmt.Print

type CMSProfile struct {
	DeviceColorSpace, PCSColorSpace icc.Signature
	ctx                             C.cmsContext
	p                               C.cmsHPROFILE
	error_messages                  []string
	pcs_output_format               C.cmsUInt32Number
	device8bit_format               C.cmsUInt32Number
	device_float_format             C.cmsUInt32Number
}

func (c *CMSProfile) Close() {
	if c.p != nil {
		C.cmsCloseProfile(c.p)
		c.p = nil
	}
	if c.ctx != nil {
		C.cmsDeleteContext(c.ctx)
		c.ctx = nil
	}
}

//export go_lcms2_error_handler
func go_lcms2_error_handler(ctx *C.void, code C.int, text *C.char) {
	profile := (*CMSProfile)(unsafe.Pointer(ctx))
	profile.error_messages = append(profile.error_messages, fmt.Sprintf("LCMS2 error: %d: %s", int(code), C.GoString(text)))
}

func (p *CMSProfile) call_func_with_error_handling(f func() string) error {
	p.error_messages = nil
	msg := f()
	if msg != "" {
		if len(p.error_messages) > 0 {
			return fmt.Errorf("%s: %s", msg, strings.Join(p.error_messages, "\n"))
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func format_for_float(s icc.Signature) (ans C.cmsUInt32Number, err error) {
	switch s {
	case icc.XYZSignature:
		ans = C.TYPE_XYZ_DBL
	case icc.LabSignature:
		ans = C.TYPE_Lab_DBL
	case icc.GraySignature:
		ans = C.TYPE_GRAY_DBL
	case icc.RGBSignature:
		ans = C.TYPE_RGB_DBL
	case icc.CMYKSignature:
		ans = C.TYPE_CMYK_DBL
	default:
		err = fmt.Errorf("unknown format: %s", s)
	}
	return
}

func format_for_8bit(s icc.Signature) (ans C.cmsUInt32Number, err error) {
	switch s {
	case icc.GraySignature:
		ans = C.TYPE_GRAY_8
	case icc.RGBSignature:
		ans = C.TYPE_RGB_8
	case icc.CMYKSignature:
		ans = C.TYPE_CMYK_8
	default:
		err = fmt.Errorf("unknown format: %s", s)
	}
	return
}

func (p *CMSProfile) NumDeviceChannels() int {
	switch p.DeviceColorSpace {
	case icc.GraySignature:
		return 1
	case icc.CMYKSignature:
		return 4
	}
	return 3
}

func CreateCMSProfile(data []byte) (ans *CMSProfile, err error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data not allowed")
	}
	ans = &CMSProfile{}
	ans.ctx = C.cmsCreateContext(nil, unsafe.Pointer(ans))
	C.set_lcms2_error_handler(ans.ctx)
	cptr := unsafe.Pointer(&data[0])
	err = ans.call_func_with_error_handling(func() string {
		ans.p = C.cmsOpenProfileFromMemTHR(ans.ctx, cptr, C.cmsUInt32Number(len(data)))
		if ans.p == nil {
			return "failed to load ICC profile from provided data"
		}
		return ""
	})
	runtime.SetFinalizer(ans, func(obj any) {
		ans := obj.(*CMSProfile)
		ans.Close()
	})
	if ans.p != nil {
		ans.DeviceColorSpace = icc.Signature(C.cmsGetColorSpace(ans.p))
		ans.PCSColorSpace = icc.Signature(C.cmsGetPCS(ans.p))
		if ans.pcs_output_format, err = format_for_float(ans.PCSColorSpace); err != nil {
			return nil, err
		}
		if ans.device8bit_format, err = format_for_8bit(ans.DeviceColorSpace); err != nil {
			return nil, err
		}
		if ans.device_float_format, err = format_for_float(ans.DeviceColorSpace); err != nil {
			return nil, err
		}
	}
	return
}

func (p *CMSProfile) TransformRGB8(data []uint8, output_profile *CMSProfile, intent icc.RenderingIntent) (ans []uint8, err error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data)%3 != 0 {
		return nil, fmt.Errorf("pixel data must be a multiple of 3")
	}
	var t C.cmsHTRANSFORM
	if err = p.call_func_with_error_handling(func() string {
		if t = C.cmsCreateTransformTHR(p.ctx, p.p, C.TYPE_RGB_8, output_profile.p, output_profile.device8bit_format, C.cmsUInt32Number(intent), C.cmsFLAGS_NOOPTIMIZE); t == nil {
			return "failed to create transform"
		}
		return ""
	}); err != nil {
		return
	}
	defer C.cmsDeleteTransform(t)
	ans = make([]uint8, len(data))
	C.cmsDoTransform(t, unsafe.Pointer(&data[0]), unsafe.Pointer(&ans[0]), C.cmsUInt32Number(len(data)/3))
	return
}

func (p *CMSProfile) TransformRGB8bitToPCS(data []uint8, intent icc.RenderingIntent) (ans []float64, err error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data)%3 != 0 {
		return nil, fmt.Errorf("pixel data must be a multiple of 3")
	}
	var t C.cmsHTRANSFORM
	if err = p.call_func_with_error_handling(func() string {
		if t = C.cmsCreateTransformTHR(p.ctx, p.p, C.TYPE_RGB_8, nil, p.pcs_output_format, C.cmsUInt32Number(intent), C.cmsFLAGS_NOOPTIMIZE); t == nil {
			return "failed to create transform"
		}
		return ""
	}); err != nil {
		return
	}
	defer C.cmsDeleteTransform(t)
	ans = make([]float64, len(data))
	C.cmsDoTransform(t, unsafe.Pointer(&data[0]), unsafe.Pointer(&ans[0]), C.cmsUInt32Number(len(data)/3))
	return
}

func (p *CMSProfile) TransformFloatToPCS(data []float64, intent icc.RenderingIntent) (ans []float64, err error) {
	if len(data) == 0 {
		return nil, nil
	}
	var t C.cmsHTRANSFORM
	if err = p.call_func_with_error_handling(func() string {
		if t = C.cmsCreateTransformTHR(p.ctx, p.p, p.device_float_format, nil, p.pcs_output_format, C.cmsUInt32Number(intent), C.cmsFLAGS_NOOPTIMIZE); t == nil {
			return "failed to create transform"
		}
		return ""
	}); err != nil {
		return
	}
	defer C.cmsDeleteTransform(t)
	num_channels := p.NumDeviceChannels()
	ans = make([]float64, 3*(len(data)/num_channels))
	C.cmsDoTransform(t, unsafe.Pointer(&data[0]), unsafe.Pointer(&ans[0]), C.cmsUInt32Number(len(data)/num_channels))
	return
}

func (p *CMSProfile) TransformFloatToDevice(data []float64, intent icc.RenderingIntent) (ans []float64, err error) {
	if len(data) == 0 {
		return nil, nil
	}
	var pcs C.cmsHPROFILE
	switch p.PCSColorSpace {
	case icc.XYZSignature:
		pcs = C.cmsCreateXYZProfile()
	case icc.LabSignature:
		pcs = C.cmsCreateLab4Profile(nil)
	default:
		return nil, fmt.Errorf("unknown PCS color space: %s", p.PCSColorSpace)
	}
	defer func() {
		C.cmsCloseProfile(pcs)
	}()

	var t C.cmsHTRANSFORM
	if err = p.call_func_with_error_handling(func() string {
		if t = C.cmsCreateTransformTHR(p.ctx, pcs, p.pcs_output_format, p.p, p.device_float_format, C.cmsUInt32Number(intent), C.cmsFLAGS_NOOPTIMIZE); t == nil {
			return "failed to create transform"
		}
		return ""
	}); err != nil {
		return
	}
	num_pixels := len(data) / 3
	num_channels := p.NumDeviceChannels()
	defer C.cmsDeleteTransform(t)
	ans = make([]float64, num_pixels*num_channels)
	C.cmsDoTransform(t, unsafe.Pointer(&data[0]), unsafe.Pointer(&ans[0]), C.cmsUInt32Number(num_pixels))
	return
}

func (p *CMSProfile) TransformFloatToSRGB(data []float64, intent icc.RenderingIntent) (ans []float64, err error) {
	if len(data) == 0 {
		return nil, nil
	}
	var output_profile C.cmsHPROFILE = C.cmsCreate_sRGBProfile()
	defer func() {
		C.cmsCloseProfile(output_profile)
	}()
	var t C.cmsHTRANSFORM
	if err = p.call_func_with_error_handling(func() string {
		if t = C.cmsCreateTransformTHR(p.ctx, p.p, p.device_float_format, output_profile, C.TYPE_RGB_DBL, C.cmsUInt32Number(intent), C.cmsFLAGS_NOOPTIMIZE); t == nil {
			return "failed to create transform"
		}
		return ""
	}); err != nil {
		return
	}
	defer C.cmsDeleteTransform(t)
	num_pixels := len(data) / p.NumDeviceChannels()
	ans = make([]float64, 3*num_pixels)
	C.cmsDoTransform(t, unsafe.Pointer(&data[0]), unsafe.Pointer(&ans[0]), C.cmsUInt32Number(num_pixels))
	return
}

func (p *CMSProfile) DetectBlackPoint(intent icc.RenderingIntent) (ans icc.XYZType, ok bool) {
	var bp C.cmsCIEXYZ
	cok := C.cmsDetectBlackPoint(&bp, p.p, C.cmsUInt32Number(intent), 0)
	ok = cok != 0
	if ok {
		ans.X, ans.Y, ans.Z = float64(bp.X), float64(bp.Y), float64(bp.Z)
	}
	return
}

// Transform device pixels with 8 or 16 bits per channel to sRGB with the
// same number of bits per channel using lcms
func (p *CMSProfile) TransformToSRGBInt(data []byte, bytes_per_channel int, intent icc.RenderingIntent) (ans []byte, err error) {
	if len(data) == 0 {
		return nil, nil
	}
	var input_format, output_format C.cmsUInt32Number
	if bytes_per_channel == 1 {
		input_format, output_format = p.device8bit_format, C.TYPE_RGB_8
	} else {
		// Go image types store 16 bit values in big endian
		switch p.DeviceColorSpace {
		case icc.GraySignature:
			input_format = C.TYPE_GRAY_16_SE
		case icc.RGBSignature:
			input_format = C.TYPE_RGB_16_SE
		case icc.CMYKSignature:
			input_format = C.TYPE_CMYK_16_SE
		}
		output_format = C.TYPE_RGB_16_SE
	}
	var output_profile C.cmsHPROFILE = C.cmsCreate_sRGBProfile()
	defer C.cmsCloseProfile(output_profile)
	var t C.cmsHTRANSFORM
	if err = p.call_func_with_error_handling(func() string {
		if t = C.cmsCreateTransformTHR(p.ctx, p.p, input_format, output_profile, output_format, C.cmsUInt32Number(intent), C.cmsFLAGS_NOOPTIMIZE); t == nil {
			return "failed to create transform"
		}
		return ""
	}); err != nil {
		return
	}
	defer C.cmsDeleteTransform(t)
	num_pixels := len(data) / (bytes_per_channel * p.NumDeviceChannels())
	ans = make([]byte, 3*bytes_per_channel*num_pixels)
	C.cmsDoTransform(t, unsafe.Pointer(&data[0]), unsafe.Pointer(&ans[0]), C.cmsUInt32Number(num_pixels))
	return
}

type GrayCurve int

const (
	GammaGrayCurve GrayCurve = iota
	SRGBGrayCurve
	TabulatedGrayCurve
	IdentityGrayCurve
)

// Specification for a monochrome ICC profile created by lcms for testing
type GrayProfileSpec struct {
	Version     float64
	WhitePoint  icc.XYZType
	Curve       GrayCurve
	Gamma       float64
	PCS         icc.Signature
	DeviceClass icc.Signature
	// Use LUT based A2B0 and B2A0 tags instead of a grayTRC tag. The LUTs
	// are tinted so that output is not neutral.
	UseLUT bool
}

// The tabulated curve used by TabulatedGrayCurve, it has a linear component
// so that it is not flat near zero when quantized to 16 bits, as the inverse
// of a flat region is ambiguous
func TabulatedGrayCurveValue(x float64) float64 { return 0.8*math.Pow(x, 2.2) + 0.2*x }

func (spec GrayProfileSpec) tone_curve(ctx C.cmsContext) *C.cmsToneCurve {
	switch spec.Curve {
	case SRGBGrayCurve:
		params := [5]C.cmsFloat64Number{2.4, 1 / 1.055, 0.055 / 1.055, 1 / 12.92, 0.04045}
		return C.cmsBuildParametricToneCurve(ctx, 4, &params[0])
	case TabulatedGrayCurve:
		const n = 1024
		vals := make([]C.cmsFloat32Number, n)
		for i := range vals {
			vals[i] = C.cmsFloat32Number(TabulatedGrayCurveValue(float64(i) / (n - 1)))
		}
		return C.cmsBuildTabulatedToneCurveFloat(ctx, n, &vals[0])
	case IdentityGrayCurve:
		return C.cmsBuildGamma(ctx, 1)
	default:
		return C.cmsBuildGamma(ctx, C.cmsFloat64Number(spec.Gamma))
	}
}

func y_to_lstar(y float64) float64 {
	if y > 216./24389 {
		return 116*math.Cbrt(y) - 16
	}
	return y * 24389. / 27
}

func lstar_to_y(l float64) float64 {
	if l > 8 {
		t := (l + 16) / 116
		return t * t * t
	}
	return l * 27. / 24389
}

func sat16(x float64) C.cmsUInt16Number {
	return C.cmsUInt16Number(max(0, min(math.Round(x), 65535)))
}

// the PCS encoding of the tinted luminance Y for the A2B0 CLUT
func (spec GrayProfileSpec) encode_pcs(y float64, v2_lab bool) (ans [3]C.cmsUInt16Number) {
	if spec.PCS == icc.LabSignature {
		l, a, b := y_to_lstar(y), 4*y, 10*y
		if v2_lab {
			return [3]C.cmsUInt16Number{sat16(l * 652.8), sat16((a + 128) * 256), sat16((b + 128) * 256)}
		}
		return [3]C.cmsUInt16Number{sat16(l * 655.35), sat16((a + 128) * 257), sat16((b + 128) * 257)}
	}
	return [3]C.cmsUInt16Number{sat16(y * 0.9642 * 1.02 * 32768), sat16(y * 32768), sat16(y * 0.8249 * 0.9 * 32768)}
}

// the luminance from the PCS encoding of the grid position of the B2A0 CLUT
func (spec GrayProfileSpec) decode_pcs(p float64, v2_lab bool) float64 {
	if spec.PCS == icc.LabSignature {
		l := p * 100
		if v2_lab {
			l *= 65535. / 65280.
		}
		return min(1, lstar_to_y(l))
	}
	return min(1, p*65535/32768)
}

func (spec GrayProfileSpec) write_luts(ctx C.cmsContext, h C.cmsHPROFILE, curve *C.cmsToneCurve) string {
	const grid = 33
	v2_lab := spec.Version < 4
	a2b := C.cmsPipelineAlloc(ctx, 1, 3)
	defer C.cmsPipelineFree(a2b)
	table := make([]C.cmsUInt16Number, 0, grid*3)
	for i := range grid {
		e := spec.encode_pcs(float64(i)/(grid-1), v2_lab)
		table = append(table, e[:]...)
	}
	curves := []*C.cmsToneCurve{curve}
	C.cmsPipelineInsertStage(a2b, C.cmsAT_END, C.cmsStageAllocToneCurves(ctx, 1, &curves[0]))
	C.cmsPipelineInsertStage(a2b, C.cmsAT_END, C.cmsStageAllocCLut16bit(ctx, grid, 1, 3, &table[0]))
	C.cmsPipelineInsertStage(a2b, C.cmsAT_END, C.cmsStageAllocToneCurves(ctx, 3, nil))
	if C.cmsWriteTag(h, C.cmsSigAToB0Tag, unsafe.Pointer(a2b)) == 0 {
		return "failed to write A2B0 tag"
	}

	const grid3 = 17
	b2a := C.cmsPipelineAlloc(ctx, 3, 1)
	defer C.cmsPipelineFree(b2a)
	table = make([]C.cmsUInt16Number, 0, grid3*grid3*grid3)
	// the first input channel varies least rapidly, the output is linear
	// luminance which is then encoded by the reversed tone curve
	idx := 0
	if spec.PCS == icc.XYZSignature {
		idx = 1
	}
	var pos [3]int
	for pos[0] = range grid3 {
		for pos[1] = range grid3 {
			for pos[2] = range grid3 {
				table = append(table, sat16(spec.decode_pcs(float64(pos[idx])/(grid3-1), v2_lab)*65535))
			}
		}
	}
	rev := C.cmsReverseToneCurve(curve)
	defer C.cmsFreeToneCurve(rev)
	curves = []*C.cmsToneCurve{rev}
	C.cmsPipelineInsertStage(b2a, C.cmsAT_END, C.cmsStageAllocToneCurves(ctx, 3, nil))
	C.cmsPipelineInsertStage(b2a, C.cmsAT_END, C.cmsStageAllocCLut16bit(ctx, grid3, 3, 1, &table[0]))
	C.cmsPipelineInsertStage(b2a, C.cmsAT_END, C.cmsStageAllocToneCurves(ctx, 1, &curves[0]))
	if C.cmsWriteTag(h, C.cmsSigBToA0Tag, unsafe.Pointer(b2a)) == 0 {
		return "failed to write B2A0 tag"
	}
	return ""
}

// Create a monochrome ICC profile using lcms
func CreateGrayProfile(spec GrayProfileSpec) (ans []byte, err error) {
	p := &CMSProfile{}
	ctx := C.cmsCreateContext(nil, unsafe.Pointer(p))
	defer C.cmsDeleteContext(ctx)
	C.set_lcms2_error_handler(ctx)
	err = p.call_func_with_error_handling(func() string {
		curve := spec.tone_curve(ctx)
		if curve == nil {
			return "failed to create tone curve"
		}
		defer C.cmsFreeToneCurve(curve)
		var h C.cmsHPROFILE
		wtpt := C.cmsCIEXYZ{C.cmsFloat64Number(spec.WhitePoint.X), C.cmsFloat64Number(spec.WhitePoint.Y), C.cmsFloat64Number(spec.WhitePoint.Z)}
		if spec.UseLUT {
			if h = C.cmsCreateProfilePlaceholder(ctx); h == nil {
				return "failed to create profile"
			}
			C.cmsSetColorSpace(h, C.cmsSigGrayData)
		} else {
			var wp C.cmsCIExyY
			C.cmsXYZ2xyY(&wp, &wtpt)
			if h = C.cmsCreateGrayProfileTHR(ctx, &wp, curve); h == nil {
				return "failed to create gray profile"
			}
		}
		defer C.cmsCloseProfile(h)
		C.cmsSetProfileVersion(h, C.cmsFloat64Number(spec.Version))
		C.cmsSetPCS(h, C.cmsColorSpaceSignature(spec.PCS))
		if spec.DeviceClass != 0 {
			C.cmsSetDeviceClass(h, C.cmsProfileClassSignature(spec.DeviceClass))
		} else {
			C.cmsSetDeviceClass(h, C.cmsSigDisplayClass)
		}
		C.cmsSetHeaderRenderingIntent(h, C.INTENT_PERCEPTUAL)
		if spec.UseLUT {
			if C.cmsWriteTag(h, C.cmsSigMediaWhitePointTag, unsafe.Pointer(&wtpt)) == 0 {
				return "failed to write wtpt tag"
			}
			if msg := spec.write_luts(ctx, h, curve); msg != "" {
				return msg
			}
		}
		var size C.cmsUInt32Number
		if C.cmsSaveProfileToMem(h, nil, &size) == 0 {
			return "failed to get size of profile"
		}
		ans = make([]byte, size)
		if C.cmsSaveProfileToMem(h, unsafe.Pointer(&ans[0]), &size) == 0 {
			return "failed to save profile"
		}
		ans = ans[:size]
		return ""
	})
	return
}
