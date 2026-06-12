package amrwb

import "testing"

func TestSaturation(t *testing.T) {
	if got := add(maxInt16, 1); got != maxInt16 {
		t.Errorf("add overflow: got %d, want %d", got, maxInt16)
	}
	if got := sub(minInt16, 1); got != minInt16 {
		t.Errorf("sub underflow: got %d, want %d", got, minInt16)
	}
	if got := abs_s(minInt16); got != maxInt16 {
		t.Errorf("abs_s(min): got %d, want %d", got, maxInt16)
	}
	if got := negate(minInt16); got != maxInt16 {
		t.Errorf("negate(min): got %d, want %d", got, maxInt16)
	}
}

func TestMult(t *testing.T) {
	// mult(a,b) = (a*b)>>15
	if got := mult(16384, 16384); got != 8192 {
		t.Errorf("mult: got %d, want 8192", got)
	}
	// L_mult(a,b) = a*b<<1
	if got := L_mult(16384, 16384); got != 0x20000000 {
		t.Errorf("L_mult: got %#x, want 0x20000000", got)
	}
	// L_mult saturation of the special case 0x40000000
	if got := L_mult(minInt16, minInt16); got != maxInt32 {
		t.Errorf("L_mult(min,min): got %#x, want maxInt32", got)
	}
}

func TestMacMsu(t *testing.T) {
	acc := int32(0)
	acc = L_mac(acc, 100, 200)
	if want := L_mult(100, 200); acc != want {
		t.Errorf("L_mac: got %d, want %d", acc, want)
	}
	acc = L_msu(acc, 100, 200)
	if acc != 0 {
		t.Errorf("L_msu round-trip: got %d, want 0", acc)
	}
}

func TestRoundAndExtract(t *testing.T) {
	v := int32(0x12348000)
	if got := round_(v); got != 0x1235 {
		t.Errorf("round_: got %#x, want 0x1235", got)
	}
	if got := extract_h(0x12345678); got != 0x1234 {
		t.Errorf("extract_h: got %#x", got)
	}
	if got := extract_l(0x12345678); got != 0x5678 {
		t.Errorf("extract_l: got %#x", got)
	}
	if got := L_deposit_h(0x1234); got != 0x12340000 {
		t.Errorf("L_deposit_h: got %#x", got)
	}
}

func TestShifts(t *testing.T) {
	if got := shl(0x1000, 4); got != maxInt16 {
		t.Errorf("shl overflow: got %d, want max", got)
	}
	if got := shr(-256, 4); got != -16 {
		t.Errorf("shr: got %d, want -16", got)
	}
	if got := L_shl(0x10000000, 4); got != maxInt32 {
		t.Errorf("L_shl overflow: got %#x, want max", got)
	}
	if got := L_shr(-1024, 4); got != -64 {
		t.Errorf("L_shr: got %d, want -64", got)
	}
	// negative shift counts reverse direction
	if got := shl(256, -4); got != 16 {
		t.Errorf("shl negative: got %d, want 16", got)
	}
}

func TestNorm(t *testing.T) {
	if got := norm_l(0x00010000); got != 14 {
		t.Errorf("norm_l: got %d, want 14", got)
	}
	if got := norm_l(0); got != 0 {
		t.Errorf("norm_l(0): got %d, want 0", got)
	}
	if got := norm_s(0x0040); got != 8 {
		t.Errorf("norm_s: got %d, want 8", got)
	}
}

func TestDivS(t *testing.T) {
	if got := div_s(0, 100); got != 0 {
		t.Errorf("div_s(0): got %d, want 0", got)
	}
	if got := div_s(100, 100); got != maxInt16 {
		t.Errorf("div_s(equal): got %d, want max", got)
	}
	// 1/2 in Q15 ~= 16384
	if got := div_s(1, 2); got != 16384 {
		t.Errorf("div_s(1,2): got %d, want 16384", got)
	}
}
