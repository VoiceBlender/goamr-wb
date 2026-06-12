package amrwb

// DTX/CNG decoder state, ported from the Apache-2.0 opencore-amrwb reference
// (dtx.h, dtx_decoder_amr_wb.cpp), derived from 3GPP TS 26.173. The handler and
// CNG-synthesis functions are ported alongside the main driver; this file holds
// the state struct and its reset.

const (
	cDTX_HIST_SIZE             = 8
	cDTX_HIST_SIZE_MIN_ONE     = 7
	cDTX_ELAPSED_FRAMES_THRESH = 24 + 7 - 1
	cDTX_HANG_CONST            = 7
	cDTX_MAX_EMPTY_THRESH      = 50
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
