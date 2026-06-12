package amrwb

// ISF parameter dequantization, ported verbatim from the Apache-2.0
// opencore-amrwb reference (qpisf_2s.cpp, qisf_ns.cpp), derived from 3GPP
// TS 26.173. Reconstructs the immittance spectral frequencies (ISF) from the
// transmitted codebook indices, then enforces ordering/minimum spacing.

// Dpisf_2s_46b dequantizes the 46-bit ISF representation (modes 12k and up).
func Dpisf_2s_46b(indice, isfQ, pastIsfq, isfold, isfBuf []int16, bfi, encDec int16) {
	var refIsf [cM]int16
	var i, j, tmp int16

	if bfi == 0 { // good frame
		for i = 0; i < 9; i++ {
			isfQ[i] = dico1_isf[(indice[0]<<3)+indice[0]+i]
		}
		for i = 0; i < 7; i++ {
			isfQ[i+9] = dico2_isf[(indice[1]<<3)-indice[1]+i]
		}
		for i = 0; i < 3; i++ {
			isfQ[i] += dico21_isf[indice[2]*3+i]
			isfQ[i+3] += dico22_isf[indice[3]*3+i]
			isfQ[i+6] += dico23_isf[indice[4]*3+i]
			isfQ[i+9] += dico24_isf[indice[5]*3+i]
			isfQ[i+12] += dico25_isf[(indice[6]<<2)+i]
		}
		isfQ[i+12] += dico25_isf[(indice[6]<<2)+i]

		for i = 0; i < cORDER; i++ {
			tmp = isfQ[i]
			isfQ[i] += mean_isf[i]
			isfQ[i] += int16((int32(cISF_MU) * int32(pastIsfq[i])) >> 15)
			pastIsfq[i] = tmp
		}

		if encDec != 0 {
			for i = 0; i < cM; i++ {
				for j = cL_MEANBUF - 1; j > 0; j-- {
					isfBuf[j*cM+i] = isfBuf[(j-1)*cM+i]
				}
				isfBuf[i] = isfQ[i]
			}
		}
	} else { // bad frame
		isfBadFrame(isfQ, pastIsfq, isfold, isfBuf, refIsf[:])
	}

	Reorder_isf(isfQ, cISF_GAP, cORDER)
}

// Dpisf_2s_36b dequantizes the 36-bit ISF representation (modes 7k, 9k).
func Dpisf_2s_36b(indice, isfQ, pastIsfq, isfold, isfBuf []int16, bfi, encDec int16) {
	var refIsf [cM]int16
	var i, j, tmp int16

	if bfi == 0 { // good frame
		for i = 0; i < 9; i++ {
			isfQ[i] = dico1_isf[indice[0]*9+i]
		}
		for i = 0; i < 7; i++ {
			isfQ[i+9] = add_int16(dico2_isf[indice[1]*7+i], dico23_isf_36b[indice[4]*7+i])
		}
		for i = 0; i < 5; i++ {
			isfQ[i] = add_int16(isfQ[i], dico21_isf_36b[indice[2]*5+i])
		}
		for i = 0; i < 4; i++ {
			isfQ[i+5] = add_int16(isfQ[i+5], dico22_isf_36b[(indice[3]<<2)+i])
		}

		for i = 0; i < cORDER; i++ {
			tmp = isfQ[i]
			isfQ[i] = add_int16(tmp, mean_isf[i])
			isfQ[i] = add_int16(isfQ[i], mult_int16(cISF_MU, pastIsfq[i]))
			pastIsfq[i] = tmp
		}

		if encDec != 0 {
			for i = 0; i < cM; i++ {
				for j = cL_MEANBUF - 1; j > 0; j-- {
					isfBuf[j*cM+i] = isfBuf[(j-1)*cM+i]
				}
				isfBuf[i] = isfQ[i]
			}
		}
	} else { // bad frame
		isfBadFrame(isfQ, pastIsfq, isfold, isfBuf, refIsf[:])
	}

	Reorder_isf(isfQ, cISF_GAP, cORDER)
}

// isfBadFrame reconstructs ISF for a lost frame, shared by the 36b/46b paths.
func isfBadFrame(isfQ, pastIsfq, isfold, isfBuf, refIsf []int16) {
	var i, j, tmp int16
	for i = 0; i < cM; i++ {
		lTmp := mul_16by16_to_int32(mean_isf[i], 8192)
		for j = 0; j < cL_MEANBUF; j++ {
			lTmp = mac_16by16_to_int32(lTmp, isfBuf[j*cM+i], 8192)
		}
		refIsf[i] = amr_wb_round(lTmp)
	}
	for i = 0; i < cORDER; i++ {
		isfQ[i] = add_int16(mult_int16(cISF_ALPHA, isfold[i]), mult_int16(cISF_ONE_ALPHA, refIsf[i]))
	}
	for i = 0; i < cORDER; i++ {
		tmp = add_int16(refIsf[i], mult_int16(pastIsfq[i], cISF_MU))
		pastIsfq[i] = sub_int16(isfQ[i], tmp)
		pastIsfq[i] >>= 1
	}
}

// Reorder_isf enforces a minimum distance between consecutive ISFs.
func Reorder_isf(isf []int16, minDist, n int16) {
	isfMin := minDist
	for i := int16(0); i < n-1; i++ {
		if isf[i] < isfMin {
			isf[i] = isfMin
		}
		isfMin = add_int16(isf[i], minDist)
	}
}

// Disf_ns dequantizes the ISF parameters for the SID (comfort-noise) frame.
func Disf_ns(indice, isfQ []int16) {
	isfQ[0] = dico1_isf_noise[indice[0]<<1]
	isfQ[1] = dico1_isf_noise[(indice[0]<<1)+1]

	for i := int16(0); i < 3; i++ {
		isfQ[i+2] = dico2_isf_noise[(indice[1]<<1)+indice[1]+i]
		isfQ[i+5] = dico3_isf_noise[(indice[2]<<1)+indice[2]+i]
	}
	for i := int16(0); i < 4; i++ {
		isfQ[i+8] = dico4_isf_noise[(indice[3]<<2)+i]
		isfQ[i+12] = dico5_isf_noise[(indice[4]<<2)+i]
	}
	for i := int16(0); i < cORDER; i++ {
		isfQ[i] = add_int16(isfQ[i], mean_isf_noise[i])
	}
	Reorder_isf(isfQ, cISF_GAP, cORDER)
}
