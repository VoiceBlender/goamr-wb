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
const maxFIRTaps = 64

// firRawAVX2 is implemented in simd_amd64.s. coef32 holds sign-extended taps.
//
//go:noescape
func firRawAVX2(dst []int32, x []int16, coef32 []int32)
