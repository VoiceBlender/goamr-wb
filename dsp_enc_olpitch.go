package amrwb

// Open-loop pitch analysis and weighted-speech high-pass, ported verbatim from
// the Apache-2.0 vo-amrwbenc reference (hp_wsp.c, p_med_ol.c), derived from
// 3GPP TS 26.190. State-coupled inputs are passed explicitly rather than via
// the Coder_State struct.

// hp_wsp filter coefficients (Q12).
var hpWspA = [4]int16{8192, 21663, -19258, 5734}
var hpWspB = [4]int16{-3432, 10280, -10280, 3432}

// Hp_wsp high-pass filters the weighted speech (mem is 9 words).
func Hp_wsp(wsp, hpWsp []int16, lg int16, mem []int16) {
	y3Hi, y3Lo := mem[0], mem[1]
	y2Hi, y2Lo := mem[2], mem[3]
	y1Hi, y1Lo := mem[4], mem[5]
	x0, x1, x2 := mem[6], mem[7], mem[8]

	for i := 0; i < int(lg); i++ {
		x3 := x2
		x2 = x1
		x1 = x0
		x0 = wsp[i]

		lTmp := int32(16384)
		lTmp += (int32(y1Lo) * int32(hpWspA[1])) << 1
		lTmp += (int32(y2Lo) * int32(hpWspA[2])) << 1
		lTmp += (int32(y3Lo) * int32(hpWspA[3])) << 1
		lTmp >>= 15
		lTmp += (int32(y1Hi) * int32(hpWspA[1])) << 1
		lTmp += (int32(y2Hi) * int32(hpWspA[2])) << 1
		lTmp += (int32(y3Hi) * int32(hpWspA[3])) << 1
		lTmp += (int32(x0) * int32(hpWspB[0])) << 1
		lTmp += (int32(x1) * int32(hpWspB[1])) << 1
		lTmp += (int32(x2) * int32(hpWspB[2])) << 1
		lTmp += (int32(x3) * int32(hpWspB[3])) << 1
		lTmp <<= 2

		y3Hi, y3Lo = y2Hi, y2Lo
		y2Hi, y2Lo = y1Hi, y1Lo
		y1Hi = int16(lTmp >> 16)
		y1Lo = int16((lTmp & 0xffff) >> 1)
		hpWsp[i] = int16((lTmp + 0x4000) >> 15)
	}
	mem[0], mem[1] = y3Hi, y3Lo
	mem[2], mem[3] = y2Hi, y2Lo
	mem[4], mem[5] = y1Hi, y1Lo
	mem[6], mem[7], mem[8] = x0, x1, x2
}

// scaleMemHpWsp rescales the Hp_wsp memory by 2^exp (scale_mem_Hp_wsp).
func scaleMemHpWsp(mem []int16, exp int16) {
	for i := 0; i < 6; i += 2 {
		lTmp := (int32(mem[i]) << 16) + (int32(mem[i+1]) << 1)
		lTmp = L_shl(lTmp, exp)
		mem[i] = int16(lTmp >> 16)
		mem[i+1] = int16((lTmp & 0xffff) >> 1)
	}
	for i := 6; i < 9; i++ {
		lTmp := L_deposit_h(mem[i])
		lTmp = L_shl(lTmp, exp)
		mem[i] = int16((lTmp + 0x8000) >> 16)
	}
}

// medOlag returns the median of the 5 previous open-loop lags (Med_olag).
func medOlag(prevOlLag int16, oldOlLag []int16) int16 {
	for i := 4; i > 0; i-- {
		oldOlLag[i] = oldOlLag[i-1]
	}
	oldOlLag[0] = prevOlLag
	return median5(oldOlLag, 2)
}

// Pitch_med_ol computes the open-loop pitch lag over the (decimated) weighted
// speech. wsp carries history before wspOff (wsp[wspOff-115 .. wspOff-1]).
// oldHpWsp has length 115+lFrame; hpWspMem is 9 words. *olGain receives the
// normalized correlation of the chosen lag.
func Pitch_med_ol(wsp []int16, wspOff int, oldT0Med int16, olGain *int16,
	hpWspMem, oldHpWsp []int16, olWghtFlg, lFrame int16) int16 {

	const lMin = 17
	const lMax = 115
	l0 := oldT0Med

	wwIdx := 198
	weIdx := 98 + lMax - int(l0)

	// Open-loop correlation r0(i) = 2·sum_j wsp[wspOff+j]·wsp[wspOff-i+j] over the
	// candidate lags. With the fixed window coef = wsp[wspOff:wspOff+lFrame] this
	// is one sliding FIR over wsp[wspOff-lMax:], so dst[lMax-i] = r0(i)>>1. firRaw
	// (AVX2) vectorises it; the wrapping int32 accumulation is bit-identical and
	// the per-term <<1 distributes over the sum.
	var dstArr [lMax - lMin]int32
	dst := dstArr[:]
	firRaw(dst, wsp[wspOff-lMax:], wsp[wspOff:wspOff+int(lFrame)])

	max := int32(min32)
	Tm := int16(0)
	for i := int16(lMax); i > lMin; i-- {
		r0 := dst[lMax-int(i)] << 1
		hi := int16(r0 >> 16)
		lo := int16((r0 & 0xffff) >> 1)
		r0 = mpy3216(hi, lo, corrweight[wwIdx])
		wwIdx--
		if l0 > 0 && olWghtFlg > 0 {
			hi = int16(r0 >> 16)
			lo = int16((r0 & 0xffff) >> 1)
			r0 = mpy3216(hi, lo, corrweight[weIdx])
			weIdx--
		}
		if r0 >= max {
			max = r0
			Tm = i
		}
	}

	// High-pass the wsp[] vector into oldHpWsp[lMax:].
	Hp_wsp(wsp[wspOff:], oldHpWsp[lMax:], lFrame, hpWspMem)

	// Normalized correlation at delay Tm — three dot products over the high-passed
	// signal, vectorised via firDot (wrapping int32, bit-identical).
	p1 := lMax
	p2 := lMax - int(Tm)
	a1 := oldHpWsp[p1 : p1+int(lFrame)]
	a2 := oldHpWsp[p2 : p2+int(lFrame)]
	r2 := firDot(a1, a1)
	r1 := firDot(a2, a2)
	r0 := firDot(a1, a2)
	r0 <<= 1
	r1 = (r1 << 1) + 1
	r2 = (r2 << 1) + 1

	expR0 := norm_l(r0)
	r0 <<= uint(expR0)
	expR1 := norm_l(r1)
	r1 <<= uint(expR1)
	expR2 := norm_l(r2)
	r2 <<= uint(expR2)

	r1 = (int32(voRound(r1)) * int32(voRound(r2))) << 1 // vo_L_mult(vo_round,vo_round)
	ii := norm_l(r1)
	r1 <<= uint(ii)
	expR1 += expR2
	expR1 += ii
	expR1 = 62 - expR1
	encIsqrtN(&r1, &expR1)

	r0 = (int32(round_(r0)) * int32(round_(r1))) << 1 // vo_L_mult(voround,voround)
	expR0 = 31 - expR0
	expR0 += expR1
	*olGain = voRound(L_shl(r0, expR0))

	for i := 0; i < lMax; i++ {
		oldHpWsp[i] = oldHpWsp[i+int(lFrame)]
	}
	return Tm
}
