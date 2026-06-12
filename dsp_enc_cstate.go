package amrwb

// Encoder state and small driver helpers, ported from the Apache-2.0
// vo-amrwbenc reference (cod_main.h, voAMRWBEnc.c Reset_encoder, deemph.c
// Deemph2), derived from 3GPP TS 26.190. VAD and DTX encode are stubbed for the
// initial (non-DTX) encode path; their full ports are tracked separately.

const cOLD_HP_WSP = cL_FRAME/cOPL_DECIM + cPIT_MAX/cOPL_DECIM // 243

// coderState mirrors the vo-amrwbenc Coder_State (cross-frame encoder memories).
type coderState struct {
	memDecim    [2 * cL_FILT16k]int16
	memSigIn    [6]int16
	memPreemph  int16
	oldSpeech   [cL_TOTAL - cL_FRAME]int16
	oldWsp      [cPIT_MAX / cOPL_DECIM]int16
	oldExc      [cPIT_MAX + cL_INTERPOL]int16
	memLevinson [cM + 2]int16
	ispold      [cM]int16
	ispoldQ     [cM]int16
	pastIsfq    [cM]int16
	memWsp      int16
	memDecim2   [3]int16
	memW0       int16
	memSyn      [cM]int16
	tiltCode    int16
	oldWspMax   int16
	oldWspShift int16
	qOld        int16
	qMax        [2]int16
	gpClip      [2]int16
	quaGain     [4]int16

	oldT0Med    int16
	olGain      int16
	adaW        int16
	olWghtFlg   int16
	oldOlLag    [5]int16
	hpWspMem    [9]int16
	oldHpWsp    [cOLD_HP_WSP]int16
	vadSt       vadState
	dtxEncSt    dtxEncState
	firstFrame  int16
	isfold      [cM]int16
	lGcThres    int32
	memSynHi    [cM]int16
	memSynLo    [cM]int16
	memDeemph   int16
	memSigOut   [6]int16
	memHp400    [6]int16
	memOversamp [2 * cL_FILT]int16
	memSynHf    [cM]int16
	memHf       [2 * cL_FILT16k]int16
	memHf2      [2 * cL_FILT16k]int16
	seed2       int16
	vadHist     int16
	gainAlpha   int16

	// synthesis() scratch (mode 8 HF gain analysis); fully overwritten each
	// call, reused to avoid per-subframe heap allocations.
	synScHi   [cM + cL_SUBFR]int16
	synScLo   [cM + cL_SUBFR]int16
	synScSyn  [cL_SUBFR]int16
	synScHf   [cL_SUBFR16k]int16
	synScHfSp [cL_SUBFR16k]int16
	synScAp   [cM + 1]int16
}

// reset mirrors Reset_encoder.
func (st *coderState) reset(resetAll bool) {
	zero16(st.oldExc[:])
	zero16(st.memSyn[:])
	zero16(st.pastIsfq[:])
	st.memW0 = 0
	st.tiltCode = 0
	st.firstFrame = 1
	initGpClip(st.gpClip[:])
	st.lGcThres = 0
	if !resetAll {
		return
	}
	zero16(st.oldSpeech[:])
	zero16(st.oldWsp[:])
	zero16(st.memDecim2[:])
	zero16(st.memDecim[:])
	zero16(st.memSigIn[:])
	zero16(st.memLevinson[:])
	initQGain2(st.quaGain[:])
	zero16(st.hpWspMem[:])
	copy(st.ispold[:], ispInit[:])
	copy(st.ispoldQ[:], ispInit[:])
	st.memPreemph = 0
	st.memWsp = 0
	st.qOld = 15
	st.qMax[0] = 15
	st.qMax[1] = 15
	st.oldWspMax = 0
	st.oldWspShift = 0
	st.oldT0Med = 40
	st.olGain = 0
	st.adaW = 0
	st.olWghtFlg = 0
	for i := range st.oldOlLag {
		st.oldOlLag[i] = 40
	}
	zero16(st.oldHpWsp[:])
	zero16(st.memSynHf[:])
	zero16(st.memSynHi[:])
	zero16(st.memSynLo[:])
	zero16(st.memSigOut[:])
	zero16(st.memHp400[:])
	zero16(st.memHf[:])
	zero16(st.memHf2[:])
	copy(st.isfold[:], isfInit[:])
	st.memDeemph = 0
	st.seed2 = 21845
	st.gainAlpha = 32767
	st.vadHist = 0
	st.vadSt.reset()
	st.dtxEncSt.reset(isfInit[:])
}

func zero16(s []int16) {
	for i := range s {
		s[i] = 0
	}
}

// Deemph2 applies 1/(1 - mu z^-1) in place (Deemph2).
func Deemph2(x []int16, mu, L int16, mem *int16) {
	lTmp := int32(x[0]) << 15
	lTmp += (int32(*mem) * int32(mu)) << 1
	x[0] = int16((lTmp + 0x8000) >> 16)
	for i := int16(1); i < L; i++ {
		lTmp = int32(x[i]) << 15
		lTmp += (int32(x[i-1]) * int32(mu)) << 1
		x[i] = int16((lTmp + 0x8000) >> 16)
	}
	*mem = x[L-1]
}

// --- VAD / DTX encode stubs (non-DTX speech path) ---

// vadState is a placeholder for the wb_vad analysis state. The full VAD is not
// yet ported; wbVad returns voice-active so the (decoder-ignored) vad_flag bit
// is deterministic. This is NOT bit-exact to the reference VAD.
type vadState struct{}

func (v *vadState) reset() {}

// wbVad returns 1 (voice active) as a placeholder.
func wbVad(v *vadState, buf []int16) int16 { return 1 }

// wbVadToneDetection is a no-op placeholder.
func wbVadToneDetection(v *vadState, olGain int16) {}

// dtxEncState is a placeholder for the DTX encoder state. Only the field the
// non-DTX synthesis path reads (dtxHangoverCount) is modelled; init to >6 so
// gain_alpha resolves to its non-DTX steady state.
type dtxEncState struct {
	dtxHangoverCount int16
}

func (d *dtxEncState) reset(isfInit []int16) { d.dtxHangoverCount = 7 }
