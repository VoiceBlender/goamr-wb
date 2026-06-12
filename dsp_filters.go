package amrwb

// Self-contained synthesis/resampling filter kernels, ported verbatim from the
// Apache-2.0 opencore-amrwb reference (deemphasis_32.cpp,
// oversamp_12k8_to_16k.cpp), derived from 3GPP TS 26.173.

// deemphasis_32 applies H(z) = 1/(1 - mu z^-1) to the 32-bit (hi/lo) input,
// writing L samples to y. mem carries y[-1] across calls.
func deemphasis_32(xHi, xLo, y []int16, mu, L int16, mem *int16) {
	lTmp := (int32(xHi[0]) << 16) + (int32(xLo[0]) << 4)
	lTmp = shl_int32(lTmp, 3)
	lTmp = fxp_mac_16by16(*mem, mu, lTmp)
	lTmp = shl_int32(lTmp, 1)
	y[0] = amr_wb_round(lTmp)

	lo := xLo[1]
	hi := xHi[1]
	var i int16
	for i = 1; i < L-1; i++ {
		lTmp = (int32(hi) << 16) + (int32(lo) << 4)
		lTmp = shl_int32(lTmp, 3)
		lTmp = fxp_mac_16by16(y[i-1], mu, lTmp)
		lTmp = shl_int32(lTmp, 1)
		y[i] = amr_wb_round(lTmp)
		lo = xLo[i+1]
		hi = xHi[i+1]
	}
	lTmp = (int32(hi) << 16) + (int32(lo) << 4)
	lTmp = shl_int32(lTmp, 3)
	lTmp = fxp_mac_16by16(y[i-1], mu, lTmp)
	lTmp = shl_int32(lTmp, 1)
	y[i] = amr_wb_round(lTmp)

	*mem = y[L-1]
}

const (
	nbCoefUp    = 12 // NB_COEF_UP
	fac5        = 5
	invFac5     = 6554 // 1/5 in Q15
	nLoopCoefUp = 4
)

// firUp is the 1/5-resolution interpolation filter (Q14), fir_up[4][24].
var firUp = [4][24]int16{
	{
		-1, 12, -33, 68, -119, 191,
		-291, 430, -634, 963, -1616, 3792,
		15317, -2496, 1288, -809, 542, -369,
		247, -160, 96, -52, 23, -6,
	},
	{
		-4, 24, -62, 124, -213, 338,
		-510, 752, -1111, 1708, -2974, 8219,
		12368, -3432, 1881, -1204, 812, -552,
		368, -235, 139, -73, 30, -7,
	},
	{
		-7, 30, -73, 139, -235, 368,
		-552, 812, -1204, 1881, -3432, 12368,
		8219, -2974, 1708, -1111, 752, -510,
		338, -213, 124, -62, 24, -4,
	},
	{
		-6, 23, -52, 96, -160, 247,
		-369, 542, -809, 1288, -2496, 15317,
		3792, -1616, 963, -634, 430, -291,
		191, -119, 68, -33, 12, -1,
	},
}

// oversamp_12k8_to_16k oversamples lg samples of sig12k8 to 5/4*lg samples in
// sig16k (output divided by 16). mem has 2*nbCoefUp samples; signal is scratch
// of length 2*nbCoefUp + lg.
func oversamp_12k8_to_16k(sig12k8 []int16, lg int16, sig16k, mem, signal []int16) {
	copy(signal[:2*nbCoefUp], mem[:2*nbCoefUp])
	copy(signal[2*nbCoefUp:2*nbCoefUp+int(lg)], sig12k8[:lg])

	lgUp := lg + (lg >> 2)
	amrWbUpSamp(signal, nbCoefUp, sig16k, lgUp)

	copy(mem[:2*nbCoefUp], signal[int(lg):int(lg)+2*nbCoefUp])
}

// amrWbUpSamp mirrors AmrWbUp_samp; sigD is signal with base offset sigDOff.
//
// This 5/4 polyphase upsampler maps each block of fac5(=5) outputs to 4 input
// samples:
//
//	sigU[5k]     = signal[sigDOff+4k]                                  (copy)
//	sigU[5k+1+p] = postproc(sum_t signal[sigDOff+4k+p-baseOff+t]*firUp[p][t])
//
// For a fixed phase p this is a stride-1 FIR sampled every 4th output, so a
// single firRaw over the contiguous range (taking every 4th result) vectorises
// it. The over-computed outputs read only samples the scalar form also reads,
// so no padding is needed. Non-multiple-of-5 frames fall back to the scalar
// reference.
func amrWbUpSamp(signal []int16, sigDOff int, sigU []int16, lFrame int16) {
	n := int(lFrame)
	nBlk := n / fac5
	nOut := 4*nBlk - 3
	if n%fac5 != 0 || nBlk == 0 || nOut > 4*cL_SUBFR16k {
		amrWbUpSampScalar(signal, sigDOff, sigU, lFrame)
		return
	}

	const baseOff = 3*nLoopCoefUp - 1 // amrWbInterpol's xOff-3*nbCoef+1 base
	var full [4 * cL_SUBFR16k]int32
	dst := full[:nOut]
	for p := 0; p < nLoopCoefUp; p++ {
		firRaw(dst, signal[sigDOff+p-baseOff:], firUp[p][:])
		for k := 0; k < nBlk; k++ {
			v := shl_int32(0x2000+dst[4*k], 2)
			sigU[5*k+p+1] = int16(v >> 16)
		}
	}
	for k := 0; k < nBlk; k++ {
		sigU[5*k] = signal[sigDOff+4*k]
	}
}

// amrWbUpSampScalar is the per-sample reference used for non-multiple-of-5
// frame lengths.
func amrWbUpSampScalar(signal []int16, sigDOff int, sigU []int16, lFrame int16) {
	frac := int16(1)
	pt := 0
	for j := int16(0); j < lFrame; j++ {
		i := (int32(j) * invFac5) >> 13 // integer part = pos * 1/5
		frac--
		if frac != 0 {
			sigU[pt] = amrWbInterpol(signal, sigDOff+int(i), firUp[(fac5-1)-frac][:], nLoopCoefUp)
		} else {
			sigU[pt] = signal[sigDOff+int(i)+12-nbCoefUp]
			frac = fac5
		}
		pt++
	}
}

// amrWbInterpol mirrors AmrWbInterpol; x is signal[xOff], filter taps follow
// the C pointer base x - 3*nb_coef + 1.
func amrWbInterpol(signal []int16, xOff int, fir []int16, nbCoef int16) int16 {
	base := xOff - int(nbCoef) - int(nbCoef<<1) + 1
	lSum := int32(0x00002000)
	fi := 0
	xi := base
	for g := 0; g < 6; g++ {
		t1 := signal[xi]
		t2 := signal[xi+1]
		t3 := signal[xi+2]
		t4 := signal[xi+3]
		xi += 4
		lSum = fxp_mac_16by16(t1, fir[fi], lSum)
		lSum = fxp_mac_16by16(t2, fir[fi+1], lSum)
		lSum = fxp_mac_16by16(t3, fir[fi+2], lSum)
		lSum = fxp_mac_16by16(t4, fir[fi+3], lSum)
		fi += 4
	}
	lSum = shl_int32(lSum, 2)
	return int16(lSum >> 16)
}
