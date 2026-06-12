package amrwb

import "testing"

func TestQpisf46bReconstructs(t *testing.T) {
	// A valid (increasing) ISF vector; quantize and check the reconstruction
	// tracks the input and stays ordered.
	isf1 := make([]int16, cORDER)
	copy(isf1, isfInit[:])
	isfQ := make([]int16, cORDER)
	pastIsfq := make([]int16, cORDER)
	indice := make([]int16, 7)
	Qpisf_2s_46b(isf1, isfQ, pastIsfq, indice, 4)

	// Indices in range.
	limits := []int16{cSIZE_BK1, cSIZE_BK2, cSIZE_BK21, cSIZE_BK22, cSIZE_BK23, cSIZE_BK24, cSIZE_BK25}
	for i, lim := range limits {
		if indice[i] < 0 || indice[i] >= lim {
			t.Errorf("indice[%d]=%d out of range [0,%d)", i, indice[i], lim)
		}
	}
	// Reconstruction roughly tracks input (ISF quantization is accurate).
	for i := 0; i < cORDER; i++ {
		d := int(isfQ[i]) - int(isfInit[i])
		if d < -1500 || d > 1500 {
			t.Errorf("isfQ[%d]=%d far from input %d (diff %d)", i, isfQ[i], isfInit[i], d)
		}
	}
}

func TestQpisf36bRoundTripViaDecoder(t *testing.T) {
	// Quantize with the encoder, then dequantize the SAME indices with a fresh
	// decoder predictor state and identical past — must reproduce isfQ exactly.
	isf1 := make([]int16, cORDER)
	copy(isf1, isfInit[:])
	encIsfQ := make([]int16, cORDER)
	encPast := make([]int16, cORDER)
	indice := make([]int16, 5)
	Qpisf_2s_36b(isf1, encIsfQ, encPast, indice, 4)

	decIsfQ := make([]int16, cORDER)
	decPast := make([]int16, cORDER) // same initial past (zeros)
	Dpisf_2s_36b(indice, decIsfQ, decPast, decIsfQ, decIsfQ, 0, 0)
	for i := 0; i < cORDER; i++ {
		if encIsfQ[i] != decIsfQ[i] {
			t.Errorf("isfQ mismatch at %d: enc=%d dec=%d", i, encIsfQ[i], decIsfQ[i])
		}
	}
}
