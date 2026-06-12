package amrwb

// Gain quantization (encode side) and the encoder Log2, ported verbatim from
// the Apache-2.0 vo-amrwbenc reference (q_gain2.c, log2.c), derived from 3GPP
// TS 26.190. Reuses t_qua_gain6b/7b (tables_gain.go), predGain (dsp_gain.go),
// and log2NormTable (dsp_mathops.go).

const (
	cRANGE         = 64
	cNB_QUA_GAIN7B = 128
)

// encLog2Norm: log2 of a normalized value (Log2_norm, non-saturating vo_L_msu).
func encLog2Norm(lX int32, exp int16, exponent, fraction *int16) {
	if lX <= 0 {
		*exponent = 0
		*fraction = 0
		return
	}
	*exponent = 30 - exp
	lX >>= 9
	i := extract_h(lX)
	lX >>= 1
	a := int16(lX) & 0x7fff
	i -= 32
	lY := L_deposit_h(log2NormTable[i])
	tmp := log2NormTable[i] - log2NormTable[i+1] // vo_sub
	lY = lY - ((int32(tmp) * int32(a)) << 1)     // vo_L_msu
	*fraction = extract_h(lY)
}

// encLog2 computes log2(L_x) (Log2).
func encLog2(lX int32, exponent, fraction *int16) {
	exp := norm_l(lX)
	encLog2Norm(lX<<uint(exp), exp, exponent, fraction)
}

// initQGain2 initializes the gain-quantizer energy predictor memory (4 words).
func initQGain2(mem []int16) {
	for i := 0; i < cPRED_ORDER; i++ {
		mem[i] = -14336
	}
}

// Q_gain2 quantizes the pitch and code gains and returns the index. Updates
// *gainPit (Q14), *gainCod (Q16), and the predictor memory mem (4 words).
func Q_gain2(xn, y1 []int16, qXn int16, y2, code, gCoeff []int16, lSubfr, nbits int16,
	gainPit *int16, gainCod *int32, gpClipFlag int16, mem []int16) int16 {

	pastQuaEn := mem
	var minInd, size int16
	var tQua []int16

	if nbits == 6 {
		tQua = t_qua_gain6b
		minInd = 0
		size = cRANGE
		if gpClipFlag == 1 {
			size -= 16
		}
	} else {
		tQua = t_qua_gain7b
		j := int16(cNB_QUA_GAIN7B - cRANGE)
		if gpClipFlag == 1 {
			j -= 27
		}
		minInd = 0
		gP := *gainPit
		for i := int16(0); i < j; i++ {
			if gP > t_qua_gain7b[cRANGE+i*2] {
				minInd++
			}
		}
		size = cRANGE
	}

	var coeff, coeffLo, expCoeff [5]int16
	var exp int16

	coeff[0] = gCoeff[0]
	expCoeff[0] = gCoeff[1]
	coeff[1] = negate(gCoeff[2])
	expCoeff[1] = gCoeff[3] + 1

	coeff[2] = extract_h(encDotProduct12(y2, y2, lSubfr, &exp))
	expCoeff[2] = (exp - 18) + (qXn << 1)

	coeff[3] = extract_h(L_negate(encDotProduct12(xn, y2, lSubfr, &exp)))
	expCoeff[3] = (exp - 8) + qXn

	coeff[4] = extract_h(encDotProduct12(y1, y2, lSubfr, &exp))
	expCoeff[4] = (exp - 8) + qXn

	var expCode int16
	lTmp := encDotProduct12(code, code, lSubfr, &expCode)
	expCode = expCode - (18 + 6 + 31)

	var frac int16
	encLog2(lTmp, &exp, &frac)
	exp += expCode
	lTmp = mpy3216(exp, frac, -24660)
	lTmp += (int32(cMEAN_ENER) * 8192) << 1

	lTmp = lTmp << 10
	lTmp += (int32(predGain[0]) * int32(pastQuaEn[0])) << 1
	lTmp += (int32(predGain[1]) * int32(pastQuaEn[1])) << 1
	lTmp += (int32(predGain[2]) * int32(pastQuaEn[2])) << 1
	lTmp += (int32(predGain[3]) * int32(pastQuaEn[3])) << 1
	gcode0 := extract_h(lTmp)

	lTmp = (int32(gcode0) * 5443) << 1 // vo_L_mult
	lTmp >>= 8
	var expGcode0 int16
	expGcode0, frac = voLExtract(lTmp)
	gcode0 = int16(encPow2(14, frac))
	expGcode0 -= 14

	expCode = expGcode0 + 4
	var expMax [5]int16
	expMax[0] = expCoeff[0] - 13
	expMax[1] = expCoeff[1] - 14
	expMax[2] = expCoeff[2] + (15 + (expCode << 1))
	expMax[3] = expCoeff[3] + expCode
	expMax[4] = expCoeff[4] + (1 + expCode)

	eMax := expMax[0]
	for i := 1; i < 5; i++ {
		if expMax[i] > eMax {
			eMax = expMax[i]
		}
	}

	for i := 0; i < 5; i++ {
		j := (eMax - expMax[i]) + 2
		lt := L_deposit_h(coeff[i])
		lt = L_shr(lt, j)
		coeff[i], coeffLo[i] = voLExtract(lt)
		coeffLo[i] = coeffLo[i] >> 3
	}

	distMin := max32
	pIdx := int(minInd) << 1
	index := int16(0)
	for i := int16(0); i < size; i++ {
		gPitch := tQua[pIdx]
		gCode := tQua[pIdx+1]
		pIdx += 2

		gCode = int16(((int32(gCode) * int32(gcode0)) + 0x4000) >> 15)
		g2Pitch := int16(((int32(gPitch) * int32(gPitch)) + 0x4000) >> 15)
		gPitCod := int16(((int32(gCode) * int32(gPitch)) + 0x4000) >> 15)
		lt := (int32(gCode) * int32(gCode)) << 1
		g2Code, g2CodeLo := voLExtract(lt)

		lt = (int32(coeff[2]) * int32(g2CodeLo)) << 1
		lt >>= 3
		lt += (int32(coeffLo[0]) * int32(g2Pitch)) << 1
		lt += (int32(coeffLo[1]) * int32(gPitch)) << 1
		lt += (int32(coeffLo[2]) * int32(g2Code)) << 1
		lt += (int32(coeffLo[3]) * int32(gCode)) << 1
		lt += (int32(coeffLo[4]) * int32(gPitCod)) << 1
		lt >>= 12
		lt += (int32(coeff[0]) * int32(g2Pitch)) << 1
		lt += (int32(coeff[1]) * int32(gPitch)) << 1
		lt += (int32(coeff[2]) * int32(g2Code)) << 1
		lt += (int32(coeff[3]) * int32(gCode)) << 1
		lt += (int32(coeff[4]) * int32(gPitCod)) << 1

		if lt < distMin {
			distMin = lt
			index = i
		}
	}

	index += minInd
	pIdx = int(index) << 1
	*gainPit = tQua[pIdx]
	gCode := tQua[pIdx+1]

	lTmp = (int32(gCode) * int32(gcode0)) << 1 // vo_L_mult
	lTmp = L_shl(lTmp, expGcode0+4)
	*gainCod = lTmp

	lTmp = L_deposit_l(gCode)
	encLog2(lTmp, &exp, &frac)
	exp -= 11
	lTmp = mpy3216(exp, frac, 24660)
	quaEner := int16(lTmp >> 3)

	pastQuaEn[3] = pastQuaEn[2]
	pastQuaEn[2] = pastQuaEn[1]
	pastQuaEn[1] = pastQuaEn[0]
	pastQuaEn[0] = quaEner

	return index
}
