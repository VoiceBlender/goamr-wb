package amrwb

import "testing"

func TestEncIsqrt(t *testing.T) {
	// 1/sqrt(2^30) = 2^-15; in Q31 = 2^16 = 65536 (± interpolation tolerance).
	got := encIsqrt(0x40000000)
	if got < 65000 || got > 66000 {
		t.Errorf("encIsqrt(2^30)=%d, want ~65536", got)
	}
}

func TestEncPow2(t *testing.T) {
	if got := encPow2(0, 0); got != 1 {
		t.Errorf("encPow2(0,0)=%d, want 1", got)
	}
	// pow(2, 1.0) with fraction ~1 and exponant 1.
	if got := encPow2(1, 0); got != 2 {
		t.Errorf("encPow2(1,0)=%d, want 2", got)
	}
}

func TestDpfRoundTripEnc(t *testing.T) {
	for _, l := range []int32{0, 1, 1234567, -1234567, max32, min32 + 1} {
		hi, lo := voLExtract(l)
		recon := lComp(hi, lo)
		if d := recon - l; d < -2 || d > 2 {
			t.Errorf("DPF round-trip %d -> %d", l, recon)
		}
	}
}

func TestLShl2(t *testing.T) {
	if got := lShl2(1000, 0); got != 0 {
		t.Errorf("lShl2(x,0)=%d, want 0", got)
	}
	if got := lShl2(100, 3); got != 800 {
		t.Errorf("lShl2(100,3)=%d, want 800", got)
	}
	if got := lShl2(0x40000000, 4); got != max32 {
		t.Errorf("lShl2 overflow=%d, want max32", got)
	}
}

func TestEncDotProduct(t *testing.T) {
	x := []int16{1000, -2000, 3000, -4000}
	var exp int16
	got := encDotProduct12(x, x, 4, &exp)
	// Self dot product must be positive and normalized (top bits set).
	if got <= 0 {
		t.Errorf("encDotProduct12 self = %d, want > 0", got)
	}
	if got < 0x40000000 {
		t.Errorf("encDotProduct12 not normalized: %#x", got)
	}
}
