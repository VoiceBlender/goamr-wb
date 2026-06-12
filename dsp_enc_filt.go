package amrwb

// Encoder analysis-support kernels, ported verbatim from the Apache-2.0
// vo-amrwbenc reference (weight_a.c, residu.c, convolve.c, int_lpc.c,
// preemph.c, scale.c), derived from 3GPP TS 26.190.

// Weight_a applies bandwidth expansion ap[i] = a[i]*gamma^i (Q12).
func Weight_a(a, ap []int16, gamma, m int16) {
	ap[0] = a[0]
	fac := gamma
	for i := int16(1); i < m; i++ {
		ap[i] = int16(((int32(a[i])*int32(fac))<<1 + 0x8000) >> 16)
		fac = int16(((int32(fac)*int32(gamma))<<1 + 0x8000) >> 16)
	}
	ap[m] = int16(((int32(a[m])*int32(fac))<<1 + 0x8000) >> 16)
}

// Residu computes the LP residual y = A(z)x. x carries m history samples before
// xOff (x[xOff-m .. xOff-1]); y receives lg samples.
func Residu(a []int16, x []int16, xOff int, y []int16, lg int16) {
	// y[i] = round(sum_k a[k]*x[xOff+i-k]) is a 17-tap FIR. Reverse the taps so
	// the SIMD kernel can run it as a forward convolution over x[xOff-cM:].
	var arev [cM + 1]int16
	for k := 0; k <= cM; k++ {
		arev[k] = a[cM-k]
	}
	n := int(lg)
	var dstArr [cL_SUBFR]int32 // callers pass lg <= cL_SUBFR
	dst := dstArr[:n]
	firRaw(dst, x[xOff-cM:], arev[:])
	for i := 0; i < n; i++ {
		s := lShl2(dst[i], 5)
		y[i] = extract_h(L_add(s, 0x8000))
	}
}

// Convolve computes the truncated convolution y[n] = sum_{j<=n} x[j]h[n-j],
// n=0..63 (vo_mult32 accumulates without saturation, so order is irrelevant).
func Convolve(x, h, y []int16) {
	x = x[:cL_SUBFR] // hoist bounds checks out of the inner loop
	h = h[:cL_SUBFR]
	y = y[:cL_SUBFR]
	for n := 0; n < cL_SUBFR; n++ {
		var s int32
		for j := 0; j <= n; j++ {
			s += int32(x[j]) * int32(h[n-j])
		}
		y[n] = int16(((s << 1) + 0x8000) >> 16)
	}
}

// Int_isp interpolates ISPs across the 4 subframes -> LP coefficients (Az).
func Int_isp(ispOld, ispNew, frac []int16, az []int16) {
	var ispArr [cM]int16
	isp := ispArr[:]
	azOff := 0
	for k := 0; k < 3; k++ {
		facNew := frac[k]
		facOld := (32767 - facNew) + 1
		for i := 0; i < cM; i++ {
			lTmp := (int32(ispOld[i]) * int32(facOld)) << 1
			lTmp += (int32(ispNew[i]) * int32(facNew)) << 1
			isp[i] = int16((lTmp + 0x8000) >> 16)
		}
		Isp_Az(isp, az[azOff:], cM, 0)
		azOff += cMP1
	}
	Isp_Az(ispNew, az[azOff:], cM, 0)
}

// Preemph applies (1 - mu z^-1) in place; mem holds x[-1].
func Preemph(x []int16, mu, lg int16, mem *int16) {
	temp := x[lg-1]
	for i := int(lg) - 1; i > 0; i-- {
		lTmp := L_deposit_h(x[i])
		lTmp -= (int32(x[i-1]) * int32(mu)) << 1
		x[i] = int16((lTmp + 0x8000) >> 16)
	}
	lTmp := L_deposit_h(x[0])
	lTmp -= (int32(*mem) * int32(mu)) << 1
	x[0] = int16((lTmp + 0x8000) >> 16)
	*mem = temp
}

// Preemph2 is Preemph with an extra <<1 (Q-scaling variant).
func Preemph2(x []int16, mu, lg int16, mem *int16) {
	temp := x[lg-1]
	for i := int(lg) - 1; i > 0; i-- {
		lTmp := L_deposit_h(x[i])
		lTmp -= (int32(x[i-1]) * int32(mu)) << 1
		lTmp <<= 1
		x[i] = int16((lTmp + 0x8000) >> 16)
	}
	lTmp := L_deposit_h(x[0])
	lTmp -= (int32(*mem) * int32(mu)) << 1
	lTmp <<= 1
	x[0] = int16((lTmp + 0x8000) >> 16)
	*mem = temp
}

// Scale_sig scales x by 2^exp: x = round(x << exp).
func Scale_sig(x []int16, lg, exp int16) {
	if exp > 0 {
		for i := int(lg) - 1; i >= 0; i-- {
			lTmp := lShl2(int32(x[i]), 16+exp)
			x[i] = extract_h(L_add(lTmp, 0x8000))
		}
	} else {
		ex := -exp
		for i := int(lg) - 1; i >= 0; i-- {
			lTmp := int32(x[i]) << 16
			lTmp >>= uint(ex)
			x[i] = int16((lTmp + 0x8000) >> 16)
		}
	}
}
