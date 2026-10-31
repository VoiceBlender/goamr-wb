package amrwb

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

func toneFrames(n int) [][]int16 {
	frames := make([][]int16, n)
	for f := range frames {
		frames[f] = make([]int16, FrameSamples)
		for i := range frames[f] {
			t := float64(f*FrameSamples+i) / SampleRate
			frames[f][i] = int16(8000*math.Sin(2*math.Pi*440*t) + 3000*math.Sin(2*math.Pi*1320*t))
		}
	}
	return frames
}

func TestEncodeToAndDecodeToMatchAllocatingAPI(t *testing.T) {
	for _, octet := range []bool{true, false} {
		for _, mode := range []Mode{Mode0660, Mode1265, Mode2385} {
			encA, _ := NewEncoder(EncoderConfig{Mode: mode, OctetAligned: octet})
			encB, _ := NewEncoder(EncoderConfig{Mode: mode, OctetAligned: octet})
			decA := NewDecoder(DecoderConfig{OctetAligned: octet})
			decB := NewDecoder(DecoderConfig{OctetAligned: octet})
			payload := make([]byte, MaxPayloadBytes)
			pcm := make([]int16, FrameSamples)
			for f, frame := range toneFrames(25) {
				want, err := encA.Encode(frame)
				if err != nil {
					t.Fatal(err)
				}
				n, err := encB.EncodeTo(payload, frame)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(payload[:n], want) {
					t.Fatalf("octet=%v mode=%d frame %d: EncodeTo differs from Encode", octet, mode, f)
				}
				wantPCM, err := decA.Decode(want)
				if err != nil {
					t.Fatal(err)
				}
				m, err := decB.DecodeTo(pcm, payload[:n])
				if err != nil {
					t.Fatal(err)
				}
				if m != FrameSamples || !slices.Equal(pcm, wantPCM) {
					t.Fatalf("octet=%v mode=%d frame %d: DecodeTo differs from Decode", octet, mode, f)
				}
			}
		}
	}
}

func TestEncodeToAndDecodeToDoNotAllocate(t *testing.T) {
	frames := toneFrames(4)
	for _, octet := range []bool{true, false} {
		enc, _ := NewEncoder(EncoderConfig{Mode: Mode2385, OctetAligned: octet})
		dec := NewDecoder(DecoderConfig{OctetAligned: octet})
		payload := make([]byte, MaxPayloadBytes)
		pcm := make([]int16, FrameSamples)
		n, _ := enc.EncodeTo(payload, frames[0])
		if _, err := dec.DecodeTo(pcm, payload[:n]); err != nil { // grow parser buffers
			t.Fatal(err)
		}
		i := 0
		if allocs := testing.AllocsPerRun(50, func() {
			n, _ = enc.EncodeTo(payload, frames[i%len(frames)])
			i++
		}); allocs != 0 {
			t.Errorf("octet=%v: EncodeTo allocates %.0f times per frame", octet, allocs)
		}
		if allocs := testing.AllocsPerRun(50, func() {
			_, _ = dec.DecodeTo(pcm, payload[:n])
		}); allocs != 0 {
			t.Errorf("octet=%v: DecodeTo allocates %.0f times per frame", octet, allocs)
		}
	}
}

func TestDTXPathsDoNotAllocate(t *testing.T) {
	silence := make([]int16, FrameSamples)
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode1265, OctetAligned: true, DTX: true})
	payload := make([]byte, MaxPayloadBytes)
	for range 40 { // through the hangover and into DTX
		if _, err := enc.EncodeTo(payload, silence); err != nil {
			t.Fatal(err)
		}
	}
	if allocs := testing.AllocsPerRun(50, func() {
		_, _ = enc.EncodeTo(payload, silence)
	}); allocs != 0 {
		t.Errorf("EncodeTo allocates %.0f times per DTX frame", allocs)
	}

	dec := NewDecoder(DecoderConfig{OctetAligned: true})
	pcm := make([]int16, FrameSamples)
	sid := []byte{0xF0, FTSID<<3 | 0x04, 0x12, 0x34, 0x56, 0x78, 0x9A | 0x10}
	if _, err := dec.DecodeTo(pcm, sid); err != nil { // grow parser buffers
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(50, func() {
		_, _ = dec.DecodeTo(pcm, sid)
		_, _ = dec.DecodeNoData(pcm)
	}); allocs != 0 {
		t.Errorf("SID and NO_DATA decoding allocates %.0f times per pair of frames", allocs)
	}
}

func TestIntoAPIsRejectShortBuffers(t *testing.T) {
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode2385, OctetAligned: true})
	if _, err := enc.EncodeTo(make([]byte, 10), toneFrames(1)[0]); err == nil {
		t.Fatal("EncodeTo accepted a destination smaller than the payload")
	}
	payload, _ := enc.Encode(toneFrames(1)[0])
	dec := NewDecoder(DecoderConfig{OctetAligned: true})
	if _, err := dec.DecodeTo(make([]int16, FrameSamples-1), payload); err == nil {
		t.Fatal("DecodeTo accepted a destination smaller than one frame")
	}
	if _, err := dec.DecodeNoData(make([]int16, FrameSamples-1)); err == nil {
		t.Fatal("DecodeNoData accepted a destination smaller than one frame")
	}
}

// TestDecodeToRejectsCorruptPayloadsWithoutPanicking feeds truncated and
// random payloads in both framings: each must decode or fail, never panic.
func TestDecodeToRejectsCorruptPayloadsWithoutPanicking(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	pcm := make([]int16, 16*FrameSamples)
	for _, octet := range []bool{true, false} {
		enc, _ := NewEncoder(EncoderConfig{Mode: Mode2385, OctetAligned: octet})
		dec := NewDecoder(DecoderConfig{OctetAligned: octet})
		valid, _ := enc.Encode(toneFrames(1)[0])
		for cut := range len(valid) {
			if _, err := dec.DecodeTo(pcm, valid[:cut]); err == nil {
				t.Fatalf("octet=%v: payload truncated to %d bytes was accepted", octet, cut)
			}
		}
		for range 2000 {
			garbage := make([]byte, rng.IntN(80))
			for i := range garbage {
				garbage[i] = byte(rng.Uint32())
			}
			_, _ = dec.DecodeTo(pcm, garbage)
		}
	}
}

func TestDecodeToHandlesSIDAndNoDataFrames(t *testing.T) {
	dec := NewDecoder(DecoderConfig{OctetAligned: true})
	pcm := make([]int16, 2*FrameSamples)
	// CMR=15, ToC: F=1 FT=SID Q=1, ToC: F=0 FT=NO_DATA Q=1, 5 SID bytes.
	payload := []byte{0xF0, 0x80 | FTSID<<3 | 0x04, FTNoData<<3 | 0x04, 0, 0, 0, 0, 0}
	n, err := dec.DecodeTo(pcm, payload)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2*FrameSamples {
		t.Fatalf("decoded %d samples for SID + NO_DATA, want %d", n, 2*FrameSamples)
	}
}
