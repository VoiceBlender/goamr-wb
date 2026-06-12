package amrwb

import "testing"

func TestDecoderStateReset(t *testing.T) {
	var st decoderStateDSP
	st.reset(true)
	if st.qOld != cQ_MAX || st.oldT0 != 64 || st.firstFrame != 1 {
		t.Errorf("reset basics wrong: qOld=%d oldT0=%d first=%d", st.qOld, st.oldT0, st.firstFrame)
	}
	if st.seed != 21845 || st.seed2 != 21845 || st.seed3 != 21845 {
		t.Error("seeds not initialized")
	}
	for i := 0; i < cL_LTPHIST; i++ {
		if st.lagHist[i] != 64 {
			t.Errorf("lagHist[%d]=%d, want 64", i, st.lagHist[i])
		}
	}
	if st.dtxDecSt.dtxGlobalState != cSPEECH || st.dtxDecSt.logEn != 3500 {
		t.Error("dtx state not reset")
	}
	// ispold/isfold seeded from init vectors.
	if st.ispold[0] != ispInit[0] || st.isfold[0] != isfInit[0] {
		t.Error("isp/isf init not loaded")
	}
}

func TestPhaseDispersionOffPreservesCode(t *testing.T) {
	code := make([]int16, cL_SUBFR)
	for i := range code {
		code[i] = int16(100 - i)
	}
	orig := append([]int16(nil), code...)
	dispMem := make([]int16, 8)
	// mode=2 (off), voiced gain, positive gain_code -> no dispersion applied.
	phase_dispersion(1000, cPITCH_0_9, code, 2, dispMem)
	for i := range code {
		if code[i] != orig[i] {
			t.Fatalf("phase dispersion (off) changed code[%d]: %d != %d", i, code[i], orig[i])
		}
	}
}

func TestIsfExtrapolationOrders(t *testing.T) {
	hf := make([]int16, cM16k)
	copy(hf, isfInit[:])
	isf_extrapolation(hf)
	// After extrapolation + Isf_isp the result is the ISP (cosine) domain, which
	// is non-increasing; just assert it ran and filled the extended vector.
	for i := 0; i < cM16k-1; i++ {
		if hf[i] == 0 && hf[i+1] == 0 {
			t.Fatalf("isf_extrapolation produced all-zero region at %d", i)
		}
	}
}

func TestSynthesisRuns(t *testing.T) {
	var st decoderStateDSP
	st.reset(true)
	aq := make([]int16, cM+1)
	aq[0] = 4096
	exc := make([]int16, cL_SUBFR)
	hfIsf := make([]int16, cM16k)
	synth16k := make([]int16, cL_SUBFR16k)
	// nbBits in (7k, 24k): exercises the else HF branch, no isf_extrapolation.
	synthesisAmrWb(aq, exc, 1, synth16k, 0, hfIsf, cNBBITS_12k, cSPEECH, &st, 0)
	// Zero excitation -> output should be silent-ish (small magnitude), no panic.
	for i, v := range synth16k {
		if v > 4000 || v < -4000 {
			t.Fatalf("unexpected large synthesis sample at %d: %d", i, v)
		}
	}
}
