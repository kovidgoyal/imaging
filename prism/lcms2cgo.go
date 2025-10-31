//go:build lcms2cgo

package prism

/*
#cgo LDFLAGS: -llcms2
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
	num_channels := 3
	if p.DeviceColorSpace == icc.CMYKSignature {
		num_channels = 4
	}
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
	num_channels := 3
	if p.DeviceColorSpace == icc.CMYKSignature {
		num_channels = 4
	}
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
	ans = make([]float64, len(data))
	C.cmsDoTransform(t, unsafe.Pointer(&data[0]), unsafe.Pointer(&ans[0]), C.cmsUInt32Number(len(data)/3))
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
