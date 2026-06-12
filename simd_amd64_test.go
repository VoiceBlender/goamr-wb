//go:build amd64

package amrwb

import "testing"

// TestScalarPathMatchesAVX2 encodes and decodes with the AVX2 kernel and again
// with the pure-Go fallback forced on, asserting byte-identical results. This
// guards the non-amd64 / no-AVX2 path (which uses firRawGeneric) against the
// AVX2 path without needing the target hardware.
func TestScalarPathMatchesAVX2(t *testing.T) {
	if !useAVX2 {
		t.Skip("AVX2 not available; both paths are already the generic one")
	}
	frames := benchGenFrames(40)

	encodeAll := func() [][]byte {
		var e encoderState
		e.reset()
		out := make([][]byte, len(frames))
		for i, f := range frames {
			p, _ := e.encodeFrame(Mode2385, f) // mode 8 exercises every firRaw site
			out[i] = append([]byte(nil), p...)
		}
		return out
	}
	decodeAll := func(packed [][]byte) [][]int16 {
		var st decoderState
		st.reset()
		out := make([][]int16, len(packed))
		synth := make([]int16, cAMR_WB_PCM_FRAME)
		for i, pk := range packed {
			params := mimeUnsort(pk, int16(Mode2385))
			st.dsp.decodeAmrWb(int16(Mode2385), params, cRX_SPEECH_GOOD, synth)
			out[i] = append([]int16(nil), synth...)
		}
		return out
	}

	useAVX2 = true
	encAVX2 := encodeAll()
	decAVX2 := decodeAll(encAVX2)

	useAVX2 = false
	encGen := encodeAll()
	decGen := decodeAll(encGen)
	useAVX2 = true

	for i := range frames {
		if string(encAVX2[i]) != string(encGen[i]) {
			t.Fatalf("encode frame %d differs between AVX2 and scalar paths", i)
		}
		for j := range decAVX2[i] {
			if decAVX2[i][j] != decGen[i][j] {
				t.Fatalf("decode frame %d sample %d differs between AVX2 and scalar paths", i, j)
			}
		}
	}
}
