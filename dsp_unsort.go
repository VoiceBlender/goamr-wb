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
// This replicates mime_unsorting (mime_io.cpp): it processes (unpacked_size>>3)
// whole bytes, then the remaining unpacked_size%8 bits of the next byte, all
// MSB-first, scattering each transmitted bit k to internal index sortTable[k].
func mimeUnsort(packed []byte, mode int16) []int16 {
	if mode < 0 || int(mode) >= len(unpacked_size) {
		return nil
	}
	return mimeUnsortInto(make([]int16, unpacked_size[mode]), packed, mode)
}

// mimeUnsortInto is mimeUnsort writing into out, which must hold at least
// unpacked_size[mode] entries; it returns out resliced to that size.
func mimeUnsortInto(out []int16, packed []byte, mode int16) []int16 {
	if mode < 0 || int(mode) >= len(unpacked_size) {
		return nil
	}
	out = out[:unpacked_size[mode]]
	for i := range out {
		out[i] = 0
	}
	if int(mode) >= len(sortTables) || sortTables[mode] == nil {
		return out
	}
	st := sortTables[mode]
	nbytes := int(unpacked_size[mode]) >> 3
	k := 0
	for b := 0; b < nbytes && b < len(packed); b++ {
		temp := packed[b]
		for range 8 {
			if temp&0x80 != 0 {
				out[st[k]] = cBIT_1
			}
			temp <<= 1
			k++
		}
	}
	// Convert the remaining 0 to 7 bits.
	if rem := int(unpacked_size[mode]) & 7; rem != 0 && nbytes < len(packed) {
		temp := packed[nbytes]
		for range rem {
			if temp&0x80 != 0 {
				out[st[k]] = cBIT_1
			}
			temp <<= 1
			k++
		}
	}
	return out
}
