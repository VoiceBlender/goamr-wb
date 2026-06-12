package amrwb

import "testing"

// End-to-end smoke test: the full decode pipeline runs for every speech mode
// and yields 320 samples without panicking. (Bit-exactness vs the 3GPP TS
// 26.174 reference vectors is validated separately before AMR-WB is offered.)
func TestDecodeAllModesRuns(t *testing.T) {
	for m := Mode0660; m < numSpeechModes; m++ {
		var st decoderState
		st.reset()
		nb := amrwbCompressed[m]
		// A deterministic non-trivial bit pattern.
		params := make([]int16, nb)
		for i := range params {
			if i%3 == 0 {
				params[i] = cBIT_1
			} else {
				params[i] = cBIT_0
			}
		}
		synth := make([]int16, cAMR_WB_PCM_FRAME)
		st.dsp.decodeAmrWb(int16(m), params, cRX_SPEECH_GOOD, synth)
		if len(synth) != FrameSamples {
			t.Fatalf("mode %d: got %d samples, want %d", m, len(synth), FrameSamples)
		}
	}
}

func TestDecoderDecodeViaPayload(t *testing.T) {
	// Octet-aligned payload for mode 12.65k, marked good.
	nb := amrwbCompressed[Mode1265]
	data := make([]byte, (nb+7)/8)
	for i := range data {
		data[i] = byte(0x5A + i)
	}
	f := frame{ft: FTMode1265, q: true, data: data}
	p := packPayloadOctet(noCMR, []frame{f})

	d := NewDecoder(DecoderConfig{OctetAligned: true})
	pcm, err := d.Decode(p)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(pcm) != FrameSamples {
		t.Fatalf("got %d samples, want %d", len(pcm), FrameSamples)
	}
}

func TestDecodeFrameAcrossFrames(t *testing.T) {
	// Multiple consecutive frames must decode without panics (state carries).
	var st decoderState
	st.reset()
	for n := 0; n < 5; n++ {
		nb := amrwbCompressed[Mode2385]
		params := make([]int16, nb)
		for i := range params {
			params[i] = int16([]int{cBIT_0, cBIT_1}[(i+n)%2])
		}
		synth := make([]int16, cAMR_WB_PCM_FRAME)
		st.dsp.decodeAmrWb(int16(Mode2385), params, cRX_SPEECH_GOOD, synth)
	}
}

func TestDecodeLostFrameConcealment(t *testing.T) {
	var st decoderState
	st.reset()
	// First a good frame, then a lost frame (no data) -> concealment, no panic.
	good := make([]int16, amrwbCompressed[Mode1265])
	for i := range good {
		good[i] = cBIT_1
	}
	synth := make([]int16, cAMR_WB_PCM_FRAME)
	st.dsp.decodeAmrWb(int16(Mode1265), good, cRX_SPEECH_GOOD, synth)

	lost := make([]int16, amrwbCompressed[Mode1265])
	st.dsp.decodeAmrWb(int16(Mode1265), lost, cRX_SPEECH_LOST, synth)
}
