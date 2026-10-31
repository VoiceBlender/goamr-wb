package amrwb

// DTX encoder, ported verbatim from the Apache-2.0 vo-amrwbenc reference (dtx.c,
// dtx.h), derived from 3GPP TS 26.190 / 26.193: the TX DTX handler, the ISF and
// energy history, and the SID parameter and comfort-noise excitation
// generation. The encoder-side functions in opencore's dtx_decoder_amr_wb.cpp
// differ in their saturation; this file follows dtx.c.

// enAdjust is the excitation energy adjustment per speech coder mode, Q7.
var enAdjust = [9]int16{
	230, // mode0 = 7k  :  -5.4dB
	179, // mode1 = 9k  :  -4.2dB
	141, // mode2 = 12k :  -3.3dB
	128, // mode3 = 14k :  -3.0dB
	122, // mode4 = 16k :  -2.85dB
	115, // mode5 = 18k :  -2.7dB
	115, // mode6 = 20k :  -2.7dB
	115, // mode7 = 23k :  -2.7dB
	115, // mode8 = 24k :  -2.7dB
}

// dtxEncState mirrors the vo-amrwbenc dtx_encState.
type dtxEncState struct {
	isfHist    [cM * cDTX_HIST_SIZE]int16
	logEnHist  [cDTX_HIST_SIZE]int16
	histPtr    int16
	logEnIndex int16
	cngSeed    int16

	// DTX handler state.
	dtxHangoverCount   int16
	decAnaElapsedCount int16
	d                  [28]int32
	sumD               [cDTX_HIST_SIZE]int32
}

// reset mirrors dtx_enc_reset, which leaves the last sumD entry untouched.
func (st *dtxEncState) reset(isfInit []int16) {
	st.histPtr = 0
	st.logEnIndex = 0
	for i := 0; i < cDTX_HIST_SIZE; i++ {
		copy(st.isfHist[i*cM:i*cM+cM], isfInit[:cM])
	}
	st.cngSeed = cRANDOM_INITSEED
	zero16(st.logEnHist[:])
	st.dtxHangoverCount = cDTX_HANG_CONST
	st.decAnaElapsedCount = 32767
	for i := range st.d {
		st.d[i] = 0
	}
	for i := 0; i < cDTX_HIST_SIZE-1; i++ {
		st.sumD[i] = 0
	}
}

// encode computes the SID parameters, serializes them to w, and generates the
// comfort-noise ISF vector isf and excitation exc2 (dtx_enc).
func (st *dtxEncState) encode(isf, exc2 []int16, w *paramWriter) {
	// VOX mode computation of SID parameters.
	var logEn int16
	var lIsf [cM]int32
	for i := 0; i < cDTX_HIST_SIZE; i++ {
		// Division by DTX_HIST_SIZE = 8 has been done in buffer. log_en is in
		// Q10.
		logEn = add(logEn, st.logEnHist[i])
	}
	var isfOrder [3]int16
	st.findFrameIndices(&isfOrder)
	st.averIsfHistory(&isfOrder, &lIsf)

	for j := 0; j < cM; j++ {
		isf[j] = int16(lIsf[j] >> 3) // divide by 8
	}

	// Quantize logarithmic energy to 6 bits (-6 : 66 dB), which corresponds to
	// -2:22 in log2(E): st.logEnIndex = (log_en + 2.0) * 2.625.
	logEn >>= 2 // increase dynamics to 7 bits (Q8)
	// Add 2 in Q8 = 512 to get log2(E) between 0:24.
	logEn = add(logEn, 512)
	// Multiply by 2.625 to get full 6 bit range. 2.625 = 21504 in Q13. The
	// result is in Q6.
	logEn = mult(logEn, 21504)

	// Quantize energy.
	st.logEnIndex = shr(logEn, 6)
	if st.logEnIndex > 63 {
		st.logEnIndex = 63
	}
	if st.logEnIndex < 0 {
		st.logEnIndex = 0
	}

	// Quantize ISFs.
	var indice [5]int16
	Qisf_ns(isf, isf, indice[:])

	w.parm(indice[0], 6)
	w.parm(indice[1], 6)
	w.parm(indice[2], 6)
	w.parm(indice[3], 5)
	w.parm(indice[4], 5)

	w.parm(st.logEnIndex, 6)

	cnDith := st.ditheringControl()
	w.parm(cnDith, 1)

	// level = pow(2.0, logEnIndex / 2.625 - 2.0); log2(E) in Q9 (log2(E) lies
	// in between -2:22).
	logEn = shl(st.logEnIndex, 15-6)

	// Divide by 2.625; log_en will be between 0:24. The result corresponds to
	// log2(gain) in Q10.
	logEn = mult(logEn, 12483)

	logEnIntE := logEn >> 10         // integer part
	logEnIntM := shl(logEn&0x3ff, 5) // fractional part
	logEnIntE = add(logEnIntE, 16-1) // Q16 result of Pow2, gain divided by 2

	level32 := encPow2(logEnIntE, logEnIntM) // Q16
	exp0 := norm_l(level32)
	level32 <<= uint(exp0) // level in Q31
	exp0 = 15 - exp0
	level := extract_h(level32) // level in Q15

	// Generate a white noise vector.
	for i := 0; i < cL_FRAME; i++ {
		exc2[i] = Random(&st.cngSeed) >> 4
	}

	// gain = level / sqrt(ener) * sqrt(L_FRAME)
	var exp int16
	ener32 := encDotProduct12(exc2, exc2, cL_FRAME, &exp)
	encIsqrtN(&ener32, &exp)

	gain := extract_h(ener32)
	gain = mult(level, gain) // Q15

	exp = add(exp0, exp)
	exp += 4 // multiply by sqrt(L_FRAME)=16, i.e. shift left by 4

	for i := 0; i < cL_FRAME; i++ {
		tmp := mult(exc2[i], gain) // Q0 * Q15
		exc2[i] = shl(tmp, exp)
	}
}

// buffer stores the ISF vector and the log energy of residual energy enr
// (in L_FRAME) in the DTX history (dtx_buffer).
func (st *dtxEncState) buffer(isfNew []int16, enr int32, codecMode int16) {
	st.histPtr = add(st.histPtr, 1)
	if st.histPtr == cDTX_HIST_SIZE {
		st.histPtr = 0
	}
	copy(st.isfHist[int(st.histPtr)*cM:int(st.histPtr)*cM+cM], isfNew[:cM])

	var logEnE, logEnM int16
	encLog2(enr, &logEnE, &logEnM)

	// Convert exponent and mantissa to Q7, which simplifies averaging in
	// encode.
	logEn := shl(logEnE, 7)
	logEn = add(logEn, shr(logEnM, 15-7))

	// Find energy per sample (divide by L_FRAME=256), i.e. subtract log2(256) =
	// 8.0 (1024 in Q7), and the mode's excitation energy adjustment.
	logEn = sub(logEn, add(1024, enAdjust[codecMode]))

	st.logEnHist[st.histPtr] = logEn
}

// txDtxHandler adds extra speech hangover to let the decoder analyze speech
// (tx_dtx_handler). It sets *usedMode to MRDTX for a DTX frame.
func (st *dtxEncState) txDtxHandler(vadFlag int16, usedMode *int16) {
	// This state machine is in sync with the GSM EFR txDtx machine.
	st.decAnaElapsedCount = add(st.decAnaElapsedCount, 1)

	if vadFlag != 0 {
		st.dtxHangoverCount = cDTX_HANG_CONST
		return
	}
	if st.dtxHangoverCount == 0 { // out of decoder analysis hangover
		st.decAnaElapsedCount = 0
		*usedMode = dMRDTX
		return
	}
	// In possible analysis hangover.
	st.dtxHangoverCount = sub(st.dtxHangoverCount, 1)

	// decAnaElapsedCount + dtxHangoverCount < DTX_ELAPSED_FRAMES_THRESH
	if sub(add(st.decAnaElapsedCount, st.dtxHangoverCount), cDTX_ELAPSED_FRAMES_THRESH) < 0 {
		// If it is a short time since the decoder update, do not add extra
		// hangover. Otherwise override the VAD and stay in speech mode.
		*usedMode = dMRDTX
	}
}

// averIsfHistory averages the ISF history, replacing the vectors at indices[0]
// and indices[1], unless negative, with the median vector at indices[2]
// (aver_isf_history).
func (st *dtxEncState) averIsfHistory(indices *[3]int16, isfAver *[cM]int32) {
	var isfTmp [2 * cM]int16
	isfOld := st.isfHist[:]

	// Memorize the ISF vectors to be replaced by the median ISF vector prior to
	// the averaging.
	for k := 0; k < 2; k++ {
		if indices[k]+1 != 0 {
			for i := 0; i < cM; i++ {
				isfTmp[k*cM+i] = isfOld[int(indices[k])*cM+i]
				isfOld[int(indices[k])*cM+i] = isfOld[int(indices[2])*cM+i]
			}
		}
	}

	// Perform the ISF averaging.
	for j := 0; j < cM; j++ {
		var lTmp int32
		for i := 0; i < cDTX_HIST_SIZE; i++ {
			lTmp = L_add(lTmp, L_deposit_l(isfOld[i*cM+j]))
		}
		isfAver[j] = lTmp
	}

	// Retrieve the ISF vectors saved prior to averaging.
	for k := 0; k < 2; k++ {
		if indices[k]+1 != 0 {
			for i := 0; i < cM; i++ {
				isfOld[int(indices[k])*cM+i] = isfTmp[k*cM+i]
			}
		}
	}
}

// findFrameIndices finds the history vectors with the largest and second
// largest distance to the others, in indices[0] and indices[1], and the median
// one, in indices[2] (find_frame_indices).
func (st *dtxEncState) findFrameIndices(indices *[3]int16) {
	isfOldTx := st.isfHist[:]

	// Remove the effect of the oldest frame from the column sum
	// sumD[0..DTX_HIST_SIZE-1]. sumD[DTX_HIST_SIZE] is not updated since it
	// will be removed later.
	tmp := int16(cDTX_HIST_SIZE_MIN_ONE)
	j := int16(-1)
	for i := 0; i < cDTX_HIST_SIZE_MIN_ONE; i++ {
		j = add(j, tmp)
		st.sumD[i] = L_sub(st.sumD[i], st.d[j])
		tmp = sub(tmp, 1)
	}

	// Shift the column sum sumD. The element sumD[DTX_HIST_SIZE-1],
	// corresponding to the oldest frame, is removed. The sum of the distances
	// between the latest ISF and the other ISFs, sumD[0], is computed during
	// this call; hence it is initialized to zero.
	for i := cDTX_HIST_SIZE_MIN_ONE; i > 0; i-- {
		st.sumD[i] = st.sumD[i-1]
	}
	st.sumD[0] = 0

	// Remove the oldest frame from the distance matrix, which is stored as a
	// one-dimensional array to save memory.
	tmp = 0
	for i := int16(27); i >= 12; i -= tmp {
		tmp = add(tmp, 1)
		for j := tmp; j > 0; j-- {
			st.d[i-j+1] = st.d[i-j-tmp]
		}
	}

	// Compute the first column of the distance matrix D (squared Euclidean
	// distances from the latest ISF to the other ISFs).
	ptr := st.histPtr
	for i := 1; i < cDTX_HIST_SIZE; i++ {
		ptr = sub(ptr, 1)
		if ptr < 0 {
			ptr = cDTX_HIST_SIZE_MIN_ONE
		}
		var lTmp int32
		for j := 0; j < cM; j++ {
			tmp = sub(isfOldTx[int(st.histPtr)*cM+j], isfOldTx[int(ptr)*cM+j])
			lTmp = L_mac(lTmp, tmp, tmp)
		}
		st.d[i-1] = lTmp

		// Update also the column sums.
		st.sumD[0] = L_add(st.sumD[0], st.d[i-1])
		st.sumD[i] = L_add(st.sumD[i], st.d[i-1])
	}

	// Find the minimum and maximum distances.
	summax := st.sumD[0]
	summin := st.sumD[0]
	indices[0] = 0
	indices[2] = 0
	for i := int16(1); i < cDTX_HIST_SIZE; i++ {
		if L_sub(st.sumD[i], summax) > 0 {
			indices[0] = i
			summax = st.sumD[i]
		}
		if L_sub(st.sumD[i], summin) < 0 {
			indices[2] = i
			summin = st.sumD[i]
		}
	}

	// Find the second largest distance.
	summax2nd := int32(-2147483647)
	indices[1] = -1
	for i := int16(0); i < cDTX_HIST_SIZE; i++ {
		if L_sub(st.sumD[i], summax2nd) > 0 && sub(i, indices[0]) != 0 {
			indices[1] = i
			summax2nd = st.sumD[i]
		}
	}

	for i := range indices {
		indices[i] = sub(st.histPtr, indices[i])
		if indices[i] < 0 {
			indices[i] = add(indices[i], cDTX_HIST_SIZE)
		}
	}

	// If maximum distance/MED_THRESH is smaller than minimum distance, the
	// median ISF vector replacement is not performed. round_ saturates like
	// voround; voRound does not.
	tmp = norm_l(summax)
	summax <<= uint(tmp)
	summin <<= uint(tmp)
	lTmp := L_mult(round_(summax), cINV_MED_THRESH)
	if lTmp <= summin {
		indices[0] = -1
	}

	// Likewise for the second largest distance.
	summax2nd = L_shl(summax2nd, tmp)
	lTmp = L_mult(round_(summax2nd), cINV_MED_THRESH)
	if lTmp <= summin {
		indices[1] = -1
	}
}

// ditheringControl decides whether the background noise is non-stationary, so
// that the decoder dithers the comfort noise (dithering_control).
func (st *dtxEncState) ditheringControl() int16 {
	// Determine how stationary the spectrum of the background noise is.
	var isfDiff int32
	for i := 0; i < 8; i++ {
		isfDiff = L_add(isfDiff, st.sumD[i])
	}
	var cnDith int16
	if isfDiff>>26 > 0 {
		cnDith = 1
	}

	// Determine how stationary the energy of the background noise is.
	var mean int16
	for i := 0; i < cDTX_HIST_SIZE; i++ {
		mean = add(mean, st.logEnHist[i])
	}
	mean >>= 3
	var gainDiff int16
	for i := 0; i < cDTX_HIST_SIZE; i++ {
		tmp := abs_s(sub(st.logEnHist[i], mean))
		gainDiff = add(gainDiff, tmp)
	}
	if gainDiff > cGAIN_THR {
		cnDith = 1
	}
	return cnDith
}
