package amrwb

// Small self-contained kernels ported verbatim from the Apache-2.0
// opencore-amrwb reference (preemph_amrwb_dec.cpp, scale_signal.cpp,
// median5.cpp, pit_shrp.cpp), derived from 3GPP TS 26.173.

// preemph_amrwb_dec applies the pre-emphasis filter (1 - mu z^-1) in place.
func preemph_amrwb_dec(x []int16, mu, lg int16) {
	for i := lg - 1; i != 0; i-- {
		lTmp := msu_16by16_from_int32(int32(x[i])<<16, x[i-1], mu)
		x[i] = amr_wb_round(lTmp)
	}
}

// scale_signal scales x by 2^exp: x = round(x << exp).
func scale_signal(x []int16, lg, exp int16) {
	if exp > 0 {
		for i := int16(0); i < lg; i++ {
			lTmp := shl_int32(int32(x[i])<<16, exp)
			x[i] = amr_wb_round(lTmp)
		}
	} else if exp < 0 {
		exp = -exp
		exp &= 0xf
		tmp := int16(int32(0x00008000) >> uint(16-exp))
		p := 0
		for i := lg >> 1; i != 0; i-- {
			x[p] = add_int16(x[p], tmp) >> uint(exp)
			p++
			x[p] = add_int16(x[p], tmp) >> uint(exp)
			p++
		}
	}
}

// median5 returns the median of the 5 samples centered at x[off]
// (x[off-2..off+2]).
func median5(x []int16, off int) int16 {
	x1 := x[off-2]
	x2 := x[off-1]
	x3 := x[off]
	x4 := x[off+1]
	x5 := x[off+2]

	if x2 < x1 {
		x1, x2 = x2, x1
	}
	if x3 < x1 {
		x1, x3 = x3, x1
	}
	if x4 < x1 {
		x1, x4 = x4, x1
	}
	if x5 < x1 {
		x5 = x1
	}
	if x3 < x2 {
		x2, x3 = x3, x2
	}
	if x4 < x2 {
		x2, x4 = x4, x2
	}
	if x5 < x2 {
		x5 = x2
	}
	if x4 < x3 {
		x3 = x4
	}
	if x5 < x3 {
		x3 = x5
	}
	return x3
}

// Pit_shrp applies pitch sharpening to the impulse response / algebraic code.
func Pit_shrp(x []int16, pitLag, sharp, lSubfr int16) {
	for i := pitLag; i < lSubfr; i++ {
		lTmp := mac_16by16_to_int32(int32(x[i])<<16, x[i-pitLag], sharp)
		x[i] = amr_wb_round(lTmp)
	}
}
