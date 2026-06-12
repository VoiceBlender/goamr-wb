package amrwb

// Post-processing/concealment kernels, ported verbatim from the Apache-2.0
// opencore-amrwb reference (phase_dispersion.cpp, isf_extrapolation.cpp,
// lagconceal.cpp), derived from 3GPP TS 26.173.

const (
	cINV_LENGTH      = 2731  // 1/12
	cPITCH_0_9       = 14746 // 0.9 in Q14
	cPITCH_0_6       = 9830  // 0.6 in Q14
	cONE_PER_3       = 10923 // 1/3 in Q15
	cONE_PER_LTPHIST = 6554  // 1/5 in Q15
)

// initLagconc initializes the pitch-lag history (Init_Lagconc).
func initLagconc(lagHist []int16) {
	for i := 0; i < cL_LTPHIST; i++ {
		lagHist[i] = 64
	}
}

// lagconceal reconstructs the pitch lag T0 for an erased frame from the lag and
// gain history (lagconceal.cpp). gainHist and lagHist are 5-element histories.
func lagconceal(gainHist, lagHist []int16, T0, oldT0, seed *int16, unusableFrame int16) {
	var lagHist2 [cL_LTPHIST]int16
	var meanLag int16

	lastGain := gainHist[4]
	secLastGain := gainHist[3]
	lastLag := lagHist[0]

	minLag := lagHist[0]
	maxLag := lagHist[0]
	for i := 1; i < cL_LTPHIST; i++ {
		if lagHist[i] < minLag {
			minLag = lagHist[i]
		}
		if lagHist[i] > maxLag {
			maxLag = lagHist[i]
		}
	}
	minGain := gainHist[0]
	for i := 1; i < cL_LTPHIST; i++ {
		if gainHist[i] < minGain {
			minGain = gainHist[i]
		}
	}
	lagDif := sub_int16(maxLag, minLag)

	if unusableFrame != 0 {
		if minGain > 8192 && lagDif < 10 {
			*T0 = *oldT0
		} else if lastGain > 8192 && secLastGain > 8192 {
			*T0 = lagHist[0]
		} else {
			copy(lagHist2[:], lagHist[:cL_LTPHIST])
			insertionSort(lagHist2[:], 5)
			lagDif = sub_int16(lagHist2[4], lagHist2[2])
			if lagDif > 40 {
				lagDif = 40
			}
			d := noise_gen_amrwb(seed)
			tmp := lagDif >> 1
			d2 := mult_int16(tmp, d)
			tmp = add_int16(add_int16(lagHist2[2], lagHist2[3]), lagHist2[4])
			*T0 = add_int16(mult_int16(tmp, cONE_PER_3), d2)
		}
		if *T0 > maxLag {
			*T0 = maxLag
		}
		if *T0 < minLag {
			*T0 = minLag
		}
		return
	}

	meanLag = 0
	for i := 0; i < cL_LTPHIST; i++ {
		meanLag = add_int16(meanLag, lagHist[i])
	}
	meanLag = mult_int16(meanLag, cONE_PER_LTPHIST)

	tmp := *T0 - maxLag
	tmp2 := *T0 - lastLag

	switch {
	case lagDif < 10 && *T0 > minLag-5 && tmp < 5:
	case lastGain > 8192 && secLastGain > 8192 && (tmp2+10) > 0 && tmp2 < 10:
	case minGain < 6554 && lastGain == minGain && *T0 > minLag && *T0 < maxLag:
	case lagDif < 70 && *T0 > minLag && *T0 < maxLag:
	case *T0 > meanLag && *T0 < maxLag:
	default:
		if (minGain > 8192) && (lagDif < 10) {
			*T0 = lagHist[0]
		} else if lastGain > 8192 && secLastGain > 8192 {
			*T0 = lagHist[0]
		} else {
			copy(lagHist2[:], lagHist[:cL_LTPHIST])
			insertionSort(lagHist2[:], 5)
			lagDif = sub_int16(lagHist2[4], lagHist2[2])
			if lagDif > 40 {
				lagDif = 40
			}
			d := noise_gen_amrwb(seed)
			t := lagDif >> 1
			d2 := mult_int16(t, d)
			t = add_int16(add_int16(lagHist2[2], lagHist2[3]), lagHist2[4])
			*T0 = add_int16(mult_int16(t, cONE_PER_3), d2)
		}
		if *T0 > maxLag {
			*T0 = maxLag
		}
		if *T0 < minLag {
			*T0 = minLag
		}
	}
}

func insertionSort(arr []int16, n int16) {
	for i := int16(0); i < n; i++ {
		insert(arr, i, arr[i])
	}
}

func insert(arr []int16, n, x int16) {
	var i int16
	for i = n - 1; i >= 0; i-- {
		if x < arr[i] {
			arr[i+1] = arr[i]
		} else {
			break
		}
	}
	arr[i+1] = x
}

// ph_imp_low: 0..6.4 kHz high-dispersion impulse response.
var phImpLow = [cL_SUBFR]int16{
	20182, 9693, 3270, -3437, 2864, -5240, 1589, -1357,
	600, 3893, -1497, -698, 1203, -5249, 1199, 5371,
	-1488, -705, -2887, 1976, 898, 721, -3876, 4227,
	-5112, 6400, -1032, -4725, 4093, -4352, 3205, 2130,
	-1996, -1835, 2648, -1786, -406, 573, 2484, -3608,
	3139, -1363, -2566, 3808, -639, -2051, -541, 2376,
	3932, -6262, 1432, -3601, 4889, 370, 567, -1163,
	-2854, 1914, 39, -2418, 3454, 2975, -4021, 3431,
}

// ph_imp_mid: 3.2..6.4 kHz low-dispersion impulse response.
var phImpMid = [cL_SUBFR]int16{
	24098, 10460, -5263, -763, 2048, -927, 1753, -3323,
	2212, 652, -2146, 2487, -3539, 4109, -2107, -374,
	-626, 4270, -5485, 2235, 1858, -2769, 744, 1140,
	-763, -1615, 4060, -4574, 2982, -1163, 731, -1098,
	803, 167, -714, 606, -560, 639, 43, -1766,
	3228, -2782, 665, 763, 233, -2002, 1291, 1871,
	-3470, 1032, 2710, -4040, 3624, -4214, 5292, -4270,
	1563, 108, -580, 1642, -2458, 957, 544, 2540,
}

// phase_dispersion applies pitch-adaptive phase dispersion to the code vector.
// dispMem is 8 words: [0]=prev_state, [1]=prev_gain_code, [2..7]=prev_gain_pit.
func phase_dispersion(gainCode, gainPit int16, code []int16, mode int16, dispMem []int16) {
	code2 := make([]int16, 2*cL_SUBFR)
	prevState := &dispMem[0]
	prevGainCode := &dispMem[1]
	prevGainPit := dispMem[2:8]

	var state int16
	if gainPit < cPITCH_0_6 {
		state = 0
	} else if gainPit < cPITCH_0_9 {
		state = 1
	} else {
		state = 2
	}

	for i := 5; i > 0; i-- {
		prevGainPit[i] = prevGainPit[i-1]
	}
	prevGainPit[0] = gainPit

	if sub_int16(gainCode, *prevGainCode) > shl_int16(*prevGainCode, 1) {
		if state < 2 {
			state++
		}
	} else {
		j := int16(0)
		for i := 0; i < 6; i++ {
			if prevGainPit[i] < cPITCH_0_6 {
				j++
			}
		}
		if j > 2 {
			state = 0
		}
		if state > *prevState+1 {
			state--
		}
	}

	*prevGainCode = gainCode
	*prevState = state

	state += mode
	if state == 0 {
		for i := 0; i < cL_SUBFR; i++ {
			if code[i] != 0 {
				for j := 0; j < cL_SUBFR; j++ {
					code2[i+j] = add_int16(code2[i+j], mult_int16_r(code[i], phImpLow[j]))
				}
			}
		}
	} else if state == 1 {
		for i := 0; i < cL_SUBFR; i++ {
			if code[i] != 0 {
				for j := 0; j < cL_SUBFR; j++ {
					code2[i+j] = add_int16(code2[i+j], mult_int16_r(code[i], phImpMid[j]))
				}
			}
		}
	}
	if state < 2 {
		for i := 0; i < cL_SUBFR; i++ {
			code[i] = add_int16(code2[i], code2[i+cL_SUBFR])
		}
	}
}

// isf_extrapolation extrapolates the 16 narrowband ISFs to the M16k HF set.
func isf_extrapolation(hfIsf []int16) {
	var isfDiff [cM - 2]int16
	var isfCorr [3]int32
	var lTmp int32

	hfIsf[cM16k-1] = hfIsf[cM-1]

	for i := 1; i < cM-1; i++ {
		isfDiff[i-1] = sub_int16(hfIsf[i], hfIsf[i-1])
	}
	lTmp = 0
	for i := 3; i < cM-1; i++ {
		lTmp = mac_16by16_to_int32(lTmp, isfDiff[i-1], cINV_LENGTH)
	}
	mean := amr_wb_round(lTmp)

	isfCorr[0] = 0
	tmp := int16(0)
	for i := 0; i < cM-2; i++ {
		if isfDiff[i] > tmp {
			tmp = isfDiff[i]
		}
	}
	exp := norm_s(tmp)
	for i := 0; i < cM-2; i++ {
		isfDiff[i] = shl_int16(isfDiff[i], exp)
	}
	mean = shl_int16(mean, exp)

	for corr := 0; corr < 3; corr++ {
		lag := int16(corr + 2)
		isfCorr[corr] = 0
		for i := 7; i < cM-2; i++ {
			tmp2 := sub_int16(isfDiff[i], mean)
			tmp3 := sub_int16(isfDiff[i-int(lag)], mean)
			l := mul_16by16_to_int32(tmp2, tmp3)
			var hi, lo int16
			int32ToDpf(l, &hi, &lo)
			l = mpyDpf32(hi, lo, hi, lo)
			isfCorr[corr] = add_int32(isfCorr[corr], l)
		}
	}

	var maxCorr int16
	if isfCorr[0] > isfCorr[1] {
		maxCorr = 0
	} else {
		maxCorr = 1
	}
	if isfCorr[2] > isfCorr[maxCorr] {
		maxCorr = 2
	}
	maxCorr++

	for i := cM - 1; i < cM16k-1; i++ {
		tmp = sub_int16(hfIsf[i-1-int(maxCorr)], hfIsf[i-2-int(maxCorr)])
		hfIsf[i] = add_int16(hfIsf[i-1], tmp)
	}

	tmp = add_int16(hfIsf[4], hfIsf[3])
	tmp = sub_int16(hfIsf[2], tmp)
	tmp = mult_int16(tmp, 5461)
	tmp += 20390
	if tmp > 19456 {
		tmp = 19456
	}
	tmp = sub_int16(tmp, hfIsf[cM-2])
	tmp2 := sub_int16(hfIsf[cM16k-2], hfIsf[cM-2])

	exp2 := norm_s(tmp2)
	exp = norm_s(tmp)
	exp--
	tmp <<= uint(exp)
	tmp2 <<= uint(exp2)
	coeff := div_16by16(tmp, tmp2)
	exp = exp2 - exp

	for i := cM - 1; i < cM16k-1; i++ {
		tmp = mult_int16(sub_int16(hfIsf[i], hfIsf[i-1]), coeff)
		isfDiff[i-(cM-1)] = shl_int16(tmp, exp)
	}
	for i := cM; i < cM16k-1; i++ {
		t := isfDiff[i-(cM-1)] + isfDiff[i-cM] - 1280
		if t < 0 {
			if isfDiff[i-(cM-1)] > isfDiff[i-cM] {
				isfDiff[i-cM] = 1280 - isfDiff[i-(cM-1)]
			} else {
				isfDiff[i-(cM-1)] = 1280 - isfDiff[i-cM]
			}
		}
	}
	for i := cM - 1; i < cM16k-1; i++ {
		hfIsf[i] = add_int16(hfIsf[i-1], isfDiff[i-(cM-1)])
	}
	for i := 0; i < cM16k-1; i++ {
		hfIsf[i] = mult_int16(hfIsf[i], 26214)
	}
	Isf_isp(hfIsf, hfIsf, cM16k)
}
