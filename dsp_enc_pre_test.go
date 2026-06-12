package amrwb

import "testing"

func TestIspIsfRoundTrip(t *testing.T) {
	// ISF -> ISP (decoder Isf_isp) -> ISF (encoder Isp_isf) must recover input.
	isf := make([]int16, cM)
	copy(isf, isfInit[:])
	isp := make([]int16, cM)
	Isf_isp(isf, isp, cM)
	isf2 := make([]int16, cM)
	Isp_isf(isp, isf2, cM)
	for i := 0; i < cM; i++ {
		d := int(isf[i]) - int(isf2[i])
		if d < -4 || d > 4 {
			t.Errorf("ISF round-trip [%d]: %d -> %d (diff %d)", i, isf[i], isf2[i], d)
		}
	}
}

func TestDecim12k8DCAndLength(t *testing.T) {
	const lg = cL_FRAME16k
	const C = 4000
	in := make([]int16, lg)
	for i := range in {
		in[i] = C
	}
	out := make([]int16, lg) // lg*4/5 used
	mem := make([]int16, 2*cNB_COEF_DOWN)
	Decim_12k8(in, lg, out, mem)
	n := lg * cDOWN_FAC >> 15 // 256
	if n != 256 {
		t.Fatalf("decimated length %d, want 256", n)
	}
	// Steady-state DC preserved (decimation FIR is unity gain).
	for i := 50; i < n; i++ {
		if d := int(out[i]) - C; d < -40 || d > 40 {
			t.Fatalf("Decim DC out[%d]=%d, want ~%d", i, out[i], C)
		}
	}
}

func TestHP50RejectsDC(t *testing.T) {
	const lg = 256
	sig := make([]int16, lg)
	for i := range sig {
		sig[i] = 4000
	}
	mem := make([]int16, 6)
	HP50_12k8(sig, lg, mem)
	// 50 Hz cutoff at 12.8 kHz settles slowly (~40-sample time constant); check
	// the tail is well attenuated relative to the 4000 DC input.
	for i := 150; i < lg; i++ {
		if sig[i] < -1000 || sig[i] > 1000 {
			t.Fatalf("HP50 insufficient DC attenuation at %d: %d (input 4000)", i, sig[i])
		}
	}
}
