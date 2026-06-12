package amrwb

// Bitstream de-indexing and decoder init tables, ported verbatim from the
// Apache-2.0 opencore-amrwb reference (get_amr_wb_bits.cpp,
// pvamrwbdecoder_api.h, pvamrwbdecoder.cpp), derived from 3GPP TS 26.173.
//
// The decoder consumes its parameters as one int16 per bit, each holding
// BIT_0 (-127) or BIT_1 (+127); Serial_parm reassembles multi-bit fields.

const (
	cBIT_0 = -127
	cBIT_1 = 127

	cAMR_WB_PCM_FRAME = 320 // output samples at 16 kHz
	cNUM_OF_MODES     = 10

	cNBBITS_7k  = 132
	cNBBITS_9k  = 177
	cNBBITS_12k = 253
	cNBBITS_14k = 285
	cNBBITS_16k = 317
	cNBBITS_18k = 365
	cNBBITS_20k = 397
	cNBBITS_23k = 461
	cNBBITS_24k = 477
	cNBBITS_SID = 35
)

// amrwbCompressed maps internal mode index (0..9) to its bit count.
var amrwbCompressed = [cNUM_OF_MODES]int16{
	cNBBITS_7k, cNBBITS_9k, cNBBITS_12k, cNBBITS_14k, cNBBITS_16k,
	cNBBITS_18k, cNBBITS_20k, cNBBITS_23k, cNBBITS_24k, cNBBITS_SID,
}

// interpol_frac: subframe interpolation fractions (Q15).
var interpolFrac = [cNB_SUBFR]int16{14746, 26214, 31457, 32767}

// isp_init / isf_init: decoder state initialization vectors.
var ispInit = [cM]int16{
	32138, 30274, 27246, 23170, 18205, 12540, 6393, 0,
	-6393, -12540, -18205, -23170, -27246, -30274, -32138, 1475,
}

var isfInit = [cM]int16{
	1024, 2048, 3072, 4096, 5120, 6144, 7168, 8192,
	9216, 10240, 11264, 12288, 13312, 14336, 15360, 3840,
}

// bitParser reads parameters from a per-bit int16 stream (BIT_0/BIT_1 values).
type bitParser struct {
	bits []int16
	pos  int
}

// next returns the next bit value (BIT_0 or BIT_1) and advances. Reading past
// the end yields BIT_0 so a short/malformed payload cannot panic the decoder.
func (p *bitParser) next() int16 {
	if p.pos >= len(p.bits) {
		p.pos++
		return cBIT_0
	}
	v := p.bits[p.pos]
	p.pos++
	return v
}

// parm reads an unsigned field of noOfBits, MSB-first (Serial_parm).
func (p *bitParser) parm(noOfBits int16) int16 {
	var value int16
	for i := noOfBits >> 1; i != 0; i-- {
		value <<= 2
		if p.next() == cBIT_1 {
			value |= 2
		}
		if p.next() == cBIT_1 {
			value |= 1
		}
	}
	if noOfBits&1 != 0 {
		value <<= 1
		if p.next() == cBIT_1 {
			value |= 1
		}
	}
	return value
}

// parm1 reads a single bit (Serial_parm_1bit).
func (p *bitParser) parm1() int16 {
	if p.next() == cBIT_1 {
		return 1
	}
	return 0
}

// expandBitsToParams converts speech bits (left-justified MSB-first in data) to
// the decoder's one-int16-per-bit representation for nbBits bits.
func expandBitsToParams(data []byte, nbBits int16) []int16 {
	out := make([]int16, nbBits)
	for i := int16(0); i < nbBits; i++ {
		bit := data[i/8] >> uint(7-i%8) & 1
		if bit != 0 {
			out[i] = cBIT_1
		} else {
			out[i] = cBIT_0
		}
	}
	return out
}
