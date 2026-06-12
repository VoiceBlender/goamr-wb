package amrwb

import "testing"

// TestFIRRawMatchesGeneric fuzzes firRaw (AVX2 across-output kernel) against the
// portable oracle over output counts spanning the 8-wide group + scalar tail,
// and tap counts including the symmetric-FIR length (31). Correctness gate for
// the FIR assembly.
func TestFIRRawMatchesGeneric(t *testing.T) {
	seed := uint32(0x12345678)
	next := func() int16 {
		seed = seed*1664525 + 1013904223
		return int16(seed >> 16)
	}
	for _, taps := range []int{1, 2, 8, 15, 30, 31, 32, 48, 64} {
		coef := make([]int16, taps)
		for i := range coef {
			coef[i] = next()
		}
		for nOut := 0; nOut <= 90; nOut++ {
			x := make([]int16, nOut+taps) // >= nOut+taps-1
			for i := range x {
				x[i] = next()
			}
			want := make([]int32, nOut)
			got := make([]int32, nOut)
			firRawGeneric(want, x, coef)
			firRaw(got, x, coef)
			for o := 0; o < nOut; o++ {
				if got[o] != want[o] {
					t.Fatalf("taps=%d nOut=%d o=%d: firRaw=%d want=%d", taps, nOut, o, got[o], want[o])
				}
			}
		}
	}

	// Worst-case magnitudes (all -32768) force 32-bit overflow; the kernel must
	// still match the wrapping reference.
	taps, nOut := 64, 80
	coef := make([]int16, taps)
	x := make([]int16, nOut+taps)
	for i := range coef {
		coef[i] = -32768
	}
	for i := range x {
		x[i] = -32768
	}
	want := make([]int32, nOut)
	got := make([]int32, nOut)
	firRawGeneric(want, x, coef)
	firRaw(got, x, coef)
	for o := 0; o < nOut; o++ {
		if got[o] != want[o] {
			t.Fatalf("overflow o=%d: firRaw=%d want=%d", o, got[o], want[o])
		}
	}
}
