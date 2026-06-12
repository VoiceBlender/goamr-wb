package amrwb

import "testing"

func bitsFor(vals ...int) []int16 {
	b := make([]int16, len(vals))
	for i, v := range vals {
		if v != 0 {
			b[i] = cBIT_1
		} else {
			b[i] = cBIT_0
		}
	}
	return b
}

func TestSerialParm(t *testing.T) {
	// 0b101 = 5 over 3 bits, MSB-first.
	p := &bitParser{bits: bitsFor(1, 0, 1)}
	if got := p.parm(3); got != 5 {
		t.Errorf("parm(3) of 101 = %d, want 5", got)
	}
	// 0b1100 = 12 over 4 bits.
	p = &bitParser{bits: bitsFor(1, 1, 0, 0)}
	if got := p.parm(4); got != 12 {
		t.Errorf("parm(4) of 1100 = %d, want 12", got)
	}
	// Sequential fields: read 2 then 3 bits from 11010 -> 3, then 010=2.
	p = &bitParser{bits: bitsFor(1, 1, 0, 1, 0)}
	if got := p.parm(2); got != 3 {
		t.Errorf("first parm(2) = %d, want 3", got)
	}
	if got := p.parm(3); got != 2 {
		t.Errorf("second parm(3) = %d, want 2", got)
	}
}

func TestSerialParm1bit(t *testing.T) {
	p := &bitParser{bits: bitsFor(1, 0, 1)}
	if p.parm1() != 1 || p.parm1() != 0 || p.parm1() != 1 {
		t.Error("parm1 sequence wrong")
	}
}

func TestExpandBitsToParams(t *testing.T) {
	// 0xA0 = 1010_0000; first 4 bits -> 1,0,1,0.
	got := expandBitsToParams([]byte{0xA0}, 4)
	want := bitsFor(1, 0, 1, 0)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("bit %d: got %d want %d", i, got[i], want[i])
		}
	}
	// Round-trip: expand then read back as a field.
	p := &bitParser{bits: expandBitsToParams([]byte{0xA0}, 8)}
	if v := p.parm(8); v != 0xA0 {
		t.Errorf("parm(8) of 0xA0 = %d, want 160", v)
	}
}

func TestAmrwbCompressedTable(t *testing.T) {
	if amrwbCompressed[dMODE_7k] != cNBBITS_7k || amrwbCompressed[dMRDTX] != cNBBITS_SID {
		t.Error("AMR_WB_COMPRESSED table mismatch")
	}
}
