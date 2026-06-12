package amrwb

// ISF quantization (encode side), ported verbatim from the Apache-2.0
// vo-amrwbenc reference (qpisf_2s.c), derived from 3GPP TS 26.190. Reuses the
// dico_* codebooks (tables_isf.go) and the decoder dequantizers Dpisf_2s_46b/
// 36b for the predictive reconstruction.

const (
	cN_SURV_MAX = 4

	cSIZE_BK1  = 256
	cSIZE_BK2  = 256
	cSIZE_BK21 = 64
	cSIZE_BK22 = 128
	cSIZE_BK23 = 128
	cSIZE_BK24 = 32
	cSIZE_BK25 = 32

	cSIZE_BK21_36b = 128
	cSIZE_BK22_36b = 128
	cSIZE_BK23_36b = 64
)

// vqStage1 finds the `surv` best first-stage codebook entries (VQ_stage1).
func vqStage1(x, dico []int16, dim, dicoSize int16, index []int16, surv int16) {
	var distMin [cN_SURV_MAX]int32
	for i := range distMin {
		distMin[i] = max32
	}
	for i := int16(0); i < cN_SURV_MAX; i++ {
		index[i] = i
	}
	for i := int16(0); i < dicoSize; i++ {
		var dist int32
		for j := int16(0); j < dim; j++ {
			temp := x[j] - dico[i*dim+j]
			dist += (int32(temp) * int32(temp)) << 1
		}
		for k := int16(0); k < surv; k++ {
			if dist < distMin[k] {
				for l := surv - 1; l > k; l-- {
					distMin[l] = distMin[l-1]
					index[l] = index[l-1]
				}
				distMin[k] = dist
				index[k] = i
				break
			}
		}
	}
}

// subVQ finds the nearest codebook entry, writes it back into x[], returns the
// index, and reports the quantization error (Sub_VQ).
func subVQ(x, dico []int16, dim, dicoSize int16, distance *int32) int16 {
	distMin := max32
	index := int16(0)
	for i := int16(0); i < dicoSize; i++ {
		var dist int32
		for j := int16(0); j < dim; j++ {
			temp := x[j] - dico[i*dim+j]
			dist += (int32(temp) * int32(temp)) << 1
		}
		if dist < distMin {
			distMin = dist
			index = i
		}
	}
	*distance = distMin
	for j := int16(0); j < dim; j++ {
		x[j] = dico[index*dim+j]
	}
	return index
}

// Qpisf_2s_46b quantizes the ISF vector (46-bit, modes 12k+), writing 7 indices
// and the reconstructed isf_q.
func Qpisf_2s_46b(isf1, isfQ, pastIsfq, indice []int16, nbSurv int16) {
	var tmpInd [5]int16
	var surv1 [cN_SURV_MAX]int16
	isf := make([]int16, cORDER)
	stage2 := make([]int16, cORDER)
	var temp, minErr, distance int32

	for i := 0; i < cORDER; i++ {
		isf[i] = isf1[i] - mean_isf[i]
		isf[i] = isf[i] - int16((int32(cISF_MU)*int32(pastIsfq[i]))>>15)
	}

	vqStage1(isf[0:], dico1_isf, 9, cSIZE_BK1, surv1[:], nbSurv)
	distance = max32
	for k := int16(0); k < nbSurv; k++ {
		for i := 0; i < 9; i++ {
			stage2[i] = isf[i] - dico1_isf[i+int(surv1[k])*9]
		}
		tmpInd[0] = subVQ(stage2[0:], dico21_isf, 3, cSIZE_BK21, &minErr)
		temp = minErr
		tmpInd[1] = subVQ(stage2[3:], dico22_isf, 3, cSIZE_BK22, &minErr)
		temp += minErr
		tmpInd[2] = subVQ(stage2[6:], dico23_isf, 3, cSIZE_BK23, &minErr)
		temp += minErr
		if temp < distance {
			distance = temp
			indice[0] = surv1[k]
			for i := 0; i < 3; i++ {
				indice[i+2] = tmpInd[i]
			}
		}
	}

	vqStage1(isf[9:], dico2_isf, 7, cSIZE_BK2, surv1[:], nbSurv)
	distance = max32
	for k := int16(0); k < nbSurv; k++ {
		for i := 0; i < 7; i++ {
			stage2[i] = isf[9+i] - dico2_isf[i+int(surv1[k])*7]
		}
		tmpInd[0] = subVQ(stage2[0:], dico24_isf, 3, cSIZE_BK24, &minErr)
		temp = minErr
		tmpInd[1] = subVQ(stage2[3:], dico25_isf, 4, cSIZE_BK25, &minErr)
		temp += minErr
		if temp < distance {
			distance = temp
			indice[1] = surv1[k]
			for i := 0; i < 2; i++ {
				indice[i+5] = tmpInd[i]
			}
		}
	}

	Dpisf_2s_46b(indice, isfQ, pastIsfq, isfQ, isfQ, 0, 0)
}

// Qpisf_2s_36b quantizes the ISF vector (36-bit, modes 6.6k/8.85k).
func Qpisf_2s_36b(isf1, isfQ, pastIsfq, indice []int16, nbSurv int16) {
	var tmpInd [5]int16
	var surv1 [cN_SURV_MAX]int16
	isf := make([]int16, cORDER)
	stage2 := make([]int16, cORDER)
	var temp, minErr, distance int32

	for i := 0; i < cORDER; i++ {
		isf[i] = isf1[i] - mean_isf[i]
		isf[i] = isf[i] - int16((int32(cISF_MU)*int32(pastIsfq[i]))>>15)
	}

	vqStage1(isf[0:], dico1_isf, 9, cSIZE_BK1, surv1[:], nbSurv)
	distance = max32
	for k := int16(0); k < nbSurv; k++ {
		for i := 0; i < 9; i++ {
			stage2[i] = isf[i] - dico1_isf[i+int(surv1[k])*9]
		}
		tmpInd[0] = subVQ(stage2[0:], dico21_isf_36b, 5, cSIZE_BK21_36b, &minErr)
		temp = minErr
		tmpInd[1] = subVQ(stage2[5:], dico22_isf_36b, 4, cSIZE_BK22_36b, &minErr)
		temp += minErr
		if temp < distance {
			distance = temp
			indice[0] = surv1[k]
			for i := 0; i < 2; i++ {
				indice[i+2] = tmpInd[i]
			}
		}
	}

	vqStage1(isf[9:], dico2_isf, 7, cSIZE_BK2, surv1[:], nbSurv)
	distance = max32
	for k := int16(0); k < nbSurv; k++ {
		for i := 0; i < 7; i++ {
			stage2[i] = isf[9+i] - dico2_isf[i+int(surv1[k])*7]
		}
		tmpInd[0] = subVQ(stage2[0:], dico23_isf_36b, 7, cSIZE_BK23_36b, &minErr)
		temp = minErr
		if temp < distance {
			distance = temp
			indice[1] = surv1[k]
			indice[4] = tmpInd[0]
		}
	}

	Dpisf_2s_36b(indice, isfQ, pastIsfq, isfQ, isfQ, 0, 0)
}
