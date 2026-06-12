package amrwb

import "testing"

func TestNoiseGenDeterministic(t *testing.T) {
	a := int16(cRANDOM_INITSEED)
	b := int16(cRANDOM_INITSEED)
	for i := 0; i < 100; i++ {
		if noise_gen_amrwb(&a) != noise_gen_amrwb(&b) {
			t.Fatal("noise generator not deterministic for equal seeds")
		}
	}
	// And it actually varies.
	s := int16(cRANDOM_INITSEED)
	first := noise_gen_amrwb(&s)
	if noise_gen_amrwb(&s) == first {
		t.Error("noise generator stuck")
	}
}

func TestHighpassZeroInput(t *testing.T) {
	for _, hp := range []func([]int16, int16, []int16){highpass_50Hz_at_12k8, highpass_400Hz_at_12k8} {
		sig := make([]int16, cL_SUBFR)
		mem := make([]int16, 6)
		hp(sig, cL_SUBFR, mem)
		for i, v := range sig {
			if v != 0 {
				t.Fatalf("highpass: zero input produced %d at %d", v, i)
			}
		}
	}
}

func TestBandPassRejectsDC(t *testing.T) {
	const lg = 256
	const C = 4000
	sig := make([]int16, lg)
	for i := range sig {
		sig[i] = C
	}
	mem := make([]int16, cL_FIR)
	x := make([]int16, cL_FIR+lg)
	band_pass_6k_7k(sig, lg, mem, x)
	// 6-7 kHz band-pass must reject DC: steady-state output ~0.
	for i := 100; i < lg; i++ {
		if sig[i] < -150 || sig[i] > 150 {
			t.Fatalf("band-pass leaked DC: sig[%d]=%d", i, sig[i])
		}
	}
}

func TestLowPassPassesDC(t *testing.T) {
	const lg = 256
	const C = 4000
	sig := make([]int16, lg)
	for i := range sig {
		sig[i] = C
	}
	mem := make([]int16, cL_FIR)
	x := make([]int16, cL_FIR+lg)
	low_pass_filt_7k(sig, lg, mem, x)
	// 7 kHz low-pass must pass DC at ~unity gain in steady state.
	for i := 100; i < lg; i++ {
		if d := int(sig[i]) - C; d < -C/10 || d > C/10 {
			t.Fatalf("low-pass attenuated DC: sig[%d]=%d (want ~%d)", i, sig[i], C)
		}
	}
}

func TestWeightLpc(t *testing.T) {
	a := make([]int16, cM+1)
	for i := range a {
		a[i] = 4096
	}
	ap := make([]int16, cM+1)
	weight_amrwb_lpc(a, ap, cGAMMA1, cM)
	if ap[0] != a[0] {
		t.Errorf("ap[0]=%d, want %d", ap[0], a[0])
	}
	// Weights shrink with index (gamma^i, gamma<1).
	for i := 2; i <= cM; i++ {
		if abs16(ap[i]) > abs16(ap[i-1]) {
			t.Errorf("weight not decreasing at %d: %d > %d", i, ap[i], ap[i-1])
		}
	}
}

func TestAgc2RaisesLowOutput(t *testing.T) {
	in := make([]int16, cL_SUBFR16k)
	out := make([]int16, cL_SUBFR16k)
	for i := range in {
		in[i] = 8000
		out[i] = 1000
	}
	agc2_amr_wb(in, out, cL_SUBFR16k)
	// Output energy was far below input; AGC must raise it.
	if out[10] <= 1000 {
		t.Errorf("agc2 did not raise low-energy output: %d", out[10])
	}
}

func abs16(v int16) int16 {
	if v < 0 {
		return -v
	}
	return v
}
