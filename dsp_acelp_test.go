package amrwb

import "testing"

func TestDecAcelp2pTwoPulses(t *testing.T) {
	code := make([]int16, cL_CODE)
	dec_acelp_2p_in_64(0x0A5, code)
	var nonzero, energy int
	for _, v := range code {
		if v != 0 {
			nonzero++
			if v != 512 && v != -512 {
				t.Errorf("pulse amplitude %d, want ±512", v)
			}
			energy += int(v) * int(v)
		}
	}
	if nonzero == 0 || nonzero > 2 {
		t.Errorf("2p codeword has %d pulses, want 1..2", nonzero)
	}
}

func TestDecAcelp4pMode20(t *testing.T) {
	// 20-bit mode: one pulse per track (4 tracks) -> up to 4 ±512 pulses.
	index := []int16{1, 5, 9, 13}
	code := make([]int16, cL_CODE)
	dec_acelp_4p_in_64(index, 20, code)
	pulses := 0
	for i, v := range code {
		if v != 0 {
			pulses++
			if v != 512 && v != -512 {
				t.Errorf("code[%d]=%d, want ±512", i, v)
			}
		}
	}
	if pulses == 0 || pulses > 4 {
		t.Errorf("mode-20 codeword has %d pulses, want 1..4", pulses)
	}
}

func TestPredLt4DCIntegerLag(t *testing.T) {
	// With frac=0 the fractional interpolation is ~unity gain, so a constant
	// past excitation must reproduce ~that constant.
	const off = 300
	const C = 4000
	exc := make([]int16, off+200)
	for i := 0; i < off+128; i++ {
		exc[i] = C
	}
	Pred_lt4(exc, off, 64, 0, 64)
	for i := off; i < off+64; i++ {
		if d := int(exc[i]) - C; d < -8 || d > 8 {
			t.Fatalf("exc[%d]=%d not ~%d", i, exc[i], C)
		}
	}
}
