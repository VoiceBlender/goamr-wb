package amrwb

import "testing"

func TestAcelp2tEncodeDecodeRoundTrip(t *testing.T) {
	// Build a plausible target correlation dn[], residual cn[], and impulse
	// response h[]; run the 2-pulse search, then decode the produced index with
	// the validated decoder and confirm the codeword matches exactly.
	dn := make([]int16, cL_SUBFR)
	cn := make([]int16, cL_SUBFR)
	h := make([]int16, cL_SUBFR)
	for i := range dn {
		dn[i] = int16((i*53 - 1000) % 2000)
		cn[i] = int16((i*31 - 500) % 1500)
	}
	// Decaying impulse response.
	h[0] = 4096
	for i := 1; i < cL_SUBFR; i++ {
		h[i] = int16(int32(h[i-1]) * 30000 / 32768)
	}

	code := make([]int16, cL_SUBFR)
	y := make([]int16, cL_SUBFR)
	var index int16
	ACELP_2t64_fx(dn, cn, h, code, y, &index)

	// Exactly 2 pulses of ±512.
	pulses := 0
	for _, v := range code {
		if v != 0 {
			pulses++
			if v != 512 && v != -512 {
				t.Errorf("pulse amplitude %d, want ±512", v)
			}
		}
	}
	if pulses == 0 || pulses > 2 {
		t.Fatalf("got %d pulses, want 1..2", pulses)
	}

	// Decode the index with the validated decoder; codeword must match.
	dec := make([]int16, cL_SUBFR)
	dec_acelp_2p_in_64(index, dec)
	for i := range code {
		if code[i] != dec[i] {
			t.Fatalf("encode/decode mismatch at %d: enc=%d dec=%d (index=%d)",
				i, code[i], dec[i], index)
		}
	}
}
