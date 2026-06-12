package amrwb

// Encoder synthesis-side filters, ported verbatim from the Apache-2.0
// vo-amrwbenc reference (deemph.c, hp400.c, hp6k.c), derived from 3GPP TS
// 26.190. These use the encoder's fixed-point formulations (distinct from the
// decoder equivalents) and are kept separate for bit-exactness.

// Deemph_32 applies 1/(1 - mu z^-1) to the 32-bit (hi/lo) synthesis (Deemph_32).
func Deemph_32(xHi, xLo, y []int16, mu, L int16, mem *int16) {
	fac := mu >> 1 // Q15 -> Q14

	lTmp := L_deposit_h(xHi[0])
	lTmp += (int32(xLo[0]) * 8) << 1
	lTmp <<= 3
	lTmp += (int32(*mem) * int32(fac)) << 1
	lTmp <<= 1
	y[0] = int16((lTmp + 0x8000) >> 16)

	for i := int16(1); i < L; i++ {
		lTmp = L_deposit_h(xHi[i])
		lTmp += (int32(xLo[i]) * 8) << 1
		lTmp <<= 3
		lTmp += (int32(y[i-1]) * int32(fac)) << 1
		lTmp <<= 1
		y[i] = int16((lTmp + 0x8000) >> 16)
	}
	*mem = y[L-1]
}

var hp400A = [3]int16{16384, 29280, -14160}
var hp400B = [3]int16{915, -1830, 915}

// HP400_12k8 high-pass filters the synthesis at 400 Hz (output /16); mem[6].
func HP400_12k8(signal []int16, lg int16, mem []int16) {
	y2Hi, y2Lo := mem[0], mem[1]
	y1Hi, y1Lo := mem[2], mem[3]
	x0, x1 := mem[4], mem[5]
	var x2 int16
	for i := 0; i < int(lg); i++ {
		x2 = x1
		x1 = x0
		x0 = signal[i]
		lTmp := int32(8192)
		lTmp += int32(y1Lo) * int32(hp400A[1])
		lTmp += int32(y2Lo) * int32(hp400A[2])
		lTmp >>= 14
		lTmp += (int32(y1Hi)*int32(hp400A[1]) + int32(y2Hi)*int32(hp400A[2]) +
			int32(x0+x2)*int32(hp400B[0]) + int32(x1)*int32(hp400B[1])) << 1
		lTmp <<= 1
		y2Hi = y1Hi
		y2Lo = y1Lo
		y1Hi = int16(lTmp >> 16)
		y1Lo = int16((lTmp & 0xffff) >> 1)
		signal[i] = int16((lTmp + 0x8000) >> 16)
	}
	mem[0], mem[1] = y2Hi, y2Lo
	mem[2], mem[3] = y1Hi, y1Lo
	mem[4], mem[5] = x0, x1
}

const cHP6K_L_FIR = 31

// fir_6k_7k (encoder): symmetric 6-7 kHz band-pass FIR, 31 taps.
var firEnc6k7k = [cHP6K_L_FIR]int16{
	-32, 47, 32, -27, -369,
	1122, -1421, 0, 3798, -8880,
	12349, -10984, 3548, 7766, -18001,
	22118, -18001, 7766, 3548, -10984,
	12349, -8880, 3798, 0, -1421,
	1122, -369, -27, 32, 47,
	-32,
}

// Filt_6k_7k band-pass filters the HF signal (gain 4); mem size L_FIR-1=30.
func Filt_6k_7k(signal []int16, lg int16, mem []int16) {
	var xArr [cL_SUBFR16k + (cHP6K_L_FIR - 1)]int16
	x := xArr[:]
	copy(x[:cHP6K_L_FIR-1], mem[:cHP6K_L_FIR-1])
	for i := int(lg) - 1; i >= 0; i-- {
		x[i+cHP6K_L_FIR-1] = signal[i] >> 2 // gain 4
	}
	for i := 0; i < int(lg); i++ {
		var lTmp int32
		for k := 0; k < 15; k++ {
			lTmp += int32(x[i+k]+x[i+30-k]) * int32(firEnc6k7k[k])
		}
		lTmp += int32(x[i+15]) * int32(firEnc6k7k[15])
		signal[i] = int16((lTmp + 0x4000) >> 15)
	}
	copy(mem[:cHP6K_L_FIR-1], x[int(lg):int(lg)+cHP6K_L_FIR-1])
}
