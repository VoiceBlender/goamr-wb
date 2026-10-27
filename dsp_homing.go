package amrwb

// Decoder homing frame detection, ported verbatim from the Apache-2.0
// opencore-amrwb reference (homing_amr_wb_dec.cpp), derived from 3GPP TS 26.173.

// cEHF_MASK is the encoder homing frame pattern output for a decoder homing frame.
const cEHF_MASK = int16(0x0008)

// dhfTest reports whether the first nparms bits of the de-sorted parameter
// frame match the decoder homing frame of the given mode (dhf_test).
func dhfTest(params []int16, mode int16, nparms int16) bool {
	var param [32]int16
	p := &bitParser{bits: params}
	var i, j, tmp, shift int16

	if mode == dMRDTX {
		return false
	}
	if mode != 8 {
		// Convert the received serial bits.
		tmp = nparms - 15
		for tmp > j {
			param[i] = p.parm(15)
			j += 15
			i++
		}
		tmp = nparms - j
		param[i] = p.parm(tmp)
		shift = 15 - tmp
		param[i] = shl_int16(param[i], shift)
	} else {
		// 23.85 kbit/s: remove the high band energy bits.
		for i = 0; i < 10; i++ {
			param[i] = p.parm(15)
		}
		param[10] = p.parm(15) & 0x61FF
		for i = 11; i < 17; i++ {
			param[i] = p.parm(15)
		}
		param[17] = p.parm(15) & 0x60FF // 0xE0FF in the reference; bit 15 is never set
		for i = 18; i < 24; i++ {
			param[i] = p.parm(15)
		}
		param[24] = p.parm(15) & 0x7F0F
		for i = 25; i < 31; i++ {
			param[i] = p.parm(15)
		}
		tmp = p.parm(8)
		param[31] = shl_int16(tmp, 7)
		shift = 0
	}

	// Check whether the parameters match the decoder homing frame of the mode.
	dhf := dhfTables[mode]
	tmp = i
	j = 0
	for i = 0; i < tmp; i++ {
		j = param[i] ^ dhf[i]
		if j != 0 {
			break
		}
	}
	tmp = 0x7fff
	tmp >>= uint(shift)
	tmp = shl_int16(tmp, shift)
	tmp = dhf[i] & tmp
	tmp = param[i] ^ tmp
	j |= tmp
	return j == 0
}

// homingFrameTest checks the complete frame (pvDecoder_AmrWb_homing_frame_test).
func homingFrameTest(params []int16, mode int16) bool {
	return dhfTest(params, mode, amrwbCompressed[mode])
}

// homingFrameTestFirst checks the first subframe only
// (pvDecoder_AmrWb_homing_frame_test_first).
func homingFrameTestFirst(params []int16, mode int16) bool {
	return dhfTest(params, mode, prmnofsf[mode])
}
