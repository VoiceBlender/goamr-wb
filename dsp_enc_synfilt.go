package amrwb

// Encoder synthesis filter, weighted-speech decimation, and RNG, ported
// verbatim from the Apache-2.0 vo-amrwbenc reference (syn_filt.c, lp_dec2.c,
// random.c), derived from 3GPP TS 26.190.

// Syn_filt is the order-16 16-bit LP synthesis filter 1/A(z). mem holds the 16
// history samples; update!=0 refreshes it from the tail of y.
func Syn_filt(a, x, y []int16, lg int16, mem []int16, update int16) {
	// Callers pass lg <= cL_SUBFR16k (80); a fixed stack array avoids a heap
	// allocation on this hot per-subframe path.
	var yArr [16 + cL_SUBFR16k]int16
	yBuf := yArr[:16+int(lg)]
	copy(yBuf[:16], mem[:16])
	const yyOff = 16
	a = a[:17] // hoist bounds checks out of the inner loop
	x = x[:int(lg)]
	a0 := a[0] >> 1
	for i := 0; i < int(lg); i++ {
		lTmp := int32(a0) * int32(x[i]) // vo_mult32
		// window yBuf[i .. i+15]; yw[16-k] == yBuf[yyOff+i-k]
		yw := yBuf[i : i+16]
		for k := 1; k <= 16; k++ {
			lTmp -= int32(a[k]) * int32(yw[16-k])
		}
		lTmp = lShl2(lTmp, 4)
		v := extract_h(L_add(lTmp, 0x8000))
		y[i] = v
		yBuf[yyOff+i] = v
	}
	if update != 0 {
		for i := 0; i < 16; i++ {
			mem[i] = yBuf[yyOff+int(lg)-16+i]
		}
	}
}

const cL_FIR2 = 5
const cL_MEM2 = 3 // L_FIR-2

var hFir2 = [cL_FIR2]int16{4260, 7536, 9175, 7536, 4260}

// LP_Decim2 low-pass filters and decimates x by 2 in place (mem size 3).
func LP_Decim2(x []int16, l int16, mem []int16) {
	// Callers pass l <= cL_FRAME (256); fixed stack array avoids a heap alloc.
	var xArr [cL_FRAME + cL_MEM2]int16
	xBuf := xArr[:int(l)+cL_MEM2]
	for i := 0; i < cL_MEM2; i++ {
		xBuf[i] = mem[i]
		mem[i] = x[int(l)-cL_MEM2+i]
	}
	for i := 0; i < int(l); i++ {
		xBuf[cL_MEM2+i] = x[i]
	}
	for i, j := 0, 0; i < int(l); i, j = i+2, j+1 {
		xw := xBuf[i : i+cL_FIR2] // l is even ⇒ i+cL_FIR2 <= len(xBuf)
		var lTmp int32
		for k := 0; k < cL_FIR2; k++ {
			lTmp += int32(xw[k]) * int32(hFir2[k])
		}
		x[j] = int16((lTmp + 0x4000) >> 15)
	}
}

// Random advances the LCG and returns the new 16-bit seed value.
func Random(seed *int16) int16 {
	*seed = int16(L_add(L_mult(*seed, 31821)>>1, 13849))
	return *seed
}
