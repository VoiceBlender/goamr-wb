package amrwb

// Gain dequantization, ported verbatim from the Apache-2.0 opencore-amrwb
// reference (dec_gain2_amr_wb.cpp), derived from 3GPP TS 26.173. Reconstructs
// the pitch gain (Q14) and code gain (Q16) from the transmitted index, with a
// 4th-order MA energy predictor and frame-erasure concealment.

const (
	cMEAN_ENER  = 30
	cPRED_ORDER = 4
	cL_LTPHIST  = 5
)

var pdownUnusable = [7]int16{32767, 31130, 29491, 24576, 7537, 1638, 328}
var cdownUnusable = [7]int16{32767, 16384, 8192, 8192, 8192, 4915, 3277}
var pdownUsable = [7]int16{32767, 32113, 31457, 24576, 7537, 1638, 328}
var cdownUsable = [7]int16{32767, 32113, 32113, 32113, 32113, 32113, 22938}

// pred holds the MA prediction coefficients {0.5,0.4,0.3,0.2} in Q13.
var predGain = [cPRED_ORDER]int16{4096, 3277, 2458, 1638}

// decGain2Init initializes the 23-word gain-decoder memory.
func decGain2Init(mem []int16) {
	mem[0] = -14336
	mem[1] = -14336
	mem[2] = -14336
	mem[3] = -14336
	for i := 4; i < 4+18; i++ {
		mem[i] = 0
	}
	mem[22] = 21845
}

// dec_gain2_amr_wb dequantizes pitch gain (Q14) and code gain (Q16).
// mem layout: [0..3]=past_qua_en, [4]=past_gain_pit, [5]=past_gain_code,
// [6]=prev_gc, [7..11]=pbuf, [12..16]=gbuf, [17..21]=pbuf2, [22]=seed.
func dec_gain2_amr_wb(index, nbits int16, code []int16, lSubfr int16,
	gainPit *int16, gainCod *int32, bfi, prevBfi, state, unusableFrame, vadHist int16,
	mem []int16) {

	pastQuaEn := mem[0:4]
	pastGainPit := &mem[4]
	pastGainCode := &mem[5]
	prevGc := &mem[6]
	pbuf := mem[7:12]
	gbuf := mem[12:17]
	pbuf2 := mem[17:22]

	var exp, frac, gcode0, expGcode0, quaEner, gcodeInov int16
	var tmp, tmp1, tmp2, gCode int16
	var lTmp int32

	lTmp = dotProduct12(code, code, lSubfr, &exp)
	exp -= 24
	one_ov_sqrt_norm(&lTmp, &exp)
	gcodeInov = extract_h(shl_int32(lTmp, exp-3)) // Q12

	if bfi != 0 {
		tmp = median5(pbuf, 2)
		*pastGainPit = tmp
		if *pastGainPit > 15565 {
			*pastGainPit = 15565
		}
		if unusableFrame != 0 {
			*gainPit = mult_int16(pdownUnusable[state], *pastGainPit)
		} else {
			*gainPit = mult_int16(pdownUsable[state], *pastGainPit)
		}
		tmp = median5(gbuf, 2)
		if vadHist > 2 {
			*pastGainCode = tmp
		} else if unusableFrame != 0 {
			*pastGainCode = mult_int16(cdownUnusable[state], tmp)
		} else {
			*pastGainCode = mult_int16(cdownUsable[state], tmp)
		}

		tmp = pastQuaEn[3]
		tmp1 = pastQuaEn[2]
		lTmp = int32(tmp)
		lTmp += int32(tmp1)
		pastQuaEn[3] = tmp
		tmp = pastQuaEn[1]
		tmp1 = pastQuaEn[0]
		lTmp += int32(tmp)
		lTmp += int32(tmp1)
		pastQuaEn[2] = tmp
		quaEner = int16(lTmp >> 3)
		pastQuaEn[1] = tmp1

		quaEner -= 3072
		if quaEner < -14336 {
			quaEner = -14336
		}
		pastQuaEn[0] = quaEner

		for i := int16(1); i < 5; i++ {
			gbuf[i-1] = gbuf[i]
			pbuf[i-1] = pbuf[i]
		}
		gbuf[4] = *pastGainCode
		pbuf[4] = *pastGainPit

		*gainCod = mul_16by16_to_int32(*pastGainCode, gcodeInov)
		return
	}

	lTmp = l_deposit_h(cMEAN_ENER)
	lTmp = shl_int32(lTmp, 8) // Q16 -> Q24
	lTmp = mac_16by16_to_int32(lTmp, predGain[0], pastQuaEn[0])
	lTmp = mac_16by16_to_int32(lTmp, predGain[1], pastQuaEn[1])
	lTmp = mac_16by16_to_int32(lTmp, predGain[2], pastQuaEn[2])
	lTmp = mac_16by16_to_int32(lTmp, predGain[3], pastQuaEn[3])

	gcode0 = extract_h(lTmp) // Q24 -> Q8

	lTmp = (int32(gcode0) * 5443) >> 7 // *0.166096 Q15 -> Q24
	int32ToDpf(lTmp, &expGcode0, &frac)
	gcode0 = int16(power_of_2(14, frac))
	expGcode0 -= 14

	var p []int16
	if nbits == 6 {
		p = t_qua_gain6b[index<<1:]
	} else {
		p = t_qua_gain7b[index<<1:]
	}
	*gainPit = p[0] // Q14
	gCode = p[1]    // Q11

	lTmp = mul_16by16_to_int32(gCode, gcode0) // Q11*Q0 -> Q12
	lTmp = shl_int32(lTmp, expGcode0+4)       // Q12 -> Q16
	*gainCod = lTmp

	if prevBfi == 1 {
		lTmp = mul_16by16_to_int32(*prevGc, 5120)
		if (*gainCod > lTmp) && (*gainCod > 6553600) {
			*gainCod = lTmp
		}
	}
	*pastGainCode = amr_wb_round(shl_int32(*gainCod, 3))
	*pastGainPit = *gainPit

	*prevGc = *pastGainCode
	tmp = gbuf[1]
	tmp1 = pbuf[1]
	tmp2 = pbuf2[1]
	for i := int16(1); i < 5; i++ {
		gbuf[i-1] = tmp
		pbuf[i-1] = tmp1
		pbuf2[i-1] = tmp2
		tmp = gbuf[i]
		tmp1 = pbuf[i]
		tmp2 = pbuf2[i]
	}
	gbuf[4] = *pastGainCode
	pbuf[4] = *pastGainPit
	pbuf2[4] = *pastGainPit

	int32ToDpf(*gainCod, &exp, &frac)
	lTmp = mul_32by16(exp, frac, gcodeInov)
	*gainCod = shl_int32(lTmp, 3) // gcode_inov in Q12

	pastQuaEn[3] = pastQuaEn[2]
	pastQuaEn[2] = pastQuaEn[1]
	pastQuaEn[1] = pastQuaEn[0]

	lTmp = int32(gCode)
	amrwbLog2(lTmp, &exp, &frac)
	exp -= 11
	lTmp = mul_32by16(exp, frac, 24660) // x 6.0206 in Q12
	pastQuaEn[0] = int16(lTmp >> 3)     // Q10
}
