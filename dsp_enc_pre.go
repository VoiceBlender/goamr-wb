package amrwb

// Encoder input preprocessing and ISP->ISF conversion, ported verbatim from the
// Apache-2.0 vo-amrwbenc reference (isp_isf.c, decim54.c, hp50.c), derived from
// 3GPP TS 26.190.

// Isp_isf converts ISP (cosine domain, Q15) to normalized ISF (0..0.5, Q15)
// via acos table interpolation. Reuses the cos table (isfAcosTable) + slope.
func Isp_isf(isp, isf []int16, m int16) {
	ind := 127
	for i := int(m) - 1; i >= 0; i-- {
		if i >= int(m)-2 {
			ind = 127
		}
		for isfAcosTable[ind] < isp[i] {
			ind--
		}
		lTmp := (int32(isp[i]-isfAcosTable[ind]) * int32(isfSlope[ind])) << 1 // vo_L_mult
		isf[i] = voRound(lTmp << 4)
		isf[i] = isf[i] + int16(ind<<7) // add1
	}
	isf[m-1] = isf[m-1] >> 1
}

const (
	cNB_COEF_DOWN = 15
	cDOWN_FAC     = 26215 // 4/5 in Q15
	cL_FRAME16k   = 320
)

// fir_down1: 1/5-resolution decimation filter (Q14), fir_down1[4][30].
var firDown1 = [4][30]int16{
	{-5, 24, -50, 54, 0, -128, 294, -408, 344, 0, -647, 1505, -2379, 3034, 13107, 3034, -2379, 1505, -647, 0, 344, -408, 294, -128, 0, 54, -50, 24, -5, 0},
	{-6, 19, -26, 0, 77, -188, 270, -233, 0, 434, -964, 1366, -1293, 0, 12254, 6575, -2746, 1030, 0, -507, 601, -441, 198, 0, -95, 99, -58, 18, 0, -1},
	{-3, 9, 0, -41, 111, -170, 153, 0, -295, 649, -888, 770, 0, -1997, 9894, 9894, -1997, 0, 770, -888, 649, -295, 0, 153, -170, 111, -41, 0, 9, -3},
	{-1, 0, 18, -58, 99, -95, 0, 198, -441, 601, -507, 0, 1030, -2746, 6575, 12254, 0, -1293, 1366, -964, 434, 0, -233, 270, -188, 77, 0, -26, 19, -6},
}

// downSamp resamples sig (with NB_COEF_DOWN history before index nbCoefDown) by
// 5/4 (16 kHz -> 12.8 kHz). signal is the full scratch buffer.
func downSamp(signal []int16, sigD []int16, lFrameD int16) {
	pos := 0
	for j := 0; j < int(lFrameD); j++ {
		i := pos >> 2
		frac := pos & 3
		x := i + 1 // signal-relative base (matches sig+i-NB_COEF_DOWN+1, sig=signal+15)
		var lSum int32
		for tcoef := 0; tcoef < 30; tcoef++ {
			lSum += int32(signal[x+tcoef]) * int32(firDown1[frac][tcoef]) // vo_mult32
		}
		lSum = lShl2(lSum, 2)
		sigD[j] = extract_h(L_add(lSum, 0x8000))
		pos += 5
	}
}

// Decim_12k8 downsamples lg samples of 16 kHz sig16k to 12.8 kHz sig12k8.
// mem has 2*NB_COEF_DOWN samples.
func Decim_12k8(sig16k []int16, lg int16, sig12k8 []int16, mem []int16) {
	signal := make([]int16, cL_FRAME16k+2*cNB_COEF_DOWN)
	copy(signal[:2*cNB_COEF_DOWN], mem[:2*cNB_COEF_DOWN])
	copy(signal[2*cNB_COEF_DOWN:2*cNB_COEF_DOWN+int(lg)], sig16k[:lg])
	lgDown := (int(lg) * cDOWN_FAC) >> 15
	downSamp(signal, sig12k8, int16(lgDown))
	copy(mem[:2*cNB_COEF_DOWN], signal[int(lg):int(lg)+2*cNB_COEF_DOWN])
}

// hp50 filter coefficients (Q12).
var hp50A = [3]int16{8192, 16211, -8021}
var hp50B = [3]int16{4053, -8106, 4053}

// HP50_12k8 high-pass filters the input at 12.8 kHz in place; mem[6].
func HP50_12k8(signal []int16, lg int16, mem []int16) {
	y2Hi, y2Lo := mem[0], mem[1]
	y1Hi, y1Lo := mem[2], mem[3]
	x0, x1 := mem[4], mem[5]
	var x2 int16
	for i := 0; i < int(lg); i++ {
		x2 = x1
		x1 = x0
		x0 = signal[i]
		lTmp := int32(8192)
		lTmp += int32(y1Lo) * int32(hp50A[1])
		lTmp += int32(y2Lo) * int32(hp50A[2])
		lTmp = L_shr(lTmp, 14)
		lTmp += (int32(y1Hi)*int32(hp50A[1]) + int32(y2Hi)*int32(hp50A[2]) +
			int32(x0+x2)*int32(hp50B[0]) + int32(x1)*int32(hp50B[1])) << 1
		lTmp = L_shl(lTmp, 2)
		y2Hi = y1Hi
		y2Lo = y1Lo
		y1Hi = int16(lTmp >> 16)
		y1Lo = int16((lTmp & 0xffff) >> 1)
		lTmp = L_shl(lTmp, 1)
		signal[i] = extract_h(L_add(lTmp, 0x8000))
	}
	mem[0], mem[1] = y2Hi, y2Lo
	mem[2], mem[3] = y1Hi, y1Lo
	mem[4], mem[5] = x0, x1
}
