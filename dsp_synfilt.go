package amrwb

// LP synthesis filters 1/A(z), ported verbatim from the Apache-2.0
// opencore-amrwb reference (wb_syn_filt.cpp), derived from 3GPP TS 26.173.
//
// Both functions take their history-bearing buffers with an explicit base
// offset (the C code passes pointers already advanced past M history samples
// and indexes negatively into that history). yBuf/sigHi/sigLo therefore hold M
// history samples in [0:off] and the working region from [off:].

// wb_syn_filt does 16-bit synthesis filtering. a is Q12 (a[0..m]); x is the
// input; y the output (lg samples); mem the m-sample filter memory; when
// update != 0 the memory is refreshed from the tail of y. yBuf is scratch of
// length m+lg.
func wb_syn_filt(a []int16, m int16, x, y []int16, lg int16, mem []int16, update int16, yBuf []int16) {
	mm := int(m)
	copy(yBuf[:mm], mem[:mm])
	// yy[k] == yBuf[mm+k]
	yy := func(k int) int16 { return yBuf[mm+k] }

	var i, j int16
	for i = 0; i < lg>>2; i++ {
		base := int(i << 2)
		lTmp1 := -(int32(x[base]) << 11)
		lTmp2 := -(int32(x[base+1]) << 11)
		lTmp3 := -(int32(x[base+2]) << 11)
		lTmp4 := -(int32(x[base+3]) << 11)

		lTmp1 = fxp_mac_16by16(yy(base-3), a[3], lTmp1)
		lTmp2 = fxp_mac_16by16(yy(base-2), a[3], lTmp2)
		lTmp1 = fxp_mac_16by16(yy(base-2), a[2], lTmp1)
		lTmp2 = fxp_mac_16by16(yy(base-1), a[2], lTmp2)
		lTmp1 = fxp_mac_16by16(yy(base-1), a[1], lTmp1)

		for j = 4; j < m; j += 2 {
			lTmp1 = fxp_mac_16by16(yy(base-1-int(j)), a[j+1], lTmp1)
			lTmp2 = fxp_mac_16by16(yy(base-int(j)), a[j+1], lTmp2)
			lTmp1 = fxp_mac_16by16(yy(base-int(j)), a[j], lTmp1)
			lTmp2 = fxp_mac_16by16(yy(base+1-int(j)), a[j], lTmp2)
			lTmp3 = fxp_mac_16by16(yy(base+1-int(j)), a[j+1], lTmp3)
			lTmp4 = fxp_mac_16by16(yy(base+2-int(j)), a[j+1], lTmp4)
			lTmp3 = fxp_mac_16by16(yy(base+2-int(j)), a[j], lTmp3)
			lTmp4 = fxp_mac_16by16(yy(base+3-int(j)), a[j], lTmp4)
		}

		lTmp1 = fxp_mac_16by16(yy(base-int(j)), a[j], lTmp1)
		lTmp2 = fxp_mac_16by16(yy(base+1-int(j)), a[j], lTmp2)
		lTmp3 = fxp_mac_16by16(yy(base+2-int(j)), a[j], lTmp3)
		lTmp4 = fxp_mac_16by16(yy(base+3-int(j)), a[j], lTmp4)

		lTmp1 = shl_int32(lTmp1, 4)
		v := amr_wb_round(-lTmp1)
		y[base] = v
		yBuf[mm+base] = v

		lTmp2 = fxp_mac_16by16(yBuf[mm+base], a[1], lTmp2)
		lTmp2 = shl_int32(lTmp2, 4)
		v = amr_wb_round(-lTmp2)
		y[base+1] = v
		yBuf[mm+base+1] = v

		lTmp3 = fxp_mac_16by16(yy(base-1), a[3], lTmp3)
		lTmp4 = fxp_mac_16by16(yBuf[mm+base], a[3], lTmp4)
		lTmp3 = fxp_mac_16by16(yBuf[mm+base], a[2], lTmp3)
		lTmp4 = fxp_mac_16by16(yBuf[mm+base+1], a[2], lTmp4)
		lTmp3 = fxp_mac_16by16(yBuf[mm+base+1], a[1], lTmp3)

		lTmp3 = shl_int32(lTmp3, 4)
		v = amr_wb_round(-lTmp3)
		y[base+2] = v
		yBuf[mm+base+2] = v

		lTmp4 = fxp_mac_16by16(yBuf[mm+base+2], a[1], lTmp4)
		lTmp4 = shl_int32(lTmp4, 4)
		v = amr_wb_round(-lTmp4)
		y[base+3] = v
		yBuf[mm+base+3] = v
	}

	if update != 0 {
		copy(mem[:mm], y[int(lg)-mm:int(lg)])
	}
}

// Syn_filt_32 does 32-bit (hi/lo) synthesis filtering. a is Q12; exc the
// excitation scaled by Qnew; sigHi/sigLo hold the synthesis split across
// bit31..16 and bit15..4, each with M history before base offset off.
func Syn_filt_32(a []int16, m int16, exc []int16, qnew int16, sigHi, sigLo []int16, off int, lg int16) {
	a0 := 9 - qnew
	mm := int(m)
	a = a[:mm+1] // hoist a[k]/a[k+1] bounds checks out of the inner loop

	for i := 0; i < int(lg>>1); i++ {
		i2 := i << 1
		var lTmp3, lTmp4 int32
		lTmp1 := fxp_mul_16by16(sigLo[off+i2-1], a[1])
		lTmp2 := fxp_mul_16by16(sigHi[off+i2-1], a[1])

		var k int
		for k = 2; k < mm; k += 2 {
			lTmp1 = fxp_mac_16by16(sigLo[off+i2-1-k], a[k+1], lTmp1)
			lTmp2 = fxp_mac_16by16(sigHi[off+i2-1-k], a[k+1], lTmp2)
			lTmp1 = fxp_mac_16by16(sigLo[off+i2-k], a[k], lTmp1)
			lTmp2 = fxp_mac_16by16(sigHi[off+i2-k], a[k], lTmp2)
			lTmp3 = fxp_mac_16by16(sigLo[off+i2-k], a[k+1], lTmp3)
			lTmp4 = fxp_mac_16by16(sigHi[off+i2-k], a[k+1], lTmp4)
			lTmp3 = fxp_mac_16by16(sigLo[off+i2+1-k], a[k], lTmp3)
			lTmp4 = fxp_mac_16by16(sigHi[off+i2+1-k], a[k], lTmp4)
		}

		lTmp1 = -fxp_mac_16by16(sigLo[off+i2-k], a[k], lTmp1)
		lTmp3 = fxp_mac_16by16(sigLo[off+i2+1-k], a[k], lTmp3)
		lTmp2 = fxp_mac_16by16(sigHi[off+i2-k], a[k], lTmp2)
		lTmp4 = fxp_mac_16by16(sigHi[off+i2+1-k], a[k], lTmp4)

		lTmp1 >>= 11
		lTmp1 += int32(exc[i2]) << uint(a0)
		lTmp1 -= lTmp2 << 1
		lTmp1 = shl_int32(lTmp1, 3)
		sigHi[off+i2] = int16(lTmp1 >> 16)

		lTmp4 = fxp_mac_16by16(int16(lTmp1>>16), a[1], lTmp4)
		sigLo[off+i2] = int16((lTmp1 >> 4) - ((lTmp1 >> 16) << 12))

		lTmp3 = fxp_mac_16by16(sigLo[off+i2], a[1], lTmp3)
		lTmp3 = (-lTmp3) >> 11
		lTmp3 += int32(exc[i2+1]) << uint(a0)
		lTmp3 -= lTmp4 << 1
		lTmp3 = shl_int32(lTmp3, 3)
		sigHi[off+i2+1] = int16(lTmp3 >> 16)
		sigLo[off+i2+1] = int16((lTmp3 >> 4) - (int32(sigHi[off+i2+1]) << 12))
	}
}
