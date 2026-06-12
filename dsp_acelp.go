package amrwb

// Adaptive and algebraic (fixed) codebook decoding, ported verbatim from the
// Apache-2.0 opencore-amrwb reference (pred_lt4.cpp, dec_acelp_2p_in_64.cpp,
// dec_acelp_4p_in_64.cpp, dec_alg_codebook.cpp), derived from 3GPP TS 26.173.

const (
	cL_CODE   = 64 // codevector length
	cNB_TRACK = 4
	cNB_POS   = 16 // positions per track (also the sign-bit mask)

	cUP_SAMP     = 4
	cL_INTERPOL2 = 16
)

// inter4_2 is the 1/4-resolution interpolation filter (Q14), inter4_2[4][32].
var inter4_2 = [cUP_SAMP][2 * cL_INTERPOL2]int16{
	{
		0, -2, 4, -2, -10, 38,
		-88, 165, -275, 424, -619, 871,
		-1207, 1699, -2598, 5531, 14031, -2147,
		780, -249, -16, 153, -213, 226,
		-209, 175, -133, 91, -55, 28,
		-10, 2,
	},
	{
		1, -7, 19, -33, 47, -52,
		43, -9, -60, 175, -355, 626,
		-1044, 1749, -3267, 10359, 10359, -3267,
		1749, -1044, 626, -355, 175, -60,
		-9, 43, -52, 47, -33, 19,
		-7, 1,
	},
	{
		2, -10, 28, -55, 91, -133,
		175, -209, 226, -213, 153, -16,
		-249, 780, -2147, 14031, 5531, -2598,
		1699, -1207, 871, -619, 424, -275,
		165, -88, 38, -10, -2, 4,
		-2, 0,
	},
	{
		1, -7, 22, -49, 92, -153,
		231, -325, 431, -544, 656, -762,
		853, -923, 968, 15401, 968, -923,
		853, -762, 656, -544, 431, -325,
		231, -153, 92, -49, 22, -7,
		1, 0,
	},
}

// Pred_lt4 interpolates the adaptive-codebook excitation at fractional pitch.
// exc is the excitation buffer and off the current subframe start; negative
// reach (exc[off-T0-...]) indexes the past-excitation history.
func Pred_lt4(exc []int16, off int, T0, frac, lSubfr int16) {
	ptBase := off - int(T0)
	frac = -frac
	if frac < 0 {
		frac += cUP_SAMP
		ptBase--
	}
	ptBase -= cL_INTERPOL2 - 1
	pt := inter4_2[cUP_SAMP-1-frac]

	var j int16
	for j = 0; j < lSubfr>>2; j++ {
		lSum1 := int32(0x00002000)
		lSum2 := int32(0x00002000)
		lSum3 := int32(0x00002000)
		lSum4 := int32(0x00002000)
		for i := 0; i < cL_INTERPOL2<<1; i += 4 {
			tmp1 := exc[ptBase+i]
			tmp2 := exc[ptBase+i+1]
			tmp3 := exc[ptBase+i+2]
			lSum1 = fxp_mac_16by16(tmp1, pt[i], lSum1)
			lSum2 = fxp_mac_16by16(tmp2, pt[i], lSum2)
			lSum1 = fxp_mac_16by16(tmp2, pt[i+1], lSum1)
			lSum2 = fxp_mac_16by16(tmp3, pt[i+1], lSum2)
			lSum3 = fxp_mac_16by16(tmp3, pt[i], lSum3)
			lSum1 = fxp_mac_16by16(tmp3, pt[i+2], lSum1)

			tmp1 = exc[ptBase+i+3]
			tmp2 = exc[ptBase+i+4]
			lSum4 = fxp_mac_16by16(tmp1, pt[i], lSum4)
			lSum3 = fxp_mac_16by16(tmp1, pt[i+1], lSum3)
			lSum2 = fxp_mac_16by16(tmp1, pt[i+2], lSum2)
			lSum1 = fxp_mac_16by16(tmp1, pt[i+3], lSum1)
			lSum4 = fxp_mac_16by16(tmp2, pt[i+1], lSum4)
			lSum2 = fxp_mac_16by16(tmp2, pt[i+3], lSum2)
			lSum3 = fxp_mac_16by16(tmp2, pt[i+2], lSum3)

			tmp1 = exc[ptBase+i+5]
			tmp2 = exc[ptBase+i+6]
			lSum4 = fxp_mac_16by16(tmp1, pt[i+2], lSum4)
			lSum3 = fxp_mac_16by16(tmp1, pt[i+3], lSum3)
			lSum4 = fxp_mac_16by16(tmp2, pt[i+3], lSum4)
		}
		exc[off+int(j<<2)] = int16(lSum1 >> 14)
		exc[off+int(j<<2)+1] = int16(lSum2 >> 14)
		exc[off+int(j<<2)+2] = int16(lSum3 >> 14)
		exc[off+int(j<<2)+3] = int16(lSum4 >> 14)
		ptBase += 4
	}

	if lSubfr&1 != 0 {
		lSum1 := int32(0x00002000)
		for i := 0; i < 2*cL_INTERPOL2; i += 4 {
			tmp1 := exc[ptBase+i]
			tmp2 := exc[ptBase+i+1]
			lSum1 = fxp_mac_16by16(tmp1, pt[i], lSum1)
			lSum1 = fxp_mac_16by16(tmp2, pt[i+1], lSum1)
			tmp1 = exc[ptBase+i+2]
			tmp2 = exc[ptBase+i+3]
			lSum1 = fxp_mac_16by16(tmp1, pt[i+2], lSum1)
			lSum1 = fxp_mac_16by16(tmp2, pt[i+3], lSum1)
		}
		exc[off+int(j<<2)] = int16(lSum1 >> 14)
	}
}

// dec_acelp_2p_in_64 decodes a 2-pulse, 12-bit algebraic codeword (NB_POS=32).
func dec_acelp_2p_in_64(index int16, code []int16) {
	const nbPos2p = 32
	for i := range code[:cL_CODE] {
		code[i] = 0
	}
	i := (index >> 5) & 0x003E
	if ((index >> 6) & nbPos2p) == 0 {
		code[i] = 512
	} else {
		code[i] = -512
	}
	i = ((index & 0x001F) << 1) + 1
	if (index & nbPos2p) == 0 {
		code[i] = 512
	} else {
		code[i] = -512
	}
}

// dec_acelp_4p_in_64 decodes the multi-track algebraic codeword for the higher
// bitrate modes (20..88 bits).
func dec_acelp_4p_in_64(index []int16, nbbits int16, code []int16) {
	var pos [6]int16
	for i := range code[:cL_CODE] {
		code[i] = 0
	}
	switch nbbits {
	case 20:
		for k := int16(0); k < cNB_TRACK; k++ {
			dec_1p_N1(int32(index[k]), 4, 0, pos[:])
			add_pulses(pos[:], 1, k, code)
		}
	case 36:
		for k := int16(0); k < cNB_TRACK; k++ {
			dec_2p_2N1(int32(index[k]), 4, 0, pos[:])
			add_pulses(pos[:], 2, k, code)
		}
	case 44:
		for k := int16(0); k < cNB_TRACK-2; k++ {
			dec_3p_3N1(int32(index[k]), 4, 0, pos[:])
			add_pulses(pos[:], 3, k, code)
		}
		for k := int16(2); k < cNB_TRACK; k++ {
			dec_2p_2N1(int32(index[k]), 4, 0, pos[:])
			add_pulses(pos[:], 2, k, code)
		}
	case 52:
		for k := int16(0); k < cNB_TRACK; k++ {
			dec_3p_3N1(int32(index[k]), 4, 0, pos[:])
			add_pulses(pos[:], 3, k, code)
		}
	case 64:
		for k := int16(0); k < cNB_TRACK; k++ {
			lIndex := (int32(index[k]) << 14) + int32(index[k+cNB_TRACK])
			dec_4p_4N(lIndex, 4, 0, pos[:])
			add_pulses(pos[:], 4, k, code)
		}
	case 72:
		for k := int16(0); k < cNB_TRACK-2; k++ {
			lIndex := (int32(index[k]) << 10) + int32(index[k+cNB_TRACK])
			dec_5p_5N(lIndex, 4, 0, pos[:])
			add_pulses(pos[:], 5, k, code)
		}
		for k := int16(2); k < cNB_TRACK; k++ {
			lIndex := (int32(index[k]) << 14) + int32(index[k+cNB_TRACK])
			dec_4p_4N(lIndex, 4, 0, pos[:])
			add_pulses(pos[:], 4, k, code)
		}
	case 88:
		for k := int16(0); k < cNB_TRACK; k++ {
			lIndex := (int32(index[k]) << 11) + int32(index[k+cNB_TRACK])
			dec_6p_6N_2(lIndex, 4, 0, pos[:])
			add_pulses(pos[:], 6, k, code)
		}
	}
}

func add_pulses(pos []int16, nbPulse, track int16, code []int16) {
	for k := int16(0); k < nbPulse; k++ {
		i := ((pos[k] & (cNB_POS - 1)) << 2) + track
		if (pos[k] & cNB_POS) == 0 {
			code[i] += 512
		} else {
			code[i] -= 512
		}
	}
}

// --- pulse-position decoders (dec_alg_codebook.cpp), NB_POS = 16 ---

func dec_1p_N1(index int32, N, offset int16, pos []int16) {
	mask := int32((1 << uint(N)) - 1)
	pos1 := int16((index & mask) + int32(offset))
	if (index>>uint(N))&1 == 1 {
		pos1 += cNB_POS
	}
	pos[0] = pos1
}

func dec_2p_2N1(index int32, N, offset int16, pos []int16) {
	mask := int32(sub_int16(shl_int16(1, N), 1))
	pos1 := int16(add_int32(shr_int32(index, N)&mask, int32(offset)))
	tmp := shl_int16(N, 1)
	i := (index >> uint(tmp)) & 1
	pos2 := add_int16(int16(index&mask), offset)

	if pos2 < pos1 {
		if i == 1 {
			pos1 += cNB_POS
		} else {
			pos2 += cNB_POS
		}
	} else {
		if i == 1 {
			pos1 += cNB_POS
			pos2 += cNB_POS
		}
	}
	pos[0] = pos1
	pos[1] = pos2
}

func dec_3p_3N1(index int32, N, offset int16, pos []int16) {
	mask := int32((1 << uint((2*N)-1)) - 1)
	idx := index & mask
	j := offset
	tmp := (N << 1) - 1
	if (index>>uint(tmp))&1 != 0 {
		j += 1 << uint(N-1)
	}
	dec_2p_2N1(idx, N-1, j, pos)

	mask = int32((1 << uint(N+1)) - 1)
	tmp = N << 1
	idx = (index >> uint(tmp)) & mask
	dec_1p_N1(idx, N, offset, pos[2:])
}

func dec_4p_4N1(index int32, N, offset int16, pos []int16) {
	tmp := (N << 1) - 1
	mask := (int32(1) << uint(tmp)) - 1
	idx := index & mask
	j := offset
	tmp = (N << 1) - 1
	if (index>>uint(tmp))&1 != 0 {
		j += 1 << uint(N-1)
	}
	dec_2p_2N1(idx, N-1, j, pos)

	tmp = (N << 1) + 1
	mask = (int32(1) << uint(tmp)) - 1
	idx = (index >> uint(N<<1)) & mask
	dec_2p_2N1(idx, N, offset, pos[2:])
}

func dec_4p_4N(index int32, N, offset int16, pos []int16) {
	n1 := N - 1
	j := offset + (1 << uint(n1))
	tmp := (N << 2) - 2
	switch (index >> uint(tmp)) & 3 {
	case 0:
		tmp = (n1 << 2) + 1
		if (index>>uint(tmp))&1 != 0 {
			dec_4p_4N1(index, n1, j, pos)
		} else {
			dec_4p_4N1(index, n1, offset, pos)
		}
	case 1:
		tmp = (3 * n1) + 1
		dec_1p_N1(index>>uint(tmp), n1, offset, pos)
		dec_3p_3N1(index, n1, j, pos[1:])
	case 2:
		tmp = (n1 << 1) + 1
		dec_2p_2N1(index>>uint(tmp), n1, offset, pos)
		dec_2p_2N1(index, n1, j, pos[2:])
	case 3:
		tmp = n1 + 1
		dec_3p_3N1(index>>uint(tmp), n1, offset, pos)
		dec_1p_N1(index, n1, j, pos[3:])
	}
}

func dec_5p_5N(index int32, N, offset int16, pos []int16) {
	n1 := N - 1
	j := add_int16(offset, shl_int16(1, n1))
	tmp := (N << 1) + 1
	idx := index >> uint(tmp)
	tmp = (5 * N) - 1
	if (index>>uint(tmp))&1 != 0 {
		dec_3p_3N1(idx, n1, j, pos)
		dec_2p_2N1(index, N, offset, pos[3:])
	} else {
		dec_3p_3N1(idx, n1, offset, pos)
		dec_2p_2N1(index, N, offset, pos[3:])
	}
}

func dec_6p_6N_2(index int32, N, offset int16, pos []int16) {
	n1 := N - 1
	j := offset + (1 << uint(n1))
	offsetA := j
	offsetB := j
	if ((index >> uint(6*N-5)) & 1) == 0 {
		offsetA = offset
	} else {
		offsetB = offset
	}
	switch (index >> uint(6*N-4)) & 3 {
	case 0:
		dec_5p_5N(index>>uint(N), n1, offsetA, pos)
		dec_1p_N1(index, n1, offsetA, pos[5:])
	case 1:
		dec_5p_5N(index>>uint(N), n1, offsetA, pos)
		dec_1p_N1(index, n1, offsetB, pos[5:])
	case 2:
		dec_4p_4N(index>>uint(2*n1+1), n1, offsetA, pos)
		dec_2p_2N1(index, n1, offsetB, pos[4:])
	case 3:
		dec_3p_3N1(index>>uint(3*n1+1), n1, offset, pos)
		dec_3p_3N1(index, n1, j, pos[3:])
	}
}
