package amrwb

import "testing"

// Weight_a (encoder) must match the validated decoder weight_amrwb_lpc — they
// are the same bandwidth-expansion with equivalent rounding.
func TestWeightAMatchesDecoder(t *testing.T) {
	a := make([]int16, cM+1)
	a[0] = 4096
	for i := 1; i <= cM; i++ {
		a[i] = int16(2000 - i*173)
	}
	enc := make([]int16, cM+1)
	dec := make([]int16, cM+1)
	Weight_a(a, enc, cGAMMA1, cM)
	weight_amrwb_lpc(a, dec, cGAMMA1, cM)
	for i := 0; i <= cM; i++ {
		if enc[i] != dec[i] {
			t.Errorf("Weight_a[%d]=%d != weight_amrwb_lpc=%d", i, enc[i], dec[i])
		}
	}
}

func TestPreemph(t *testing.T) {
	// (1 - mu z^-1): a constant input becomes constant*(1-mu) after the first
	// sample (steady state), with the first sample using mem.
	x := make([]int16, 8)
	for i := range x {
		x[i] = 1000
	}
	var mem int16
	Preemph(x, cPREEMPH_FAC, 8, &mem)
	// x[1..] = 1000 - 1000*mu (mu≈0.68) ~ 320.
	for i := 1; i < 8; i++ {
		if x[i] < 280 || x[i] > 360 {
			t.Errorf("Preemph steady x[%d]=%d, want ~320", i, x[i])
		}
	}
	if mem != 1000 {
		t.Errorf("Preemph mem=%d, want 1000", mem)
	}
}

func TestScaleSigEnc(t *testing.T) {
	x := []int16{100, -100, 1000}
	Scale_sig(x, 3, 2) // *4
	want := []int16{400, -400, 4000}
	for i := range x {
		if x[i] != want[i] {
			t.Errorf("Scale_sig x[%d]=%d, want %d", i, x[i], want[i])
		}
	}
}

func TestResiduConvolveShapes(t *testing.T) {
	// Smoke: Residu over a buffer with history, Convolve of an impulse response.
	a := make([]int16, cM+1)
	a[0] = 4096
	buf := make([]int16, cM+cL_SUBFR)
	for i := range buf {
		buf[i] = int16((i * 37) % 500)
	}
	y := make([]int16, cL_SUBFR)
	Residu(a, buf, cM, y, cL_SUBFR)

	h := make([]int16, 64)
	h[0] = 4096
	x := make([]int16, 64)
	for i := range x {
		x[i] = int16(i)
	}
	out := make([]int16, 64)
	Convolve(x, h, out)
	// With h = unit impulse (Q12 1.0), Convolve ~ x scaled by h[0]/2^? -> nonzero.
	if out[10] == 0 && out[20] == 0 {
		t.Error("Convolve produced all-zero output for impulse response")
	}
}
