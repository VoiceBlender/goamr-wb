package amrwb

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"testing"
)

// TestDiffAgainstCReference decodes identical AMR-WB frames with this Go port
// and the Apache-2.0 opencore-amrwb C reference, asserting bit-exact PCM. Set
// AMRWB_DIFF to the compiled reference harness (reads N speech bytes/frame on
// stdin for the mode given as argv[1], writes 320 int16 LE/frame to stdout).
func TestDiffAgainstCReference(t *testing.T) {
	bin := os.Getenv("AMRWB_DIFF")
	if bin == "" {
		t.Skip("set AMRWB_DIFF to the opencore-amr reference harness to run")
	}
	speechBytes := []int{17, 23, 32, 36, 40, 46, 50, 58, 60} // modes 0..8

	for mode := 0; mode <= 8; mode++ {
		n := speechBytes[mode]
		nb := int(amrwbCompressed[Mode(mode)])
		const frames = 30

		// Deterministic pseudo-random frames; zero padding bits past nbBits so
		// both decoders see exactly the same meaningful bitstream.
		seed := uint32(0x12345 + mode*7)
		raw := make([]byte, n*frames)
		for i := range raw {
			seed = seed*1664525 + 1013904223
			raw[i] = byte(seed >> 16)
		}
		for f := 0; f < frames; f++ {
			base := f * n
			for bit := nb; bit < n*8; bit++ {
				raw[base+bit/8] &^= 1 << uint(7-bit%8)
			}
		}

		// C reference.
		cmd := exec.Command(bin, itoa(mode))
		cmd.Stdin = bytes.NewReader(raw)
		var cout bytes.Buffer
		cmd.Stdout = &cout
		if err := cmd.Run(); err != nil {
			t.Fatalf("mode %d: C harness: %v", mode, err)
		}
		cpcm := make([]int16, cout.Len()/2)
		binary.Read(&cout, binary.LittleEndian, &cpcm)
		if len(cpcm) != frames*FrameSamples {
			t.Fatalf("mode %d: C produced %d samples, want %d", mode, len(cpcm), frames*FrameSamples)
		}

		// Go decode of the same frames.
		var st decoderState
		st.reset()
		gpcm := make([]int16, 0, frames*FrameSamples)
		for f := 0; f < frames; f++ {
			data := raw[f*n : f*n+n]
			synth := make([]int16, cAMR_WB_PCM_FRAME)
			params := mimeUnsort(data, int16(mode))
			st.dsp.decodeAmrWb(int16(mode), params, cRX_SPEECH_GOOD, synth)
			gpcm = append(gpcm, synth...)
		}

		// Compare (the C reference masks to 14-bit; apply the same mask).
		mism, firstAt, maxDiff, leadExact := 0, -1, 0, -1
		for i := range cpcm {
			d := int(gpcm[i]&^3) - int(cpcm[i])
			if d != 0 {
				mism++
				if firstAt < 0 {
					firstAt = i
					leadExact = i
				}
				if d < 0 {
					d = -d
				}
				if d > maxDiff {
					maxDiff = d
				}
			}
		}
		if mism != 0 {
			t.Errorf("mode %d: %d/%d differ, leadExact=%d, maxDiff=%d (first %d: go=%d c=%d)",
				mode, mism, len(cpcm), leadExact, maxDiff, firstAt, gpcm[firstAt]&^3, cpcm[firstAt])
		} else {
			t.Logf("mode %d: bit-exact over %d frames", mode, frames)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
