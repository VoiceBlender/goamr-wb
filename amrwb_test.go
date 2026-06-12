package amrwb

import (
	"testing"
)

func TestNewEncoderValidatesMode(t *testing.T) {
	if _, err := NewEncoder(EncoderConfig{Mode: Mode(99)}); err == nil {
		t.Error("expected error for invalid mode")
	}
	if _, err := NewEncoder(EncoderConfig{Mode: Mode1265, OctetAligned: true}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEncodeRejectsWrongFrameSize(t *testing.T) {
	e, _ := NewEncoder(EncoderConfig{Mode: Mode1265, OctetAligned: true})
	if _, err := e.Encode(make([]int16, 100)); err == nil {
		t.Error("expected error for wrong sample count")
	}
}

// Encode (6.6k) now produces a real RFC 4867 payload of the correct length.
func TestEncodeMode0ProducesPayload(t *testing.T) {
	e, _ := NewEncoder(EncoderConfig{Mode: Mode0660, OctetAligned: true})
	samples := make([]int16, FrameSamples)
	for i := range samples {
		samples[i] = int16(2000 * sinApprox(float64(i)*0.2))
	}
	payload, err := e.Encode(samples)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	// octet-aligned: CMR + ToC + ceil(132/8)=17 speech bytes = 19.
	if want := 1 + 1 + 17; len(payload) != want {
		t.Errorf("payload len %d, want %d", len(payload), want)
	}
}

func TestDecodeShortPayloadErrors(t *testing.T) {
	d := NewDecoder(DecoderConfig{OctetAligned: true})
	if _, err := d.Decode(nil); err == nil {
		t.Error("expected error on empty payload")
	}
}
