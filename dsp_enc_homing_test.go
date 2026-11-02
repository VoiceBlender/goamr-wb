package amrwb

import (
	"bytes"
	"testing"
)

// TestEncoderHomingResets checks that an encoder homing frame resets the
// encoder after it is coded: what follows encodes as on a fresh encoder.
func TestEncoderHomingResets(t *testing.T) {
	sig := testSignal()
	homing := make([]int16, FrameSamples)
	for i := range homing {
		homing[i] = cEHF_MASK
	}
	next := sig[300*FrameSamples : 330*FrameSamples]
	for _, dtx := range []bool{false, true} {
		for mode := Mode0660; mode <= Mode2385; mode++ {
			used, err := NewEncoder(EncoderConfig{Mode: mode, OctetAligned: true, DTX: dtx})
			if err != nil {
				t.Fatal(err)
			}
			fresh, _ := NewEncoder(EncoderConfig{Mode: mode, OctetAligned: true, DTX: dtx})
			for f := 0; f < 50; f++ {
				if _, err := used.Encode(sig[f*FrameSamples : (f+1)*FrameSamples]); err != nil {
					t.Fatal(err)
				}
			}
			// The fresh encoder sees a homing frame too, so that the DTX
			// frame-type counters, which homing leaves alone, also match.
			if _, err := used.Encode(homing); err != nil {
				t.Fatal(err)
			}
			if _, err := fresh.Encode(homing); err != nil {
				t.Fatal(err)
			}
			for f := 0; f < len(next)/FrameSamples; f++ {
				in := next[f*FrameSamples : (f+1)*FrameSamples]
				a, _ := used.Encode(in)
				b, _ := fresh.Encode(in)
				if !bytes.Equal(a, b) {
					t.Fatalf("dtx=%v mode %d frame %d after homing: %x, fresh encoder %x", dtx, mode, f, a, b)
				}
			}
		}
	}
}

// TestEncoderHomingNearMiss checks that a frame differing from the homing
// pattern in one sample does not reset the encoder.
func TestEncoderHomingNearMiss(t *testing.T) {
	homing := make([]int16, FrameSamples)
	for i := range homing {
		homing[i] = cEHF_MASK
	}
	if !encoderHomingFrameTest(homing) {
		t.Fatal("homing frame not detected")
	}
	for _, i := range []int{0, 159, 319} {
		homing[i] = cEHF_MASK | 1 // equal after the 14-bit mask, but not a homing frame
		if encoderHomingFrameTest(homing) {
			t.Fatalf("sample %d changed: still detected as a homing frame", i)
		}
		homing[i] = cEHF_MASK
	}
}
