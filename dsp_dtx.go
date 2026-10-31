package amrwb

// DTX/CNG decoder, ported from the Apache-2.0 opencore-amrwb reference (dtx.h,
// dtx_decoder_amr_wb.cpp), derived from 3GPP TS 26.173: the state struct, the
// RX DTX handler and the comfort-noise parameter/excitation generation.

const (
	cDTX_HIST_SIZE             = 8
	cDTX_HIST_SIZE_MIN_ONE     = 7
	cDTX_ELAPSED_FRAMES_THRESH = 24 + 7 - 1
	cDTX_HANG_CONST            = 7
	cDTX_MAX_EMPTY_THRESH      = 50

	cINV_MED_THRESH  = 14564
	cISF_DITH_GAP    = 448
	cISF_FACTOR_LOW  = 256
	cISF_FACTOR_STEP = 2
	cGAIN_THR        = 180
	cGAIN_FACTOR     = 75
)

type dtxDecState struct {
	sinceLastSid     int16
	trueSidPeriodInv int16
	logEn            int16
	oldLogEn         int16
	level            int16
	isf              [cM]int16
	isfOld           [cM]int16
	cngSeed          int16

	isfHist   [cM * cDTX_HIST_SIZE]int16
	logEnHist [cDTX_HIST_SIZE]int16
	histPtr   int16

	dtxHangoverCount   int16
	decAnaElapsedCount int16

	sidFrame         int16
	validData        int16
	dtxHangoverAdded int16

	dtxGlobalState int16
	dataUpdated    int16

	ditherSeed int16
	cnDith     int16
}

// reset mirrors dtx_dec_amr_wb_reset.
func (st *dtxDecState) reset(isfInitVec []int16) {
	st.sinceLastSid = 0
	st.trueSidPeriodInv = 1 << 13
	st.logEn = 3500
	st.oldLogEn = 3500
	st.cngSeed = cRANDOM_INITSEED
	st.histPtr = 0

	copy(st.isf[:], isfInitVec[:cM])
	copy(st.isfOld[:], isfInitVec[:cM])
	for i := 0; i < cDTX_HIST_SIZE; i++ {
		copy(st.isfHist[i*cM:i*cM+cM], isfInitVec[:cM])
		st.logEnHist[i] = st.logEn
	}

	st.dtxHangoverCount = cDTX_HANG_CONST
	st.decAnaElapsedCount = 32767
	st.sidFrame = 0
	st.validData = 0
	st.dtxHangoverAdded = 0
	st.dtxGlobalState = cSPEECH
	st.dataUpdated = 0
	st.ditherSeed = cRANDOM_INITSEED
	st.cnDith = 0
}

// decode computes the comfort-noise ISF vector isf and excitation exc2
// (dtx_dec_amr_wb). It is called when the synthesis state is not SPEECH and
// reads the SID parameters from p for SID_UPDATE frames.
func (st *dtxDecState) decode(exc2 []int16, newState int16, isf []int16, p *bitParser) {
	if st.dtxHangoverAdded != 0 && st.sidFrame != 0 {
		// SID_FIRST or SID_UPDATE after DTX hangover: consider the last frame
		// twice, and compute mean log energy and ISF from the decoded signal.
		ptr := int(st.histPtr) + 1
		if ptr == cDTX_HIST_SIZE {
			ptr = 0
		}
		last := int(st.histPtr)
		copy(st.isfHist[ptr*cM:ptr*cM+cM], st.isfHist[last*cM:last*cM+cM])
		st.logEnHist[ptr] = st.logEnHist[last]

		var lIsf [cM]int32
		st.logEn = 0
		for i := 0; i < cDTX_HIST_SIZE; i++ {
			// Division by DTX_HIST_SIZE was done in the encoder's dtx_buffer.
			st.logEn = add_int16(st.logEn, st.logEnHist[i])
			for j := 0; j < cM; j++ {
				lIsf[j] = add_int32(lIsf[j], int32(st.isfHist[i*cM+j]))
			}
		}

		st.logEn >>= 1 // Q9
		// Add 2 in Q9 to have only positive values for Pow2; subtracted back
		// after Pow2.
		st.logEn += 1024
		if st.logEn < 0 {
			st.logEn = 0
		}
		for j := 0; j < cM; j++ {
			st.isf[j] = int16(lIsf[j] >> 3)
		}
	}

	if st.sidFrame != 0 {
		// Set old SID parameters, always shift even if there is no new data.
		st.isfOld = st.isf
		st.oldLogEn = st.logEn

		if st.validData != 0 { // new data available (no CRC)
			// The division only works for since_last_sid < 32: limit the
			// interpolation to 32 frames.
			tmpIntLength := st.sinceLastSid
			if tmpIntLength > 32 {
				tmpIntLength = 32
			}
			if tmpIntLength >= 2 {
				st.trueSidPeriodInv = div_16by16(1<<10, shl_int16(tmpIntLength, 10))
			} else {
				st.trueSidPeriodInv = 1 << 14 // 0.5 in Q15
			}

			var ind [5]int16
			ind[0] = p.parm(6)
			ind[1] = p.parm(6)
			ind[2] = p.parm(6)
			ind[3] = p.parm(5)
			ind[4] = p.parm(5)
			Disf_ns(ind[:], st.isf[:])

			logEnIndex := p.parm(6)

			// Background noise stationarity information.
			st.cnDith = p.parm1()

			// log2(E) in Q9 (log2(E) lies in between -2:22), divided by 2.625.
			// Subtracting 2 in Q9 is done later, after Pow2.
			st.logEn = shl_int16(logEnIndex, 15-6)
			st.logEn = mult_int16(st.logEn, 12483)

			// No interpolation at startup after coder reset or when SID_UPDATE
			// has been received right after SPEECH.
			if st.dataUpdated == 0 || st.dtxGlobalState == cSPEECH {
				st.isfOld = st.isf
				st.oldLogEn = st.logEn
			}
		}
	}

	if st.sidFrame != 0 && st.validData != 0 {
		st.sinceLastSid = 0
	}

	// Interpolate SID info.
	intFac := shl_int16(st.sinceLastSid, 10)         // Q10
	intFac = mult_int16(intFac, st.trueSidPeriodInv) // Q10 * Q15 -> Q10
	if intFac > 1024 {                               // maximize to 1.0 in Q10
		intFac = 1024
	}
	intFac = shl_int16(intFac, 4) // Q10 -> Q14

	lLogEnInt := mul_16by16_to_int32(intFac, st.logEn) // Q14 * Q9 -> Q24
	for i := 0; i < cM; i++ {
		isf[i] = mult_int16(intFac, st.isf[i]) // Q14 * Q15 -> Q14
	}

	intFac = 16384 - intFac // 1-k in Q14

	lLogEnInt = mac_16by16_to_int32(lLogEnInt, intFac, st.oldLogEn) // Q24
	for i := 0; i < cM; i++ {
		isf[i] = add_int16(isf[i], mult_int16(intFac, st.isfOld[i])) // Q14
		isf[i] = shl_int16(isf[i], 1)                                // Q14 -> Q15
	}

	// If background noise is non-stationary, insert comfort noise dithering.
	if st.cnDith != 0 {
		cnDithering(isf, &lLogEnInt, &st.ditherSeed)
	}

	// lLogEnInt is log2(E)+2 in Q24, i.e. log2(gain)+1 in Q25.
	lLogEnInt >>= 9 // Q25 -> Q16

	logEnIntE := extract_h(lLogEnInt)                                     // integer part
	logEnIntM := int16(sub_int32(lLogEnInt, L_deposit_h(logEnIntE)) >> 1) // fractional part

	// Subtract 2 in Q9, i.e. divide the gain by 2 (energy by 4); add 16 to have
	// the result of Pow2 in Q16.
	logEnIntE += 15

	level32 := power_of_2(logEnIntE, logEnIntM) // Q16
	exp0 := normalize_amr_wb(level32)
	level32 <<= uint(exp0) // level in Q31
	exp0 = 15 - exp0
	level := int16(level32 >> 16) // level in Q15

	// Generate a white noise vector.
	for i := 0; i < cL_FRAME; i++ {
		exc2[i] = noise_gen_amrwb(&st.cngSeed) >> 4
	}

	// gain = level / sqrt(ener) * sqrt(L_FRAME)
	var exp int16
	ener32 := dotProduct12(exc2, exc2, cL_FRAME, &exp)
	one_ov_sqrt_norm(&ener32, &exp)
	gain := extract_h(ener32)
	gain = mult_int16(level, gain) // Q15
	exp += exp0
	exp += 4 // multiply by sqrt(L_FRAME)=16, i.e. shift left by 4

	for i := 0; i < cL_FRAME; i++ {
		tmp := mult_int16(exc2[i], gain) // Q0 * Q15
		exc2[i] = shl_int16(tmp, exp)
	}

	if newState == cDTX_MUTE {
		// Mute comfort noise as it has been a long time since the last SID
		// update.
		tmpIntLength := st.sinceLastSid
		if tmpIntLength > 32 {
			tmpIntLength = 32
		}
		st.trueSidPeriodInv = div_16by16(1<<10, shl_int16(tmpIntLength, 10))

		st.sinceLastSid = 0
		st.oldLogEn = st.logEn
		st.logEn -= 64 // subtract 1/8 in Q9 (energy), i.e. -3/8 dB
		if st.logEn < 0 {
			st.logEn = 0
		}
	}

	// Reset the interpolation length timer if data has been updated.
	if st.sidFrame != 0 &&
		(st.validData != 0 || (st.validData == 0 && st.dtxHangoverAdded != 0)) {
		st.sinceLastSid = 0
		st.dataUpdated = 1
	}
}

// cnDithering inserts comfort noise dithering into the energy and the ISF
// vector (CN_dithering).
func cnDithering(isf []int16, lLogEnInt *int32, ditherSeed *int16) {
	// Energy parameter.
	randDith := noise_gen_amrwb(ditherSeed) >> 1
	randDith2 := noise_gen_amrwb(ditherSeed) >> 1
	randDith += randDith2
	*lLogEnInt = add_int32(*lLogEnInt, mul_16by16_to_int32(randDith, cGAIN_FACTOR))
	if *lLogEnInt < 0 {
		*lLogEnInt = 0
	}

	// Spectral parameters (ISF vector).
	ditherFac := int16(cISF_FACTOR_LOW)

	randDith = noise_gen_amrwb(ditherSeed) >> 1
	randDith2 = noise_gen_amrwb(ditherSeed) >> 1
	randDith += randDith2
	temp := add_int16(isf[0], mult_int16_r(randDith, ditherFac))

	// Make sure that isf[0] will not get negative values.
	if temp < cISF_GAP {
		isf[0] = cISF_GAP
	} else {
		isf[0] = temp
	}

	for i := 1; i < cM-1; i++ {
		ditherFac = add_int16(ditherFac, cISF_FACTOR_STEP)

		randDith = noise_gen_amrwb(ditherSeed) >> 1
		randDith2 = noise_gen_amrwb(ditherSeed) >> 1
		randDith += randDith2
		temp = add_int16(isf[i], mult_int16_r(randDith, ditherFac))
		temp1 := sub_int16(temp, isf[i-1])

		// Make sure that the ISF spacing remains at least ISF_DITH_GAP.
		if temp1 < cISF_DITH_GAP {
			isf[i] = isf[i-1] + cISF_DITH_GAP
		} else {
			isf[i] = temp
		}
	}

	// Make sure that isf[M-2] will not get values above 16384.
	if isf[cM-2] > 16384 {
		isf[cM-2] = 16384
	}
}

// dtxDecActivityUpdate logs ISF and excitation-frame energy into the DTX
// history (dtx_dec_amr_wb_activity_update), called for every speech frame.
func (st *dtxDecState) activityUpdate(isf, exc []int16) {
	st.histPtr++
	if st.histPtr == cDTX_HIST_SIZE {
		st.histPtr = 0
	}
	copy(st.isfHist[int(st.histPtr)*cM:int(st.histPtr)*cM+cM], isf[:cM])

	lFrameEn := int32(0)
	for i := 0; i < cL_FRAME; i++ {
		lFrameEn = mac_16by16_to_int32(lFrameEn, exc[i], exc[i])
	}
	lFrameEn >>= 1
	var logEnE, logEnM int16
	amrwbLog2(lFrameEn, &logEnE, &logEnM)
	logEn := shl_int16(logEnE, 7)
	logEn += logEnM >> 8
	logEn -= 1024
	st.logEnHist[st.histPtr] = logEn
}

// rxDtxHandler updates DTX state from the received frame type and returns the
// new synthesis state (SPEECH/DTX/DTX_MUTE) (rx_amr_wb_dtx_handler).
func rxDtxHandler(st *dtxDecState, frameType int16) int16 {
	var newState int16

	if frameType == cRX_SID_FIRST || frameType == cRX_SID_UPDATE || frameType == cRX_SID_BAD ||
		((st.dtxGlobalState == cDTX || st.dtxGlobalState == cDTX_MUTE) &&
			(frameType == cRX_NO_DATA || frameType == cRX_SPEECH_BAD || frameType == cRX_SPEECH_LOST)) {
		newState = cDTX
		if st.dtxGlobalState == cDTX_MUTE &&
			(frameType == cRX_SID_BAD || frameType == cRX_SID_FIRST ||
				frameType == cRX_SPEECH_LOST || frameType == cRX_NO_DATA) {
			newState = cDTX_MUTE
		}
		st.sinceLastSid = add_int16(st.sinceLastSid, 1)
		if st.sinceLastSid > cDTX_MAX_EMPTY_THRESH {
			newState = cDTX_MUTE
		}
	} else {
		newState = cSPEECH
		st.sinceLastSid = 0
	}

	if st.dataUpdated == 0 && frameType == cRX_SID_UPDATE {
		st.decAnaElapsedCount = 0
	}
	st.decAnaElapsedCount = add_int16(st.decAnaElapsedCount, 1)
	st.dtxHangoverAdded = 0

	var encState int16
	if frameType == cRX_SID_FIRST || frameType == cRX_SID_UPDATE ||
		frameType == cRX_SID_BAD || frameType == cRX_NO_DATA {
		encState = cDTX
	} else {
		encState = cSPEECH
	}

	if encState == cSPEECH {
		st.dtxHangoverCount = cDTX_HANG_CONST
	} else {
		if st.decAnaElapsedCount > cDTX_ELAPSED_FRAMES_THRESH {
			st.dtxHangoverAdded = 1
			st.decAnaElapsedCount = 0
			st.dtxHangoverCount = 0
		} else if st.dtxHangoverCount == 0 {
			st.decAnaElapsedCount = 0
		} else {
			st.dtxHangoverCount--
		}
	}

	if newState != cSPEECH {
		st.sidFrame = 0
		st.validData = 0
		if frameType == cRX_SID_FIRST {
			st.sidFrame = 1
		} else if frameType == cRX_SID_UPDATE {
			st.sidFrame = 1
			st.validData = 1
		} else if frameType == cRX_SID_BAD {
			st.sidFrame = 1
			st.dtxHangoverAdded = 0
		}
	}
	return newState
}
