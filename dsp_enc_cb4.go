package amrwb

// 4-pulse / multi-track algebraic codebook search (modes 1..8), ported verbatim
// from the Apache-2.0 vo-amrwbenc reference (c4t64fx.c), derived from 3GPP TS
// 26.190. NB_TRACK=4, NB_POS=16, STEP=4, MSIZE=256, NB_MAX=8, NPMAXPT=6.

const (
	cMSIZE4   = 256
	cNB_MAX   = 8
	cNPMAXPT  = 6
	cNBPULMAX = 24
)

var tipos = [36]int16{
	0, 1, 2, 3, 1, 2, 3, 0, 2, 3, 0, 1, 3, 0, 1, 2,
	0, 1, 2, 3, 1, 2, 3, 0, 2, 3, 0, 1, 3, 0, 1, 2,
	0, 1, 2, 3,
}

// corHVec30 correlates h with vec for track 3 (with the wrap to track 0).
func corHVec30(h, vec []int16, track int16, sign []int16, rrixix *[4][cNB_POS]int16, cor1, cor2 []int16) {
	corX := 0
	p0i := 0 // rrixix[track]
	p3i := 0 // rrixix[0]
	pos := int(track)
	for i := 0; i < cNB_POS; i += 2 {
		for half := 0; half < 2; half++ {
			var lSum1, lSum2 int32
			hp := 0
			vp := pos
			for j := pos; j < cL_SUBFR; j++ {
				lSum1 += int32(h[hp]) * int32(vec[vp])
				lSum2 += int32(h[hp]) * int32(vec[vp-3])
				hp++
				vp++
			}
			vp -= 3
			lSum2 += int32(h[hp]) * int32(vec[vp])
			hp++
			vp++
			lSum2 += int32(h[hp]) * int32(vec[vp])
			hp++
			vp++
			lSum2 += int32(h[hp]) * int32(vec[vp])
			lSum1 <<= 2
			lSum2 <<= 2
			cor1[corX] = voMult(voRound(lSum1), sign[pos]) + rrixix[track][p0i]
			p0i++
			cor2[corX] = voMult(voRound(lSum2), sign[pos-3]) + rrixix[0][p3i]
			p3i++
			corX++
			pos += cSTEP
		}
	}
}

// corHVec012 correlates h with vec for tracks 0,1,2.
func corHVec012(h, vec []int16, track int16, sign []int16, rrixix *[4][cNB_POS]int16, cor1, cor2 []int16) {
	p0i := 0 // rrixix[track]
	p3i := 0 // rrixix[track+1]
	pos := int(track)
	for i := 0; i < cNB_POS; i += 2 {
		for half := 0; half < 2; half++ {
			var lSum1, lSum2 int32
			hp := 0
			vp := pos
			for j := 62 - pos; j >= 0; j-- {
				lSum1 += int32(h[hp]) * int32(vec[vp])
				vp++
				lSum2 += int32(h[hp]) * int32(vec[vp])
				hp++
			}
			lSum1 += int32(h[hp]) * int32(vec[vp])
			lSum1 <<= 2
			lSum2 <<= 2
			cor1[i+half] = voMult(int16((lSum1+0x8000)>>16), sign[pos]) + rrixix[track][p0i]
			p0i++
			cor2[i+half] = voMult(int16((lSum2+0x8000)>>16), sign[pos+1]) + rrixix[track+1][p3i]
			p3i++
			pos += cSTEP
		}
	}
}

// searchIxiy finds the best positions of 2 pulses in adjacent tracks.
func searchIxiy(nbPosIx, trackX, trackY int16, ps, alp, ix, iy *int16,
	dn, dn2, corX, corY []int16, rrixiy *[4][cMSIZE4]int16) {

	p0 := 0 // corX
	p1 := 0 // corY
	p2 := 0 // rrixiy[trackX]
	thresIx := int32(nbPosIx) - cNB_MAX

	alp0 := L_deposit_h(*alp) + 0x00008000
	sqk := int16(-1)
	alpk := int16(1)

	for x := int(trackX); x < cL_SUBFR; x += cSTEP {
		ps1 := *ps + dn[x]
		alp1 := alp0 + (int32(corX[p0]) << 13)
		p0++
		if int32(dn2[x]) < thresIx {
			pos := -1
			for y := int(trackY); y < cL_SUBFR; y += cSTEP {
				ps2 := ps1 + dn[y]
				alp2 := alp1 + (int32(corY[p1]) << 13)
				p1++
				alp2 = alp2 + (int32(rrixiy[trackX][p2]) << 14)
				p2++
				alp16 := extract_h(alp2)
				sq := voMult(ps2, ps2)
				s := (int32(alpk)*int32(sq))<<1 - (int32(sqk)*int32(alp16))<<1
				if s > 0 {
					sqk = sq
					alpk = alp16
					pos = y
				}
			}
			p1 -= cNB_POS
			if pos >= 0 {
				*ix = int16(x)
				*iy = int16(pos)
			}
		} else {
			p2 += cNB_POS
		}
	}
	*ps = *ps + dn[*ix] + dn[*iy]
	*alp = alpk
}

// ACELP_4t64_fx searches the multi-track algebraic codebook for the modes that
// use 4 tracks (20/36/44/52/64/72/88-bit codewords).
func ACELP_4t64_fx(dn, cn, h, code, y []int16, nbbits, serSize int16, index []int16) {
	var nbiter, alp, nbPulse int16
	var nbpos [10]int16
	switch nbbits {
	case 20:
		nbiter, alp, nbPulse = 4, 8192, 4
		nbpos[0], nbpos[1] = 4, 8
	case 36:
		nbiter, alp, nbPulse = 4, 4096, 8
		nbpos[0], nbpos[1], nbpos[2] = 4, 8, 8
	case 44:
		nbiter, alp, nbPulse = 4, 4096, 10
		nbpos[0], nbpos[1], nbpos[2], nbpos[3] = 4, 6, 8, 8
	case 52:
		nbiter, alp, nbPulse = 4, 4096, 12
		nbpos[0], nbpos[1], nbpos[2], nbpos[3] = 4, 6, 8, 8
	case 64:
		nbiter, alp, nbPulse = 3, 3277, 16
		nbpos[0], nbpos[1], nbpos[2], nbpos[3], nbpos[4], nbpos[5] = 4, 4, 6, 6, 8, 8
	case 72:
		nbiter, alp, nbPulse = 3, 3072, 18
		nbpos[0], nbpos[1], nbpos[2], nbpos[3], nbpos[4], nbpos[5], nbpos[6] = 2, 3, 4, 5, 6, 7, 8
	case 88:
		if serSize > 462 {
			nbiter = 1
		} else {
			nbiter = 2
		}
		alp, nbPulse = 2048, 24
		nbpos[0], nbpos[1], nbpos[2], nbpos[3], nbpos[4] = 2, 2, 3, 4, 5
		nbpos[5], nbpos[6], nbpos[7], nbpos[8], nbpos[9] = 6, 7, 8, 8, 8
	default:
		return
	}

	var codvec [cNBPULMAX]int16
	for i := int16(0); i < nbPulse; i++ {
		codvec[i] = i
	}
	var exp int16
	s := encDotProduct12(cn, cn, cL_SUBFR, &exp)
	encIsqrtN(&s, &exp)
	s = L_shl(s, exp+5)
	kCn := extract_h(L_add(s, 0x8000))

	s = encDotProduct12(dn, dn, cL_SUBFR, &exp)
	encIsqrtN(&s, &exp)
	kDn := int16((L_shl(s, exp+5+3) + 0x8000) >> 16)
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
			dn2[i] = -dn2[i]
		}
	}

	var posMax [cNB_TRACK]int16
	pos := 0
	for i := 0; i < cNB_TRACK; i++ {
		for k := 0; k < cNB_MAX; k++ {
			ps := int16(-1)
			for j := i; j < cL_SUBFR; j += cSTEP {
				if dn2[j] > ps {
					ps = dn2[j]
					pos = j
				}
			}
			dn2[pos] = int16(k - cNB_MAX)
			if k == 0 {
				posMax[i] = int16(pos)
			}
		}
	}

	// Scale h[] and build h / h_inv impulse buffers.
	hBuf := make([]int16, 4*cL_SUBFR)
	const hOff = cL_SUBFR
	const hInvOff = hOff + 2*cL_SUBFR
	var lTmp int32
	for i := 0; i < cL_SUBFR; i++ {
		lTmp += (int32(h[i]) * int32(h[i])) << 1
	}
	val := extract_h(lTmp)
	hShift := int16(0)
	if nbPulse >= 12 && val > 1024 {
		hShift = 1
	}
	for i := 0; i < cL_SUBFR; i++ {
		hBuf[hOff+i] = h[i] >> uint(hShift)
		hBuf[hInvOff+i] = -hBuf[hOff+i]
	}
	hh := hBuf[hOff:] // h

	// rrixix[track][pos]: impulse-response energies.
	var rrixix [4][cNB_POS]int16
	{
		p0, p1, p2, p3 := cNB_POS-1, cNB_POS-1, cNB_POS-1, cNB_POS-1
		ph := 0
		cor := int32(0x00008000)
		for i := 0; i < cNB_POS; i++ {
			cor += int32(hh[ph]) * int32(hh[ph]) << 1
			ph++
			rrixix[3][p3] = extract_h(cor)
			p3--
			cor += int32(hh[ph]) * int32(hh[ph]) << 1
			ph++
			rrixix[2][p2] = extract_h(cor)
			p2--
			cor += int32(hh[ph]) * int32(hh[ph]) << 1
			ph++
			rrixix[1][p1] = extract_h(cor)
			p1--
			cor += int32(hh[ph]) * int32(hh[ph]) << 1
			ph++
			rrixix[0][p0] = extract_h(cor)
			p0--
		}
	}

	// rrixiy[track][...]: cross-correlations between adjacent tracks.
	var rrixiy [4][cMSIZE4]int16
	{
		pos := cMSIZE4 - 1
		ptrHf := 1 // h+1
		for k := 0; k < cNB_POS; k++ {
			p3, p2, p1 := pos, pos, pos
			p0 := pos - cNB_POS
			cor := int32(0x00008000)
			ph1, ph2 := 0, ptrHf
			for i := k + 1; i < cNB_POS; i++ {
				cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
				ph1++
				ph2++
				rrixiy[2][p3] = extract_h(cor)
				cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
				ph1++
				ph2++
				rrixiy[1][p2] = extract_h(cor)
				cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
				ph1++
				ph2++
				rrixiy[0][p1] = extract_h(cor)
				cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
				ph1++
				ph2++
				rrixiy[3][p0] = extract_h(cor)
				p3 -= cNB_POS + 1
				p2 -= cNB_POS + 1
				p1 -= cNB_POS + 1
				p0 -= cNB_POS + 1
			}
			cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
			ph1++
			ph2++
			rrixiy[2][p3] = extract_h(cor)
			cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
			ph1++
			ph2++
			rrixiy[1][p2] = extract_h(cor)
			cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
			ph1++
			ph2++
			rrixiy[0][p1] = extract_h(cor)
			pos -= cNB_POS
			ptrHf += cSTEP
		}
	}
	{
		pos := cMSIZE4 - 1
		ptrHf := 3 // h+3
		for k := 0; k < cNB_POS; k++ {
			p3 := pos
			p2 := pos - 1
			p1 := pos - 1
			p0 := pos - 1
			cor := int32(0x00008000)
			ph1, ph2 := 0, ptrHf
			for i := k + 1; i < cNB_POS; i++ {
				cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
				ph1++
				ph2++
				rrixiy[3][p3] = extract_h(cor)
				cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
				ph1++
				ph2++
				rrixiy[2][p2] = extract_h(cor)
				cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
				ph1++
				ph2++
				rrixiy[1][p1] = extract_h(cor)
				cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
				ph1++
				ph2++
				rrixiy[0][p0] = extract_h(cor)
				p3 -= cNB_POS + 1
				p2 -= cNB_POS + 1
				p1 -= cNB_POS + 1
				p0 -= cNB_POS + 1
			}
			cor += int32(hh[ph1]) * int32(hh[ph2]) << 1
			ph1++
			ph2++
			rrixiy[3][p3] = extract_h(cor)
			pos--
			ptrHf += cSTEP
		}
	}

	// Apply signs to rrixiy.
	{
		p0 := 0 // flat index into rrixiy[0][...] then [1]... — C uses &rrixiy[0][0] linearly
		// rrixiy is [4][256]; the C iterates a flat pointer over all 1024. We mirror
		// by tracking (track,offset).
		flat := func(idx int) (int, int) { return idx / cMSIZE4, idx % cMSIZE4 }
		for k := 0; k < cNB_TRACK; k++ {
			jTemp := (k + 1) & 0x03
			for i := k; i < cL_SUBFR; i += cSTEP {
				psign := sign
				if sign[i] < 0 {
					psign = vec
				}
				for j := jTemp; j < cL_SUBFR; j += cSTEP {
					tr, off := flat(p0)
					rrixiy[tr][off] = voMult(rrixiy[tr][off], psign[j])
					p0++
				}
			}
		}
	}

	// Deep-first search.
	var ind [cNPMAXPT * cNB_TRACK]int16
	var ipos [cNBPULMAX]int16
	corXbuf := make([]int16, cNB_POS)
	corYbuf := make([]int16, cNB_POS)
	psk := int16(-1)
	alpk := int16(1)

	// ix/iy persist across stages and iterations: search_ixiy only writes them
	// when it finds a valid position, otherwise the prior value is reused.
	var ix, iy int16

	for k := int16(0); k < nbiter; k++ {
		jTemp := int(k) << 2
		for i := int16(0); i < nbPulse; i++ {
			ipos[i] = tipos[jTemp+int(i)]
		}
		var ps, alpv int16
		var startPos int16
		if nbbits == 20 {
			startPos = 0
			ps = 0
			alpv = 0
			for i := 0; i < cL_SUBFR; i++ {
				vec[i] = 0
			}
		} else if nbbits == 36 || nbbits == 44 {
			startPos = 2
			ix = posMax[ipos[0]]
			iy = posMax[ipos[1]]
			ind[0] = ix
			ind[1] = iy
			ps = dn[ix] + dn[iy]
			ii := int(ix >> 2)
			jj := int(iy >> 2)
			ss := int32(rrixix[ipos[0]][ii]) << 13
			ss += int32(rrixix[ipos[1]][jj]) << 13
			ii = (ii << 4) + jj
			ss += int32(rrixiy[ipos[0]][ii]) << 14
			alpv = int16((ss + 0x8000) >> 16)
			var p0b, p1b int
			if sign[ix] < 0 {
				p0b = hInvOff - int(ix)
			} else {
				p0b = hOff - int(ix)
			}
			if sign[iy] < 0 {
				p1b = hInvOff - int(iy)
			} else {
				p1b = hOff - int(iy)
			}
			for i := 0; i < cL_SUBFR; i++ {
				vec[i] = hBuf[p0b+i] + hBuf[p1b+i]
			}
			if nbbits == 44 {
				ipos[8] = 0
				ipos[9] = 1
			}
		} else {
			startPos = 4
			ix = posMax[ipos[0]]
			iy = posMax[ipos[1]]
			ii := posMax[ipos[2]]
			jj := posMax[ipos[3]]
			ind[0], ind[1], ind[2], ind[3] = ix, iy, ii, jj
			ps = dn[ix] + dn[iy] + dn[ii] + dn[jj]
			pb := func(p int16) int {
				if sign[p] < 0 {
					return hInvOff - int(p)
				}
				return hOff - int(p)
			}
			p0b, p1b, p2b, p3b := pb(ix), pb(iy), pb(ii), pb(jj)
			var lt int32
			for i := 0; i < cL_SUBFR; i++ {
				vec[i] = hBuf[p0b+i] + hBuf[p1b+i] + hBuf[p2b+i] + hBuf[p3b+i]
				lt += (int32(vec[i]) * int32(vec[i])) << 1
			}
			alpv = int16(((lt >> 3) + 0x8000) >> 16)
			if nbbits == 72 {
				ipos[16] = 0
				ipos[17] = 1
			}
		}

		st := 0
		for j := startPos; j < nbPulse; j, st = j+2, st+1 {
			if ipos[j] == 3 {
				corHVec30(hh, vec, ipos[j], sign, &rrixix, corXbuf, corYbuf)
			} else {
				corHVec012(hh, vec, ipos[j], sign, &rrixix, corXbuf, corYbuf)
			}
			searchIxiy(nbpos[st], ipos[j], ipos[j+1], &ps, &alpv, &ix, &iy, dn, dn2, corXbuf, corYbuf, &rrixiy)
			ind[j] = ix
			ind[j+1] = iy
			var p0b, p1b int
			if sign[ix] < 0 {
				p0b = hInvOff - int(ix)
			} else {
				p0b = hOff - int(ix)
			}
			if sign[iy] < 0 {
				p1b = hInvOff - int(iy)
			} else {
				p1b = hOff - int(iy)
			}
			for i := 0; i < cL_SUBFR; i++ {
				vec[i] += hBuf[p0b+i] + hBuf[p1b+i]
			}
		}

		// Memorise the best codevector.
		psSq := voMult(ps, ps)
		sCmp := (int32(alpk)*int32(psSq))<<1 - (int32(psk)*int32(alpv))<<1
		if sCmp > 0 {
			psk = psSq
			alpk = alpv
			for i := int16(0); i < nbPulse; i++ {
				codvec[i] = ind[i]
			}
			copy(y[:cL_SUBFR], vec[:cL_SUBFR])
		}
	}

	// Build the codeword, filtered codeword and index of codevector.
	for i := 0; i < cNPMAXPT*cNB_TRACK; i++ {
		ind[i] = -1
	}
	for i := 0; i < cL_SUBFR; i++ {
		code[i] = 0
		y[i] = voShrR(y[i], 3)
	}
	pulseVal := int16(512 >> uint(hShift))
	for k := int16(0); k < nbPulse; k++ {
		i := int(codvec[k])
		j := sign[i]
		idx := int16(i >> 2)
		track := int16(i & 0x03)
		if j > 0 {
			code[i] += pulseVal
			codvec[k] += 128
		} else {
			code[i] -= pulseVal
			idx += cNB_POS
		}
		ii := int(track) * cNPMAXPT
		for ind[ii] >= 0 {
			ii++
		}
		ind[ii] = idx
	}

	k := 0
	switch nbbits {
	case 20:
		for track := 0; track < cNB_TRACK; track++ {
			index[track] = int16(quant1pN1(ind[k], 4))
			k += cNPMAXPT
		}
	case 36:
		for track := 0; track < cNB_TRACK; track++ {
			index[track] = int16(quant2p2N1(ind[k], ind[k+1], 4))
			k += cNPMAXPT
		}
	case 44:
		for track := 0; track < cNB_TRACK-2; track++ {
			index[track] = int16(quant3p3N1(ind[k], ind[k+1], ind[k+2], 4))
			k += cNPMAXPT
		}
		for track := 2; track < cNB_TRACK; track++ {
			index[track] = int16(quant2p2N1(ind[k], ind[k+1], 4))
			k += cNPMAXPT
		}
	case 52:
		for track := 0; track < cNB_TRACK; track++ {
			index[track] = int16(quant3p3N1(ind[k], ind[k+1], ind[k+2], 4))
			k += cNPMAXPT
		}
	case 64:
		for track := 0; track < cNB_TRACK; track++ {
			l := quant4p4N(ind[k:], 4)
			index[track] = int16((l >> 14) & 3)
			index[track+cNB_TRACK] = int16(l & 0x3FFF)
			k += cNPMAXPT
		}
	case 72:
		for track := 0; track < cNB_TRACK-2; track++ {
			l := quant5p5N(ind[k:], 4)
			index[track] = int16((l >> 10) & 0x03FF)
			index[track+cNB_TRACK] = int16(l & 0x03FF)
			k += cNPMAXPT
		}
		for track := 2; track < cNB_TRACK; track++ {
			l := quant4p4N(ind[k:], 4)
			index[track] = int16((l >> 14) & 3)
			index[track+cNB_TRACK] = int16(l & 0x3FFF)
			k += cNPMAXPT
		}
	case 88:
		for track := 0; track < cNB_TRACK; track++ {
			l := quant6p6N2(ind[k:], 4)
			index[track] = int16((l >> 11) & 0x07FF)
			index[track+cNB_TRACK] = int16(l & 0x07FF)
			k += cNPMAXPT
		}
	}
}
