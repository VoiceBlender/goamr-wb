package amrwb

import "testing"

func TestGPitchUnity(t *testing.T) {
	// xn == y1 -> pitch gain = 1.0 (Q14 = 16384).
	y1 := make([]int16, cL_SUBFR)
	for i := range y1 {
		y1[i] = int16(2000 - i*20)
	}
	xn := append([]int16(nil), y1...)
	g := make([]int16, 4)
	if got := G_pitch(xn, y1, g, cL_SUBFR); got < 16300 || got > 16450 {
		t.Errorf("G_pitch unity = %d, want ~16384", got)
	}
	// Negative correlation -> zero gain.
	for i := range xn {
		xn[i] = -y1[i]
	}
	if got := G_pitch(xn, y1, g, cL_SUBFR); got != 0 {
		t.Errorf("G_pitch anti-correlated = %d, want 0", got)
	}
}

func TestUpdtTarZeroGain(t *testing.T) {
	x := make([]int16, cL_SUBFR)
	y := make([]int16, cL_SUBFR)
	x2 := make([]int16, cL_SUBFR)
	for i := range x {
		x[i] = int16(i * 10)
		y[i] = int16(i)
	}
	Updt_tar(x, x2, y, 0, cL_SUBFR)
	for i := range x {
		if x2[i] != x[i] {
			t.Fatalf("Updt_tar gain=0: x2[%d]=%d != x=%d", i, x2[i], x[i])
		}
	}
}

func TestGpClip(t *testing.T) {
	mem := make([]int16, 2)
	initGpClip(mem)
	if gpClip(mem) != 0 {
		t.Error("fresh gp_clip should not clip")
	}
	// Drive ISF distance small and pitch gain high -> clip engages.
	closeIsf := make([]int16, cM)
	for i := range closeIsf {
		closeIsf[i] = int16(i * 100) // small consecutive distance
	}
	for n := 0; n < 20; n++ {
		gpClipTestIsf(closeIsf, mem)
		gpClipTestGainPit(16384, mem)
	}
	if gpClip(mem) != 1 {
		t.Errorf("gp_clip should engage with close ISF + high gain: mem=%v", mem)
	}
}

func TestCorHxNonzero(t *testing.T) {
	h := make([]int16, cL_SUBFR)
	x := make([]int16, cL_SUBFR)
	h[0] = 4096
	h[1] = 2048
	for i := range x {
		x[i] = int16(500 - i*7)
	}
	dn := make([]int16, cL_SUBFR)
	cor_h_x(h, x, dn)
	nz := 0
	for _, v := range dn {
		if v != 0 {
			nz++
		}
	}
	if nz == 0 {
		t.Error("cor_h_x produced all-zero correlation")
	}
}

func TestPitchFr4FindsPeriod(t *testing.T) {
	const period = 60
	const off = 300
	exc := make([]int16, off+cL_SUBFR)
	for i := range exc {
		// strongly periodic excitation at `period`
		exc[i] = int16(6000 * sinApprox(float64(i)*2*3.14159265/period))
	}
	xn := make([]int16, cL_SUBFR)
	copy(xn, exc[off:off+cL_SUBFR])
	h := make([]int16, cL_SUBFR)
	h[0] = 8192 // ~0.25 in Q15, near-impulse response
	var frac int16
	t0 := Pitch_fr4(exc, off, xn, h, period-8, period+8, &frac, 0, cPIT_FR2, cPIT_FR1_9b, cL_SUBFR)
	if t0 < period-2 || t0 > period+2 {
		t.Errorf("Pitch_fr4 found lag %d (frac %d), want ~%d", t0, frac, period)
	}
}
