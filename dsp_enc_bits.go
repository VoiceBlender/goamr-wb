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

// txState is the frame-type state of PackBits, kept apart from coderState
// because Reset_encoder does not clear it. sid_handover_debt is left out: the
// reference never sets it.
type txState struct {
	sidUpdateCounter int16
	prevFt           int16
}

func (st *txState) reset() {
	st.sidUpdateCounter = 3
	st.prevFt = cTX_SPEECH
}

// frameType returns the TX_* type of a frame coded in codingMode (PackBits).
func (st *txState) frameType(codingMode int16) int16 {
	var ft int16
	if codingMode == dMRDTX {
		st.sidUpdateCounter--
		switch {
		case st.prevFt == cTX_SPEECH:
			ft = cTX_SID_FIRST
			st.sidUpdateCounter = 3
		case st.sidUpdateCounter == 0:
			ft = cTX_SID_UPDATE
			st.sidUpdateCounter = 8
		default:
			ft = cTX_NO_DATA
		}
	} else {
		st.sidUpdateCounter = 8
		ft = cTX_SPEECH
	}
	st.prevFt = ft
	return ft
}

// packSIDInto packs a SID frame into out, which must hold at least
// packed_size[dMRDTX] bytes: the 35 parameter bits (zero for SID_FIRST), the
// SID type indication and the speech mode (PackBits).
func packSIDInto(out []byte, prms []int16, txType, mode int16) []byte {
	if txType == cTX_SID_FIRST {
		for i := range prms[:cNBBITS_SID] {
			prms[i] = cBIT_0
		}
	}
	out = packFrameInto(out, prms, dMRDTX)
	if txType == cTX_SID_UPDATE {
		out[4] |= 0x10
	}
	out[4] |= byte(mode & 0x000F)
	return out
}

// packFrame packs internal-order parameter bits prms into the octet-aligned
// frame payload for the given internal mode index (PackBits, speech path).
func packFrame(prms []int16, mode int16) []byte {
	if mode < 0 || int(mode) >= len(packed_size) {
		return nil
	}
	return packFrameInto(make([]byte, packed_size[mode]), prms, mode)
}

// packFrameInto is packFrame writing into out, which must hold at least
// packed_size[mode] bytes; it returns out resliced to that size.
func packFrameInto(out []byte, prms []int16, mode int16) []byte {
	if mode < 0 || int(mode) >= len(packed_size) {
		return nil
	}
	out = out[:packed_size[mode]]
	for i := range out {
		out[i] = 0
	}
	st := sortTables[mode]
	n := int(unpacked_size[mode])
	for i := 0; i < n && i < len(st); i++ {
		if prms[st[i]] == cBIT_1 {
			out[i/8] |= 1 << uint(7-i%8)
		}
	}
	return out
}
