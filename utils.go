package imaging

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

var max_procs atomic.Int64

// SetMaxProcs limits the number of concurrent processing goroutines to the given value.
// A value <= 0 clears the limit.
func SetMaxProcs(value int) {
	max_procs.Store(int64(value))
}

func format_stacktrace_on_panic(r any) (err error) {
	pcs := make([]uintptr, 512)
	n := runtime.Callers(3, pcs)
	lines := []string{}
	frames := runtime.CallersFrames(pcs[:n])
	lines = append(lines, fmt.Sprintf("\r\nPanicked with error: %s\r\nStacktrace (most recent call first):\r\n", r))
	found_first_frame := false
	for frame, more := frames.Next(); more; frame, more = frames.Next() {
		if !found_first_frame {
			if strings.HasPrefix(frame.Function, "runtime.") {
				continue
			}
			found_first_frame = true
		}
		lines = append(lines, fmt.Sprintf("%s\r\n\t%s:%d\r\n", frame.Function, frame.File, frame.Line))
	}
	text := strings.Join(lines, "")
	return fmt.Errorf("%s", strings.TrimSpace(text))
}

// Run the specified function in parallel over chunks from the specified range.
// If the function panics, it is turned into a regular error.
func run_in_parallel_over_range(num_procs int, f func(int, int), start, limit int) (err error) {
	num_items := limit - start
	if num_procs <= 0 {
		num_procs = runtime.GOMAXPROCS(0)
		if mp := int(max_procs.Load()); mp > 0 {
			num_procs = min(num_procs, mp)
		}
	}
	num_procs = max(1, min(num_procs, num_items))
	if num_procs < 2 {
		defer func() {
			if r := recover(); r != nil {
				err = format_stacktrace_on_panic(r)
			}
		}()
		f(start, limit)
		return
	}
	chunk_sz := max(1, num_items/num_procs)
	var wg sync.WaitGroup
	echan := make(chan error, num_procs)
	for start < limit {
		end := min(start+chunk_sz, limit)
		wg.Add(1)
		go func(start, end int) {
			defer func() {
				if r := recover(); r != nil {
					echan <- format_stacktrace_on_panic(r)
				}
				wg.Done()
			}()
			f(start, end)
		}(start, end)
		start = end
	}
	wg.Wait()
	close(echan)
	for qerr := range echan {
		return qerr
	}
	return
}

// absint returns the absolute value of i.
func absint(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// clamp rounds and clamps float64 value to fit into uint8.
func clamp(x float64) uint8 {
	v := int64(x + 0.5)
	if v > 255 {
		return 255
	}
	if v > 0 {
		return uint8(v)
	}
	return 0
}

func reverse(pix []uint8) {
	if len(pix) <= 4 {
		return
	}
	i := 0
	j := len(pix) - 4
	for i < j {
		pi := pix[i : i+4 : i+4]
		pj := pix[j : j+4 : j+4]
		pi[0], pj[0] = pj[0], pi[0]
		pi[1], pj[1] = pj[1], pi[1]
		pi[2], pj[2] = pj[2], pi[2]
		pi[3], pj[3] = pj[3], pi[3]
		i += 4
		j -= 4
	}
}

func toNRGBA(img image.Image) *image.NRGBA {
	if img, ok := img.(*image.NRGBA); ok {
		return &image.NRGBA{
			Pix:    img.Pix,
			Stride: img.Stride,
			Rect:   img.Rect.Sub(img.Rect.Min),
		}
	}
	return Clone(img)
}

// rgbToHSL converts a color from RGB to HSL.
func rgbToHSL(r, g, b uint8) (float64, float64, float64) {
	rr := float64(r) / 255
	gg := float64(g) / 255
	bb := float64(b) / 255

	max := math.Max(rr, math.Max(gg, bb))
	min := math.Min(rr, math.Min(gg, bb))

	l := (max + min) / 2

	if max == min {
		return 0, 0, l
	}

	var h, s float64
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}

	switch max {
	case rr:
		h = (gg - bb) / d
		if g < b {
			h += 6
		}
	case gg:
		h = (bb-rr)/d + 2
	case bb:
		h = (rr-gg)/d + 4
	}
	h /= 6

	return h, s, l
}

// hslToRGB converts a color from HSL to RGB.
func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	var r, g, b float64
	if s == 0 {
		v := clamp(l * 255)
		return v, v, v
	}

	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q

	r = hueToRGB(p, q, h+1/3.0)
	g = hueToRGB(p, q, h)
	b = hueToRGB(p, q, h-1/3.0)

	return clamp(r * 255), clamp(g * 255), clamp(b * 255)
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	if t < 1/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1/2.0 {
		return q
	}
	if t < 2/3.0 {
		return p + (q-p)*(2/3.0-t)*6
	}
	return p
}
