package amrwb

// High-band gain analysis for the 23.85 kbit/s mode, ported verbatim from the
// Apache-2.0 vo-amrwbenc reference (voAMRWBEnc.c synthesis()), derived from 3GPP
// TS 26.190. Synthesizes the 12.8 kHz speech for the subframe, generates a
// shaped 6-7 kHz high band from noise, compares its energy to the original
// 16 kHz signal's high band, and quantizes the correction gain to a 4-bit index
// (returned). Cross-frame HF memories live in coderState.
func (st *coderState) synthesis(aq, exc []int16, qNew int16, synth16k []int16) int16 {
	synthHi := st.synScHi[:]
	synthLo := st.synScLo[:]
	synth := st.synScSyn[:]
	hf := st.synScHf[:]
	hfSp := st.synScHfSp[:]
	ap := st.synScAp[:]

	// Speech synthesis at 12.8 kHz, fixed deemphasis and 50 Hz HP filtering.
	copy(synthHi[:cM], st.memSynHi[:])
	copy(synthLo[:cM], st.memSynLo[:])
	Syn_filt_32(aq, cM, exc, qNew, synthHi, synthLo, cM, cL_SUBFR)
	copy(st.memSynHi[:], synthHi[cL_SUBFR:cL_SUBFR+cM])
	copy(st.memSynLo[:], synthLo[cL_SUBFR:cL_SUBFR+cM])
	Deemph_32(synthHi[cM:], synthLo[cM:], synth, cPREEMPH_FAC, cL_SUBFR, &st.memDeemph)
	HP50_12k8(synth, cL_SUBFR, st.memSigOut[:])

	// Original speech as reference for high-band gain quantization.
	for i := 0; i < cL_SUBFR16k; i++ {
		hfSp[i] = synth16k[i]
	}

	// White-noise vector.
	for i := 0; i < cL_SUBFR16k; i++ {
		hf[i] = Random(&st.seed2) >> 3
	}

	// Energy of excitation.
	Scale_sig(exc, cL_SUBFR, -3)
	qNew -= 3
	var expEner int16
	ener := extract_h(encDotProduct12(exc, exc, cL_SUBFR, &expEner))
	expEner -= qNew + qNew
	var exp int16
	tmp := extract_h(encDotProduct12(hf, hf, cL_SUBFR16k, &exp))
	if tmp > ener {
		tmp >>= 1
		exp++
	}
	lTmp := L_deposit_h(div_s(tmp, ener))
	exp -= expEner
	encIsqrtN(&lTmp, &exp)
	lTmp = L_shl(lTmp, exp+1) // x2, Q31
	tmp = extract_h(lTmp)     // 2 x sqrt(ener_exc/ener_hf)
	for i := 0; i < cL_SUBFR16k; i++ {
		hf[i] = voMult(hf[i], tmp)
	}

	// Tilt of synthesis speech (1=voiced, -1=unvoiced).
	HP400_12k8(synth, cL_SUBFR, st.memHp400[:])
	lTmp = 1
	for i := 0; i < cL_SUBFR; i++ {
		lTmp += (int32(synth[i]) * int32(synth[i])) << 1
	}
	exp = norm_l(lTmp)
	ener = extract_h(lTmp << uint(exp)) // r[0]
	lTmp = 1
	for i := 1; i < cL_SUBFR; i++ {
		lTmp += (int32(synth[i]) * int32(synth[i-1])) << 1
	}
	tmp = extract_h(lTmp << uint(exp)) // r[1]
	var fac int16
	if tmp > 0 {
		fac = div_s(tmp, ener)
	}

	// Modify white-noise energy according to synthesis tilt.
	gain1 := int16(32767) - fac
	gain2 := voMult(gain1, 20480)
	gain2 = shl(gain2, 1)
	var weight1, weight2 int16
	if st.vadHist > 0 {
		weight1, weight2 = 0, 32767
	} else {
		weight1, weight2 = 32767, 0
	}
	tmp = voMult(weight1, gain1)
	tmp = tmp + voMult(weight2, gain2)
	if tmp != 0 {
		tmp++
	}
	hpEstGain := tmp
	if hpEstGain < 3277 {
		hpEstGain = 3277 // 0.1 in Q15
	}

	// Synthesis of noise band: 4.8-5.6 kHz -> 6-7 kHz.
	Weight_a(aq, ap, 19661, cM) // fac = 0.6
	Syn_filt(ap, hf, hf, cL_SUBFR16k, st.memSynHf[:], 1)
	Filt_6k_7k(hf, cL_SUBFR16k, st.memHf[:])
	Filt_6k_7k(hfSp, cL_SUBFR16k, st.memHf2[:])

	// Check the gain difference vs the original signal's high band.
	Scale_sig(hfSp, cL_SUBFR16k, -1)
	ener = extract_h(encDotProduct12(hfSp, hfSp, cL_SUBFR16k, &expEner))
	tmp = extract_h(encDotProduct12(hf, hf, cL_SUBFR16k, &exp))
	if tmp > ener {
		tmp >>= 1
		exp++
	}
	lTmp = L_deposit_h(div_s(tmp, ener))
	exp -= expEner
	encIsqrtN(&lTmp, &exp)
	lTmp = L_shl(lTmp, exp) // Q31
	hpCalcGain := extract_h(lTmp)

	// st.gainAlpha *= dtxHangoverCount/7
	lTmp = (int32(st.dtxEncSt.dtxHangoverCount) * 4681 << 1) << 15
	st.gainAlpha = voMult(st.gainAlpha, extract_h(lTmp))
	if st.dtxEncSt.dtxHangoverCount > 6 {
		st.gainAlpha = 32767
	}
	hpEstGain >>= 1 // Q15 -> Q14
	hpCorrGain := voMult(hpCalcGain, st.gainAlpha) + voMult(int16(32767)-st.gainAlpha, hpEstGain)

	// Quantize the correction gain.
	distMin := int16(32767)
	hpGainInd := int16(0)
	for i := 0; i < 16; i++ {
		d := hpCorrGain - hpGain[i]
		dist := voMult(d, d)
		if distMin > dist {
			distMin = dist
			hpGainInd = int16(i)
		}
	}
	return hpGainInd
}
