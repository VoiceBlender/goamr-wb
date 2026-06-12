package amrwb

// RFC 4867 / 3GPP TS 26.201 bit de-sorting. The payload carries the speech bits
// in descending-sensitivity (sorted) order; the decoder consumes them in
// encoder-parameter order. mimeUnsort scatters transmitted bit k to internal
// parameter index sortTable[k], mirroring mime_unsorting in mime_io.cpp.

var sortTables = [10][]int16{
	sort_660, sort_885, sort_1265, sort_1425, sort_1585,
	sort_1825, sort_1985, sort_2305, sort_2385, sort_SID,
}

// mimeUnsort converts a packed (octet-aligned, MSB-first) frame for the given
// internal mode index into the per-bit parameter array consumed by the decoder.
//
// This replicates mime_unsorting (mime_io.cpp) exactly: it processes
// (unpacked_size>>3) whole bytes — 8 bits each, MSB-first — scattering each
// transmitted bit k to internal index sortTable[k]. (The reference's trailing
// %4 handler is a no-op because its working byte is already fully consumed, so
// the final unpacked_size%8 bits are not mapped; matching that is required for
// bit-exactness with the de-facto reference decoder.)
func mimeUnsort(packed []byte, mode int16) []int16 {
	if mode < 0 || int(mode) >= len(unpacked_size) {
		return nil
	}
	out := make([]int16, unpacked_size[mode])
	if int(mode) >= len(sortTables) || sortTables[mode] == nil {
		return out
	}
	st := sortTables[mode]
	nbytes := int(unpacked_size[mode]) >> 3
	k := 0
	for b := 0; b < nbytes && b < len(packed); b++ {
		temp := packed[b]
		for bit := 0; bit < 8; bit++ {
			if temp&0x80 != 0 {
				out[st[k]] = cBIT_1
			}
			temp <<= 1
			k++
		}
	}
	return out
}
