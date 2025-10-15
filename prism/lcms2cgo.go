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
	}
	return
}
