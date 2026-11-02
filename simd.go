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
	// Four outputs at a time: independent accumulators let the CPU overlap
	// the multiply-adds, and each output sums its terms in the same order.
	o := 0
	for ; o+4 <= len(dst); o += 4 {
		xw := x[o : o+taps+3]
		var s0, s1, s2, s3 int32
		for t, c := range coef {
			c32 := int32(c)
			w := xw[t : t+4 : t+4]
			s0 += int32(w[0]) * c32
			s1 += int32(w[1]) * c32
			s2 += int32(w[2]) * c32
			s3 += int32(w[3]) * c32
		}
		dst[o], dst[o+1], dst[o+2], dst[o+3] = s0, s1, s2, s3
	}
	for ; o < len(dst); o++ {
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
	// Four accumulators, so the multiply-adds overlap; the int32 sum wraps,
	// so the order of the terms does not change the result.
	var s0, s1, s2, s3 int32
	i := 0
	for ; i+4 <= len(a); i += 4 {
		x, y := a[i:i+4:i+4], bb[i:i+4:i+4]
		s0 += int32(x[0]) * int32(y[0])
		s1 += int32(x[1]) * int32(y[1])
		s2 += int32(x[2]) * int32(y[2])
		s3 += int32(x[3]) * int32(y[3])
	}
	for ; i < len(a); i++ {
		s0 += int32(a[i]) * int32(bb[i])
	}
	return s0 + s1 + s2 + s3
}
