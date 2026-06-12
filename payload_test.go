package amrwb

import (
	"bytes"
	"testing"
)

func makeFrame(ft int, q bool) frame {
	nb, _ := frameBytesByFT(ft)
	bits, _ := frameBitsByFT(ft)
	data := make([]byte, nb)
	for i := range data {
		data[i] = byte(0xA0 + i)
	}
	// Zero the padding bits beyond `bits` in the final octet: bandwidth-efficient
	// framing carries exactly `bits` bits, so padding must be zero to round-trip.
	if rem := bits % 8; rem != 0 && nb > 0 {
		data[nb-1] &= byte(0xFF) << uint(8-rem)
	}
	return frame{ft: ft, q: q, data: data}
}

func framesEqual(t *testing.T, got, want []frame, format string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: frame count got %d, want %d", format, len(got), len(want))
	}
	for i := range want {
		if got[i].ft != want[i].ft {
			t.Errorf("%s: frame %d ft got %d, want %d", format, i, got[i].ft, want[i].ft)
		}
		if got[i].q != want[i].q {
			t.Errorf("%s: frame %d q got %v, want %v", format, i, got[i].q, want[i].q)
		}
		if !bytes.Equal(got[i].data, want[i].data) {
			t.Errorf("%s: frame %d data got %x, want %x", format, i, got[i].data, want[i].data)
		}
	}
}

func TestOctetRoundTrip(t *testing.T) {
	for _, ft := range []int{FTMode0660, FTMode1265, FTMode2385, FTSID} {
		in := []frame{makeFrame(ft, true)}
		p := packPayloadOctet(noCMR, in)
		cmr, out, err := unpackPayloadOctet(p)
		if err != nil {
			t.Fatalf("ft=%d unpack: %v", ft, err)
		}
		if cmr != noCMR {
			t.Errorf("ft=%d cmr got %d, want %d", ft, cmr, noCMR)
		}
		framesEqual(t, out, in, "octet")
	}
}

func TestBandwidthEfficientRoundTrip(t *testing.T) {
	for _, ft := range []int{FTMode0660, FTMode1265, FTMode2385, FTSID} {
		in := []frame{makeFrame(ft, true)}
		p := packPayloadBandwidthEfficient(3, in)
		cmr, out, err := unpackPayloadBandwidthEfficient(p)
		if err != nil {
			t.Fatalf("ft=%d unpack: %v", ft, err)
		}
		if cmr != 3 {
			t.Errorf("ft=%d cmr got %d, want 3", ft, cmr)
		}
		framesEqual(t, out, in, "be")
	}
}

func TestOctetIsByteAligned(t *testing.T) {
	in := []frame{makeFrame(FTMode1265, true)}
	p := packPayloadOctet(noCMR, in)
	// 1 CMR byte + 1 ToC byte + ceil(253/8)=32 speech bytes = 34
	if want := 1 + 1 + 32; len(p) != want {
		t.Errorf("octet len got %d, want %d", len(p), want)
	}
}

func TestBandwidthEfficientShorterThanOctet(t *testing.T) {
	in := []frame{makeFrame(FTMode1265, true)}
	be := packPayloadBandwidthEfficient(noCMR, in)
	oa := packPayloadOctet(noCMR, in)
	if len(be) >= len(oa) {
		t.Errorf("bandwidth-efficient (%d) should be shorter than octet-aligned (%d)", len(be), len(oa))
	}
}

func TestUnpackShortPayload(t *testing.T) {
	if _, _, err := unpackPayloadOctet(nil); err == nil {
		t.Error("expected error on empty octet payload")
	}
	// CMR + ToC claiming a large frame but no speech bytes
	p := []byte{0xF0, byte(FTMode2385) << 3}
	if _, _, err := unpackPayloadOctet(p); err == nil {
		t.Error("expected short-payload error")
	}
}

func TestMultiFrameOctet(t *testing.T) {
	in := []frame{makeFrame(FTMode1265, true), makeFrame(FTMode1265, false)}
	p := packPayloadOctet(noCMR, in)
	_, out, err := unpackPayloadOctet(p)
	if err != nil {
		t.Fatalf("multiframe unpack: %v", err)
	}
	framesEqual(t, out, in, "octet-multi")
}
