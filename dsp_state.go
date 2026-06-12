package amrwb

// decoderStateDSP mirrors the opencore-amrwb Decoder_State (e_pv_amrwbdec.h):
// the persistent fixed-point memories carried across frames. Field names follow
// the C struct so the driver port reads against the source.
type decoderStateDSP struct {
	oldExc      [cPIT_MAX + cL_INTERPOL]int16
	ispold      [cM]int16
	isfold      [cM]int16
	isfBuf      [cL_MEANBUF * cM]int16
	pastIsfq    [cM]int16
	tiltCode    int16
	qOld        int16
	qsubfr      [4]int16
	lGcThres    int32
	memSynHi    [cM]int16
	memSynLo    [cM]int16
	memDeemph   int16
	memSigOut   [6]int16
	memOversamp [2 * cL_FILT]int16
	memSynHf    [cM16k]int16
	memHf       [2 * cL_FILT16k]int16
	memHf2      [2 * cL_FILT16k]int16
	memHf3      [2 * cL_FILT16k]int16
	seed        int16
	seed2       int16
	oldT0       int16
	oldT0Frac   int16
	lagHist     [5]int16
	decGain     [23]int16
	seed3       int16
	dispMem     [8]int16
	memHp400    [6]int16

	prevBfi    int16
	state      int16
	firstFrame int16
	dtxDecSt   dtxDecState
	vadHist    int16
}

// reset clears the decoder state. resetAll mirrors pvDecoder_AmrWb_Reset's
// reset_all flag: when true, all routine memories and init vectors are loaded.
func (st *decoderStateDSP) reset(resetAll bool) {
	for i := range st.oldExc {
		st.oldExc[i] = 0
	}
	for i := range st.pastIsfq {
		st.pastIsfq[i] = 0
	}
	st.oldT0Frac = 0
	st.oldT0 = 64
	st.firstFrame = 1
	st.lGcThres = 0
	st.tiltCode = 0
	for i := range st.dispMem {
		st.dispMem[i] = 0
	}
	st.qOld = cQ_MAX
	st.qsubfr[0], st.qsubfr[1], st.qsubfr[2], st.qsubfr[3] = cQ_MAX, cQ_MAX, cQ_MAX, cQ_MAX

	if !resetAll {
		return
	}

	decGain2Init(st.decGain[:])
	oversampInitMem(st.memOversamp[:])
	for i := range st.memHf {
		st.memHf[i] = 0
	}
	for i := range st.memHf3 {
		st.memHf3[i] = 0
	}
	for i := range st.memSigOut {
		st.memSigOut[i] = 0
	}
	for i := range st.memHp400 {
		st.memHp400[i] = 0
	}
	initLagconc(st.lagHist[:])

	copy(st.ispold[:], ispInit[:])
	copy(st.isfold[:], isfInit[:])
	for i := 0; i < cL_MEANBUF; i++ {
		copy(st.isfBuf[i*cM:i*cM+cM], isfInit[:])
	}
	st.memDeemph = 0
	st.seed = 21845
	st.seed2 = 21845
	st.seed3 = 21845
	st.state = 0
	st.prevBfi = 0
	for i := range st.memSynHf {
		st.memSynHf[i] = 0
	}
	for i := range st.memSynHi {
		st.memSynHi[i] = 0
	}
	for i := range st.memSynLo {
		st.memSynLo[i] = 0
	}
	st.dtxDecSt.reset(isfInit[:])
	st.vadHist = 0
}

// oversampInitMem zeros the 12.8->16 kHz oversampler memory (2*L_FILT).
func oversampInitMem(mem []int16) {
	for i := range mem {
		mem[i] = 0
	}
}
