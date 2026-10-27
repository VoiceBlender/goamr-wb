package amrwb

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"testing"
)

// TestEncDiffAgainstCReference encodes identical PCM frames with this Go encoder
// and the Apache-2.0 vo-amrwbenc C reference, comparing the packed bitstream.
// Set AMRWB_ENC to the compiled reference harness (reads 320 int16 LE/frame on
// stdin for the mode given as argv[1], writes storage-format frames: 1 ToC byte
// + packed_size bytes each).
func TestEncDiffAgainstCReference(t *testing.T) {
	bin := os.Getenv("AMRWB_ENC")
	if bin == "" {
		t.Skip("set AMRWB_ENC to the vo-amrwbenc reference harness to run")
	}
	const frames = 20

	// Deterministic speech-like input (sum of tones).
	raw := new(bytes.Buffer)
	pcm := make([][]int16, frames)
	phase := 0.0
	for f := 0; f < frames; f++ {
		s := make([]int16, 320)
		for i := range s {
			v := 5000*sinApprox(phase) + 2000*sinApprox(phase*2.7)
			s[i] = int16(v)
			phase += 2 * 3.14159265 * 300 / 16000
		}
		pcm[f] = s
		binary.Write(raw, binary.LittleEndian, s)
	}

	for mode := 0; mode <= 8; mode++ {
		mode := mode
		t.Run(modeNames[mode], func(t *testing.T) {
			packed := int((amrwbCompressed[mode] + 7) / 8) // speech bytes, excl. ToC

			cmd := exec.Command(bin, itoa(mode))
			cmd.Stdin = bytes.NewReader(raw.Bytes())
			var cout bytes.Buffer
			cmd.Stdout = &cout
			if err := cmd.Run(); err != nil {
				t.Fatalf("C encoder harness: %v", err)
			}
			cbytes := cout.Bytes()
			frameLen := 1 + packed

			var enc encoderState
			enc.reset()
			for f := 0; f < frames; f++ {
				goPacked, _ := enc.encodeFrame(Mode(mode), pcm[f])
				cFrame := cbytes[f*frameLen : (f+1)*frameLen]
				cPacked := cFrame[1:] // skip ToC byte
				if !bytes.Equal(goPacked, cPacked) {
					// Localize: find the first differing INTERNAL-order parameter bit.
					gp := mimeUnsort(goPacked, int16(mode))
					cp := mimeUnsort(cPacked, int16(mode))
					first := -1
					for i := range gp {
						if (gp[i] == cBIT_1) != (cp[i] == cBIT_1) {
							first = i
							break
						}
					}
					field := "?"
					if mode == 0 {
						field = fieldOf(first)
					}
					t.Fatalf("frame %d: diverges at internal bit %d (%s)", f, first, field)
				}
			}
			t.Logf("mode %d: all %d frames bit-exact", mode, frames)
		})
	}
}

var modeNames = []string{
	"6.60k", "8.85k", "12.65k", "14.25k", "15.85k",
	"18.25k", "19.85k", "23.05k", "23.85k",
}

// fieldOf maps an internal mode-0 parameter bit index to its field name.
func fieldOf(i int) string {
	// vad(1) isf(36) then per-subframe: sf0 pitch8 cb12 gain6; sf1 pitch5 cb12 gain6; ...
	if i < 0 {
		return "?"
	}
	if i < 1 {
		return "vad"
	}
	if i < 37 {
		return "ISF"
	}
	p := i - 37
	// subframe layout for mode 0: [pitchN, cb12, gain6]; pitchN=8 for sf0/sf2, 5 for sf1/sf3
	sf := 0
	for _, pn := range []int{8, 5, 8, 5} {
		blk := pn + 12 + 6
		if p < blk {
			if p < pn {
				return "sf" + itoa(sf) + ".pitch"
			}
			if p < pn+12 {
				return "sf" + itoa(sf) + ".code"
			}
			return "sf" + itoa(sf) + ".gain"
		}
		p -= blk
		sf++
	}
	return "?tail"
}

// TestEncSpeechDiffAgainstCReference encodes speech-like input from quiet to
// clipped (testSignal), and pathological input (stressSignal), with the Go
// encoder and with vo-amrwbenc (AMRWB_ENC),
// requiring identical frames in every mode, including the VAD flag.
func TestEncSpeechDiffAgainstCReference(t *testing.T) {
	for _, c := range []struct {
		name string
		sig  []int16
	}{{"speech", testSignal()}, {"stress", stressSignal()}} {
		for mode := 0; mode <= 8; mode++ {
			t.Run(c.name+"/"+modeNames[mode], func(t *testing.T) {
				encCompare(t, c.sig, mode)
			})
		}
	}
}

// encCompare encodes sig in mode with the Go encoder and vo-amrwbenc and
// requires identical frames.
func encCompare(t *testing.T, sig []int16, mode int) {
	t.Helper()
	ref := encodeFrames(t, sig, func(int) int { return mode })
	var enc encoderState
	enc.reset()
	for f := range ref {
		var got []byte
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("frame %d: encoder panic: %v", f, r)
				}
			}()
			got, _ = enc.encodeFrame(Mode(mode), sig[f*FrameSamples:(f+1)*FrameSamples])
		}()
		if !bytes.Equal(got, ref[f].data) {
			gp := mimeUnsort(got, int16(mode))
			cp := mimeUnsort(ref[f].data, int16(mode))
			first := -1
			for i := range gp {
				if (gp[i] == cBIT_1) != (cp[i] == cBIT_1) {
					first = i
					break
				}
			}
			t.Fatalf("frame %d: diverges at internal bit %d", f, first)
		}
	}
}

// TestEncAPIDiffAgainstCReference drives the public Encoder (both payload
// formats, switching modes with SetMode) and vo-amrwbenc's E_IF_encode
// (AMRWB_ENC) with the same PCM, requiring identical frames.
func TestEncAPIDiffAgainstCReference(t *testing.T) {
	sig := testSignal()
	modeOf := func(f int) int {
		if f < 200 {
			return 8
		}
		return (f / 7) % 9
	}
	ref := encodeFrames(t, sig, modeOf)
	for _, octet := range []bool{true, false} {
		enc, err := NewEncoder(EncoderConfig{Mode: Mode(modeOf(0)), OctetAligned: octet})
		if err != nil {
			t.Fatal(err)
		}
		var pp payloadParser
		dst := make([]byte, MaxPayloadBytes)
		for f := range ref {
			enc.SetMode(Mode(modeOf(f)))
			n, err := enc.EncodeTo(dst, sig[f*FrameSamples:(f+1)*FrameSamples])
			if err != nil {
				t.Fatalf("octet=%v frame %d: %v", octet, f, err)
			}
			var frames []frame
			if octet {
				frames, err = pp.parseOctet(dst[:n])
			} else {
				frames, err = pp.parseBandwidthEfficient(dst[:n])
			}
			if err != nil || len(frames) != 1 {
				t.Fatalf("octet=%v frame %d: parse: %d frames, %v", octet, f, len(frames), err)
			}
			if frames[0].ft != ref[f].ft || !bytes.Equal(frames[0].data, ref[f].data) {
				t.Fatalf("octet=%v frame %d (mode %d): differs from the reference", octet, f, modeOf(f))
			}
		}
	}
}
