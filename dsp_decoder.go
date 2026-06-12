package amrwb

// Main AMR-WB decode driver, ported verbatim from the Apache-2.0 opencore-amrwb
// reference (pvamrwbdecoder.cpp pvDecoder_AmrWb), derived from 3GPP TS 26.173.
// Decodes one 20 ms frame (mode = internal mode index 0..8, or 9 for SID) into
// 320 PCM samples at 16 kHz.
//
// CNG (comfort-noise) synthesis for SID/DTX frames is not yet ported; non-speech
// states currently emit silence. Speech (and erased-speech concealment) frames
// are fully decoded.

const cExcBase = cPIT_MAX + cL_INTERPOL // 248: excitation history length

// decodeAmrWb decodes one frame. params holds the speech bits as one int16 per
// bit (BIT_0/BIT_1); frameType is an RX_* value. synth16k receives 320 samples.
func (st *decoderStateDSP) decodeAmrWb(mode int16, params []int16, frameType int16, synth16k []int16) {
	p := &bitParser{bits: params}
	nbBits := amrwbCompressed[mode]

	newDTXState := rxDtxHandler(&st.dtxDecSt, frameType)

	var bfi, unusableFrame int16
	switch frameType {
	case cRX_SPEECH_BAD, cRX_SPEECH_PROBABLY_DEGRADED:
		bfi, unusableFrame = 1, 0
	case cRX_NO_DATA, cRX_SPEECH_LOST:
		bfi, unusableFrame = 1, 1
	default:
		bfi, unusableFrame = 0, 0
	}

	if bfi != 0 {
		st.state++
		if st.state > 6 {
			st.state = 6
		}
	} else {
		st.state >>= 1
	}
	if st.dtxDecSt.dtxGlobalState == cDTX {
		st.state = 5
		st.prevBfi = 0
	} else if st.dtxDecSt.dtxGlobalState == cDTX_MUTE {
		st.state = 5
		st.prevBfi = 1
	}

	if newDTXState == cSPEECH {
		vadFlag := p.parm1()
		if bfi == 0 {
			if vadFlag == 0 {
				st.vadHist = add_int16(st.vadHist, 1)
			} else {
				st.vadHist = 0
			}
		}
	}

	isf := make([]int16, cM)
	ispnew := make([]int16, cM)
	isfTmp := make([]int16, cM)

	if newDTXState != cSPEECH {
		// CNG/DTX comfort-noise synthesis is not yet ported: emit silence and
		// keep speech memories reset so the next speech frame restarts cleanly.
		for i := range synth16k[:cAMR_WB_PCM_FRAME] {
			synth16k[i] = 0
		}
		st.reset(false)
		st.prevBfi = bfi
		st.dtxDecSt.dtxGlobalState = newDTXState
		return
	}

	// --- ACELP speech decode ---
	// Length (L_FRAME+1)+PIT_MAX+L_INTERPOL: the LP-filter pitch path reads one
	// sample past exc[i_subfr+L_SUBFR-1] in the last subframe.
	oldExc := make([]int16, cExcBase+cL_FRAME+1)
	copy(oldExc[:cExcBase], st.oldExc[:])

	// Decode the ISFs.
	var ind [8]int16
	if nbBits > cNBBITS_7k {
		ind[0] = p.parm(8)
		ind[1] = p.parm(8)
		ind[2] = p.parm(6)
		ind[3] = p.parm(7)
		ind[4] = p.parm(7)
		ind[5] = p.parm(5)
		ind[6] = p.parm(5)
		Dpisf_2s_46b(ind[:], isf, st.pastIsfq[:], st.isfold[:], st.isfBuf[:], bfi, 1)
	} else {
		ind[0] = p.parm(8)
		ind[1] = p.parm(8)
		ind[2] = p.parm(14)
		ind[3] = ind[2] & 0x007F
		ind[2] >>= 7
		ind[4] = p.parm(6)
		Dpisf_2s_36b(ind[:], isf, st.pastIsfq[:], st.isfold[:], st.isfBuf[:], bfi, 1)
	}

	Isf_isp(isf, ispnew, cM)
	if st.firstFrame != 0 {
		st.firstFrame = 0
		copy(st.ispold[:], ispnew)
	}

	aq := make([]int16, cNB_SUBFR*(cM+1))
	interpolate_isp(st.ispold[:], ispnew, interpolFrac[:], aq)
	copy(st.ispold[:], ispnew)

	// Stability factor from ISF distance.
	var lTmp int32
	for i := 0; i < cM-1; i++ {
		t := sub_int16(isf[i], st.isfold[i])
		lTmp = mac_16by16_to_int32(lTmp, t, t)
	}
	tmp := extract_h(shl_int32(lTmp, 8))
	tmp = mult_int16(tmp, 26214)
	tmp = 20480 - tmp
	stabFac := shl_int16(tmp, 1)
	if stabFac < 0 {
		stabFac = 0
	}
	copy(isfTmp, st.isfold[:])
	copy(st.isfold[:], isf)

	code := make([]int16, cL_SUBFR)
	excp := make([]int16, cL_SUBFR)
	exc2 := make([]int16, cL_FRAME)
	hfIsf := make([]int16, cM16k)

	var qNew int16
	var T0, T0Frac, T0Min, T0Max int16

	aqOff := 0
	for iSubfr := 0; iSubfr < cL_FRAME; iSubfr += cL_SUBFR {
		pitFlag := int16(iSubfr)
		if iSubfr == 2*cL_SUBFR && nbBits > cNBBITS_7k {
			pitFlag = 0
		}

		// Decode pitch lag.
		if pitFlag == 0 {
			if nbBits <= cNBBITS_9k {
				index := p.parm(8)
				if index < (cPIT_FR1_8b-cPIT_MIN)*2 {
					T0 = cPIT_MIN + (index >> 1)
					T0Frac = sub_int16(index, shl_int16(sub_int16(T0, cPIT_MIN), 1))
					T0Frac = shl_int16(T0Frac, 1)
				} else {
					T0 = add_int16(index, cPIT_FR1_8b-((cPIT_FR1_8b-cPIT_MIN)*2))
					T0Frac = 0
				}
			} else {
				index := p.parm(9)
				if index < (cPIT_FR2-cPIT_MIN)*4 {
					T0 = cPIT_MIN + (index >> 2)
					T0Frac = sub_int16(index, shl_int16(sub_int16(T0, cPIT_MIN), 2))
				} else if index < ((cPIT_FR2-cPIT_MIN)<<2)+((cPIT_FR1_9b-cPIT_FR2)<<1) {
					index -= (cPIT_FR2 - cPIT_MIN) << 2
					T0 = cPIT_FR2 + (index >> 1)
					T0Frac = sub_int16(index, shl_int16(sub_int16(T0, cPIT_FR2), 1))
					T0Frac = shl_int16(T0Frac, 1)
				} else {
					T0 = add_int16(index, cPIT_FR1_9b-((cPIT_FR2-cPIT_MIN)*4)-((cPIT_FR1_9b-cPIT_FR2)*2))
					T0Frac = 0
				}
			}
			T0Min = T0 - 8
			if T0Min < cPIT_MIN {
				T0Min = cPIT_MIN
			}
			T0Max = T0Min + 15
			if T0Max > cPIT_MAX {
				T0Max = cPIT_MAX
				T0Min = cPIT_MAX - 15
			}
		} else {
			if nbBits <= cNBBITS_9k {
				index := p.parm(5)
				T0 = T0Min + (index >> 1)
				T0Frac = sub_int16(index, shl_int16(T0-T0Min, 1))
				T0Frac = shl_int16(T0Frac, 1)
			} else {
				index := p.parm(6)
				T0 = T0Min + (index >> 2)
				T0Frac = sub_int16(index, shl_int16(T0-T0Min, 2))
			}
		}

		if bfi != 0 {
			lagconceal(st.decGain[17:], st.lagHist[:], &T0, &st.oldT0, &st.seed3, unusableFrame)
			T0Frac = 0
		}

		Pred_lt4(oldExc, cExcBase+iSubfr, T0, T0Frac, cL_SUBFR+1)

		var sel int16
		if unusableFrame != 0 {
			sel = 1
		} else if nbBits <= cNBBITS_9k {
			sel = 0
		} else {
			sel = p.parm1()
		}

		if sel == 0 {
			for i := 0; i < cL_SUBFR; i++ {
				l := int32(oldExc[cExcBase+iSubfr+i-1]) + int32(oldExc[cExcBase+iSubfr+i+1])
				l *= 5898
				l += int32(oldExc[cExcBase+iSubfr+i]) * 20972
				code[i] = amr_wb_round(l << 1)
			}
			copy(oldExc[cExcBase+iSubfr:cExcBase+iSubfr+cL_SUBFR], code)
		}

		// Decode innovative codebook.
		if unusableFrame != 0 {
			for i := 0; i < cL_SUBFR; i++ {
				code[i] = noise_gen_amrwb(&st.seed) >> 3
			}
		} else if nbBits <= cNBBITS_7k {
			ind[0] = p.parm(12)
			dec_acelp_2p_in_64(ind[0], code)
		} else if nbBits <= cNBBITS_9k {
			for i := 0; i < 4; i++ {
				ind[i] = p.parm(5)
			}
			dec_acelp_4p_in_64(ind[:], 20, code)
		} else if nbBits <= cNBBITS_12k {
			for i := 0; i < 4; i++ {
				ind[i] = p.parm(9)
			}
			dec_acelp_4p_in_64(ind[:], 36, code)
		} else if nbBits <= cNBBITS_14k {
			ind[0] = p.parm(13)
			ind[1] = p.parm(13)
			ind[2] = p.parm(9)
			ind[3] = p.parm(9)
			dec_acelp_4p_in_64(ind[:], 44, code)
		} else if nbBits <= cNBBITS_16k {
			for i := 0; i < 4; i++ {
				ind[i] = p.parm(13)
			}
			dec_acelp_4p_in_64(ind[:], 52, code)
		} else if nbBits <= cNBBITS_18k {
			for i := 0; i < 4; i++ {
				ind[i] = p.parm(2)
			}
			for i := 4; i < 8; i++ {
				ind[i] = p.parm(14)
			}
			dec_acelp_4p_in_64(ind[:], 64, code)
		} else if nbBits <= cNBBITS_20k {
			ind[0] = p.parm(10)
			ind[1] = p.parm(10)
			ind[2] = p.parm(2)
			ind[3] = p.parm(2)
			ind[4] = p.parm(10)
			ind[5] = p.parm(10)
			ind[6] = p.parm(14)
			ind[7] = p.parm(14)
			dec_acelp_4p_in_64(ind[:], 72, code)
		} else {
			for i := 0; i < 8; i++ {
				ind[i] = p.parm(11)
			}
			dec_acelp_4p_in_64(ind[:], 88, code)
		}

		preemph_amrwb_dec(code, st.tiltCode, cL_SUBFR)
		tmpL := T0
		if T0Frac > 2 {
			tmpL++
		}
		Pit_shrp(code, tmpL, cPIT_SHARP, cL_SUBFR)

		// Decode gains.
		var gainPit int16
		var lGainCode int32
		if nbBits <= cNBBITS_9k {
			index := p.parm(6)
			dec_gain2_amr_wb(index, 6, code, cL_SUBFR, &gainPit, &lGainCode,
				bfi, st.prevBfi, st.state, unusableFrame, st.vadHist, st.decGain[:])
		} else {
			index := p.parm(7)
			dec_gain2_amr_wb(index, 7, code, cL_SUBFR, &gainPit, &lGainCode,
				bfi, st.prevBfi, st.state, unusableFrame, st.vadHist, st.decGain[:])
		}

		// Q_new scaling search.
		qtmp := st.qsubfr[0]
		for i := 1; i < 4; i++ {
			if st.qsubfr[i] < qtmp {
				qtmp = st.qsubfr[i]
			}
		}
		if qtmp > cQ_MAX {
			qtmp = cQ_MAX
		}
		qNew = 0
		l := lGainCode
		for l < 0x08000000 && qNew < qtmp {
			l <<= 1
			qNew++
		}
		gainCode := amr_wb_round(l)

		scale_signal(oldExc[iSubfr:], cExcBase+cL_SUBFR, qNew-st.qOld)
		st.qOld = qNew

		if bfi == 0 {
			for i := 4; i > 0; i-- {
				st.lagHist[i] = st.lagHist[i-1]
			}
			st.lagHist[0] = T0
			st.oldT0 = T0
			st.oldT0Frac = 0
		}

		for i := cL_SUBFR - 1; i >= 0; i-- {
			e := oldExc[cExcBase+iSubfr+i]
			var add int32
			if e != max16 {
				add = 4
			}
			exc2[i] = int16((int32(e) + add) >> 3)
		}

		var pitSharp int16
		if nbBits <= cNBBITS_9k {
			pitSharp = shl_int16(gainPit, 1)
			if pitSharp > 16384 {
				for i := 0; i < cL_SUBFR; i++ {
					t := mult_int16(exc2[i], pitSharp)
					ll := mul_16by16_to_int32(t, gainPit)
					ll >>= 1
					excp[i] = amr_wb_round(ll)
				}
			}
		} else {
			pitSharp = 0
		}

		voiceFac := voice_factor(exc2, -3, gainPit, code, gainCode, cL_SUBFR)
		st.tiltCode = (voiceFac >> 2) + 8192

		copy(exc2, oldExc[cExcBase+iSubfr:cExcBase+iSubfr+cL_SUBFR])
		maxv := int32(1)
		for i := 0; i < cL_SUBFR; i++ {
			ll := mul_16by16_to_int32(code[i], gainCode)
			ll = shl_int32(ll, 5)
			ll = mac_16by16_to_int32(ll, oldExc[cExcBase+iSubfr+i], gainPit)
			ll = shl_int32(ll, 1)
			t := amr_wb_round(ll)
			oldExc[cExcBase+iSubfr+i] = t
			t2 := t
			if t2 < 0 {
				t2--
			}
			maxv |= int32(t2 ^ (t2 >> 15))
		}

		qScale := add_int16(normalize_amr_wb(maxv)-16, qNew) - 1
		st.qsubfr[3] = st.qsubfr[2]
		st.qsubfr[2] = st.qsubfr[1]
		st.qsubfr[1] = st.qsubfr[0]
		st.qsubfr[0] = qScale

		var disp int16
		if nbBits <= cNBBITS_7k {
			disp = 0
		} else if nbBits <= cNBBITS_9k {
			disp = 1
		} else {
			disp = 2
		}
		phase_dispersion(int16(lGainCode>>16), gainPit, code, disp, st.dispMem[:])

		// Noise enhancer.
		t := int16(16384) - (voiceFac >> 1)
		fac := mult_int16(stabFac, t)
		ll := lGainCode
		if ll < st.lGcThres {
			ll += fxp_mul32_by_16b(lGainCode, 6226) << 1
			if ll > st.lGcThres {
				ll = st.lGcThres
			}
		} else {
			ll = fxp_mul32_by_16b(lGainCode, 27536) << 1
			if ll < st.lGcThres {
				ll = st.lGcThres
			}
		}
		st.lGcThres = ll
		lGainCode = fxp_mul32_by_16b(lGainCode, int32(32767)-int32(fac)) << 1
		lGainCode = add_int32(lGainCode, fxp_mul32_by_16b(ll, int32(fac))<<1)

		// Pitch enhancer + build excitation.
		t = (voiceFac >> 3) + 4096
		gainCode = amr_wb_round(shl_int32(lGainCode, qNew))

		la := int32(code[0]) << 16
		la = msu_16by16_from_int32(la, code[1], t)
		la = mul_16by16_to_int32(amr_wb_round(la), gainCode)
		la = shl_int32(la, 5)
		la = mac_16by16_to_int32(la, exc2[0], gainPit)
		la = shl_int32(la, 1)
		exc2[0] = amr_wb_round(la)
		for i := 1; i < cL_SUBFR-1; i++ {
			la = int32(code[i]) << 16
			la = msu_16by16_from_int32(la, code[i+1]+code[i-1], t)
			la = mul_16by16_to_int32(amr_wb_round(la), gainCode)
			la = shl_int32(la, 5)
			la = mac_16by16_to_int32(la, exc2[i], gainPit)
			la = shl_int32(la, 1)
			exc2[i] = amr_wb_round(la)
		}
		la = int32(code[cL_SUBFR-1]) << 16
		la = msu_16by16_from_int32(la, code[cL_SUBFR-2], t)
		la = mul_16by16_to_int32(amr_wb_round(la), gainCode)
		la = shl_int32(la, 5)
		la = mac_16by16_to_int32(la, exc2[cL_SUBFR-1], gainPit)
		la = shl_int32(la, 1)
		exc2[cL_SUBFR-1] = amr_wb_round(la)

		if nbBits <= cNBBITS_9k && pitSharp > 16384 {
			for i := 0; i < cL_SUBFR; i++ {
				excp[i] = add_int16(excp[i], exc2[i])
			}
			agc2_amr_wb(exc2, excp, cL_SUBFR)
			copy(exc2, excp)
		}

		if nbBits <= cNBBITS_7k {
			j := iSubfr >> 6
			for i := 0; i < cM; i++ {
				ll2 := mul_16by16_to_int32(isfTmp[i], sub_int16(32767, interpolFrac[j]))
				ll2 = mac_16by16_to_int32(ll2, isf[i], interpolFrac[j])
				hfIsf[i] = amr_wb_round(ll2)
			}
		} else {
			for i := 0; i < cM16k-cM; i++ {
				st.memSynHf[i] = 0
			}
		}

		var corrGain int16
		if nbBits >= cNBBITS_24k {
			corrGain = p.parm(4)
		}

		synthesisAmrWb(aq[aqOff:], exc2, qNew, synth16k[iSubfr+(iSubfr>>2):],
			corrGain, hfIsf, nbBits, newDTXState, st, bfi)

		aqOff += cM + 1
	}

	copy(st.oldExc[:], oldExc[cL_FRAME:cL_FRAME+cExcBase])
	scale_signal(oldExc[cExcBase:], cL_FRAME, -qNew)
	st.dtxDecSt.activityUpdate(isf, oldExc[cExcBase:])
	st.dtxDecSt.dtxGlobalState = newDTXState
	st.prevBfi = bfi
}
