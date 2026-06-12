package amrwb

// Fixed-point operators for the AMR-WB decoder, ported verbatim from the
// Apache-2.0 opencore-amrwb reference (pvamrwbdecoder_basic_op_cequivalent.h,
// pvamrwbdecoder_basic_op.h, normalize_amr_wb.cpp), which itself derives from
// the 3GPP TS 26.173 ANSI-C reference. Names mirror the C functions so the
// decoder port reads against the source. Saturation is replicated explicitly.
//
// These intentionally duplicate some semantics of the ITU operators in
// basicops.go: the decoder calls this exact set, so keeping it verbatim avoids
// subtle saturation drift between conventions.

const (
	max16 = int16(32767)
	min16 = int16(-32768)
	max32 = int32(0x7fffffff)
	min32 = int32(-0x80000000)
)

func add_int16(var1, var2 int16) int16 {
	lSum := int32(var1) + int32(var2)
	if (lSum >> 15) != (lSum >> 31) {
		lSum = (lSum >> 31) ^ int32(max16)
	}
	return int16(lSum)
}

func sub_int16(var1, var2 int16) int16 {
	lDiff := int32(var1) - int32(var2)
	if (lDiff >> 15) != (lDiff >> 31) {
		lDiff = (lDiff >> 31) ^ int32(max16)
	}
	return int16(lDiff)
}

func mult_int16(var1, var2 int16) int16 {
	lProduct := (int32(var1) * int32(var2)) >> 15
	if (lProduct >> 15) != (lProduct >> 31) {
		lProduct = (lProduct >> 31) ^ int32(max16)
	}
	return int16(lProduct)
}

func add_int32(lVar1, lVar2 int32) int32 {
	out := int64(lVar1) + int64(lVar2)
	lOut := int32(out)
	if ((lVar1 ^ lVar2) & min32) == 0 { // same sign
		if (lOut^lVar1)&min32 != 0 { // sign flipped -> overflow
			lOut = (lVar1 >> 31) ^ max32
		}
	}
	return lOut
}

func sub_int32(lVar1, lVar2 int32) int32 {
	lOut := int32(int64(lVar1) - int64(lVar2))
	if ((lVar1 ^ lVar2) & min32) != 0 { // different sign
		if (lOut^lVar1)&min32 != 0 {
			lOut = (lVar1 >> 31) ^ max32
		}
	}
	return lOut
}

func mul_16by16_to_int32(var1, var2 int16) int32 {
	lMul := int32(var1) * int32(var2)
	if lMul != 0x40000000 {
		lMul <<= 1
	} else {
		lMul = max32
	}
	return lMul
}

func mac_16by16_to_int32(lVar3 int32, var1, var2 int16) int32 {
	lMul := int32(var1) * int32(var2)
	if lMul != 0x40000000 {
		lMul <<= 1
	} else {
		lMul = max32
	}
	lOut := int32(int64(lVar3) + int64(lMul))
	if ((lMul ^ lVar3) & min32) == 0 {
		if (lOut^lVar3)&min32 != 0 {
			lOut = (lVar3 >> 31) ^ max32
		}
	}
	return lOut
}

func msu_16by16_from_int32(lVar3 int32, var1, var2 int16) int32 {
	lMul := int32(var1) * int32(var2)
	if lMul != 0x40000000 {
		lMul <<= 1
	} else {
		lMul = max32
	}
	lOut := int32(int64(lVar3) - int64(lMul))
	if ((lMul ^ lVar3) & min32) != 0 {
		if (lOut^lVar3)&min32 != 0 {
			lOut = (lVar3 >> 31) ^ max32
		}
	}
	return lOut
}

func amr_wb_round(lVar1 int32) int16 {
	if lVar1 != max32 {
		lVar1 += 0x00008000
	}
	return int16(lVar1 >> 16)
}

func amr_wb_shl1_round(lVar1 int32) int16 {
	if (lVar1<<1)>>1 == lVar1 {
		return int16((lVar1 + 0x00004000) >> 15)
	}
	return int16(((lVar1 >> 31) ^ max32) >> 16)
}

func mul_32by16(hi, lo, n int16) int32 {
	return ((int32(hi) * int32(n)) + ((int32(lo) * int32(n)) >> 15)) << 1
}

// fxp_* are the non-saturating fixed-point helpers from the cequivalent header.
func fxp_mac_16by16(var1, var2 int16, lAdd int32) int32 {
	return lAdd + int32(var1)*int32(var2)
}

func fxp_mul_16by16(var1, var2 int16) int32 { return int32(var1) * int32(var2) }

func fxp_mul32_by_16b(lVar1, lVar2 int32) int32 {
	return int32((int64(lVar1) * int64(lVar2<<16)) >> 32)
}

func negate_int16(var1 int16) int16 {
	if var1 == min16 {
		return max16
	}
	return -var1
}

func shl_int16(var1, var2 int16) int16 {
	var varOut int16
	if var2 < 0 {
		var2 = (-var2) & 0xf
		varOut = var1 >> uint(var2)
	} else {
		var2 &= 0xf
		varOut = int16(int32(var1) << uint(var2))
		if varOut>>uint(var2) != var1 {
			varOut = (var1 >> 15) ^ max16
		}
	}
	return varOut
}

func shr_int16(var1, var2 int16) int16 { return shl_int16(var1, -var2) }

func shl_int32(lVar1 int32, var2 int16) int32 {
	var lOut int32
	if var2 > 0 {
		lOut = int32(int64(lVar1) << uint(var2))
		if lOut>>uint(var2) != lVar1 {
			lOut = (lVar1 >> 31) ^ max32
		}
	} else {
		var2 = (-var2) & 0xf
		lOut = lVar1 >> uint(var2)
	}
	return lOut
}

func shr_int32(lVar1 int32, var2 int16) int32 {
	var lOut int32
	if var2 >= 0 {
		lOut = lVar1 >> uint(var2&0x1f)
	} else {
		var2 = -var2
		var2 &= 0x1f
		lOut = int32(int64(lVar1) << uint(var2))
		if lOut>>uint(var2) != lVar1 {
			lOut = (lVar1 >> 31) ^ max32
		}
	}
	return lOut
}

// l_deposit_h / norm_s_oc mirror the C macros in pvamrwb_math_op.h. (extract_h
// is shared with basicops.go, which defines the identical int16(x>>16).)
func l_deposit_h(x int16) int32 { return int32(x) << 16 }
func norm_s_oc(x int16) int16   { return normalize_amr_wb(int32(x)) - 16 }

// normalize_amr_wb returns the normalization shift of x, ported verbatim from
// normalize_amr_wb.cpp (the C_EQUIVALENT branch).
func normalize_amr_wb(x int32) int16 {
	var i int16
	switch {
	case x > 0x0FFFFFFF:
		i = 0
	case x > 0x00FFFFFF:
		i = 3
	case x > 0x0000FFFF:
		if x > 0x000FFFFF {
			i = 7
		} else {
			i = 11
		}
	default:
		if x > 0x000000FF {
			if x > 0x00000FFF {
				i = 15
			} else {
				i = 19
			}
		} else {
			if x > 0x0000000F {
				i = 23
			} else {
				i = 27
			}
		}
	}

	x <<= uint(i)

	switch x & 0x78000000 {
	case 0x08000000:
		i += 3
	case 0x18000000, 0x10000000:
		i += 2
	case 0x28000000, 0x20000000, 0x38000000, 0x30000000:
		i++
	}
	return i
}
