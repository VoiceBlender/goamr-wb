package amrwb

// SIMD-accelerated non-saturating int16 FIR, used by the hot filter kernels.
// The reference accumulates products into a plain int32 (no per-term
// saturation), and integer addition is associative modulo 2^32, so accumulating
// in the lane-parallel order an AVX2 kernel uses yields a bit-identical result.
// firRawGeneric is the portable fallback and the oracle the assembly is
// fuzz-tested against.

// firRawGeneric computes the raw (pre-round, pre-shift) FIR sums
// dst[o] = sum_t x[o+t]*coef[t] for every output o, accumulating in wrapping
// int32. The amd64 build vectorises this across 8 outputs at a time; this is
// the portable fallback and the fuzz-test oracle. Requires
// len(x) >= len(dst)+len(coef)-1.
func firRawGeneric(dst []int32, x []int16, coef []int16) {
	taps := len(coef)
	for o := range dst {
		xw := x[o : o+taps]
		var s int32
		for t, c := range coef {
			s += int32(xw[t]) * int32(c)
		}
		dst[o] = s
	}
}

// firDotGeneric computes the wrapping-int32 dot product sum_i a[i]*b[i] over the
// first len(a) elements (requires len(b) >= len(a)). The amd64 build vectorises
// it with VPMADDWD; this is the portable fallback and the fuzz-test oracle.
func firDotGeneric(a, b []int16) int32 {
	bb := b[:len(a)]
	var s int32
	for i, av := range a {
		s += int32(av) * int32(bb[i])
	}
	return s
}
