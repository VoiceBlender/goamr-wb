//go:build amd64

package amrwb

import "golang.org/x/sys/cpu"

var useAVX2 = cpu.X86.HasAVX2

// firRaw computes dst[o] = sum_t x[o+t]*coef[t] (wrapping int32). On AVX2 it
// vectorises across 8 outputs per iteration; otherwise it falls back.
func firRaw(dst []int32, x []int16, coef []int16) {
	if useAVX2 && len(dst) >= 8 && len(coef) <= maxFIRTaps {
		var c32 [maxFIRTaps]int32
		for i, c := range coef {
			c32[i] = int32(c)
		}
		firRawAVX2(dst, x, c32[:len(coef)])
		return
	}
	firRawGeneric(dst, x, coef)
}

// maxFIRTaps bounds the stack-resident sign-extended coefficient buffer.
// maxFIRTaps covers the longest FIR: the open-loop pitch correlation window
// (a whole frame, 256 samples).
const maxFIRTaps = 256

// firRawAVX2 is implemented in simd_amd64.s. coef32 holds sign-extended taps.
//
//go:noescape
func firRawAVX2(dst []int32, x []int16, coef32 []int32)

// firDot computes the wrapping-int32 dot product sum_i a[i]*b[i] over len(a)
// elements. On AVX2 it uses VPMADDWD across 16 int16 per iteration; otherwise it
// falls back.
func firDot(a, b []int16) int32 {
	if useAVX2 && len(a) >= 16 {
		return firDotAVX2(a, b)
	}
	return firDotGeneric(a, b)
}

// firDotAVX2 is implemented in simd_amd64.s. It reads len(a) int16 from a and b.
//
//go:noescape
func firDotAVX2(a, b []int16) int32
