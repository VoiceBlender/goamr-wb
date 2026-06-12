package amrwb

// LP-analysis chain, ported verbatim from the Apache-2.0 vo-amrwbenc reference
// (autocorr.c, lag_wind.c, levinson.c, az_isp.c), derived from 3GPP TS 26.190:
// windowed autocorrelation -> lag windowing -> Levinson-Durbin -> LPC->ISP.

const cGRID_POINTS = 100

// Autocorr computes the windowed autocorrelation r[0..m] in double precision.
func Autocorr(x []int16, m int16, rH, rL []int16) {
	y := make([]int16, cL_WINDOW)
	for i := 0; i < cL_WINDOW; i++ {
		y[i] = voMultR(x[i], vo_window[i])
	}

	lSum := int32(16) << 16 // vo_L_deposit_h(16)
	for i := 0; i < cL_WINDOW; i++ {
		lTmp := (int32(y[i]) * int32(y[i])) << 1
		lTmp >>= 8
		lSum += lTmp
	}

	norm := norm_l(lSum)
	shift := 4 - (norm >> 1)
	if shift > 0 {
		for i := 0; i < cL_WINDOW; i++ {
			y[i] = voShrR(y[i], shift)
		}
	}

	lSum = 1
	for i := 0; i < cL_WINDOW; i++ {
		lSum += (int32(y[i]) * int32(y[i])) << 1
	}
	norm = norm_l(lSum)
	lSum <<= uint(norm)
	rH[0] = int16(lSum >> 16)
	rL[0] = int16((lSum & 0xffff) >> 1)

	for i := int16(1); i <= 8; i++ {
		var lSum1, lSum2 int32
		flen := cL_WINDOW - 2*int(i)
		p1 := 0
		p2 := 2*int(i) - 1
		for {
			lSum1 += int32(y[p1]) * int32(y[p2])
			p2++
			lSum2 += int32(y[p1]) * int32(y[p2])
			p1++
			flen--
			if flen == 0 {
				break
			}
		}
		lSum1 += int32(y[p1]) * int32(y[p2])

		lSum1 <<= uint(norm)
		lSum2 <<= uint(norm)
		rH[2*i-1] = int16(lSum1 >> 15)
		rL[2*i-1] = int16(lSum1 & 0x7fff)
		rH[2*i] = int16(lSum2 >> 15)
		rL[2*i] = int16(lSum2 & 0x7fff)
	}
}

// Lag_window applies the lag window to the autocorrelations in place.
func Lag_window(rH, rL []int16) {
	for i := 1; i <= cM; i++ {
		x := mpy32(rH[i], rL[i], volag_h[i-1], volag_l[i-1])
		rH[i] = int16(x >> 16)
		rL[i] = int16((x & 0xffff) >> 1)
	}
}

// Levinson solves the normal equations (Levinson-Durbin), producing LPC coeffs
// A[0..M] (Q12) and reflection coeffs rc[0..M-1] (Q15). mem is 18 words.
func Levinson(rH, rL, a, rc []int16, mem []int16) {
	var ah, al, anh, anl [cM + 1]int16
	oldA := mem[:cM]
	oldRc := mem[cM:]

	t1 := (int32(rH[1]) << 16) + (int32(rL[1]) << 1)
	t2 := L_abs(t1)
	t0 := div32(t2, rH[0], rL[0])
	if t1 > 0 {
		t0 = -t0
	}
	kh := int16(t0 >> 16)
	kl := int16((t0 & 0xffff) >> 1)
	rc[0] = kh
	t0 >>= 4
	ah[1] = int16(t0 >> 16)
	al[1] = int16((t0 & 0xffff) >> 1)

	t0 = mpy32(kh, kl, kh, kl)
	t0 = L_abs(t0)
	t0 = 0x7fffffff - t0
	hi := int16(t0 >> 16)
	lo := int16((t0 & 0xffff) >> 1)
	t0 = mpy32(rH[0], rL[0], hi, lo)

	alpExp := norm_l(t0)
	t0 <<= uint(alpExp)
	alpH := int16(t0 >> 16)
	alpL := int16((t0 & 0xffff) >> 1)

	for i := int16(2); i <= cM; i++ {
		t0 = 0
		for j := int16(1); j < i; j++ {
			t0 += mpy32(rH[j], rL[j], ah[i-j], al[i-j])
		}
		t0 <<= 4
		t1 = (int32(rH[i]) << 16) + (int32(rL[i]) << 1)
		t0 += t1

		t1 = L_abs(t0)
		t2 = div32(t1, alpH, alpL)
		if t0 > 0 {
			t2 = -t2
		}
		t2 <<= uint(alpExp)
		kh = int16(t2 >> 16)
		kl = int16((t2 & 0xffff) >> 1)
		rc[i-1] = kh

		if abs_s(kh) > 32750 {
			a[0] = 4096
			for j := int16(0); j < cM; j++ {
				a[j+1] = oldA[j]
			}
			rc[0] = oldRc[0]
			rc[1] = oldRc[1]
			return
		}

		for j := int16(1); j < i; j++ {
			t0 = mpy32(kh, kl, ah[i-j], al[i-j])
			t0 += (int32(ah[j]) << 16) + (int32(al[j]) << 1)
			anh[j] = int16(t0 >> 16)
			anl[j] = int16((t0 & 0xffff) >> 1)
		}
		t2 >>= 4
		anh[i], anl[i] = voLExtract(t2)

		t0 = mpy32(kh, kl, kh, kl)
		t0 = L_abs(t0)
		t0 = 0x7fffffff - t0
		hi = int16(t0 >> 16)
		lo = int16((t0 & 0xffff) >> 1)
		t0 = mpy32(alpH, alpL, hi, lo)

		jn := norm_l(t0)
		t0 <<= uint(jn)
		alpH = int16(t0 >> 16)
		alpL = int16((t0 & 0xffff) >> 1)
		alpExp += jn

		for j := int16(1); j <= i; j++ {
			ah[j] = anh[j]
			al[j] = anl[j]
		}
	}

	a[0] = 4096
	for i := int16(1); i <= cM; i++ {
		t0 = (int32(ah[i]) << 16) + (int32(al[i]) << 1)
		v := int16((t0<<1 + 0x8000) >> 16) // vo_round(t0<<1)
		oldA[i-1] = v
		a[i] = v
	}
	oldRc[0] = rc[0]
	oldRc[1] = rc[1]
}

// chebps2 evaluates the Chebyshev polynomial of f[] at x (Q24 internal).
func chebps2(x int16, f []int16, n int16) int16 {
	t0 := int32(f[0]) << 13
	b2H := int16(t0 >> 16)
	b2L := int16((t0 & 0xffff) >> 1)

	t0 = ((int32(b2H) * int32(x)) << 1) + (((int32(b2L) * int32(x)) >> 15) << 1)
	t0 <<= 1
	t0 += int32(f[1]) << 13
	b1H := int16(t0 >> 16)
	b1L := int16((t0 & 0xffff) >> 1)

	var b0H, b0L int16
	for i := int16(2); i < n; i++ {
		t0 = ((int32(b1H) * int32(x)) << 1) + (((int32(b1L) * int32(x)) >> 15) << 1)
		t0 += int32(b2H) * (-16384) << 1
		t0 += int32(f[i]) << 12
		t0 <<= 1
		t0 -= int32(b2L) << 1
		b0H = int16(t0 >> 16)
		b0L = int16((t0 & 0xffff) >> 1)
		b2L = b1L
		b2H = b1H
		b1L = b0L
		b1H = b0H
	}

	t0 = ((int32(b1H) * int32(x)) << 1) + (((int32(b1L) * int32(x)) >> 15) << 1)
	t0 += int32(b2H) * (-32768) << 1
	t0 -= int32(b2L) << 1
	t0 += int32(f[n]) << 12
	t0 = lShl2(t0, 6)
	cheb := extract_h(t0)
	if cheb == -32768 {
		cheb = -32767
	}
	return cheb
}

// Az_isp converts LPC coefficients a[0..M] (Q12) to ISPs (Q15) via Chebyshev
// root search; falls back to oldIsp if fewer than M-1 roots are found.
func Az_isp(a, isp, oldIsp []int16) {
	var f1 [cNC + 1]int16
	var f2 [cNC]int16
	for i := 0; i < cNC; i++ {
		t0 := int32(a[i]) << 15
		f1[i] = int16((t0 + (int32(a[cM-i]) << 15) + 0x8000) >> 16) // vo_round
		f2[i] = int16((t0 - (int32(a[cM-i]) << 15) + 0x8000) >> 16)
	}
	f1[cNC] = a[cNC]
	for i := 2; i < cNC; i++ {
		f2[i] = f2[i] + f2[i-2]
	}

	nf := 0
	ip := 0
	coef := f1[:]
	order := int16(cNC)
	xlow := vogrid[0]
	ylow := chebps2(xlow, coef, order)
	j := 0
	for nf < cM-1 && j < cGRID_POINTS {
		j++
		xhigh := xlow
		yhigh := ylow
		xlow = vogrid[j]
		ylow = chebps2(xlow, coef, order)
		if int32(ylow)*int32(yhigh) <= 0 {
			for i := 0; i < 2; i++ {
				xmid := (xlow >> 1) + (xhigh >> 1)
				ymid := chebps2(xmid, coef, order)
				if int32(ylow)*int32(ymid) <= 0 {
					yhigh = ymid
					xhigh = xmid
				} else {
					ylow = ymid
					xlow = xmid
				}
			}
			var xint int16
			x := xhigh - xlow
			y := yhigh - ylow
			if y == 0 {
				xint = xlow
			} else {
				sign := y
				y = abs_s(y)
				exp := norm_s(y)
				y <<= uint(exp)
				y = div_s(16383, y)
				t0 := int32(x) * int32(y)
				t0 = t0 >> uint(19-exp)
				y = int16(t0)
				if sign < 0 {
					y = -y
				}
				t0 = int32(ylow) * int32(y)
				t0 = t0 >> 10
				xint = xlow - int16(t0)
			}
			isp[nf] = xint
			xlow = xint
			nf++
			if ip == 0 {
				ip = 1
				coef = f2[:]
				order = cNC - 1
			} else {
				ip = 0
				coef = f1[:]
				order = cNC
			}
			ylow = chebps2(xlow, coef, order)
		}
	}

	if nf < cM-1 {
		for i := 0; i < cM; i++ {
			isp[i] = oldIsp[i]
		}
	} else {
		isp[cM-1] = shl(a[cM], 3) // Q12 -> Q15 with saturation
	}
}
