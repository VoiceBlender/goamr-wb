package amrwb

import (
	"bytes"
	"testing"
)

// allFormats are the payload formats the package supports.
var allFormats = []struct {
	name string
	pf   payloadFormat
}{
	{"be", payloadFormat{}},
	{"octet", payloadFormat{octetAligned: true}},
	{"crc", payloadFormat{octetAligned: true, crc: true}},
	{"robust", payloadFormat{octetAligned: true, robustSorting: true}},
	{"crc+robust", payloadFormat{octetAligned: true, crc: true, robustSorting: true}},
}

// TestFrameCRCByHand checks frameCRC against values worked out by hand from
// the procedure in RFC 4867, section 4.4.2.1.
func TestFrameCRCByHand(t *testing.T) {
	for _, c := range []struct {
		data []byte
		n    int
		want byte
	}{
		{[]byte{0x00}, 8, 0x00}, // zeros never set the register
		{[]byte{0x80}, 1, 0xB8}, // 1: LSB 0 ^ 1 = 1, shift, XOR 10111000
		{[]byte{0x80}, 2, 0x5C}, // 10: then LSB 0 ^ 0 = 0, shift
		{[]byte{0xC0}, 2, 0xE4}, // 11: then 0 ^ 1 = 1, shift to 01011100, XOR
		{[]byte{0x40}, 2, 0xB8}, // 01: the first 0 leaves the register clear
	} {
		if got := frameCRC(c.data, c.n); got != c.want {
			t.Errorf("CRC of %d bits of %08b = %08b, want %08b", c.n, c.data[0], got, c.want)
		}
	}
}

// mixedFrames returns frames of every kind, with random bits.
func mixedFrames() []frame {
	seed := uint32(7)
	mk := func(ft int, q bool) frame {
		bits, _ := frameBitsByFT(ft)
		data := make([]byte, (bits+7)/8)
		for i := range data {
			seed = seed*1664525 + 1013904223
			data[i] = byte(seed >> 24)
		}
		if bits%8 != 0 {
			data[len(data)-1] &= 0xFF << uint(8-bits%8)
		}
		return frame{ft: ft, q: q, data: data}
	}
	return []frame{
		mk(FTNoData, true), mk(FTMode0660, true), mk(FTSID, true), mk(FTSpeechLost, false),
		mk(FTMode2385, false), mk(FTNoData, true), mk(FTMode1265, true), mk(FTMode0885, true),
	}
}

func TestPayloadFormatsRoundTrip(t *testing.T) {
	in := mixedFrames()
	for _, f := range allFormats {
		for k := 1; k <= len(in); k++ {
			p := packPayload(f.pf, 5, in[:k])
			if len(p) != f.pf.size(in[:k]) {
				t.Fatalf("%s: %d frames: payload %d bytes, size says %d", f.name, k, len(p), f.pf.size(in[:k]))
			}
			cmr, out, err := unpackPayload(p, f.pf)
			if err != nil {
				t.Fatalf("%s: %d frames: %v", f.name, k, err)
			}
			if cmr != 5 {
				t.Errorf("%s: CMR %d, want 5", f.name, cmr)
			}
			framesEqual(t, out, in[:k], f.name)
		}
	}
}

// TestRobustSortingLayout checks the octet order of RFC 4867, section 4.4.4:
// octet k of each frame in turn, skipping frames without data or that have
// run out of octets.
func TestRobustSortingLayout(t *testing.T) {
	sid := frame{ft: FTSID, q: true, data: []byte{0xA0, 0xA1, 0xA2, 0xA3, 0xA4}}
	sp := frame{ft: FTMode0660, q: true, data: make([]byte, 17)}
	for i := range sp.data {
		sp.data[i] = byte(0xB0 + i)
	}
	sp.data[16] = 0xFF // 132 bits: the last 4 are padding, cleared when packed
	nd := frame{ft: FTNoData, q: true}
	p := packPayload(payloadFormat{octetAligned: true, robustSorting: true}, noCMR, []frame{sid, nd, sp})
	want := []byte{0xF0, 0x80 | FTSID<<3 | 4, 0x80 | FTNoData<<3 | 4, FTMode0660<<3 | 4,
		0xA0, 0xB0, 0xA1, 0xB1, 0xA2, 0xB2, 0xA3, 0xB3, 0xA4, 0xB4}
	for i := 5; i < 16; i++ {
		want = append(want, byte(0xB0+i))
	}
	want = append(want, 0xF0)
	if !bytes.Equal(p, want) {
		t.Fatalf("robust sorting:\n got %x\nwant %x", p, want)
	}
}

// TestCRCPlacementAndMismatch checks that CRCs follow the ToC, one per frame
// with data, and that a corrupted class A bit clears Q while a corrupted
// class B bit does not.
func TestCRCPlacementAndMismatch(t *testing.T) {
	in := mixedFrames()
	pf := payloadFormat{octetAligned: true, crc: true}
	p := packPayload(pf, noCMR, in)
	crcAt := 1 + len(in)
	c := 0
	for _, f := range in {
		if bits, _ := frameBitsByFT(f.ft); bits > 0 {
			if p[crcAt+c] != frameCRC(f.data, classABits[f.ft]) {
				t.Fatalf("CRC %d misplaced or wrong", c)
			}
			c++
		}
	}
	dataAt := crcAt + c
	// Frame 1 (6.60, 54 class A bits) starts the data: flip bit 0 (class A),
	// then bit 60 (class B).
	for _, bit := range []int{0, 60} {
		q := append([]byte(nil), p...)
		q[dataAt+bit/8] ^= 0x80 >> uint(bit%8)
		_, out, err := unpackPayload(q, pf)
		if err != nil {
			t.Fatal(err)
		}
		if wantQ := bit >= 54; out[1].q != wantQ {
			t.Errorf("bit %d flipped: Q = %v, want %v", bit, out[1].q, wantQ)
		}
		for i, f := range out {
			if i != 1 && f.q != in[i].q {
				t.Errorf("bit %d flipped: frame %d Q changed", bit, i)
			}
		}
	}
	// A payload cut inside the CRC list is rejected.
	if _, _, err := unpackPayload(p[:crcAt+2], pf); err == nil {
		t.Error("payload cut in the CRC list parsed")
	}
}

// TestFormatImplications checks that crc and robust-sorting imply
// octet-aligned framing, as RFC 4867, section 8.2, says.
func TestFormatImplications(t *testing.T) {
	if !newPayloadFormat(false, true, false).octetAligned || !newPayloadFormat(false, false, true).octetAligned {
		t.Fatal("crc or robust-sorting without octet-align is not octet-aligned")
	}
}
