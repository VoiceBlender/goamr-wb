package amrwb

// Per-subframe synthesis driver, ported verbatim from the Apache-2.0
// opencore-amrwb reference (synthesis_amr_wb.cpp), derived from 3GPP TS 26.173.
// Produces the 16 kHz synthesis: LP synthesis at 12.8 kHz -> deemphasis -> hp50
// -> oversample to 16 kHz, plus a synthesized 6-7 kHz high band added on top.
//
// The C threads one shared ScratchMem through every sub-call; the Go port gives
// each sub-call its own scratch slice (equivalent, since each is only live for
// its own call) and uses named local buffers for the synthesis vectors.

// HP_gain: HF correction gains for the 23.85 kbit/s mode.
var hpGain = [16]int16{
	3624, 4673, 5597, 6479, 7425, 8378, 9324, 10264,
	11210, 12206, 13391, 14844, 16770, 19655, 24289, 32728,
}

func synthesisAmrWb(aq, exc []int16, qNew int16, synth16k []int16, corrGain int16,
	hfIsf []int16, nbBits, newDTXState int16, st *decoderStateDSP, bfi int16) {

	synthHi := make([]int16, cM+cL_SUBFR)
	synthLo := make([]int16, cM+cL_SUBFR)
	synth := make([]int16, cL_SUBFR)
	hf := make([]int16, cL_SUBFR16k)
	ap := make([]int16, cM16k+1)
	hfA := make([]int16, cM16k+1)

	copy(synthHi[:cM], st.memSynHi[:])
	copy(synthLo[:cM], st.memSynLo[:])

	Syn_filt_32(aq, cM, exc, qNew, synthHi, synthLo, cM, cL_SUBFR)

	copy(st.memSynHi[:], synthHi[cL_SUBFR:cL_SUBFR+cM])
	copy(st.memSynLo[:], synthLo[cL_SUBFR:cL_SUBFR+cM])

	deemphasis_32(synthHi[cM:], synthLo[cM:], synth, cPREEMPH_FAC, cL_SUBFR, &st.memDeemph)

	highpass_50Hz_at_12k8(synth, cL_SUBFR, st.memSigOut[:])

	oversampScratch := make([]int16, 2*nbCoefUp+cL_SUBFR)
	oversamp_12k8_to_16k(synth, cL_SUBFR, synth16k, st.memOversamp[:], oversampScratch)

	// Generate white-noise HF vector.
	for i := 0; i < cL_SUBFR16k; i++ {
		hf[i] = noise_gen_amrwb(&st.seed2) >> 3
	}

	// Scale excitation down to compute its energy.
	for i := 0; i < cL_SUBFR; i++ {
		exc[i] = add_int16(exc[i], 0x0004) >> 3
	}
	qNew -= 3

	var expEner int16
	ener := extract_h(dotProduct12(exc, exc, cL_SUBFR, &expEner))
	expEner -= qNew << 1

	var exp int16
	tmp := extract_h(dotProduct12(hf, hf, cL_SUBFR16k, &exp))
	if tmp > ener {
		tmp >>= 1
		exp++
	}
	lTmp := l_deposit_h(div_16by16(tmp, ener))
	exp -= expEner
	one_ov_sqrt_norm(&lTmp, &exp)
	lTmp = shl_int32(lTmp, exp+1)
	tmp = int16(lTmp >> 16) // 2 x sqrt(ener_exc/ener_hf)

	for i := 0; i < cL_SUBFR16k; i++ {
		hf[i] = int16(fxp_mul_16by16(hf[i], tmp) >> 15)
	}

	// Find tilt of synthesis speech.
	highpass_400Hz_at_12k8(synth, cL_SUBFR, st.memHp400[:])

	lTmpA := int32(1)
	lTmpB := int32(1)
	lTmpA = mac_16by16_to_int32(lTmpA, synth[0], synth[0])
	for i := 1; i < cL_SUBFR; i++ {
		lTmpA = mac_16by16_to_int32(lTmpA, synth[i], synth[i])
		lTmpB = mac_16by16_to_int32(lTmpB, synth[i], synth[i-1])
	}
	exp = normalize_amr_wb(lTmpA)
	ener = int16((lTmpA << uint(exp)) >> 16)
	tmp = int16((lTmpB << uint(exp)) >> 16)
	var fac int16
	if tmp > 0 {
		fac = div_16by16(tmp, ener)
	} else {
		fac = 0
	}

	gain1 := int16(32767) - fac
	gain2 := mult_int16(gain1, 20480)
	gain2 = shl_int16(gain2, 1)
	if st.vadHist > 0 {
		tmp = gain2 - 1
	} else {
		tmp = gain1 - 1
	}
	if tmp != 0 {
		tmp++
	}
	if tmp < 3277 {
		tmp = 3277
	}

	if nbBits >= cNBBITS_24k && bfi == 0 {
		hfCorrGain := hpGain[corrGain]
		for i := 0; i < cL_SUBFR16k; i++ {
			hf[i] = mult_int16(hf[i], hfCorrGain) << 1
		}
	} else {
		for i := 0; i < cL_SUBFR16k; i++ {
			hf[i] = mult_int16(hf[i], tmp)
		}
	}

	yBuf := make([]int16, cM16k+cL_SUBFR16k)
	if nbBits <= cNBBITS_7k && newDTXState == cSPEECH {
		isf_extrapolation(hfIsf)
		Isp_Az(hfIsf, hfA, cM16k, 0)
		weight_amrwb_lpc(hfA, ap, 29491, cM16k)
		wb_syn_filt(ap, cM16k, hf, hf, cL_SUBFR16k, st.memSynHf[:], 1, yBuf)
	} else {
		weight_amrwb_lpc(aq, ap, 19661, cM)
		wb_syn_filt(ap, cM, hf, hf, cL_SUBFR16k, st.memSynHf[cM16k-cM:], 1, yBuf)
	}

	bpScratch := make([]int16, cL_FIR+cL_SUBFR16k)
	band_pass_6k_7k(hf, cL_SUBFR16k, st.memHf[:], bpScratch)

	if nbBits >= cNBBITS_24k {
		lpScratch := make([]int16, cL_FIR+cL_SUBFR16k)
		low_pass_filt_7k(hf, cL_SUBFR16k, st.memHf3[:], lpScratch)
	}

	for i := 0; i < cL_SUBFR16k; i++ {
		synth16k[i] = add_int16(synth16k[i], hf[i])
	}
}
