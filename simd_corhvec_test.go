package amrwb

import "testing"

// corHVec30Ref and corHVec012Ref are the verbatim ports corHVec30 and
// corHVec012 replaced; the firDot versions are tested against them.
func corHVec30Ref(h, vec []int16, track int16, sign []int16, rrixix *[4][cNB_POS]int16, cor1, cor2 []int16) {
	corX, p0i, p3i, pos := 0, 0, 0, int(track)
	for i := 0; i < cNB_POS; i += 2 {
		for half := 0; half < 2; half++ {
			var lSum1, lSum2 int32
			hp, vp := 0, pos
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

func corHVec012Ref(h, vec []int16, track int16, sign []int16, rrixix *[4][cNB_POS]int16, cor1, cor2 []int16) {
	p0i, p3i, pos := 0, 0, int(track)
	for i := 0; i < cNB_POS; i += 2 {
		for half := 0; half < 2; half++ {
			var lSum1, lSum2 int32
			hp, vp := 0, pos
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

// TestCorHVecMatchesReference fuzzes corHVec30/corHVec012 against the
// verbatim ports, including full-scale inputs where the sums wrap.
func TestCorHVecMatchesReference(t *testing.T) {
	seed := uint32(3)
	rnd := func() int16 { seed = seed*1664525 + 1013904223; return int16(seed >> 16) }
	var h, vec, sign [cL_SUBFR]int16
	var rr [4][cNB_POS]int16
	var a1, a2, b1, b2 [cNB_POS]int16
	for iter := 0; iter < 3000; iter++ {
		for i := range h {
			switch iter % 5 {
			case 0:
				h[i], vec[i] = -32768, -32768
			case 1:
				h[i], vec[i] = 32767, -32768
			default:
				h[i], vec[i] = rnd(), rnd()
			}
			sign[i] = rnd()
		}
		for i := range rr {
			for j := range rr[i] {
				rr[i][j] = rnd()
			}
		}
		corHVec30(h[:], vec[:], 3, sign[:], &rr, a1[:], a2[:])
		corHVec30Ref(h[:], vec[:], 3, sign[:], &rr, b1[:], b2[:])
		if a1 != b1 || a2 != b2 {
			t.Fatalf("iteration %d: corHVec30 differs", iter)
		}
		for track := int16(0); track < 3; track++ {
			corHVec012(h[:], vec[:], track, sign[:], &rr, a1[:], a2[:])
			corHVec012Ref(h[:], vec[:], track, sign[:], &rr, b1[:], b2[:])
			if a1 != b1 || a2 != b2 {
				t.Fatalf("iteration %d track %d: corHVec012 differs", iter, track)
			}
		}
	}
}

// searchIxiyRef is the verbatim port searchIxiy replaced. It finds the best positions of 2 pulses in adjacent tracks.
func searchIxiyRef(nbPosIx, trackX, trackY int16, ps, alp, ix, iy *int16,
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

// TestSearchIxiyMatchesReference fuzzes searchIxiy against the verbatim port,
// with values that make the comparison products wrap.
func TestSearchIxiyMatchesReference(t *testing.T) {
	seed := uint32(5)
	rnd := func() int16 { seed = seed*1664525 + 1013904223; return int16(seed >> 16) }
	var dn, dn2 [cL_SUBFR]int16
	var corX, corY [cNB_POS]int16
	var rr [4][cMSIZE4]int16
	for iter := 0; iter < 20000; iter++ {
		big := iter%3 == 0
		for i := range dn {
			dn[i] = rnd()
			if !big {
				dn[i] >>= 4
			}
			dn2[i] = int16(seed>>8) & 15
		}
		for i := range corX {
			corX[i], corY[i] = rnd(), rnd()
		}
		for i := range rr {
			for j := range rr[i] {
				rr[i][j] = rnd()
			}
		}
		trackX, trackY := int16(iter%4), int16((iter/4)%4)
		nbPos := int16(8 + iter%9)
		ps0, alp0 := rnd(), rnd()
		ixInit, iyInit := trackX, trackY
		ps1, alp1, ix1, iy1 := ps0, alp0, ixInit, iyInit
		ps2, alp2, ix2, iy2 := ps0, alp0, ixInit, iyInit
		searchIxiy(nbPos, trackX, trackY, &ps1, &alp1, &ix1, &iy1, dn[:], dn2[:], corX[:], corY[:], &rr)
		searchIxiyRef(nbPos, trackX, trackY, &ps2, &alp2, &ix2, &iy2, dn[:], dn2[:], corX[:], corY[:], &rr)
		if ps1 != ps2 || alp1 != alp2 || ix1 != ix2 || iy1 != iy2 {
			t.Fatalf("iteration %d: got ps %d alp %d ix %d iy %d, want %d %d %d %d", iter, ps1, alp1, ix1, iy1, ps2, alp2, ix2, iy2)
		}
	}
}
