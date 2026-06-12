package amrwb

// Algebraic-codebook pulse-index encoding, ported verbatim from the Apache-2.0
// vo-amrwbenc reference (q_pulse.c), derived from 3GPP TS 26.190. These encode
// pulse positions/signs into the transmitted indices (inverse of the decoder's
// dec_*p_* functions). NB_POS = 16 here (the sign-bit mask).

func quant1pN1(pos, N int16) int32 {
	mask := int16((1 << uint(N)) - 1)
	index := L_deposit_l(pos & mask)
	if pos&cNB_POS != 0 {
		index += int32(1) << uint(N)
	}
	return index
}

func quant2p2N1(pos1, pos2, N int16) int32 {
	mask := int16((1 << uint(N)) - 1)
	var index int32
	if ((pos2 ^ pos1) & cNB_POS) == 0 {
		if pos1 <= pos2 {
			index = L_deposit_l(((pos1 & mask) << uint(N)) + (pos2 & mask))
		} else {
			index = L_deposit_l(((pos2 & mask) << uint(N)) + (pos1 & mask))
		}
		if pos1&cNB_POS != 0 {
			index += int32(1) << uint(N<<1)
		}
	} else {
		if (pos1&mask)-(pos2&mask) <= 0 {
			index = L_deposit_l(((pos2 & mask) << uint(N)) + (pos1 & mask))
			if pos2&cNB_POS != 0 {
				index += int32(1) << uint(N<<1)
			}
		} else {
			index = L_deposit_l(((pos1 & mask) << uint(N)) + (pos2 & mask))
			if pos1&cNB_POS != 0 {
				index += int32(1) << uint(N<<1)
			}
		}
	}
	return index
}

func quant3p3N1(pos1, pos2, pos3, N int16) int32 {
	nbPos := int16(1 << uint(N-1))
	var index int32
	if ((pos1 ^ pos2) & nbPos) == 0 {
		index = quant2p2N1(pos1, pos2, N-1)
		index += L_deposit_l(pos1&nbPos) << uint(N)
		index += quant1pN1(pos3, N) << uint(N<<1)
	} else if ((pos1 ^ pos3) & nbPos) == 0 {
		index = quant2p2N1(pos1, pos3, N-1)
		index += L_deposit_l(pos1&nbPos) << uint(N)
		index += quant1pN1(pos2, N) << uint(N<<1)
	} else {
		index = quant2p2N1(pos2, pos3, N-1)
		index += L_deposit_l(pos2&nbPos) << uint(N)
		index += quant1pN1(pos1, N) << uint(N<<1)
	}
	return index
}

func quant4p4N1(pos1, pos2, pos3, pos4, N int16) int32 {
	nbPos := int16(1 << uint(N-1))
	var index int32
	if ((pos1 ^ pos2) & nbPos) == 0 {
		index = quant2p2N1(pos1, pos2, N-1)
		index += L_deposit_l(pos1&nbPos) << uint(N)
		index += quant2p2N1(pos3, pos4, N) << uint(N<<1)
	} else if ((pos1 ^ pos3) & nbPos) == 0 {
		index = quant2p2N1(pos1, pos3, N-1)
		index += L_deposit_l(pos1&nbPos) << uint(N)
		index += quant2p2N1(pos2, pos4, N) << uint(N<<1)
	} else {
		index = quant2p2N1(pos2, pos3, N-1)
		index += L_deposit_l(pos2&nbPos) << uint(N)
		index += quant2p2N1(pos1, pos4, N) << uint(N<<1)
	}
	return index
}

func quant4p4N(pos []int16, N int16) int32 {
	n1 := N - 1
	nbPos := int16(1 << uint(n1))
	var posA, posB [4]int16
	i, j := int16(0), int16(0)
	for k := 0; k < 4; k++ {
		if pos[k]&nbPos == 0 {
			posA[i] = pos[k]
			i++
		} else {
			posB[j] = pos[k]
			j++
		}
	}
	var index int32
	switch i {
	case 0:
		index = int32(1) << uint((N<<2)-3)
		index += quant4p4N1(posB[0], posB[1], posB[2], posB[3], n1)
	case 1:
		index = L_shl(quant1pN1(posA[0], n1), 3*n1+1)
		index += quant3p3N1(posB[0], posB[1], posB[2], n1)
	case 2:
		index = L_shl(quant2p2N1(posA[0], posA[1], n1), (n1<<1)+1)
		index += quant2p2N1(posB[0], posB[1], n1)
	case 3:
		index = L_shl(quant3p3N1(posA[0], posA[1], posA[2], n1), N)
		index += quant1pN1(posB[0], n1)
	case 4:
		index = quant4p4N1(posA[0], posA[1], posA[2], posA[3], n1)
	}
	index += L_shl(int32(i)&3, (N<<2)-2)
	return index
}

func quant5p5N(pos []int16, N int16) int32 {
	n1 := N - 1
	nbPos := int16(1 << uint(n1))
	var posA, posB [5]int16
	i, j := int16(0), int16(0)
	for k := 0; k < 5; k++ {
		if pos[k]&nbPos == 0 {
			posA[i] = pos[k]
			i++
		} else {
			posB[j] = pos[k]
			j++
		}
	}
	var index int32
	switch i {
	case 0:
		index = L_shl(1, 5*N-1)
		index += L_shl(quant3p3N1(posB[0], posB[1], posB[2], n1), (N<<1)+1)
		index += quant2p2N1(posB[3], posB[4], N)
	case 1:
		index = L_shl(1, 5*N-1)
		index += L_shl(quant3p3N1(posB[0], posB[1], posB[2], n1), (N<<1)+1)
		index += quant2p2N1(posB[3], posA[0], N)
	case 2:
		index = L_shl(1, 5*N-1)
		index += L_shl(quant3p3N1(posB[0], posB[1], posB[2], n1), (N<<1)+1)
		index += quant2p2N1(posA[0], posA[1], N)
	case 3:
		index = L_shl(quant3p3N1(posA[0], posA[1], posA[2], n1), (N<<1)+1)
		index += quant2p2N1(posB[0], posB[1], N)
	case 4:
		index = L_shl(quant3p3N1(posA[0], posA[1], posA[2], n1), (N<<1)+1)
		index += quant2p2N1(posA[3], posB[0], N)
	case 5:
		index = L_shl(quant3p3N1(posA[0], posA[1], posA[2], n1), (N<<1)+1)
		index += quant2p2N1(posA[3], posA[4], N)
	}
	return index
}

func quant6p6N2(pos []int16, N int16) int32 {
	n1 := N - 1
	nbPos := int16(1 << uint(n1))
	var posA, posB [6]int16
	i, j := int16(0), int16(0)
	for k := 0; k < 6; k++ {
		if pos[k]&nbPos == 0 {
			posA[i] = pos[k]
			i++
		} else {
			posB[j] = pos[k]
			j++
		}
	}
	var index int32
	switch i {
	case 0:
		index = int32(1) << uint(6*N-5)
		index += quant5p5N(posB[:], n1) << uint(N)
		index += quant1pN1(posB[5], n1)
	case 1:
		index = int32(1) << uint(6*N-5)
		index += quant5p5N(posB[:], n1) << uint(N)
		index += quant1pN1(posA[0], n1)
	case 2:
		index = int32(1) << uint(6*N-5)
		index += quant4p4N(posB[:], n1) << uint(2*n1+1)
		index += quant2p2N1(posA[0], posA[1], n1)
	case 3:
		index = quant3p3N1(posA[0], posA[1], posA[2], n1) << uint(3*n1+1)
		index += quant3p3N1(posB[0], posB[1], posB[2], n1)
	case 4:
		i = 2
		index = quant4p4N(posA[:], n1) << uint(2*n1+1)
		index += quant2p2N1(posB[0], posB[1], n1)
	case 5:
		i = 1
		index = quant5p5N(posA[:], n1) << uint(N)
		index += quant1pN1(posB[0], n1)
	case 6:
		i = 0
		index = quant5p5N(posA[:], n1) << uint(N)
		index += quant1pN1(posA[5], n1)
	}
	index += (int32(i) & 3) << uint(6*N-4)
	return index
}
