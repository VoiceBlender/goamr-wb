package amrwb

import "testing"

// TestModeRequestRoundTrip sends every mode request, and none, from an Encoder
// to a Decoder in both payload formats.
func TestModeRequestRoundTrip(t *testing.T) {
	pcm := testSignal()[100*FrameSamples : 101*FrameSamples]
	for _, octet := range []bool{true, false} {
		enc, err := NewEncoder(EncoderConfig{Mode: Mode1265, OctetAligned: octet})
		if err != nil {
			t.Fatal(err)
		}
		dec := NewDecoder(DecoderConfig{OctetAligned: octet})
		if got := dec.ModeRequest(); got != NoModeRequest {
			t.Fatalf("octet=%v: new decoder ModeRequest = %d", octet, got)
		}
		for _, m := range []Mode{Mode0660, Mode0885, Mode1265, Mode1425, Mode1585, Mode1825, Mode1985, Mode2305, Mode2385, NoModeRequest, Mode2385, Mode(-1), Mode(9), Mode(14)} {
			enc.SetModeRequest(m)
			want := m
			if !m.Valid() {
				want = NoModeRequest
			}
			p, err := enc.Encode(pcm)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := dec.Decode(p); err != nil {
				t.Fatal(err)
			}
			if got := dec.ModeRequest(); got != want {
				t.Fatalf("octet=%v: sent request %d, decoder reports %d (want %d)", octet, m, got, want)
			}
		}
	}
}

// TestModeRequestKeptAndCleared checks that the request survives DecodeNoData
// and payloads that fail to parse, that reserved CMR values read as
// NoModeRequest, and that Reset clears it.
func TestModeRequestKeptAndCleared(t *testing.T) {
	dec := NewDecoder(DecoderConfig{OctetAligned: true})
	f := frame{ft: FTNoData, q: true}
	if _, err := dec.Decode(packPayloadOctet(int(Mode1985), []frame{f})); err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, FrameSamples)
	if _, err := dec.DecodeNoData(pcm); err != nil {
		t.Fatal(err)
	}
	if got := dec.ModeRequest(); got != Mode1985 {
		t.Fatalf("after DecodeNoData: ModeRequest = %d, want %d", got, Mode1985)
	}
	// CMR 2 on a payload whose speech frame is truncated: rejected, so ignored.
	bad := packPayloadOctet(int(Mode1265), []frame{{ft: FTMode2385, q: true, data: make([]byte, 60)}})
	if _, err := dec.Decode(bad[:10]); err == nil {
		t.Fatal("truncated payload decoded")
	}
	if got := dec.ModeRequest(); got != Mode1985 {
		t.Fatalf("after a rejected payload: ModeRequest = %d, want %d", got, Mode1985)
	}
	// Reserved CMR values 9..14 ask for nothing.
	for cmr := 9; cmr <= 14; cmr++ {
		if _, err := dec.Decode(packPayloadOctet(cmr, []frame{f})); err != nil {
			t.Fatal(err)
		}
		if got := dec.ModeRequest(); got != NoModeRequest {
			t.Fatalf("CMR %d: ModeRequest = %d, want NoModeRequest", cmr, got)
		}
	}
	if _, err := dec.Decode(packPayloadOctet(int(Mode0885), []frame{f})); err != nil {
		t.Fatal(err)
	}
	dec.Reset()
	if got := dec.ModeRequest(); got != NoModeRequest {
		t.Fatalf("after Reset: ModeRequest = %d, want NoModeRequest", got)
	}
}
