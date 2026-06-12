package amrwb

// Higher-level fixed-point math, ported verbatim from the Apache-2.0
// opencore-amrwb reference (pvamrwb_math_op.cpp), derived from 3GPP TS 26.173.
// Representations follow the C file:
//
//	int32 L_32         : standard signed 32-bit
//	int16 hi, lo       : L_32 = hi<<16 + lo<<1  (double-precision format, DPF)
//	int32 frac, exp    : L_32 = frac << (exp-31) (normalized format)

func mult_int16_r(var1, var2 int16) int16 {
	p := int32(var1) * int32(var2)
	p += 0x00004000
	p >>= 15
	if (p >> 15) != (p >> 31) {
		p = (p >> 31) ^ int32(max16)
	}
	return int16(p)
}

func shr_rnd(var1, var2 int16) int16 {
	varOut := var1 >> uint(var2&0xf)
	if var2 != 0 {
		if (var1 & (int16(1) << uint(var2-1))) != 0 {
			varOut++
		}
	}
	return varOut
}

func div_16by16(var1, var2 int16) int16 {
	var varOut int16
	if (var1 > var2) || (var1 < 0) {
		return 0
	}
	if var1 != 0 {
		if var1 != var2 {
			lNum := int32(var1)
			lDenom := int32(var2)
			lDenomBy2 := lDenom << 1
			lDenomBy4 := lDenom << 2
			for iteration := 5; iteration > 0; iteration-- {
				varOut <<= 3
				lNum <<= 3
				if lNum >= lDenomBy4 {
					lNum -= lDenomBy4
					varOut |= 4
				}
				if lNum >= lDenomBy2 {
					lNum -= lDenomBy2
					varOut |= 2
				}
				if lNum >= lDenom {
					lNum -= lDenom
					varOut |= 1
				}
			}
		} else {
			varOut = max16
		}
	}
	return varOut
}

var tableIsqrt = [49]int16{
	32767, 31790, 30894, 30070, 29309, 28602, 27945, 27330, 26755, 26214,
	25705, 25225, 24770, 24339, 23930, 23541, 23170, 22817, 22479, 22155,
	21845, 21548, 21263, 20988, 20724, 20470, 20225, 19988, 19760, 19539,
	19326, 19119, 18919, 18725, 18536, 18354, 18176, 18004, 17837, 17674,
	17515, 17361, 17211, 17064, 16921, 16782, 16646, 16514, 16384,
}

func one_ov_sqrt(lX int32) int32 {
	exp := normalize_amr_wb(lX)
	lX <<= uint(exp)
	exp = 31 - exp
	frac := lX
	one_ov_sqrt_norm(&frac, &exp)
	return shl_int32(frac, exp)
}

func one_ov_sqrt_norm(frac *int32, exp *int16) {
	if *frac <= 0 {
		*exp = 0
		*frac = 0x7fffffff
		return
	}
	if (*exp & 1) == 1 {
		*frac >>= 1
	}
	*exp = negate_int16((*exp - 1) >> 1)

	*frac >>= 9
	i := extract_h(*frac)
	*frac >>= 1
	a := int16(*frac) & 0x7fff

	i -= 16

	*frac = l_deposit_h(tableIsqrt[i])
	tmp := tableIsqrt[i] - tableIsqrt[i+1]
	*frac = msu_16by16_from_int32(*frac, tmp, a)
}

var tablePow2 = [33]int16{
	16384, 16743, 17109, 17484, 17867, 18258, 18658, 19066, 19484, 19911,
	20347, 20792, 21247, 21713, 22188, 22674, 23170, 23678, 24196, 24726,
	25268, 25821, 26386, 26964, 27554, 28158, 28774, 29405, 30048, 30706,
	31379, 32066, 32767,
}

func power_of_2(exponant, fraction int16) int32 {
	lX := int32(fraction) << 5
	i := fraction >> 10
	a := int16(lX) & 0x7fff

	lX = (int32(tablePow2[i]) << 15)
	tmp := tablePow2[i] - tablePow2[i+1]
	lX -= int32(tmp) * int32(a)

	exp := 29 - exponant
	if exp != 0 {
		lX = (lX >> uint(exp)) + ((lX >> uint(exp-1)) & 1)
	}
	return lX
}

func dotProduct12(x, y []int16, lg int16, exp *int16) int32 {
	var lSum int32 = 1
	pi := 0
	for i := lg >> 3; i != 0; i-- {
		for k := 0; k < 8; k++ {
			lSum = mac_16by16_to_int32(lSum, x[pi], y[pi])
			pi++
		}
	}
	sft := normalize_amr_wb(lSum)
	lSum <<= uint(sft)
	*exp = 30 - sft
	return lSum
}

var log2NormTable = [33]int16{
	0, 1455, 2866, 4236, 5568, 6863, 8124, 9352, 10549, 11716,
	12855, 13967, 15054, 16117, 17156, 18172, 19167, 20142, 21097, 22033,
	22951, 23852, 24735, 25603, 26455, 27291, 28113, 28922, 29716, 30497,
	31266, 32023, 32767,
}

func lg2Normalized(lX int32, exp int16, exponent, fraction *int16) {
	if lX <= 0 {
		*exponent = 0
		*fraction = 0
		return
	}
	*exponent = 30 - exp
	lX >>= 9
	i := extract_h(lX)
	lX >>= 1
	a := int16(lX) & 0x7fff
	i -= 32
	lY := l_deposit_h(log2NormTable[i])
	tmp := log2NormTable[i] - log2NormTable[i+1]
	lY = msu_16by16_from_int32(lY, tmp, a)
	*fraction = extract_h(lY)
}

func amrwbLog2(lX int32, exponent, fraction *int16) {
	exp := normalize_amr_wb(lX)
	lg2Normalized(shl_int32(lX, exp), exp, exponent, fraction)
}

func int32ToDpf(l32 int32, hi, lo *int16) {
	*hi = int16(l32 >> 16)
	*lo = int16((l32 - (int32(*hi) << 16)) >> 1)
}

func mpyDpf32(hi1, lo1, hi2, lo2 int16) int32 {
	l32 := mul_16by16_to_int32(hi1, hi2)
	l32 = mac_16by16_to_int32(l32, mult_int16(hi1, lo2), 1)
	l32 = mac_16by16_to_int32(l32, mult_int16(lo1, hi2), 1)
	return l32
}
