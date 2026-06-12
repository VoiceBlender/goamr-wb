package amrwb

// Encoder fixed-point helpers, ported verbatim from the Apache-2.0 vo-amrwbenc
// reference (oper_32b.c, math_op.c, the vo_* macros in basic_op.h), derived
// from 3GPP TS 26.190. These complement the standard saturating operators in
// basicops.go (which match vo-amrwbenc's basic_op.h add/sub/mult/L_mac/...).
//
// The vo_* operators are the reference's NON-saturating fast variants; several
// encoder math kernels (Dot_product12, Isqrt_n) depend on that exact wrapping
// behaviour, so they are ported separately from the decoder equivalents.

// vo_* non-saturating helpers.
func voRound(a int32) int16    { return int16((a + 0x8000) >> 16) }
func voMultR(a, b int16) int16 { return int16((int32(a)*int32(b) + 0x4000) >> 15) }
func voShrR(var1, var2 int16) int16 {
	return int16((int32(var1) + (int32(1) << uint(var2-1))) >> uint(var2))
}

// L_abs returns the saturating 32-bit absolute value.
func L_abs(a int32) int32 {
	if a == min32 {
		return max32
	}
	if a < 0 {
		return -a
	}
	return a
}

// voLExtract splits a 32-bit value into double-precision hi/lo (VO_L_Extract).
func voLExtract(l int32) (hi, lo int16) {
	hi = int16(l >> 16)
	lo = int16((l & 0xffff) >> 1)
	return
}

// lComp recombines a double-precision (hi,lo) pair: hi<<16 + lo<<1 (L_Comp).
func lComp(hi, lo int16) int32 { return L_mac(L_deposit_h(hi), lo, 1) }

// mpy32 multiplies two DPF numbers, result / 2^31 (Mpy_32).
func mpy32(hi1, lo1, hi2, lo2 int16) int32 {
	l := int32(hi1) * int32(hi2)
	l += (int32(hi1) * int32(lo2)) >> 15
	l += (int32(lo1) * int32(hi2)) >> 15
	return l << 1
}

// mpy3216 multiplies a DPF number by a 16-bit value, result / 2^15 (Mpy_32_16).
func mpy3216(hi, lo, n int16) int32 {
	l := (int32(hi) * int32(n)) << 1
	l += ((int32(lo) * int32(n)) >> 15) << 1
	return l
}

// lShl2 is the saturating left shift used by the encoder; it returns 0 when
// var2 <= 0 (L_shl2 in basic_op.h).
func lShl2(l int32, var2 int16) int32 {
	var out int32
	for ; var2 > 0; var2-- {
		if l > 0x3fffffff {
			out = max32
			break
		}
		if l < -0x40000000 {
			out = min32
			break
		}
		l <<= 1
		out = l
	}
	return out
}

// div32 computes L_num / L_denom in Q31 via Newton refinement (Div_32).
func div32(lNum int32, denomHi, denomLo int16) int32 {
	approx := div_s(0x3fff, denomHi)
	l := mpy3216(denomHi, denomLo, approx)
	l = L_sub(0x7fffffff, l)
	hi := int16(l >> 16)
	lo := int16((l & 0xffff) >> 1)
	l = mpy3216(hi, lo, approx)
	hi = int16(l >> 16)
	lo = int16((l & 0xffff) >> 1)
	nHi, nLo := voLExtract(lNum)
	l = mpy32(nHi, nLo, hi, lo)
	return lShl2(l, 2)
}

// encIsqrtN computes 1/sqrt(frac) for a normalized frac with exponent (Isqrt_n).
func encIsqrtN(frac *int32, exp *int16) {
	if *frac <= 0 {
		*exp = 0
		*frac = 0x7fffffff
		return
	}
	if *exp&1 == 1 {
		*frac >>= 1
	}
	*exp = negate((*exp - 1) >> 1)
	*frac >>= 9
	i := extract_h(*frac)
	*frac >>= 1
	a := int16(*frac) & 0x7fff
	i -= 16
	*frac = L_deposit_h(tableIsqrt[i])
	tmp := tableIsqrt[i] - tableIsqrt[i+1]         // vo_sub (non-saturating)
	*frac = *frac - ((int32(tmp) * int32(a)) << 1) // vo_L_msu (non-saturating)
}

// encIsqrt computes 1/sqrt(L_x) in Q31 (Isqrt).
func encIsqrt(lX int32) int32 {
	exp := norm_l(lX)
	lX <<= uint(exp)
	exp = 31 - exp
	encIsqrtN(&lX, &exp)
	if exp >= 0 {
		return lX << uint(exp)
	}
	return lX >> uint(-exp)
}

// encPow2 computes pow(2, exponant.fraction) (Pow2).
func encPow2(exponant, fraction int16) int32 {
	lX := (int32(fraction) * 32) << 1 // vo_L_mult(fraction, 32)
	i := extract_h(lX)
	lX >>= 1
	a := int16(lX) & 0x7fff
	lX = L_deposit_h(tablePow2[i])
	tmp := tablePow2[i] - tablePow2[i+1]
	lX -= (int32(tmp) * int32(a)) << 1
	exp := 30 - exponant
	// vo_L_shr_r: (L + (1<<(exp-1))) >> exp
	if exp > 0 {
		lX = (lX + (int32(1) << uint(exp-1))) >> uint(exp)
	}
	return lX
}

// encDotProduct12 is the encoder's normalized dot product (Dot_product12); it
// accumulates without per-term saturation, unlike the decoder's version.
func encDotProduct12(x, y []int16, lg int16, exp *int16) int32 {
	var lSum int32
	for i := int16(0); i < lg; i++ {
		lSum += int32(x[i]) * int32(y[i])
	}
	lSum = (lSum << 1) + 1
	sft := norm_l(lSum)
	lSum <<= uint(sft)
	*exp = 30 - sft
	return lSum
}
