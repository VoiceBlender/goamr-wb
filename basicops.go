package amrwb

// Fixed-point basic operators, ported from the ITU-T / ETSI "basicop2"
// reference (the same primitives used by opencore-amrwb and vo-amrwbenc).
//
// Word16 maps to int16, Word32 to int32. All operators saturate to the 16- or
// 32-bit range exactly as the reference does; Go has no implicit integer
// saturation, so every operation that can overflow clamps explicitly. The
// reference's global Overflow/Carry flags are not modelled — no AMR-WB code
// path depends on reading them back.

const (
	maxInt16 = 32767
	minInt16 = -32768
	maxInt32 = 2147483647
	minInt32 = -2147483648
)

// satWord16 clamps a 32-bit value to the int16 range.
func satWord16(v int32) int16 {
	if v > maxInt16 {
		return maxInt16
	}
	if v < minInt16 {
		return minInt16
	}
	return int16(v)
}

// satWord32 clamps a 64-bit value to the int32 range.
func satWord32(v int64) int32 {
	if v > maxInt32 {
		return maxInt32
	}
	if v < minInt32 {
		return minInt32
	}
	return int32(v)
}

// add returns the saturated 16-bit sum a+b.
func add(a, b int16) int16 { return satWord16(int32(a) + int32(b)) }

// sub returns the saturated 16-bit difference a-b.
func sub(a, b int16) int16 { return satWord16(int32(a) - int32(b)) }

// abs_s returns the saturated 16-bit absolute value (|-32768| -> 32767).
func abs_s(a int16) int16 {
	if a == minInt16 {
		return maxInt16
	}
	if a < 0 {
		return -a
	}
	return a
}

// shl returns a shifted left by b bits with saturation; negative b shifts right.
func shl(a, b int16) int16 {
	if b < 0 {
		return shr(a, -b)
	}
	result := int32(a) << uint(b)
	// Detect overflow by checking the shift is reversible.
	if b >= 31 {
		if a == 0 {
			return 0
		}
		if a > 0 {
			return maxInt16
		}
		return minInt16
	}
	return satWord16(result)
}

// shr returns a shifted right by b bits (arithmetic); negative b shifts left.
func shr(a, b int16) int16 {
	if b < 0 {
		return shl(a, -b)
	}
	if b >= 15 {
		if a < 0 {
			return -1
		}
		return 0
	}
	return int16(int32(a) >> uint(b))
}

// L_shl returns the saturated 32-bit left shift; negative b shifts right.
func L_shl(a int32, b int16) int32 {
	if b <= 0 {
		if b == 0 {
			return a
		}
		return L_shr(a, -b)
	}
	if b >= 31 {
		if a > 0 {
			return maxInt32
		}
		if a < 0 {
			return minInt32
		}
		return 0
	}
	// The reference shifts one bit at a time, saturating as soon as the value
	// reaches the overflow region. Since |a<<k| grows monotonically, saturation
	// is decided by the last pre-shift value a<<(b-1): >= 2^30 -> MAX_32,
	// < -2^30 -> MIN_32; otherwise a<<b fits. Computed in int64 this is O(1) and
	// bit-identical to the loop.
	last := int64(a) << uint(b-1)
	if last >= 0x40000000 {
		return maxInt32
	}
	if last < -0x40000000 {
		return minInt32
	}
	return a << uint(b)
}

// L_shr returns the 32-bit arithmetic right shift; negative b shifts left.
func L_shr(a int32, b int16) int32 {
	if b < 0 {
		return L_shl(a, -b)
	}
	if b >= 31 {
		if a < 0 {
			return -1
		}
		return 0
	}
	return a >> uint(b)
}

// mult returns satWord16((a*b)>>15).
func mult(a, b int16) int16 {
	p := (int32(a) * int32(b)) >> 15
	return satWord16(p)
}

// L_mult returns satWord32(a*b<<1) — the 32-bit product with the fractional
// left shift.
func L_mult(a, b int16) int32 {
	p := int32(a) * int32(b)
	if p != 0x40000000 {
		return p << 1
	}
	return maxInt32
}

// L_add returns the saturated 32-bit sum.
func L_add(a, b int32) int32 { return satWord32(int64(a) + int64(b)) }

// L_sub returns the saturated 32-bit difference.
func L_sub(a, b int32) int32 { return satWord32(int64(a) - int64(b)) }

// L_mac returns acc + (a*b<<1), saturated.
func L_mac(acc int32, a, b int16) int32 { return L_add(acc, L_mult(a, b)) }

// L_msu returns acc - (a*b<<1), saturated.
func L_msu(acc int32, a, b int16) int32 { return L_sub(acc, L_mult(a, b)) }

// negate returns the saturated 16-bit negation.
func negate(a int16) int16 {
	if a == minInt16 {
		return maxInt16
	}
	return -a
}

// L_negate returns the saturated 32-bit negation.
func L_negate(a int32) int32 {
	if a == minInt32 {
		return maxInt32
	}
	return -a
}

// extract_h returns the high 16 bits of a 32-bit value.
func extract_h(a int32) int16 { return int16(a >> 16) }

// extract_l returns the low 16 bits of a 32-bit value.
func extract_l(a int32) int16 { return int16(a & 0xFFFF) }

// round_ returns the rounded high word: extract_h(L_add(a, 0x8000)).
func round_(a int32) int16 { return extract_h(L_add(a, 0x8000)) }

// mult_r returns the rounded multiply: satWord16(((a*b)+0x4000)>>15).
func mult_r(a, b int16) int16 {
	p := (int32(a)*int32(b) + 0x4000) >> 15
	return satWord16(p)
}

// L_deposit_h places a 16-bit value in the high half of a 32-bit word.
func L_deposit_h(a int16) int32 { return int32(a) << 16 }

// L_deposit_l sign-extends a 16-bit value into a 32-bit word.
func L_deposit_l(a int16) int32 { return int32(a) }

// norm_l returns the number of left shifts needed to normalize a (0 for 0).
func norm_l(a int32) int16 {
	if a == 0 {
		return 0
	}
	if a == -1 {
		return 31
	}
	if a < 0 {
		a = ^a
	}
	var n int16
	for a < 0x40000000 {
		a <<= 1
		n++
	}
	return n
}

// norm_s returns the number of left shifts needed to normalize a 16-bit value.
func norm_s(a int16) int16 {
	if a == 0 {
		return 0
	}
	if a == -1 {
		return 15
	}
	v := a
	if v < 0 {
		v = ^v
	}
	var n int16
	for v < 0x4000 {
		v <<= 1
		n++
	}
	return n
}

// div_s returns the fractional division (a/b)<<15 for 0 <= a <= b, b != 0.
func div_s(a, b int16) int16 {
	if a == 0 {
		return 0
	}
	if a == b {
		return maxInt16
	}
	num := int32(a)
	den := int32(b)
	var quot int32
	for i := 0; i < 15; i++ {
		quot <<= 1
		num <<= 1
		if num >= den {
			num -= den
			quot++
		}
	}
	return int16(quot)
}
