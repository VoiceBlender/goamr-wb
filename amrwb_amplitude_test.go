package amrwb

import (
	"math"
	"math/rand"
	"testing"
)

// AMR-WB's LPC synthesis filter can overshoot the input envelope by several dB
// on voiced content, producing decoded samples that saturate at ±32767. To
// keep callers from having to second-guess input scaling, the encoder applies
// 6 dB of pre-attenuation. This test guards the resulting end-to-end behavior:
// realistic speech-band content at peak -3 dBFS roundtrips without clipping.
func TestEncoderHeadroomPreventsDecoderSaturation(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	// 5 seconds of speech-band-filtered random noise at peak -3 dBFS — well
	// inside the range where the unmitigated decoder saturates on real speech.
	const (
		seconds      = 5
		samplesPerFr = 320
		framesPerSec = 50
		warmup       = 5
	)
	peakScale := math.Pow(10, -3.0/20) * 32767

	type result struct {
		peak    int16
		clipped int
	}
	check := func(t *testing.T, mode Mode, octetAligned bool) result {
		enc, err := NewEncoder(EncoderConfig{Mode: mode, OctetAligned: octetAligned})
		if err != nil {
			t.Fatalf("NewEncoder: %v", err)
		}
		dec := NewDecoder(DecoderConfig{OctetAligned: octetAligned})

		var prev int16
		var peak int16
		clipped := 0
		for f := 0; f < seconds*framesPerSec; f++ {
			buf := make([]int16, samplesPerFr)
			for i := range buf {
				raw := int16((rng.Float64()*2 - 1) * peakScale)
				s := int16(int32(prev)*7/10 + int32(raw)*3/10)
				prev = s
				buf[i] = s
			}
			payload, err := enc.Encode(buf)
			if err != nil {
				t.Fatalf("encode frame %d: %v", f, err)
			}
			out, err := dec.Decode(payload)
			if err != nil {
				t.Fatalf("decode frame %d: %v", f, err)
			}
			if f < warmup {
				continue
			}
			for _, s := range out {
				if s == 32767 || s == -32768 {
					clipped++
				}
				a := s
				if a < 0 {
					a = -a
				}
				if a > peak {
					peak = a
				}
			}
		}
		return result{peak: peak, clipped: clipped}
	}

	for _, octetAligned := range []bool{true, false} {
		for mode := Mode0660; mode <= Mode2385; mode++ {
			r := check(t, mode, octetAligned)
			t.Logf("mode=%d octet_aligned=%v peak=%d clipped_samples=%d",
				mode, octetAligned, r.peak, r.clipped)
			if r.clipped > 0 {
				t.Errorf("mode=%d octet_aligned=%v: %d clipped samples in decoded output",
					mode, octetAligned, r.clipped)
			}
		}
	}
}
