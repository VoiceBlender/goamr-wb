package amrwb

// ISF->ISP conversion and ISP->LP-coefficient conversion, ported verbatim from
// the Apache-2.0 opencore-amrwb reference (isp_isf.cpp, isp_az.cpp,
// interpolate_isp.cpp), derived from 3GPP TS 26.173.

// cosTable is cos(x) in Q15, table[129] from isp_isf.cpp.
var cosTable = [129]int16{
	32767,
	32758, 32729, 32679, 32610, 32522, 32413, 32286, 32138,
	31972, 31786, 31581, 31357, 31114, 30853, 30572, 30274,
	29957, 29622, 29269, 28899, 28511, 28106, 27684, 27246,
	26791, 26320, 25833, 25330, 24812, 24279, 23732, 23170,
	22595, 22006, 21403, 20788, 20160, 19520, 18868, 18205,
	17531, 16846, 16151, 15447, 14733, 14010, 13279, 12540,
	11793, 11039, 10279, 9512, 8740, 7962, 7180, 6393,
	5602, 4808, 4011, 3212, 2411, 1608, 804, 0,
	-804, -1608, -2411, -3212, -4011, -4808, -5602, -6393,
	-7180, -7962, -8740, -9512, -10279, -11039, -11793, -12540,
	-13279, -14010, -14733, -15447, -16151, -16846, -17531, -18205,
	-18868, -19520, -20160, -20788, -21403, -22006, -22595, -23170,
	-23732, -24279, -24812, -25330, -25833, -26320, -26791, -27246,
	-27684, -28106, -28511, -28899, -29269, -29622, -29957, -30274,
	-30572, -30853, -31114, -31357, -31581, -31786, -31972, -32138,
	-32286, -32413, -32522, -32610, -32679, -32729, -32758, -32768,
}

// Isf_isp converts normalized ISF (0..0.5, Q15) to ISP (cosine domain, Q15).
func Isf_isp(isf, isp []int16, m int16) {
	for i := int16(0); i < m-1; i++ {
		isp[i] = isf[i]
	}
	isp[m-1] = shl_int16(isf[m-1], 1)

	for i := int16(0); i < m; i++ {
		ind := isp[i] >> 7
		offset := isp[i] & 0x007f
		lTmp := mul_16by16_to_int32(cosTable[ind+1]-cosTable[ind], offset)
		isp[i] = add_int16(cosTable[ind], int16(lTmp>>8))
	}
}

// Get_isp_pol expands F1(z) or F2(z) from the ISPs (Q23 output).
func Get_isp_pol(isp []int16, f []int32, n int16) {
	f[0] = 0x00800000
	f[1] = -int32(isp[0]) << 9

	fp := 2
	ip := 2
	for i := int16(2); i <= n; i++ {
		f[fp] = f[fp-2]
		for j := int16(1); j < i; j++ {
			t0 := fxp_mul32_by_16b(f[fp-1], int32(isp[ip]))
			t0 = shl_int32(t0, 2)
			f[fp] -= t0
			f[fp] += f[fp-2]
			fp--
		}
		f[fp] -= int32(isp[ip]) << 9
		fp += int(i)
		ip += 2
	}
}

// Get_isp_pol_16kHz is the M16k (order-20) variant; included for completeness.
func Get_isp_pol_16kHz(isp []int16, f []int32, n int16) {
	f[0] = 0x00200000
	f[1] = -int32(isp[0]) << 7

	fp := 2
	ip := 2
	for i := int16(2); i <= n; i++ {
		f[fp] = f[fp-2]
		for j := int16(1); j < i; j++ {
			t0 := fxp_mul32_by_16b(f[fp-1], int32(isp[ip]))
			t0 = shl_int32(t0, 2)
			f[fp] -= t0
			f[fp] += f[fp-2]
			fp--
		}
		f[fp] -= int32(isp[ip]) << 7
		fp += int(i)
		ip += 2
	}
}

// absI32 mirrors the C abs trick t1 = (t0 - (t0<0)) ^ sign.
func absI32(t0 int32) int32 {
	var dec int32
	if t0 < 0 {
		dec = 1
	}
	t1 := t0 - dec
	return t1 ^ (t1 >> 31)
}

// Isp_Az converts ISP (Q15) to LP coefficients a[] (Q12, order m).
func Isp_Az(isp, a []int16, m, adaptiveScaling int16) {
	f1 := make([]int32, cNC16k+1)
	f2 := make([]int32, cNC16k)
	nc := m >> 1

	if nc > 8 {
		Get_isp_pol_16kHz(isp, f1, nc)
		for i := int16(0); i <= nc; i++ {
			f1[i] = shl_int32(f1[i], 2)
		}
		Get_isp_pol_16kHz(isp[1:], f2, nc-1)
		for i := int16(0); i <= nc-1; i++ {
			f2[i] = shl_int32(f2[i], 2)
		}
	} else {
		Get_isp_pol(isp, f1, nc)
		Get_isp_pol(isp[1:], f2, nc-1)
	}

	// Multiply F2(z) by (1 - z^-2)
	for i := nc - 1; i > 1; i-- {
		f2[i] -= f2[i-2]
	}

	// Scale F1(z) by (1+isp[m-1]) and F2(z) by (1-isp[m-1])
	for i := int16(0); i < nc; i++ {
		t0 := f1[i]
		t1 := f2[i]
		t0 = fxp_mul32_by_16b(t0, int32(isp[m-1])) << 1
		t1 = fxp_mul32_by_16b(t1, int32(isp[m-1])) << 1
		f1[i] += t0
		f2[i] -= t1
	}

	// A(z) = (F1(z)+F2(z))/2
	a[0] = 4096
	tmax := int32(1)
	j := m - 1
	for i := int16(1); i < nc; i++ {
		t0 := add_int32(f1[i], f2[i])
		tmax |= absI32(t0)
		a[i] = int16((t0 >> 12) + ((t0 >> 11) & 1))

		t0 = sub_int32(f1[i], f2[i])
		tmax |= absI32(t0)
		a[j] = int16((t0 >> 12) + ((t0 >> 11) & 1))
		j--
	}

	var q, qSug int16
	if adaptiveScaling == 1 {
		q = 4 - normalize_amr_wb(tmax)
	} else {
		q = 0
	}

	if q > 0 {
		qSug = 12 + q
		for i, jj := int16(1), m-1; i < nc; i, jj = i+1, jj-1 {
			t0 := add_int32(f1[i], f2[i])
			a[i] = int16((t0 >> uint(qSug)) + ((t0 >> uint(qSug-1)) & 1))
			t0 = sub_int32(f1[i], f2[i])
			a[jj] = int16((t0 >> uint(qSug)) + ((t0 >> uint(qSug-1)) & 1))
		}
		a[0] >>= uint(q)
	} else {
		qSug = 12
		q = 0
	}

	// a[nc] = 0.5*f1[nc]*(1.0 + isp[m-1])
	t0 := int32((int64(f1[nc])*int64(isp[m-1]))>>16) << 1
	t0 = add_int32(f1[nc], t0)
	a[nc] = int16((t0 >> uint(qSug)) + ((t0 >> uint(qSug-1)) & 1))
	a[m] = shr_rnd(isp[m-1], 3+q)
}

// interpolate_isp interpolates ISPs across the 4 subframes and converts each to
// LP coefficients in Az (4 * MP1 values).
func interpolate_isp(ispOld, ispNew []int16, frac []int16, az []int16) {
	isp := make([]int16, cM)
	azOff := 0
	for k := 0; k < 3; k++ {
		facNew := frac[k]
		facOld := add_int16(sub_int16(32767, facNew), 1)
		for i := 0; i < cM; i++ {
			lTmp := mul_16by16_to_int32(ispOld[i], facOld)
			lTmp = mac_16by16_to_int32(lTmp, ispNew[i], facNew)
			isp[i] = amr_wb_round(lTmp)
		}
		Isp_Az(isp, az[azOff:], cM, 0)
		azOff += cMP1
	}
	Isp_Az(ispNew, az[azOff:], cM, 0)
}
