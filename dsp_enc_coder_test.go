package amrwb

import (
	"math"
	"testing"
)

// End-to-end encode->decode smoke test for the 6.60 kbit/s mode. Correctness is
// proven bit-exactly by TestEncDiffAgainstCReference (Go encoder == C reference)
// combined with the bit-exact decoder; this test just exercises the full
// Encoder.Encode -> Decoder.Decode path across frames (deterministic, no panic,
// 320 samples/frame, non-silent output).
func TestEncodeDecodeRoundTripMode0(t *testing.T) {
	const frames = 12
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode0660, OctetAligned: true})
	dec := NewDecoder(DecoderConfig{OctetAligned: true})

	phase := 0.0
	var energy int64
	for f := 0; f < frames; f++ {
		samples := make([]int16, FrameSamples)
		for i := range samples {
			samples[i] = int16(6000*math.Sin(phase) + 2000*math.Sin(phase*2.7))
			phase += 2 * math.Pi * 300 / 16000
		}
		payload, err := enc.Encode(samples)
		if err != nil {
			t.Fatalf("frame %d encode: %v", f, err)
		}
		pcm, err := dec.Decode(payload)
		if err != nil {
			t.Fatalf("frame %d decode: %v", f, err)
		}
		if len(pcm) != FrameSamples {
			t.Fatalf("frame %d: decoded %d samples, want %d", f, len(pcm), FrameSamples)
		}
		for _, v := range pcm {
			energy += int64(v) * int64(v)
		}
	}
	if energy == 0 {
		t.Fatal("encode->decode produced only silence")
	}
}

// Encoding is deterministic for identical input.
func TestEncodeDeterministic(t *testing.T) {
	samples := make([]int16, FrameSamples)
	for i := range samples {
		samples[i] = int16(3000 * math.Sin(float64(i)*0.2))
	}
	e1, _ := NewEncoder(EncoderConfig{Mode: Mode0660, OctetAligned: true})
	e2, _ := NewEncoder(EncoderConfig{Mode: Mode0660, OctetAligned: true})
	p1, _ := e1.Encode(samples)
	p2, _ := e2.Encode(samples)
	if string(p1) != string(p2) {
		t.Error("encoder not deterministic for identical input/state")
	}
}
