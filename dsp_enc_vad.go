package amrwb

// Voice activity detection, ported verbatim from the Apache-2.0 vo-amrwbenc
// reference (wb_vad.c, wb_vad_c.h), derived from 3GPP TS 26.194.
//
// The reference uses the vo_* macros (vo_mult, vo_sub, add1, vo_L_add, ...),
// which expand to plain int arithmetic: intermediate results are full width
// and are only truncated to 16 bits where they are assigned to a Word16 or
// passed to a Word16 parameter. The int16(...) conversions below mark exactly
// those points.

const (
	cVAD_FRAME_LEN = 256 // samples per VAD frame
	cCOMPLEN       = 12  // number of sub-bands
	cUNIRSHFT      = 7

	cTONE_THR = 21298 // (Word16)(0.65*MAX_16)

	// Speech level estimation.
	cSP_EST_COUNT      = 80
	cSP_ACTIVITY_COUNT = 25
	cALPHA_SP_UP       = 4915 // (Word16)((1.0 - 0.85)*MAX_16)
	cALPHA_SP_DOWN     = 4915
	cSPEECH_LEVEL_INIT = 2050 // NOM_LEVEL, about -26 dBov
	cMIN_SPEECH_LEVEL1 = 129  // (Word16)(NOM_LEVEL * 0.063)
	cMIN_SPEECH_LEVEL2 = 410  // (Word16)(NOM_LEVEL * 0.2)
	cMIN_SPEECH_SNR    = 4096

	// Background spectrum update time constants.
	cALPHA_UP1   = 1638  // (Word16)((1.0 - 0.95)*MAX_16)
	cALPHA_DOWN1 = 2097  // (Word16)((1.0 - 0.936)*MAX_16)
	cALPHA_UP2   = 491   // (Word16)((1.0 - 0.985)*MAX_16)
	cALPHA_DOWN2 = 1867  // (Word16)((1.0 - 0.943)*MAX_16)
	cALPHA3      = 1638  // (Word16)((1.0 - 0.95)*MAX_16)
	cALPHA4      = 3276  // (Word16)((1.0 - 0.9)*MAX_16)
	cALPHA5      = 16383 // (Word16)((1.0 - 0.5)*MAX_16)

	// VAD threshold. The slopes are evaluated in single-precision float by the
	// reference macros; these are the resulting values.
	cTHR_MIN   = 204 // (Word16)(1.6*SCALE)
	cTHR_HIGH  = 768 // (Word16)(6*SCALE)
	cTHR_LOW   = 217 // (Word16)(1.7*SCALE)
	cNO_P1     = 31744
	cNO_SLOPE  = 1509
	cSP_CH_MIN = -96
	cSP_CH_MAX = 96
	cSP_P1     = 22527
	cSP_SLOPE  = -1339

	// Hangover and burst length.
	cHANG_HIGH   = 12
	cHANG_LOW    = 2
	cHANG_P1     = cTHR_LOW
	cHANG_SLOPE  = -1110
	cBURST_HIGH  = 8
	cBURST_P1    = cTHR_HIGH
	cBURST_SLOPE = 297

	// Background spectrum recovery.
	cSTAT_COUNT     = 20
	cSTAT_THR_LEVEL = 184
	cSTAT_THR       = 1000

	// Background noise estimate limits.
	cNOISE_MIN  = 40
	cNOISE_MAX  = 20000
	cNOISE_INIT = 150

	// Signal power thresholds (over two frames).
	cVAD_POW_LOW  = 30000
	cPOW_TONE_THR = 686080

	// Filter bank.
	cCOEFF3    = 13363
	cCOEFF5_1  = 21955
	cCOEFF5_2  = 6390
	cF_5TH_CNT = 5
	cF_3TH_CNT = 6
)

// vadState is the wb_vad analysis state (VadVars).
type vadState struct {
	bckrEst     [cCOMPLEN]int16 // background noise estimate
	aveLevel    [cCOMPLEN]int16 // averaged input components for stationary estimation
	oldLevel    [cCOMPLEN]int16 // input levels of the previous frame
	subLevel    [cCOMPLEN]int16 // input levels calculated at the end of a frame (lookahead)
	aData5      [cF_5TH_CNT][2]int16
	aData3      [cF_3TH_CNT]int16
	burstCount  int16
	hangCount   int16
	statCount   int16
	vadreg      int16 // intermediate VAD decisions, newest in bit 14
	toneFlag    int16 // tone detection flags, newest in bit 14
	spEstCnt    int16
	spMax       int16
	spMaxCnt    int16
	speechLevel int16
	prevPowSum  int32
}

// reset initializes the state (wb_vad_reset). statCount is not reset by the
// reference either; it is always assigned before it is read.
func (st *vadState) reset() {
	st.toneFlag = 0
	st.vadreg = 0
	st.hangCount = 0
	st.burstCount = 0
	st.aData5 = [cF_5TH_CNT][2]int16{}
	st.aData3 = [cF_3TH_CNT]int16{}
	for i := 0; i < cCOMPLEN; i++ {
		st.bckrEst[i] = cNOISE_INIT
		st.oldLevel[i] = cNOISE_INIT
		st.aveLevel[i] = cNOISE_INIT
		st.subLevel[i] = 0
	}
	st.spEstCnt = 0
	st.spMax = 0
	st.spMaxCnt = 0
	st.speechLevel = cSPEECH_LEVEL_INIT
	st.prevPowSum = 0
}

// vadIlog2 returns a log2 approximation of mant (ilog2).
func vadIlog2(mant int16) int16 {
	if mant <= 0 {
		mant = 1
	}
	ex := norm_s(mant)
	mant = int16(int32(mant) << uint(ex))
	for i := 0; i < 3; i++ {
		mant = int16((int32(mant) * int32(mant)) >> 15)
	}
	lTemp := (int32(mant) * int32(mant)) << 1
	ex2 := norm_l(lTemp)
	mant = int16((lTemp << uint(ex2)) >> 16)

	res := int16(int32(ex+16) << 10)
	res = int16(int32(res) + int32(ex2)<<6)
	res = int16(int32(res) + 127 - int32(mant>>8))
	return res
}

// vadFilter5 is the fifth-order half-band lowpass/highpass pair (filter5).
func vadFilter5(in0, in1 *int16, data *[2]int16) {
	temp0 := int16(int32(*in0) - (cCOEFF5_1*int32(data[0]))>>15)
	temp1 := int16(int32(data[0]) + (cCOEFF5_1*int32(temp0))>>15)
	data[0] = temp0

	temp0 = int16(int32(*in1) - (cCOEFF5_2*int32(data[1]))>>15)
	temp2 := int16(int32(data[1]) + (cCOEFF5_2*int32(temp0))>>15)
	data[1] = temp0

	*in0 = int16(((int32(temp1) + int32(temp2)) << 15) >> 16)
	*in1 = int16(((int32(temp1) - int32(temp2)) << 15) >> 16)
}

// vadFilter3 is the third-order half-band lowpass/highpass pair (filter3).
func vadFilter3(in0, in1, data *int16) {
	temp1 := int16(int32(*in1) - (cCOEFF3*int32(*data))>>15)
	temp2 := int16(int32(*data) + (cCOEFF3*int32(temp1))>>15)
	*data = temp1

	*in1 = int16(((int32(*in0) - int32(temp2)) << 15) >> 16)
	*in0 = int16(((int32(*in0) + int32(temp2)) << 15) >> 16)
}

// vadLevelCalculation sums absolute values in a sub-band (level_calculation).
// The level of the last count2-count1 samples is stored to *subLevel and added
// to the level of the next frame.
func vadLevelCalculation(data []int16, subLevel *int16, count1, count2, indM, indA, scale int16) int16 {
	var lTemp1 int32
	for i := count1; i < count2; i++ {
		lTemp1 += int32(abs_s(data[indM*i+indA])) << 1
	}
	lTemp2 := lTemp1 + L_shl(int32(*subLevel), 16-scale)
	*subLevel = extract_h(L_shl(lTemp1, scale))

	for i := int16(0); i < count1; i++ {
		lTemp2 += int32(abs_s(data[indM*i+indA])) << 1
	}
	return extract_h(lShl2(lTemp2, scale))
}

// vadFilterBank splits the input into sub-bands and computes their levels
// (filter_bank).
func (st *vadState) filterBank(in []int16, level *[cCOMPLEN]int16) {
	var tmpBuf [cVAD_FRAME_LEN]int16
	for i := 0; i < cVAD_FRAME_LEN; i++ {
		tmpBuf[i] = in[i] >> 1
	}

	for i := 0; i < 128; i++ {
		vadFilter5(&tmpBuf[2*i], &tmpBuf[2*i+1], &st.aData5[0])
	}
	for i := 0; i < 64; i++ {
		vadFilter5(&tmpBuf[4*i], &tmpBuf[4*i+2], &st.aData5[1])
		vadFilter5(&tmpBuf[4*i+1], &tmpBuf[4*i+3], &st.aData5[2])
	}
	for i := 0; i < 32; i++ {
		vadFilter5(&tmpBuf[8*i], &tmpBuf[8*i+4], &st.aData5[3])
		vadFilter5(&tmpBuf[8*i+2], &tmpBuf[8*i+6], &st.aData5[4])
		vadFilter3(&tmpBuf[8*i+3], &tmpBuf[8*i+7], &st.aData3[0])
	}
	for i := 0; i < 16; i++ {
		vadFilter3(&tmpBuf[16*i+0], &tmpBuf[16*i+8], &st.aData3[1])
		vadFilter3(&tmpBuf[16*i+4], &tmpBuf[16*i+12], &st.aData3[2])
		vadFilter3(&tmpBuf[16*i+6], &tmpBuf[16*i+14], &st.aData3[3])
	}
	for i := 0; i < 8; i++ {
		vadFilter3(&tmpBuf[32*i+0], &tmpBuf[32*i+16], &st.aData3[4])
		vadFilter3(&tmpBuf[32*i+8], &tmpBuf[32*i+24], &st.aData3[5])
	}

	b := tmpBuf[:]
	level[11] = vadLevelCalculation(b, &st.subLevel[11], 16, 64, 4, 1, 14)
	level[10] = vadLevelCalculation(b, &st.subLevel[10], 8, 32, 8, 7, 15)
	level[9] = vadLevelCalculation(b, &st.subLevel[9], 8, 32, 8, 3, 15)
	level[8] = vadLevelCalculation(b, &st.subLevel[8], 8, 32, 8, 2, 15)
	level[7] = vadLevelCalculation(b, &st.subLevel[7], 4, 16, 16, 14, 16)
	level[6] = vadLevelCalculation(b, &st.subLevel[6], 4, 16, 16, 6, 16)
	level[5] = vadLevelCalculation(b, &st.subLevel[5], 4, 16, 16, 4, 16)
	level[4] = vadLevelCalculation(b, &st.subLevel[4], 4, 16, 16, 12, 16)
	level[3] = vadLevelCalculation(b, &st.subLevel[3], 2, 8, 32, 8, 17)
	level[2] = vadLevelCalculation(b, &st.subLevel[2], 2, 8, 32, 24, 17)
	level[1] = vadLevelCalculation(b, &st.subLevel[1], 2, 8, 32, 16, 17)
	level[0] = vadLevelCalculation(b, &st.subLevel[0], 2, 8, 32, 0, 17)
}

// updateCntrl controls the update speed of the background noise estimate
// (update_cntrl).
func (st *vadState) updateCntrl(level *[cCOMPLEN]int16) {
	if sub(st.toneFlag&0x7c00, 0x7c00) == 0 {
		st.statCount = cSTAT_COUNT
	} else if (st.vadreg & 0x7f80) == 0 {
		st.statCount = cSTAT_COUNT
	} else {
		var statRat int16
		for i := 0; i < cCOMPLEN; i++ {
			var num, denom int16
			if level[i] > st.aveLevel[i] {
				num = level[i]
				denom = st.aveLevel[i]
			} else {
				num = st.aveLevel[i]
				denom = level[i]
			}
			if num < cSTAT_THR_LEVEL {
				num = cSTAT_THR_LEVEL
			}
			if denom < cSTAT_THR_LEVEL {
				denom = cSTAT_THR_LEVEL
			}
			exp := norm_s(denom)
			denom = int16(int32(denom) << uint(exp))
			temp := div_s(num>>1, denom)
			statRat = int16(int32(statRat) + int32(shr(temp, 8-exp)))
		}
		if statRat > cSTAT_THR {
			st.statCount = cSTAT_COUNT
		} else if (st.vadreg & 0x4000) != 0 {
			if st.statCount != 0 {
				st.statCount--
			}
		}
	}

	alpha := int32(cALPHA4)
	if st.statCount == cSTAT_COUNT {
		alpha = 32767
	} else if (st.vadreg & 0x4000) == 0 {
		alpha = cALPHA5
	}
	for i := 0; i < cCOMPLEN; i++ {
		d := int32(level[i]) - int32(st.aveLevel[i])
		st.aveLevel[i] = int16(int32(st.aveLevel[i]) + (alpha*d+0x4000)>>15)
	}
}

// hangoverAddition adds hangover after speech bursts (hangover_addition).
func (st *vadState) hangoverAddition(lowPower, hangLen, burstLen int16) int16 {
	if lowPower != 0 {
		st.burstCount = 0
		st.hangCount = 0
		return 0
	}
	if (st.vadreg & 0x4000) != 0 {
		st.burstCount++
		if st.burstCount >= burstLen {
			st.hangCount = hangLen
		}
		return 1
	}
	st.burstCount = 0
	if st.hangCount > 0 {
		st.hangCount--
		return 1
	}
	return 0
}

// noiseEstimateUpdate updates the background noise estimate
// (noise_estimate_update).
func (st *vadState) noiseEstimateUpdate(level *[cCOMPLEN]int16) {
	var alphaUp, alphaDown int16
	bckrAdd := int32(2)

	st.updateCntrl(level)

	if (0x7800 & st.vadreg) == 0 {
		alphaUp = cALPHA_UP1
		alphaDown = cALPHA_DOWN1
	} else if st.statCount == 0 {
		alphaUp = cALPHA_UP2
		alphaDown = cALPHA_DOWN2
	} else {
		alphaUp = 0
		alphaDown = cALPHA3
		bckrAdd = 0
	}

	for i := 0; i < cCOMPLEN; i++ {
		temp := int16(int32(st.oldLevel[i]) - int32(st.bckrEst[i]))
		if temp < 0 { // update downwards
			st.bckrEst[i] = int16(-2 + int32(add(st.bckrEst[i], voMultR(alphaDown, temp))))
			if st.bckrEst[i] < cNOISE_MIN {
				st.bckrEst[i] = cNOISE_MIN
			}
		} else { // update upwards
			st.bckrEst[i] = int16(bckrAdd + int32(st.bckrEst[i]) + int32(voMultR(alphaUp, temp)))
			if st.bckrEst[i] > cNOISE_MAX {
				st.bckrEst[i] = cNOISE_MAX
			}
		}
	}

	st.oldLevel = *level
}

// vadDecision makes the VAD decision for one frame (vad_decision).
func (st *vadState) vadDecision(level *[cCOMPLEN]int16, powSum int32) int16 {
	// Squared sum of the input levels divided by the background noise.
	var lSnrSum int32
	for i := 0; i < cCOMPLEN; i++ {
		exp := norm_s(st.bckrEst[i])
		temp := int16(int32(st.bckrEst[i]) << uint(exp))
		temp = div_s(level[i]>>1, temp)
		temp = shl(temp, exp-(cUNIRSHFT-1))
		lSnrSum = L_mac(lSnrSum, temp, temp)
	}

	// Average level of the estimated background noise (ignoring the lowest band).
	var lTemp int32
	for i := 1; i < cCOMPLEN; i++ {
		lTemp += int32(st.bckrEst[i])
	}
	noiseLevel := int16((lTemp << 12) >> 16)

	// At very low SNR, raise speech_level to MIN_SPEECH_SNR*noise_level.
	temp := int16(((int32(noiseLevel) * cMIN_SPEECH_SNR) >> 15) << 3)
	if st.speechLevel < temp {
		st.speechLevel = temp
	}
	ilog2NoiseLevel := vadIlog2(noiseLevel)

	// Remove the noise contribution from the speech level.
	ilog2SpeechLevel := vadIlog2(int16(int32(st.speechLevel) - int32(temp)))

	temp = int16((cNO_SLOPE*(int32(ilog2NoiseLevel)-cNO_P1))>>15 + cTHR_HIGH)

	temp2 := int16(cSP_CH_MIN + (cSP_SLOPE*(int32(ilog2SpeechLevel)-cSP_P1))>>15)
	if temp2 < cSP_CH_MIN {
		temp2 = cSP_CH_MIN
	}
	if temp2 > cSP_CH_MAX {
		temp2 = cSP_CH_MAX
	}
	vadThr := int16(int32(temp) + int32(temp2))
	if vadThr < cTHR_MIN {
		vadThr = cTHR_MIN
	}

	// Shift the decision register and make the intermediate decision.
	st.vadreg >>= 1
	if lSnrSum > (int32(vadThr)*(512*cCOMPLEN))<<1 {
		st.vadreg |= 0x4000
	}

	var lowPowerFlag int16
	if powSum < cVAD_POW_LOW {
		lowPowerFlag = 1
	}

	st.noiseEstimateUpdate(level)

	hangLen := int16((cHANG_SLOPE*(int32(vadThr)-cHANG_P1))>>15 + cHANG_HIGH)
	if hangLen < cHANG_LOW {
		hangLen = cHANG_LOW
	}
	burstLen := int16((cBURST_SLOPE*(int32(vadThr)-cBURST_P1))>>15 + cBURST_HIGH)

	return st.hangoverAddition(lowPowerFlag, hangLen, burstLen)
}

// estimateSpeech tracks the speech level (Estimate_Speech).
func (st *vadState) estimateSpeech(inLevel int16) {
	// If the required activity count cannot be achieved, reset the counters.
	if int32(st.spEstCnt)-int32(st.spMaxCnt) > cSP_EST_COUNT-cSP_ACTIVITY_COUNT {
		st.spEstCnt = 0
		st.spMax = 0
		st.spMaxCnt = 0
	}
	st.spEstCnt++

	if ((st.vadreg&0x4000) != 0 || inLevel > st.speechLevel) && inLevel > cMIN_SPEECH_LEVEL1 {
		if inLevel > st.spMax {
			st.spMax = inLevel
		}
		st.spMaxCnt++

		if st.spMaxCnt >= cSP_ACTIVITY_COUNT {
			tmp := st.spMax >> 1 // scale to get an "average" speech level
			var alpha int32
			if tmp > st.speechLevel {
				alpha = cALPHA_SP_UP
			} else {
				alpha = cALPHA_SP_DOWN
			}
			if tmp > cMIN_SPEECH_LEVEL2 {
				d := int32(tmp) - int32(st.speechLevel)
				st.speechLevel = int16(int32(st.speechLevel) + (alpha*d+0x4000)>>15)
			}
			st.spMax = 0
			st.spMaxCnt = 0
			st.spEstCnt = 0
		}
	}
}

// wbVadToneDetection sets the tone flag when the pitch gain is high
// (wb_vad_tone_detection).
func wbVadToneDetection(st *vadState, pGain int16) {
	st.toneFlag >>= 1
	if pGain > cTONE_THR {
		st.toneFlag |= 0x4000
	}
}

// wbVad returns the VAD decision for one frame: 1 = speech, 0 = noise (wb_vad).
func wbVad(st *vadState, inBuf []int16) int16 {
	var level [cCOMPLEN]int16

	// Power of the current frame, and of the current plus previous frame.
	var lTemp int32
	for i := 0; i < cVAD_FRAME_LEN; i++ {
		lTemp = L_mac(lTemp, inBuf[i], inBuf[i])
	}
	powSum := L_add(lTemp, st.prevPowSum)
	st.prevPowSum = lTemp

	// If the input power is very low, clear the tone flag.
	if powSum < cPOW_TONE_THR {
		st.toneFlag &= 0x1fff
	}

	st.filterBank(inBuf, &level)
	vadFlag := st.vadDecision(&level, powSum)

	// Input level, ignoring the lowest band.
	lTemp = 0
	for i := 1; i < cCOMPLEN; i++ {
		lTemp += int32(level[i])
	}
	st.estimateSpeech(int16((lTemp << 12) >> 16))
	return vadFlag
}
