package amrwb

import "testing"

func TestOversampLengthAndDC(t *testing.T) {
	const lg = 256
	const C = 1000
	sig := make([]int16, lg)
	for i := range sig {
		sig[i] = C
	}
	out := make([]int16, lg+lg/4)
	mem := make([]int16, 2*nbCoefUp)
	scratch := make([]int16, 2*nbCoefUp+lg)

	oversamp_12k8_to_16k(sig, lg, out, mem, scratch)

	// Steady-state DC: a constant input must oversample to ~constant once the
	// filter clears the zero-initialized memory (interp filter is unity-gain).
	for i := 100; i < len(out); i++ {
		if d := int(out[i]) - C; d < -2 || d > 2 {
			t.Fatalf("out[%d]=%d not ~%d (DC invariance broken)", i, out[i], C)
		}
	}
}

func TestDeemphasisZeroInput(t *testing.T) {
	const L = 64
	xHi := make([]int16, L)
	xLo := make([]int16, L)
	y := make([]int16, L)
	var mem int16
	deemphasis_32(xHi, xLo, y, 22282, L, &mem) // mu = 0.68 Q15
	for i, v := range y {
		if v != 0 {
			t.Fatalf("zero input produced y[%d]=%d", i, v)
		}
	}
	if mem != 0 {
		t.Fatalf("mem=%d, want 0", mem)
	}
}

func TestDeemphasisDecays(t *testing.T) {
	// An initial memory value with zero input must decay monotonically toward
	// zero through the 1/(1-mu z^-1) recursion (all same sign, shrinking).
	const L = 16
	xHi := make([]int16, L)
	xLo := make([]int16, L)
	y := make([]int16, L)
	mem := int16(10000)
	deemphasis_32(xHi, xLo, y, 22282, L, &mem)
	prev := int16(32767)
	for i, v := range y {
		if v < 0 || v > prev {
			t.Fatalf("non-decaying response at y[%d]=%d (prev %d)", i, v, prev)
		}
		prev = v
	}
}
