package amrwb

// Encoder bit serialization and RFC 4867 packing, ported from the Apache-2.0
// vo-amrwbenc reference (bits.c Parm_serial / PackBits), derived from 3GPP TS
// 26.190. Packing is the exact inverse of the decoder's mimeUnsort: it gathers
// internal-order parameter bits into transmitted (sorted) order via the same
// sort tables.

// paramWriter serializes parameters into a per-bit array (internal order).
type paramWriter struct {
	bits []int16
	pos  int
}

// parm writes value as noOfBits bits, MSB-first (Parm_serial).
func (w *paramWriter) parm(value, noOfBits int16) {
	for i := int16(0); i < noOfBits; i++ {
		if (value>>uint(noOfBits-1-i))&1 != 0 {
			w.bits[w.pos] = cBIT_1
		} else {
			w.bits[w.pos] = cBIT_0
		}
		w.pos++
	}
}

// packFrame packs internal-order parameter bits prms into the octet-aligned
// frame payload for the given internal mode index (PackBits, speech path).
func packFrame(prms []int16, mode int16) []byte {
	if mode < 0 || int(mode) >= len(packed_size) {
		return nil
	}
	out := make([]byte, packed_size[mode])
	st := sortTables[mode]
	n := int(unpacked_size[mode])
	for i := 0; i < n && i < len(st); i++ {
		if prms[st[i]] == cBIT_1 {
			out[i/8] |= 1 << uint(7-i%8)
		}
	}
	return out
}
