package amrwb

import "testing"

func TestParmSerialMSBFirst(t *testing.T) {
	w := &paramWriter{bits: make([]int16, 16)}
	w.parm(0b1011, 4) // MSB-first: 1,0,1,1
	want := []int16{cBIT_1, cBIT_0, cBIT_1, cBIT_1}
	for i, v := range want {
		if w.bits[i] != v {
			t.Errorf("bit %d: got %d want %d", i, w.bits[i], v)
		}
	}
}

// Encoder packFrame must be the inverse of the decoder mimeUnsort: pack an
// internal-order parameter array, then unsort the packed bytes; the internal
// positions that survive the reference's (unpacked_size>>3)*8 truncation must
// be recovered exactly.
func TestPackUnsortRoundTrip(t *testing.T) {
	for mode := 0; mode <= 8; mode++ {
		n := int(unpacked_size[mode])
		prms := make([]int16, n)
		seed := uint32(0x9e37 + mode)
		for i := range prms {
			seed = seed*1103515245 + 12345
			if (seed>>20)&1 == 1 {
				prms[i] = cBIT_1
			} else {
				prms[i] = cBIT_0
			}
		}
		packed := packFrame(prms, int16(mode))
		got := mimeUnsort(packed, int16(mode))

		st := sortTables[mode]
		nbytes := n >> 3
		for i := 0; i < nbytes*8; i++ { // only the bits the decoder maps
			idx := st[i]
			gv := got[idx] == cBIT_1
			pv := prms[idx] == cBIT_1
			if gv != pv {
				t.Fatalf("mode %d: internal bit %d (transmitted %d) mismatch: pack=%v unsort=%v",
					mode, idx, i, pv, gv)
			}
		}
	}
}
