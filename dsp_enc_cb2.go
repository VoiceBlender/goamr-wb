package amrwb

// 2-pulse algebraic codebook search (6.60 kbit/s mode), ported verbatim from
// the Apache-2.0 vo-amrwbenc reference (c2t64fx.c, ACELP_2t64_fx), derived from
// 3GPP TS 26.190. NB_TRACK=2, NB_POS=32, STEP=2, MSIZE=1024.

const (
	cb2NbPos = 32
	cb2Step  = 2
	cb2Msize = 1024
)

// ACELP_2t64_fx finds the 2 pulses minimizing the weighted error; writes the
// Q9 codeword code[], the filtered codeword y[], and the 12-bit *index.
func ACELP_2t64_fx(dn, cn, h []int16, code, y []int16, index *int16) {
	const alp = 8192 // 2.0 Q12
	var exp int16

	s := encDotProduct12(cn, cn, cL_SUBFR, &exp)
	encIsqrtN(&s, &exp)
	s = L_shl(s, exp+5)
	kCn := voRound(s)

	s = encDotProduct12(dn, dn, cL_SUBFR, &exp)
	encIsqrtN(&s, &exp)
	kDn := voRound(L_shl(s, exp+8))
	kDn = voMultR(alp, kDn)

	dn2 := make([]int16, cL_SUBFR)
	for i := 0; i < cL_SUBFR; i++ {
		v := int32(kCn)*int32(cn[i]) + int32(kDn)*int32(dn[i])
		dn2[i] = int16(v >> 7)
	}

	sign := make([]int16, cL_SUBFR)
	vec := make([]int16, cL_SUBFR)
	for i := 0; i < cL_SUBFR; i++ {
		val := dn[i]
		if dn2[i] >= 0 {
			sign[i] = 32767
			vec[i] = -32768
		} else {
			sign[i] = -32768
			vec[i] = 32767
			dn[i] = -val
		}
	}

	// Impulse response buffer: h at hOff, h_inv at hInvOff (negatives).
	hBuf := make([]int16, 4*cL_SUBFR)
	const hOff = cL_SUBFR
	const hInvOff = hOff + (cL_SUBFR << 1)
	for i := 0; i < cL_SUBFR; i++ {
		hBuf[hOff+i] = h[i]
		hBuf[hInvOff+i] = -h[i]
	}

	// rrixix[2][32]: autocorrelation of h per track (x0.5).
	var rrixix [2][cb2NbPos]int16
	{
		p0 := cb2NbPos - 1
		p1 := cb2NbPos - 1
		ph := hOff
		cor := int32(0x00010000)
		for i := 0; i < cb2NbPos; i++ {
			cor += (int32(hBuf[ph]) * int32(hBuf[ph])) << 1
			ph++
			rrixix[1][p1] = extract_h(cor) >> 1
			p1--
			cor += (int32(hBuf[ph]) * int32(hBuf[ph])) << 1
			ph++
			rrixix[0][p0] = extract_h(cor) >> 1
			p0--
		}
	}

	// rrixiy[1024]: cross-correlation of h between tracks.
	rrixiy := make([]int16, cb2Msize)
	{
		pos := cb2Msize - 1
		pos2 := cb2Msize - 2
		ptrHf := hOff + 1
		for k := 0; k < cb2NbPos; k++ {
			p1 := pos
			p0 := pos2
			cor := int32(0x00008000)
			ph1 := hOff
			ph2 := ptrHf
			for i := k + 1; i < cb2NbPos; i++ {
				cor += (int32(hBuf[ph1]) * int32(hBuf[ph2])) << 1
				ph1++
				ph2++
				rrixiy[p1] = extract_h(cor)
				cor += (int32(hBuf[ph1]) * int32(hBuf[ph2])) << 1
				ph1++
				ph2++
				rrixiy[p0] = extract_h(cor)
				p1 -= cb2NbPos + 1
				p0 -= cb2NbPos + 1
			}
			cor += (int32(hBuf[ph1]) * int32(hBuf[ph2])) << 1
			ph1++
			ph2++
			rrixiy[p1] = extract_h(cor)
			pos -= cb2NbPos
			pos2--
			ptrHf += cb2Step
		}
	}

	// Apply signs to rrixiy.
	{
		idx := 0
		for i := 0; i < cL_SUBFR; i += cb2Step {
			psign := sign
			if sign[i] < 0 {
				psign = vec
			}
			for j := 1; j < cL_SUBFR; j += cb2Step {
				rrixiy[idx] = voMult(rrixiy[idx], psign[j])
				idx++
			}
		}
	}

	// Search 2 pulses (32x32).
	p0i := 0 // rrixix[0]
	p1i := 0 // rrixix[1]
	p2i := 0 // rrixiy
	psk := int16(-1)
	alpk := int16(1)
	ix := int16(0)
	iy := int16(1)
	for i0 := 0; i0 < cL_SUBFR; i0 += cb2Step {
		ps1 := dn[i0]
		alp1 := rrixix[0][p0i]
		p0i++
		pos := int16(-1)
		for i1 := 1; i1 < cL_SUBFR; i1 += cb2Step {
			ps2 := ps1 + dn[i1]
			alp2 := alp1 + rrixix[1][p1i] + rrixiy[p2i]
			p1i++
			p2i++
			sq := voMult(ps2, ps2)
			s := (int32(alpk)*int32(sq))<<1 - (int32(psk)*int32(alp2))<<1
			if s > 0 {
				psk = sq
				alpk = alp2
				pos = int16(i1)
			}
		}
		p1i -= cb2NbPos
		if pos >= 0 {
			ix = int16(i0)
			iy = pos
		}
	}

	for i := 0; i < cL_SUBFR; i++ {
		code[i] = 0
	}
	i0 := ix >> 1
	i1 := iy >> 1
	var p0base, p1base int
	if sign[ix] > 0 {
		code[ix] = 512
		p0base = hOff - int(ix)
	} else {
		code[ix] = -512
		i0 += cb2NbPos
		p0base = hInvOff - int(ix)
	}
	if sign[iy] > 0 {
		code[iy] = 512
		p1base = hOff - int(iy)
	} else {
		code[iy] = -512
		i1 += cb2NbPos
		p1base = hInvOff - int(iy)
	}
	*index = (i0 << 6) + i1
	for i := 0; i < cL_SUBFR; i++ {
		y[i] = voShrR(hBuf[p0base+i]+hBuf[p1base+i], 3)
	}
}
