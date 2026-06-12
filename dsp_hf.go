package amrwb

// High-band synthesis and post-processing kernels, ported verbatim from the
// Apache-2.0 opencore-amrwb reference (noise_gen_amrwb.cpp,
// highpass_50hz_at_12k8.cpp, highpass_400hz_at_12k8.cpp, band_pass_6k_7k.cpp,
// low_pass_filt_7k.cpp, agc2_amr_wb.cpp, voice_factor.cpp,
// weight_amrwb_lpc.cpp), derived from 3GPP TS 26.173.

const cL_FIR = 30

// noise_gen_amrwb advances the LCG noise generator and returns the new sample.
func noise_gen_amrwb(seed *int16) int16 {
	*seed = int16(fxp_mac_16by16(*seed, 31821, 13849))
	return *seed
}

// highpass_50Hz_at_12k8 is a 2nd-order IIR high-pass at 50 Hz (mem[6]).
func highpass_50Hz_at_12k8(signal []int16, lg int16, mem []int16) {
	y2Hi, y2Lo, y1Hi, y1Lo := mem[0], mem[1], mem[2], mem[3]
	x0, x1 := mem[4], mem[5]
	var x2 int16
	for i := int16(0); i < lg; i++ {
		lTmp1 := fxp_mac_16by16(y1Lo, 16211, 8192)
		lTmp1 = fxp_mac_16by16(y2Lo, -8021, lTmp1)
		lTmp2 := fxp_mul_16by16(y1Hi, 32422)
		lTmp2 = fxp_mac_16by16(y2Hi, -16042, lTmp2)
		x2 = x1
		x1 = x0
		x0 = signal[i]
		lTmp2 = fxp_mac_16by16(x2, 8106, lTmp2)
		lTmp2 = fxp_mac_16by16(x1, -16212, lTmp2)
		lTmp2 = fxp_mac_16by16(x0, 8106, lTmp2)

		lTmp1 = ((lTmp1 >> 14) + lTmp2) << 2
		y2Hi = y1Hi
		y2Lo = y1Lo
		y1Hi = int16(lTmp1 >> 16)
		y1Lo = int16((lTmp1 - (int32(y1Hi) << 16)) >> 1)
		signal[i] = amr_wb_shl1_round(lTmp1)
	}
	mem[0], mem[1], mem[2], mem[3] = y2Hi, y2Lo, y1Hi, y1Lo
	mem[4], mem[5] = x0, x1
}

// highpass_400Hz_at_12k8 is a 2nd-order IIR high-pass at 400 Hz (mem[6]);
// output is divided by 16.
func highpass_400Hz_at_12k8(signal []int16, lg int16, mem []int16) {
	y2Hi, y2Lo, y1Hi, y1Lo := mem[0], mem[1], mem[2], mem[3]
	x0, x1 := mem[4], mem[5]
	var x2 int16
	for i := int16(0); i < lg; i++ {
		lTmp1 := fxp_mac_16by16(y1Lo, 29280, 8192)
		lTmp2 := fxp_mul_16by16(y1Hi, 29280)
		lTmp1 = fxp_mac_16by16(y2Lo, -14160, lTmp1)
		lTmp2 = fxp_mac_16by16(y2Hi, -14160, lTmp2)
		x2 = x1
		x1 = x0
		x0 = signal[i]
		lTmp2 = fxp_mac_16by16(x2, 915, lTmp2)
		lTmp2 = fxp_mac_16by16(x1, -1830, lTmp2)
		lTmp2 = fxp_mac_16by16(x0, 915, lTmp2)

		lTmp1 = (lTmp1 >> 13) + (lTmp2 << 2)
		y2Hi = y1Hi
		y2Lo = y1Lo
		signal[i] = int16((lTmp1 + 0x00008000) >> 16)
		y1Hi = int16(lTmp1 >> 16)
		y1Lo = int16((lTmp1 - (int32(y1Hi) << 16)) >> 1)
	}
	mem[0], mem[1], mem[2], mem[3] = y2Hi, y2Lo, y1Hi, y1Lo
	mem[4], mem[5] = x0, x1
}

// fir_6k_7k is the 6-7 kHz band-pass FIR (gain 4.0), L_FIR taps.
var fir_6k_7k = [cL_FIR]int16{
	-32, 47, 32, -27,
	-369, 1122, -1421, 0,
	3798, -8880, 12349, -10984,
	3548, 7766, -18001,
	22118,
	-18001, 7766, 3548, -10984,
	12349, -8880, 3798, 0,
	-1421, 1122, -369, -27,
	32, 47,
}

// firBand6k7k31 is the 31-tap symmetric 6-7 kHz band-pass kernel. The hand-
// unrolled reference folds the equal endpoints (coef[0]==coef[30]==fir_6k_7k[0])
// as two -32 (<<5) terms; expressing it as a plain 31-tap FIR lets the SIMD
// kernel run it without changing the (non-saturating int32) arithmetic.
var firBand6k7k31 = func() [cL_FIR + 1]int16 {
	var c [cL_FIR + 1]int16
	copy(c[:cL_FIR], fir_6k_7k[:])
	c[cL_FIR] = fir_6k_7k[0]
	return c
}()

// band_pass_6k_7k band-pass filters in place; mem[L_FIR], x scratch[L_FIR+lg].
func band_pass_6k_7k(signal []int16, lg int16, mem, x []int16) {
	copy(x[:cL_FIR], mem[:cL_FIR])
	n := int(lg)
	for i := 0; i < n; i++ {
		x[cL_FIR+i] = signal[i] >> 2 // gain 1/4
	}
	var dstArr [cL_FRAME]int32
	var dst []int32
	if n <= cL_FRAME {
		dst = dstArr[:n]
	} else {
		dst = make([]int32, n)
	}
	firRaw(dst, x, firBand6k7k31[:])
	for i := 0; i < n; i++ {
		signal[i] = int16((0x4000 + dst[i]) >> 15)
	}
	copy(mem[:cL_FIR], x[n:n+cL_FIR])
}

// fir_7k is the 7 kHz low-pass FIR, L_FIR+1 taps.
var fir_7k = [cL_FIR + 1]int16{
	-21, 47, -89, 146, -203,
	229, -177, 0, 335, -839,
	1485, -2211, 2931, -3542, 3953,
	28682, 3953, -3542, 2931, -2211,
	1485, -839, 335, 0, -177,
	229, -203, 146, -89, 47,
	-21,
}

// low_pass_filt_7k low-pass filters in place; mem[L_FIR], x scratch[L_FIR+lg].
func low_pass_filt_7k(signal []int16, lg int16, mem, x []int16) {
	copy(x[:cL_FIR], mem[:cL_FIR])
	n := int(lg)
	for i := 0; i < n; i++ {
		x[cL_FIR+i] = signal[i]
	}
	// fir_7k is the full 31-tap symmetric kernel (fir_7k[0]==fir_7k[cL_FIR]),
	// so out = (0x4000 + sum_t x[i+t]*fir_7k[t]) >> 15 reproduces the folded
	// reference exactly (non-saturating int32 accumulation).
	var dstArr [cL_FRAME]int32
	var dst []int32
	if n <= cL_FRAME {
		dst = dstArr[:n]
	} else {
		dst = make([]int32, n)
	}
	firRaw(dst, x, fir_7k[:])
	for i := 0; i < n; i++ {
		signal[i] = int16((0x4000 + dst[i]) >> 15)
	}
	copy(mem[:cL_FIR], x[n:n+cL_FIR])
}

// agc2_amr_wb scales sig_out so its energy matches sig_in.
func agc2_amr_wb(sigIn, sigOut []int16, lTrm int16) {
	temp := sigOut[0] >> 2
	s := fxp_mul_16by16(temp, temp) << 1
	for i := int16(1); i < lTrm; i++ {
		temp = sigOut[i] >> 2
		s = mac_16by16_to_int32(s, temp, temp)
	}
	if s == 0 {
		return
	}
	exp := normalize_amr_wb(s) - 1
	gainOut := amr_wb_round(s << uint(exp))

	temp = sigIn[0] >> 2
	s = mul_16by16_to_int32(temp, temp)
	for i := int16(1); i < lTrm; i++ {
		temp = sigIn[i] >> 2
		s = mac_16by16_to_int32(s, temp, temp)
	}

	var g0 int16
	if s == 0 {
		g0 = 0
	} else {
		i := normalize_amr_wb(s)
		gainIn := amr_wb_round(s << uint(i))
		exp -= i
		s = int32(div_16by16(gainOut, gainIn))
		s = shl_int32(s, 7)
		s = shr_int32(s, exp)
		s = one_ov_sqrt(s)
		g0 = amr_wb_round(shl_int32(s, 9))
	}
	for i := int16(0); i < lTrm; i++ {
		sigOut[i] = extract_h(shl_int32(fxp_mul_16by16(sigOut[i], g0), 3))
	}
}

// voice_factor returns the voicing factor (-1 unvoiced .. 1 voiced) in Q15.
func voice_factor(exc []int16, qExc, gainPit int16, code []int16, gainCode, lSubfr int16) int16 {
	var exp int16
	ener1 := extract_h(dotProduct12(exc, exc, lSubfr, &exp))
	exp1 := sub_int16(exp, qExc<<1)
	lTmp := mul_16by16_to_int32(gainPit, gainPit)
	e := normalize_amr_wb(lTmp)
	tmp := int16((lTmp << uint(e)) >> 16)
	ener1 = mult_int16(ener1, tmp)
	exp1 -= e + 10

	var exp2 int16
	ener2 := extract_h(dotProduct12(code, code, lSubfr, &exp2))
	e = norm_s_oc(gainCode)
	tmp = shl_int16(gainCode, e)
	tmp = mult_int16(tmp, tmp)
	ener2 = mult_int16(ener2, tmp)
	exp2 -= e << 1

	i := exp1 - exp2
	if i >= 0 {
		ener1 >>= 1
		ener2 >>= uint(i + 1)
	} else {
		ener1 >>= uint(1 - i)
		ener2 >>= 1
	}

	tmp = ener1 - ener2
	ener1 += ener2 + 1
	if tmp >= 0 {
		tmp = div_16by16(tmp, ener1)
	} else {
		tmp = negate_int16(div_16by16(negate_int16(tmp), ener1))
	}
	return tmp
}

// weight_amrwb_lpc applies bandwidth expansion ap[i] = a[i] * gamma^i.
func weight_amrwb_lpc(a, ap []int16, gamma, m int16) {
	roundFactor := int32(0x00004000)
	ap[0] = a[0]
	fac := gamma
	var i int16
	for i = 1; i < m; i++ {
		ap[i] = int16(fxp_mac_16by16(a[i], fac, roundFactor) >> 15)
		fac = int16(fxp_mac_16by16(fac, gamma, roundFactor) >> 15)
	}
	ap[i] = int16(fxp_mac_16by16(a[i], fac, roundFactor) >> 15)
}
