package amrwb

import "testing"

// Cross-validate the opencore decoder operators against the independently
// ported ITU operators in basicops.go over the full int16 range / sampled
// int32 range. Two independent ports agreeing is strong evidence of
// correctness for the overlapping ops.
func TestOpsCrossValidate16(t *testing.T) {
	vals := []int16{min16, -32767, -1000, -1, 0, 1, 1000, 16384, 32766, max16}
	for _, a := range vals {
		for _, b := range vals {
			if add_int16(a, b) != add(a, b) {
				t.Errorf("add_int16(%d,%d)=%d != add=%d", a, b, add_int16(a, b), add(a, b))
			}
			if sub_int16(a, b) != sub(a, b) {
				t.Errorf("sub_int16(%d,%d) mismatch", a, b)
			}
			if mult_int16(a, b) != mult(a, b) {
				t.Errorf("mult_int16(%d,%d)=%d != mult=%d", a, b, mult_int16(a, b), mult(a, b))
			}
			if mul_16by16_to_int32(a, b) != L_mult(a, b) {
				t.Errorf("mul_16by16_to_int32(%d,%d) mismatch", a, b)
			}
		}
	}
}

func TestMacMsuCrossValidate(t *testing.T) {
	accs := []int32{min32, -1 << 20, -1, 0, 1, 1 << 20, max32}
	vals := []int16{min16, -1, 0, 1, 16384, max16}
	for _, acc := range accs {
		for _, a := range vals {
			for _, b := range vals {
				if mac_16by16_to_int32(acc, a, b) != L_mac(acc, a, b) {
					t.Errorf("mac(%d,%d,%d) mismatch: %d vs %d", acc, a, b,
						mac_16by16_to_int32(acc, a, b), L_mac(acc, a, b))
				}
				if msu_16by16_from_int32(acc, a, b) != L_msu(acc, a, b) {
					t.Errorf("msu(%d,%d,%d) mismatch", acc, a, b)
				}
			}
		}
	}
}

func TestNormalizeAmrWb(t *testing.T) {
	// normalize_amr_wb returns the left-shift to normalize x. For an already
	// large value (bit 28+ set) it returns 0.
	if got := normalize_amr_wb(0x40000000); got != 0 {
		t.Errorf("normalize_amr_wb(0x40000000)=%d, want 0", got)
	}
	// Verify the post-shift lands in the normalized band [0x40000000,0x7fffffff].
	for _, x := range []int32{1, 15, 255, 4095, 0x10000, 0x100000, 0x1000000, 0x12345} {
		n := normalize_amr_wb(x)
		shifted := x << uint(n)
		if shifted < 0x40000000 || shifted > 0x7fffffff {
			t.Errorf("normalize_amr_wb(%#x)=%d -> %#x not normalized", x, n, shifted)
		}
	}
}

func TestDiv16by16(t *testing.T) {
	cases := map[[2]int16]int16{
		{1, 2}:     16384, // 0.5 in Q15
		{1, 4}:     8192,  // 0.25
		{3, 4}:     24576, // 0.75
		{100, 100}: 32767, // equal -> max
		{0, 100}:   0,
		{5, 4}:     0, // var1>var2 -> 0
	}
	for in, want := range cases {
		if got := div_16by16(in[0], in[1]); got != want {
			t.Errorf("div_16by16(%d,%d)=%d, want %d", in[0], in[1], got, want)
		}
	}
}

func TestMultInt16R(t *testing.T) {
	if got := mult_int16_r(16384, 16384); got != 8192 {
		t.Errorf("mult_int16_r(16384,16384)=%d, want 8192", got)
	}
	if got := mult_int16_r(min16, min16); got != max16 {
		t.Errorf("mult_int16_r(min,min)=%d, want max", got)
	}
}

func TestPowerOf2(t *testing.T) {
	if got := power_of_2(0, 0); got != 1 {
		t.Errorf("power_of_2(0,0)=%d, want 1 (2^0)", got)
	}
	if got := power_of_2(0, 32767); got != 2 {
		t.Errorf("power_of_2(0,~1)=%d, want 2", got)
	}
}

func TestOneOvSqrt(t *testing.T) {
	// 1/sqrt(2^30) = 2^-15; in Q31 that is 2^16 = 65536 (±interp tolerance).
	got := one_ov_sqrt(0x40000000)
	if got < 65000 || got > 66000 {
		t.Errorf("one_ov_sqrt(2^30)=%d, want ~65536", got)
	}
	// Non-positive input yields the saturated 0x7fffffff.
	if got := one_ov_sqrt(0); got != 0x7fffffff {
		t.Errorf("one_ov_sqrt(0)=%d, want 0x7fffffff", got)
	}
}

func TestDpfRoundTrip(t *testing.T) {
	for _, l := range []int32{0, 1, 1234567, -1234567, max32, min32 + 1} {
		var hi, lo int16
		int32ToDpf(l, &hi, &lo)
		recon := (int32(hi) << 16) + (int32(lo) << 1)
		if d := recon - l; d < -1 || d > 1 {
			t.Errorf("dpf round-trip %d -> %d (diff %d)", l, recon, d)
		}
	}
}
