package amrwb

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

// noiseFrames returns n frames of white noise with peak amp.
func noiseFrames(n int, amp float64, seed uint64) [][]int16 {
	rng := rand.New(rand.NewPCG(seed, 1))
	frames := make([][]int16, n)
	for f := range frames {
		frames[f] = make([]int16, FrameSamples)
		for i := range frames[f] {
			frames[f][i] = int16(amp * (2*rng.Float64() - 1))
		}
	}
	return frames
}

func rms(s []int16) float64 {
	var sum float64
	for _, v := range s {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum / float64(len(s)))
}

// TestDTXRoundTrip runs stationary noise through the public API with DTX on, as
// an RTP sender and receiver would: it must settle into SID updates every 8th
// frame, with comfort noise a little below the level of the input.
func TestDTXRoundTrip(t *testing.T) {
	for _, octet := range []bool{true, false} {
		for _, mode := range []Mode{Mode0660, Mode1265, Mode2385} {
			enc, _ := NewEncoder(EncoderConfig{Mode: mode, OctetAligned: octet, DTX: true})
			dec := NewDecoder(DecoderConfig{OctetAligned: octet})
			pcm := make([]int16, FrameSamples)
			var sid, empty int
			var noise []int16
			for f, in := range noiseFrames(400, 1000, uint64(mode)) {
				payload, err := enc.Encode(in)
				if err != nil {
					t.Fatal(err)
				}
				if len(payload) == 0 {
					empty++
					if _, err := dec.DecodeNoData(pcm); err != nil {
						t.Fatal(err)
					}
				} else {
					if len(payload) < 10 {
						sid++
					}
					if _, err := dec.DecodeTo(pcm, payload); err != nil {
						t.Fatal(err)
					}
				}
				if f >= 200 {
					noise = append(noise, pcm...)
				}
			}
			if sid == 0 || empty < 7*sid-60 {
				t.Errorf("octet=%v mode=%d: %d SID and %d empty payloads in 400 frames", octet, mode, sid, empty)
			}
			// Noise of amplitude 1000 has an RMS of 577, and en_adjust lowers
			// comfort noise by 2.7 to 5.4 dB.
			if got := rms(noise); got < 0.4*577 || got > 577 {
				t.Errorf("octet=%v mode=%d: comfort noise RMS %.0f for input RMS 577", octet, mode, got)
			}
		}
	}
}

func TestDTXOffNeverEmitsSIDOrNoData(t *testing.T) {
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode1265, OctetAligned: true})
	want := 1 + 1 + Mode1265.Bytes() // CMR + ToC + speech
	for f, in := range noiseFrames(200, 100, 3) {
		payload, err := enc.Encode(in)
		if err != nil {
			t.Fatal(err)
		}
		if len(payload) != want {
			t.Fatalf("frame %d: %d byte payload, want %d", f, len(payload), want)
		}
	}
}

func TestEncodeToMatchesEncodeWithDTX(t *testing.T) {
	a, _ := NewEncoder(EncoderConfig{Mode: Mode0885, OctetAligned: true, DTX: true})
	b, _ := NewEncoder(EncoderConfig{Mode: Mode0885, OctetAligned: true, DTX: true})
	dst := make([]byte, MaxPayloadBytes)
	for f, in := range noiseFrames(120, 500, 4) {
		want, err := a.Encode(in)
		if err != nil {
			t.Fatal(err)
		}
		n, err := b.EncodeTo(dst, in)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(dst[:n], want) {
			t.Fatalf("frame %d: EncodeTo differs from Encode", f)
		}
	}
}

func TestTalkspurtMarksFirstSpeechFrames(t *testing.T) {
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode1265, OctetAligned: true, DTX: true})
	silence := make([]int16, FrameSamples)
	speech := toneFrames(1)[0]
	encode := func(in []int16) []byte {
		t.Helper()
		p, err := enc.Encode(in)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	if encode(silence); !enc.Talkspurt() {
		t.Error("first frame is not a talkspurt")
	}
	if encode(silence); enc.Talkspurt() {
		t.Error("second speech frame is a talkspurt")
	}
	for f := 0; ; f++ {
		if len(encode(silence)) == 0 {
			break
		}
		if f == 100 {
			t.Fatal("no NO_DATA frame for digital silence")
		}
	}
	if enc.Talkspurt() {
		t.Error("NO_DATA frame is a talkspurt")
	}
	for f := 0; !enc.Talkspurt(); f++ {
		if p := encode(speech); len(p) < 10 && enc.Talkspurt() {
			t.Fatalf("%d byte frame is a talkspurt", len(p))
		}
		if f == 100 {
			t.Fatal("speech never started a talkspurt")
		}
	}
	if encode(speech); enc.Talkspurt() {
		t.Error("speech frame after a talkspurt start is a talkspurt")
	}
	enc.Reset()
	if encode(silence); !enc.Talkspurt() {
		t.Error("first frame after Reset is not a talkspurt")
	}
	enc.Reset()
	if enc.Talkspurt() {
		t.Error("Talkspurt survives Reset")
	}
}

func TestEncoderResetRestartsDTX(t *testing.T) {
	frames := noiseFrames(150, 300, 5)
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode1265, OctetAligned: true, DTX: true})
	var first [][]byte
	for _, in := range frames {
		p, _ := enc.Encode(in)
		first = append(first, p)
	}
	enc.Reset()
	for f, in := range frames {
		if p, _ := enc.Encode(in); !slices.Equal(p, first[f]) {
			t.Fatalf("frame %d differs after Reset", f)
		}
	}
}

// TestDecodeNoDataMatchesNoDataPayload checks that DecodeNoData is the same as
// decoding an FT=15 payload, in and out of DTX.
func TestDecodeNoDataMatchesNoDataPayload(t *testing.T) {
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode1265, OctetAligned: true, DTX: true})
	a := NewDecoder(DecoderConfig{OctetAligned: true})
	b := NewDecoder(DecoderConfig{OctetAligned: true})
	noData := []byte{0xF0, FTNoData<<3 | 0x04}
	pa, pb := make([]int16, FrameSamples), make([]int16, FrameSamples)
	for f, in := range noiseFrames(150, 300, 6) {
		payload, _ := enc.Encode(in)
		if len(payload) == 0 || f%13 == 0 { // gaps in speech too
			if _, err := a.DecodeNoData(pa); err != nil {
				t.Fatal(err)
			}
			if _, err := b.DecodeTo(pb, noData); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := a.DecodeTo(pa, payload); err != nil {
				t.Fatal(err)
			}
			if _, err := b.DecodeTo(pb, payload); err != nil {
				t.Fatal(err)
			}
		}
		if !slices.Equal(pa, pb) {
			t.Fatalf("frame %d differs", f)
		}
	}
}

// TestBadSIDKeepsComfortNoise covers RX_SID_BAD, which the differential tests
// cannot reach because D_IF_decode ignores the Q bit: a SID frame with Q=0 keeps
// the previous noise parameters rather than reading its bits.
func TestBadSIDKeepsComfortNoise(t *testing.T) {
	dec := NewDecoder(DecoderConfig{OctetAligned: true})
	pcm := make([]int16, FrameSamples)
	update := []byte{0xF0, FTSID<<3 | 0x04, 0x12, 0x34, 0x56, 0x78, 0x9A | 0x10}
	bad := []byte{0xF0, FTSID << 3, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	for range 3 {
		if _, err := dec.DecodeTo(pcm, update); err != nil {
			t.Fatal(err)
		}
	}
	dtx := &dec.st.dsp.dtxDecSt
	isf, logEn := dtx.isf, dtx.logEn
	if _, err := dec.DecodeTo(pcm, bad); err != nil {
		t.Fatal(err)
	}
	if dtx.sidFrame != 1 || dtx.validData != 0 || dtx.dtxHangoverAdded != 0 {
		t.Errorf("sidFrame=%d validData=%d dtxHangoverAdded=%d after a bad SID",
			dtx.sidFrame, dtx.validData, dtx.dtxHangoverAdded)
	}
	if dtx.isf != isf || dtx.logEn != logEn {
		t.Error("a bad SID frame changed the comfort noise parameters")
	}
	if dtx.dtxGlobalState != cDTX {
		t.Errorf("DTX state %d after a bad SID", dtx.dtxGlobalState)
	}
	if rms(pcm) == 0 {
		t.Error("no comfort noise for a bad SID frame")
	}

	// The same past the elapsed frames threshold, where a good SID would take
	// its parameters from the history of decoded speech.
	dec.Reset()
	if _, err := dec.DecodeTo(pcm, bad); err != nil {
		t.Fatal(err)
	}
	if dtx.dtxHangoverAdded != 0 || dtx.logEn != 3500 {
		t.Errorf("dtxHangoverAdded=%d logEn=%d after a bad first SID", dtx.dtxHangoverAdded, dtx.logEn)
	}
}

// TestFindFrameIndicesSaturatesRounding pins the saturating voround of
// find_frame_indices: a sum near full scale must not wrap negative, which would
// wrongly skip the median replacement. Real signals rarely get there.
func TestFindFrameIndicesSaturatesRounding(t *testing.T) {
	var st dtxEncState
	st.reset(isfInit[:])
	// findFrameIndices shifts sumD up by one before looking for the maxima.
	st.sumD[2], st.sumD[3] = 0x7FFFFFFF, 0x7FFFFFFF
	var indices [3]int16
	st.findFrameIndices(&indices)
	if indices[0] == -1 || indices[1] == -1 {
		t.Fatalf("median replacement skipped for near-full-scale distances: %v", indices)
	}
}
