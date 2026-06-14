package amrwb

// Pitch-analysis and target-update kernels, ported verbatim from the Apache-2.0
// vo-amrwbenc reference (gpclip.c, updt_tar.c, g_pitch.c, cor_h_x.c), derived
// from 3GPP TS 26.190.

const (
	cDIST_ISF_MAX   = 307   // 120 Hz
	cDIST_ISF_THRES = 154   // 60 Hz
	cGAIN_PIT_THRES = 14746 // 0.9 Q14
	cGAIN_PIT_MIN   = 9830  // 0.6 Q14
	cSTEP           = 4
)

// initGpClip initializes the pitch-gain clipping memory (Init_gp_clip).
func initGpClip(mem []int16) {
	mem[0] = cDIST_ISF_MAX
	mem[1] = cGAIN_PIT_MIN
}

// gpClip reports whether pitch-gain clipping should be applied (Gp_clip).
func gpClip(mem []int16) int16 {
	if mem[0] < cDIST_ISF_THRES && mem[1] > cGAIN_PIT_THRES {
		return 1
	}
	return 0
}

// gpClipTestIsf updates the ISF-distance clip memory (Gp_clip_test_isf).
func gpClipTestIsf(isf, mem []int16) {
	distMin := sub_int16(isf[1], isf[0])
	for i := 2; i < cM-1; i++ {
		d := sub_int16(isf[i], isf[i-1])
		if d < distMin {
			distMin = d
		}
	}
	dist := extract_h(L_mac((int32(26214)*int32(mem[0]))<<1, 6554, distMin))
	if dist > cDIST_ISF_MAX {
		dist = cDIST_ISF_MAX
	}
	mem[0] = dist
}

// gpClipTestGainPit updates the gain-pitch clip memory (Gp_clip_test_gain_pit).
func gpClipTestGainPit(gainPit int16, mem []int16) {
	lTmp := (int32(29491) * int32(mem[1])) << 1
	lTmp += (int32(3277) * int32(gainPit)) << 1
	gain := extract_h(lTmp)
	if gain < cGAIN_PIT_MIN {
		gain = cGAIN_PIT_MIN
	}
	mem[1] = gain
}

// Updt_tar updates the target vector: x2 = x - gain*y (Updt_tar).
func Updt_tar(x, x2, y []int16, gain, L int16) {
	for i := 0; i < int(L); i++ {
		lTmp := int32(x[i]) << 15
		lTmp -= (int32(y[i]) * int32(gain)) << 1
		x2[i] = extract_h(lShl2(lTmp, 1))
	}
}

// G_pitch computes the pitch gain (Q14, saturated to 1.2) and the correlation
// coefficients used by gain quantization (G_pitch).
func G_pitch(xn, y1, gCoeff []int16, lSubfr int16) int16 {
	var expXy, expYy int16
	xy := extract_h(encDotProduct12(xn, y1, lSubfr, &expXy))
	yy := extract_h(encDotProduct12(y1, y1, lSubfr, &expYy))

	gCoeff[0] = yy
	gCoeff[1] = expYy
	gCoeff[2] = xy
	gCoeff[3] = expXy

	if xy < 0 {
		return 0
	}
	xy >>= 1
	gain := div_s(xy, yy)
	i := expXy - expYy
	gain = shl(gain, i)
	if gain > 19661 {
		gain = 19661
	}
	return gain
}

// cor_h_x correlates the target x with the impulse response h, producing the
// normalized backward-filtered target dn[] for the codebook search (cor_h_x).
func cor_h_x(h, x, dn []int16) {
	var y32 [cL_SUBFR]int32
	var lMax [cSTEP]int32

	// corr[pos] = sum_{m=0}^{63-pos} x[pos+m]*h[m]. Zero-padding x past cL_SUBFR
	// makes the out-of-range taps contribute 0, so the whole set of positions is
	// one cL_SUBFR-tap FIR over the padded input — vectorisable via firRaw. The
	// reference doubles each product (<<1); 2*sum is bit-identical mod 2^32.
	var xpad [2 * cL_SUBFR]int16
	copy(xpad[:cL_SUBFR], x[:cL_SUBFR])
	var dst [cL_SUBFR]int32
	firRaw(dst[:], xpad[:], h[:cL_SUBFR])

	for pos := 0; pos < cL_SUBFR; pos++ {
		lTmp := int32(1) + (dst[pos] << 1)
		y32[pos] = lTmp
		if lTmp < 0 {
			lTmp = -lTmp
		}
		ph := pos & (cSTEP - 1)
		if lTmp > lMax[ph] {
			lMax[ph] = lTmp
		}
	}

	mx := (lMax[0] + lMax[1] + lMax[2] + lMax[3]) >> 2
	lTot := int32(1) + mx + (mx >> 1)

	j := norm_l(lTot) - 4
	for i := 0; i < cL_SUBFR; i++ {
		dn[i] = int16((L_shl(y32[i], j) + 0x8000) >> 16) // vo_round(L_shl(...))
	}
}

const cL_INTERPOL1 = 4

// inter4_1: 1/4-resolution interpolation filter for the closed-loop pitch.
var inter4_1Enc = [4][8]int16{
	{-12, 420, -1732, 5429, 13418, -1242, 73, 32},
	{-26, 455, -2142, 9910, 9910, -2142, 455, -26},
	{32, 73, -1242, 13418, 5429, -1732, 420, -12},
	{206, -766, 1376, 14746, 1376, -766, 206, 0},
}

func voMult(a, b int16) int16 { return int16((int32(a) * int32(b)) >> 15) }

// normCorr computes the normalized correlation between the target xn and the
// filtered past excitation, for lags t_min..t_max (Norm_Corr). corrV is indexed
// by (t - tMin). exc carries past excitation; excOff is the current position.
func normCorr(exc []int16, excOff int, xn, h []int16, tMin, tMax int16, corrV []int16) {
	excf := make([]int16, cL_SUBFR)
	k := -int(tMin)
	Convolve(exc[excOff+k:], h, excf)

	lTmp := firDot(xn[:64], xn[:64]) // sum_i xn[i]^2 (wrapping int32) via AVX2
	lTmp = (lTmp << 1) + 1
	e := int(norm_l(lTmp))
	e = 32 - e
	scale := -(e >> 1)

	for t := tMin; t <= tMax; t++ {
		l0 := firDot(xn[:64], excf) // <xn, excf>  (wrapping int32) via AVX2
		l1 := firDot(excf, excf)    // <excf, excf>
		l0 = (l0 << 1) + 1
		l1 = (l1 << 1) + 1

		ec := norm_l(l0)
		lt := l0 << uint(ec)
		expCorr := int(30 - ec)
		corr := extract_h(lt)

		en := norm_l(l1)
		lt = l1 << uint(en)
		expNorm := int16(30 - en)
		encIsqrtN(&lt, &expNorm)
		norm := extract_h(lt)

		lt = (int32(corr) * int32(norm)) << 1 // vo_L_mult
		l2 := expCorr + int(expNorm) + scale
		if l2 < 0 {
			lt = lt >> uint(-l2)
		} else {
			lt = lt << uint(l2)
		}
		corrV[int(t-tMin)] = int16((lt + 0x8000) >> 16) // vo_round

		if t != tMax {
			k = -(int(t) + 1)
			tmp := exc[excOff+k]
			for i := 63; i > 0; i-- {
				excf[i] = voMult(tmp, h[i]) + excf[i-1]
			}
			excf[0] = voMult(tmp, h[0])
		}
	}
}

// interpol4 interpolates corrV around pos at the given fraction (Interpol_4).
func interpol4(corrV []int16, pos int, frac int32) int16 {
	if frac < 0 {
		frac += cUP_SAMP
		pos--
	}
	pos = pos - cL_INTERPOL1 + 1
	k := cUP_SAMP - 1 - int(frac)
	var lSum int32
	for j := 0; j < 8; j++ {
		lSum += int32(corrV[pos+j]) * int32(inter4_1Enc[k][j]) // vo_mult32
	}
	return extract_h(L_add(lShl2(lSum, 2), 0x8000))
}

// Pitch_fr4 performs the closed-loop pitch search with 1/4-sample resolution
// (Pitch_fr4). Returns the integer lag; *pitFrac receives the fraction (0..3).
func Pitch_fr4(exc []int16, excOff int, xn, h []int16, t0Min, t0Max int16,
	pitFrac *int16, iSubfr, t0Fr2, t0Fr1, lSubfr int16) int16 {

	tMin := t0Min - cL_INTERPOL1
	tMax := t0Max + cL_INTERPOL1
	corrV := make([]int16, 40)
	normCorr(exc, excOff, xn, h, tMin, tMax, corrV)
	// corr[t] == corrV[t - tMin].
	cmax := corrV[int(t0Min-tMin)]
	t0 := t0Min
	for i := t0Min + 1; i <= t0Max; i++ {
		if corrV[int(i-tMin)] >= cmax {
			cmax = corrV[int(i-tMin)]
			t0 = i
		}
	}
	if iSubfr == 0 && t0 >= t0Fr1 {
		*pitFrac = 0
		return t0
	}

	step := int32(1)
	fraction := int32(-3)
	if t0Fr2 == cPIT_MIN || (iSubfr == 0 && t0 >= t0Fr2) {
		step = 2
		fraction = -2
	}
	if t0 == t0Min {
		fraction = 0
	}
	pos := int(t0 - tMin)
	cmax = interpol4(corrV, pos, fraction)
	for i := fraction + step; i <= 3; i += step {
		temp := interpol4(corrV, pos, i)
		if temp > cmax {
			cmax = temp
			fraction = i
		}
	}
	if fraction < 0 {
		fraction += cUP_SAMP
		t0--
	}
	*pitFrac = int16(fraction)
	return t0
}
