package amrwb

// Main AMR-WB encoder driver, ported from the Apache-2.0 vo-amrwbenc reference
// (voAMRWBEnc.c coder()), derived from 3GPP TS 26.190. Encodes one 20 ms frame
// (320 samples @ 16 kHz) into the internal-order parameter bit array.
//
// All nine speech modes are supported: 6.60k uses the 2-pulse algebraic
// codebook (ACELP_2t64_fx); 8.85-23.85k use the multi-track 4-pulse search
// (ACELP_4t64_fx). The 23.85k mode additionally runs synthesis() per subframe
// to quantize the high-band correction gain.

// (cL_NEXT, cGAMMA1, cTILT_FAC, cGP_CLIP, cPIT_SHARP, cPREEMPH_FAC, cQ_MAX are
// defined in dsp_const.go.)

// coder encodes one frame. modePtr is the internal mode index (0..8); speech16k
// is 320 input samples; prms receives the internal-order parameter bits;
// returns the frame's bit count. allowDtx is ignored (DTX encode not ported).
func (st *coderState) coder(modePtr *int16, speech16k []int16, prms []int16, allowDtx int16) int16 {
	mode := *modePtr
	serSize := amrwbCompressed[mode]
	w := &paramWriter{bits: prms}

	const lTotal = cL_TOTAL
	oldSpeech := make([]int16, lTotal)
	newOff := lTotal - cL_FRAME - cL_FILT    // 116
	speechOff := lTotal - cL_FRAME - cL_NEXT // 64
	pWindowOff := lTotal - cL_WINDOW         // 0

	oldWsp := make([]int16, cL_FRAME+cPIT_MAX/cOPL_DECIM)
	wspOff := cPIT_MAX / cOPL_DECIM // 115

	oldExc := make([]int16, (cL_FRAME+1)+cPIT_MAX+cL_INTERPOL)
	excBase := cPIT_MAX + cL_INTERPOL // 248

	copy(oldSpeech[:lTotal-cL_FRAME], st.oldSpeech[:])
	copy(oldWsp[:cPIT_MAX/cOPL_DECIM], st.oldWsp[:])
	copy(oldExc[:cPIT_MAX+cL_INTERPOL], st.oldExc[:])

	// Down-sample 16k -> 12.8k into new_speech.
	Decim_12k8(speech16k, cL_FRAME16k, oldSpeech[newOff:], st.memDecim[:])
	code := make([]int16, cL_SUBFR)
	errBuf := make([]int16, cM+cL_SUBFR)
	copy(code[:2*cL_FILT16k], st.memDecim[:2*cL_FILT16k])
	zero16(errBuf[:cL_FILT16k])
	Decim_12k8(errBuf, cL_FILT16k, oldSpeech[newOff+cL_FRAME:], code)

	// 50 Hz HP.
	HP50_12k8(oldSpeech[newOff:], cL_FRAME, st.memSigIn[:])
	copy(code[:6], st.memSigIn[:6])
	HP50_12k8(oldSpeech[newOff+cL_FRAME:], cL_FILT, code)

	// Pre-emphasis with adaptive scaling.
	mu := int16(cPREEMPH_FAC >> 1)
	ns := oldSpeech[newOff:]
	lTmp := int32(ns[0]) << 15
	lTmp -= (int32(st.memPreemph) * int32(mu)) << 1
	lMax := L_abs(lTmp)
	for i := 1; i < cL_FRAME+cL_FILT; i++ {
		t := int32(ns[i])<<15 - (int32(ns[i-1])*int32(mu))<<1
		if a := L_abs(t); a > lMax {
			lMax = a
		}
	}
	tmp := extract_h(lMax)
	var shift int16
	if tmp == 0 {
		shift = cQ_MAX
	} else {
		shift = norm_s(tmp) - 1
		if shift < 0 {
			shift = 0
		}
		if shift > cQ_MAX {
			shift = cQ_MAX
		}
	}
	qNew := shift
	if qNew > st.qMax[0] {
		qNew = st.qMax[0]
	}
	if qNew > st.qMax[1] {
		qNew = st.qMax[1]
	}
	exp := qNew - st.qOld
	st.qOld = qNew
	st.qMax[1] = st.qMax[0]
	st.qMax[0] = shift

	tmp = ns[cL_FRAME-1]
	for i := cL_FRAME + cL_FILT - 1; i > 0; i-- {
		t := int32(ns[i])<<15 - (int32(ns[i-1])*int32(mu))<<1
		t = L_shl(t, qNew)
		ns[i] = voRound(t)
	}
	t0 := int32(ns[0])<<15 - (int32(st.memPreemph)*int32(mu))<<1
	t0 = L_shl(t0, qNew)
	ns[0] = voRound(t0)
	st.memPreemph = tmp

	Scale_sig(oldSpeech[:lTotal-cL_FRAME-cL_FILT], int16(lTotal-cL_FRAME-cL_FILT), exp)
	Scale_sig(oldExc[:cPIT_MAX+cL_INTERPOL], cPIT_MAX+cL_INTERPOL, exp)
	Scale_sig(st.memSyn[:], cM, exp)
	Scale_sig(st.memDecim2[:], 3, exp)
	{
		m := []int16{st.memWsp}
		Scale_sig(m, 1, exp)
		st.memWsp = m[0]
		m2 := []int16{st.memW0}
		Scale_sig(m2, 1, exp)
		st.memW0 = m2[0]
	}

	// VAD (stubbed) -> vad_flag.
	buf := make([]int16, cL_FRAME)
	copy(buf, ns[:cL_FRAME])
	Scale_sig(buf, cL_FRAME, 1-qNew)
	vadFlag := wbVad(&st.vadSt, buf)
	if vadFlag == 0 {
		st.vadHist++
	} else {
		st.vadHist = 0
	}
	_ = allowDtx
	if mode != dMRDTX {
		w.parm(vadFlag, 1)
	}

	// LP analysis (centered at 4th subframe).
	rH := make([]int16, cM+1)
	rL := make([]int16, cM+1)
	A := make([]int16, cNB_SUBFR*(cM+1))
	rc := make([]int16, cM)
	ispnew := make([]int16, cM)
	Autocorr(oldSpeech[pWindowOff:], cM, rH, rL)
	Lag_window(rH, rL)
	Levinson(rH, rL, A, rc, st.memLevinson[:])
	Az_isp(A, ispnew, st.ispold[:])
	Int_isp(st.ispold[:], ispnew, interpolFrac[:], A)
	copy(st.ispold[:], ispnew)

	isf := make([]int16, cM)
	Isp_isf(ispnew, isf, cM)
	gpClipTestIsf(isf, st.gpClip[:])

	// Open-loop pitch.
	wsp := oldWsp[wspOff:]
	pA := 0
	for iSub := 0; iSub < cL_FRAME; iSub += cL_SUBFR {
		ap := make([]int16, cM+1)
		Weight_a(A[pA:], ap, cGAMMA1, cM)
		Residu(ap, oldSpeech, speechOff+iSub, wsp[iSub:], cL_SUBFR)
		pA += cM + 1
	}
	Deemph2(wsp, cTILT_FAC, cL_FRAME, &st.memWsp)

	max := int16(0)
	for i := 0; i < cL_FRAME; i++ {
		if a := abs_s(wsp[i]); a > max {
			max = a
		}
	}
	t := st.oldWspMax
	if max > t {
		t = max
	}
	st.oldWspMax = max
	shift = norm_s(t) - 3
	if shift > 0 {
		shift = 0
	}
	LP_Decim2(wsp, cL_FRAME, st.memDecim2[:])
	Scale_sig(wsp, cL_FRAME/cOPL_DECIM, shift)
	exp = exp + (shift - st.oldWspShift)
	st.oldWspShift = shift
	Scale_sig(oldWsp[:cPIT_MAX/cOPL_DECIM], cPIT_MAX/cOPL_DECIM, exp)
	Scale_sig(st.oldHpWsp[:cPIT_MAX/cOPL_DECIM], cPIT_MAX/cOPL_DECIM, exp)
	scaleMemHpWsp(st.hpWspMem[:], exp)

	var tOp, tOp2 int16
	if serSize == cNBBITS_7k {
		tOp = Pitch_med_ol(oldWsp, wspOff, st.oldT0Med, &st.olGain, st.hpWspMem[:], st.oldHpWsp[:], st.olWghtFlg, cL_FRAME/cOPL_DECIM)
	} else {
		tOp = Pitch_med_ol(oldWsp, wspOff, st.oldT0Med, &st.olGain, st.hpWspMem[:], st.oldHpWsp[:], st.olWghtFlg, (cL_FRAME/2)/cOPL_DECIM)
	}
	if st.olGain > 19661 {
		st.oldT0Med = medOlag(tOp, st.oldOlLag[:])
		st.adaW = 32767
	} else {
		st.adaW = mult(st.adaW, 29491)
	}
	if st.adaW < 26214 {
		st.olWghtFlg = 0
	} else {
		st.olWghtFlg = 1
	}
	wbVadToneDetection(&st.vadSt, st.olGain)
	tOp *= cOPL_DECIM
	if serSize != cNBBITS_7k {
		tOp2 = Pitch_med_ol(oldWsp, wspOff+(cL_FRAME/2)/cOPL_DECIM, st.oldT0Med, &st.olGain, st.hpWspMem[:], st.oldHpWsp[:], st.olWghtFlg, (cL_FRAME/2)/cOPL_DECIM)
		if st.olGain > 19661 {
			st.oldT0Med = medOlag(tOp2, st.oldOlLag[:])
			st.adaW = 32767
		} else {
			st.adaW = mult(st.adaW, 29491)
		}
		if st.adaW < 26214 {
			st.olWghtFlg = 0
		} else {
			st.olWghtFlg = 1
		}
		wbVadToneDetection(&st.vadSt, st.olGain)
		tOp2 *= cOPL_DECIM
	} else {
		tOp2 = tOp
	}

	// ISF quantization.
	indice := make([]int16, 8)
	if serSize <= cNBBITS_7k {
		Qpisf_2s_36b(isf, isf, st.pastIsfq[:], indice, 4)
		w.parm(indice[0], 8)
		w.parm(indice[1], 8)
		w.parm(indice[2], 7)
		w.parm(indice[3], 7)
		w.parm(indice[4], 6)
	} else {
		Qpisf_2s_46b(isf, isf, st.pastIsfq[:], indice, 4)
		w.parm(indice[0], 8)
		w.parm(indice[1], 8)
		w.parm(indice[2], 6)
		w.parm(indice[3], 7)
		w.parm(indice[4], 7)
		w.parm(indice[5], 5)
		w.parm(indice[6], 5)
	}

	// Stability factor.
	var lAcc int32
	for i := 0; i < cM-1; i++ {
		d := isf[i] - st.isfold[i]
		lAcc += (int32(d) * int32(d)) << 1
	}
	tmp = extract_h(lShl2(lAcc, 8))
	tmp = voMult(tmp, 26214)
	tmp = 20480 - tmp
	stabFac := shl(tmp, 1)
	if stabFac < 0 {
		stabFac = 0
	}
	copy(st.isfold[:], isf)

	ispnewQ := make([]int16, cM)
	Isf_isp(isf, ispnewQ, cM)
	if st.firstFrame != 0 {
		st.firstFrame = 0
		copy(st.ispoldQ[:], ispnewQ)
	}
	Aq := make([]int16, cNB_SUBFR*(cM+1))
	Int_isp(st.ispoldQ[:], ispnewQ, interpolFrac[:], Aq)
	copy(st.ispoldQ[:], ispnewQ)

	pAq := 0
	for iSub := 0; iSub < cL_FRAME; iSub += cL_SUBFR {
		Residu(Aq[pAq:], oldSpeech, speechOff+iSub, oldExc[excBase+iSub:], cL_SUBFR)
		pAq += cM + 1
	}

	t0min := tOp - 8
	if t0min < cPIT_MIN {
		t0min = cPIT_MIN
	}
	t0max := t0min + 15
	if t0max > cPIT_MAX {
		t0max = cPIT_MAX
		t0min = t0max - 15
	}

	xn := make([]int16, cL_SUBFR)
	xn2 := make([]int16, cL_SUBFR)
	dn := make([]int16, cL_SUBFR)
	cn := make([]int16, cL_SUBFR)
	h1 := make([]int16, cL_SUBFR)
	h2 := make([]int16, cL_SUBFR)
	y1 := make([]int16, cL_SUBFR)
	y2 := make([]int16, cL_SUBFR)
	code2 := make([]int16, cL_SUBFR)
	gCoeff := make([]int16, 4)
	gCoeff2 := make([]int16, 4)
	exc2 := make([]int16, cL_FRAME)

	pA = 0
	pAq = 0
	for iSub := 0; iSub < cL_FRAME; iSub += cL_SUBFR {
		pitFlag := int16(iSub)
		if iSub == 2*cL_SUBFR && serSize > cNBBITS_7k {
			pitFlag = 0
			t0min = tOp2 - 8
			if t0min < cPIT_MIN {
				t0min = cPIT_MIN
			}
			t0max = t0min + 15
			if t0max > cPIT_MAX {
				t0max = cPIT_MAX
				t0min = t0max - 15
			}
		}

		// Target xn for pitch search.
		for i := 0; i < cM; i++ {
			errBuf[i] = oldSpeech[speechOff+iSub+i-cM] - st.memSyn[i]
		}
		Residu(Aq[pAq:], oldSpeech, speechOff+iSub, oldExc[excBase+iSub:], cL_SUBFR)
		Syn_filt(Aq[pAq:], oldExc[excBase+iSub:], errBuf[cM:], cL_SUBFR, errBuf, 0)
		ap := make([]int16, cM+1)
		Weight_a(A[pA:], ap, cGAMMA1, cM)
		Residu(ap, errBuf, cM, xn, cL_SUBFR)
		Deemph2(xn, cTILT_FAC, cL_SUBFR, &st.memW0)

		// cn: residual-domain target.
		zero16(code[:cM])
		copy(code[cM:cM+cL_SUBFR/2], xn[:cL_SUBFR/2])
		var pm int16
		Preemph2(code[cM:], cTILT_FAC, cL_SUBFR/2, &pm)
		Weight_a(A[pA:], ap, cGAMMA1, cM)
		Syn_filt(ap, code[cM:], code[cM:], cL_SUBFR/2, code, 0)
		Residu(Aq[pAq:], code, cM, cn, cL_SUBFR/2)
		copy(cn[cL_SUBFR/2:], oldExc[excBase+iSub+cL_SUBFR/2:excBase+iSub+cL_SUBFR])

		// Impulse response h1.
		zero16(errBuf[:cM+cL_SUBFR])
		Weight_a(A[pA:], errBuf[cM:], cGAMMA1, cM)
		for i := 0; i < cL_SUBFR; i++ {
			lt := int32(errBuf[cM+i]) << 14
			for k := 1; k <= cM; k++ {
				lt -= int32(Aq[pAq+k]) * int32(errBuf[cM+i-k])
			}
			v := voRound(lt << 4)
			errBuf[cM+i] = v
			h1[i] = v
		}
		var pm2 int16
		Deemph2(h1, cTILT_FAC, cL_SUBFR, &pm2)
		copy(h2, h1)

		Scale_sig(h2, cL_SUBFR, -2)
		Scale_sig(xn, cL_SUBFR, shift)
		Scale_sig(h1, cL_SUBFR, 1+shift)

		// Closed-loop pitch.
		var T0, T0frac, index int16
		if serSize <= cNBBITS_9k {
			T0 = Pitch_fr4(oldExc, excBase+iSub, xn, h1, t0min, t0max, &T0frac, pitFlag, cPIT_MIN, cPIT_FR1_8b, cL_SUBFR)
			if pitFlag == 0 {
				if T0 < cPIT_FR1_8b {
					index = (T0 << 1) + (T0frac >> 1) - (cPIT_MIN << 1)
				} else {
					index = (T0 - cPIT_FR1_8b) + ((cPIT_FR1_8b - cPIT_MIN) * 2)
				}
				w.parm(index, 8)
				t0min = T0 - 8
				if t0min < cPIT_MIN {
					t0min = cPIT_MIN
				}
				t0max = t0min + 15
				if t0max > cPIT_MAX {
					t0max = cPIT_MAX
					t0min = t0max - 15
				}
			} else {
				index = ((T0 - t0min) << 1) + (T0frac >> 1)
				w.parm(index, 5)
			}
		} else {
			T0 = Pitch_fr4(oldExc, excBase+iSub, xn, h1, t0min, t0max, &T0frac, pitFlag, cPIT_FR2, cPIT_FR1_9b, cL_SUBFR)
			if pitFlag == 0 {
				if T0 < cPIT_FR2 {
					index = (T0 << 2) + T0frac - (cPIT_MIN << 2)
				} else if T0 < cPIT_FR1_9b {
					index = ((T0 << 1) + (T0frac >> 1) - (cPIT_FR2 << 1)) + ((cPIT_FR2 - cPIT_MIN) << 2)
				} else {
					index = (T0 - cPIT_FR1_9b) + ((cPIT_FR2 - cPIT_MIN) << 2) + ((cPIT_FR1_9b - cPIT_FR2) << 1)
				}
				w.parm(index, 9)
				t0min = T0 - 8
				if t0min < cPIT_MIN {
					t0min = cPIT_MIN
				}
				t0max = t0min + 15
				if t0max > cPIT_MAX {
					t0max = cPIT_MAX
					t0min = t0max - 15
				}
			} else {
				index = ((T0 - t0min) << 2) + T0frac
				w.parm(index, 6)
			}
		}

		clipGain := int16(0)
		if st.gpClip[0] < 154 && st.gpClip[1] > 14746 {
			clipGain = 1
		}

		// Adaptive codebook excitation.
		Pred_lt4(oldExc, excBase+iSub, T0, T0frac, cL_SUBFR+1)
		var gain1 int16
		if serSize > cNBBITS_9k {
			Convolve(oldExc[excBase+iSub:], h1, y1)
			gain1 = G_pitch(xn, y1, gCoeff, cL_SUBFR)
			if clipGain != 0 && gain1 > cGP_CLIP {
				gain1 = cGP_CLIP
			}
			Updt_tar(xn, dn, y1, gain1, cL_SUBFR)
		}

		// LP-filtered pitch excitation -> code.
		for i := 0; i < cL_SUBFR/2; i++ {
			p := excBase + iSub - 1 + 2*i
			l := 5898*int32(oldExc[p]) + 20972*int32(oldExc[p+1]) + 5898*int32(oldExc[p+2])
			l2 := 5898*int32(oldExc[p+1]) + 20972*int32(oldExc[p+2]) + 5898*int32(oldExc[p+3])
			code[2*i] = int16((l + 0x4000) >> 15)
			code[2*i+1] = int16((l2 + 0x4000) >> 15)
		}
		Convolve(code, h1, y2)
		gain2 := G_pitch(xn, y2, gCoeff2, cL_SUBFR)
		if clipGain != 0 && gain2 > cGP_CLIP {
			gain2 = cGP_CLIP
		}
		Updt_tar(xn, xn2, y2, gain2, cL_SUBFR)

		var gainPit int16
		sel := int16(0)
		if serSize > cNBBITS_9k {
			var l int32
			for i := 0; i < cL_SUBFR; i++ {
				l += int32(dn[i]) * int32(dn[i])
				l -= int32(xn2[i]) * int32(xn2[i])
			}
			if l <= 0 {
				sel = 1
			}
			w.parm(sel, 1)
		}
		if sel == 0 {
			gainPit = gain2
			copy(oldExc[excBase+iSub:excBase+iSub+cL_SUBFR], code)
			copy(y1, y2)
			copy(gCoeff, gCoeff2)
		} else {
			gainPit = gain1
			copy(xn2, dn)
		}

		Updt_tar(cn, cn, oldExc[excBase+iSub:], gainPit, cL_SUBFR)
		Scale_sig(cn, cL_SUBFR, shift)

		var pm3 int16
		Preemph(h2, st.tiltCode, cL_SUBFR, &pm3)
		if T0frac > 2 {
			T0 = T0 + 1
		}
		Pit_shrp(h2, T0, cPIT_SHARP, cL_SUBFR)

		cor_h_x(h2, xn2, dn)
		if serSize <= cNBBITS_7k {
			ACELP_2t64_fx(dn, cn, h2, code, y2, &indice[0])
			w.parm(indice[0], 12)
		} else if serSize <= cNBBITS_9k {
			ACELP_4t64_fx(dn, cn, h2, code, y2, 20, serSize, indice)
			for k := 0; k < 4; k++ {
				w.parm(indice[k], 5)
			}
		} else if serSize <= cNBBITS_12k {
			ACELP_4t64_fx(dn, cn, h2, code, y2, 36, serSize, indice)
			for k := 0; k < 4; k++ {
				w.parm(indice[k], 9)
			}
		} else if serSize <= cNBBITS_14k {
			ACELP_4t64_fx(dn, cn, h2, code, y2, 44, serSize, indice)
			w.parm(indice[0], 13)
			w.parm(indice[1], 13)
			w.parm(indice[2], 9)
			w.parm(indice[3], 9)
		} else if serSize <= cNBBITS_16k {
			ACELP_4t64_fx(dn, cn, h2, code, y2, 52, serSize, indice)
			for k := 0; k < 4; k++ {
				w.parm(indice[k], 13)
			}
		} else if serSize <= cNBBITS_18k {
			ACELP_4t64_fx(dn, cn, h2, code, y2, 64, serSize, indice)
			for k := 0; k < 4; k++ {
				w.parm(indice[k], 2)
			}
			for k := 4; k < 8; k++ {
				w.parm(indice[k], 14)
			}
		} else if serSize <= cNBBITS_20k {
			ACELP_4t64_fx(dn, cn, h2, code, y2, 72, serSize, indice)
			w.parm(indice[0], 10)
			w.parm(indice[1], 10)
			w.parm(indice[2], 2)
			w.parm(indice[3], 2)
			w.parm(indice[4], 10)
			w.parm(indice[5], 10)
			w.parm(indice[6], 14)
			w.parm(indice[7], 14)
		} else {
			ACELP_4t64_fx(dn, cn, h2, code, y2, 88, serSize, indice)
			for k := 0; k < 8; k++ {
				w.parm(indice[k], 11)
			}
		}

		var pm4 int16
		Preemph(code, st.tiltCode, cL_SUBFR, &pm4)
		Pit_shrp(code, T0, cPIT_SHARP, cL_SUBFR)

		var lGainCode int32
		if serSize <= cNBBITS_9k {
			index = Q_gain2(xn, y1, qNew+shift, y2, code, gCoeff, cL_SUBFR, 6, &gainPit, &lGainCode, clipGain, st.quaGain[:])
			w.parm(index, 6)
		} else {
			index = Q_gain2(xn, y1, qNew+shift, y2, code, gCoeff, cL_SUBFR, 7, &gainPit, &lGainCode, clipGain, st.quaGain[:])
			w.parm(index, 7)
		}
		gpClipTestGainPit(gainPit, st.gpClip[:])

		gainCode := extract_h(L_add(L_shl(lGainCode, qNew), 0x8000))

		copy(exc2[:cL_SUBFR], oldExc[excBase+iSub:excBase+iSub+cL_SUBFR])
		Scale_sig(exc2[:cL_SUBFR], cL_SUBFR, shift)
		voiceFac := voice_factor(exc2, shift, gainPit, code, gainCode, cL_SUBFR)
		st.tiltCode = (voiceFac >> 2) + 8192

		l := (int32(gainCode) * int32(y2[cL_SUBFR-1])) << 1
		l = L_shl(l, 5+shift)
		l = L_negate(l)
		l += (int32(xn[cL_SUBFR-1]) * 16384) << 1
		l -= (int32(y1[cL_SUBFR-1]) * int32(gainPit)) << 1
		l = L_shl(l, 1-shift)
		st.memW0 = extract_h(L_add(l, 0x8000))

		// The 23.85k mode reuses the unscaled adaptive excitation to build a
		// post-processed excitation for the high-band gain analysis.
		if serSize >= cNBBITS_24k {
			copy(exc2[:cL_SUBFR], oldExc[excBase+iSub:excBase+iSub+cL_SUBFR])
		}

		for i := 0; i < cL_SUBFR; i++ {
			l := (int32(gainCode) * int32(code[i])) << 1
			l <<= 5
			l += (int32(oldExc[excBase+iSub+i]) * int32(gainPit)) << 1
			l = lShl2(l, 1)
			oldExc[excBase+iSub+i] = extract_h(L_add(l, 0x8000))
		}
		Syn_filt(Aq[pAq:], oldExc[excBase+iSub:], code2, cL_SUBFR, st.memSyn[:], 1)

		if serSize >= cNBBITS_24k {
			// Noise enhancer: move code gain toward the gc threshold.
			gcHi, gcLo := voLExtract(lGainCode)
			tmp := int16(16384) - (voiceFac >> 1)
			fac := voMult(stabFac, tmp)
			lt := lGainCode
			if lt < st.lGcThres {
				lt += mpy3216(gcHi, gcLo, 6226)
				if lt > st.lGcThres {
					lt = st.lGcThres
				}
			} else {
				lt = mpy3216(gcHi, gcLo, 27536)
				if lt < st.lGcThres {
					lt = st.lGcThres
				}
			}
			st.lGcThres = lt
			lGainCode = mpy3216(gcHi, gcLo, 32767-fac)
			gcHi, gcLo = voLExtract(lt)
			lGainCode += mpy3216(gcHi, gcLo, fac)

			// Pitch enhancer: smooth HP filtering of the code.
			t := (voiceFac >> 3) + 4096
			lt = L_deposit_h(code[0])
			lt -= (int32(code[1]) * int32(t)) << 1
			code2[0] = voRound(lt)
			for i := 1; i < cL_SUBFR-1; i++ {
				lt = L_deposit_h(code[i])
				lt -= (int32(code[i+1]) * int32(t)) << 1
				lt -= (int32(code[i-1]) * int32(t)) << 1
				code2[i] = voRound(lt)
			}
			lt = L_deposit_h(code[cL_SUBFR-1])
			lt -= (int32(code[cL_SUBFR-2]) * int32(t)) << 1
			code2[cL_SUBFR-1] = voRound(lt)

			// Build the post-processed excitation.
			gc := voRound(L_shl(lGainCode, qNew))
			for i := 0; i < cL_SUBFR; i++ {
				lt = (int32(code2[i]) * int32(gc)) << 1
				lt <<= 5
				lt += (int32(exc2[i]) * int32(gainPit)) << 1
				lt <<= 1
				exc2[i] = voRound(lt)
			}

			corrGain := st.synthesis(Aq[pAq:], exc2[:cL_SUBFR], qNew, speech16k[iSub*5/4:])
			w.parm(corrGain, 4)
		}

		pA += cM + 1
		pAq += cM + 1
	}

	copy(st.oldSpeech[:], oldSpeech[cL_FRAME:cL_FRAME+(lTotal-cL_FRAME)])
	copy(st.oldWsp[:], oldWsp[cL_FRAME/cOPL_DECIM:cL_FRAME/cOPL_DECIM+cPIT_MAX/cOPL_DECIM])
	copy(st.oldExc[:], oldExc[cL_FRAME:cL_FRAME+cPIT_MAX+cL_INTERPOL])
	return serSize
}
