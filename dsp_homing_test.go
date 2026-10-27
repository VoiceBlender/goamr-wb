package amrwb

import "testing"

// A fresh decoder is homed, so decoder homing frames produce the encoder
// homing pattern without decoding; after speech, a homing frame is decoded
// normally and resets the decoder, so the next homing frame again produces the
// pattern.
func TestDecoderHomingFrames(t *testing.T) {
	for mode := 0; mode <= 8; mode++ {
		h := homingFrame(mode)
		if !homingFrameTest(mimeUnsort(h, int16(mode)), int16(mode)) {
			t.Fatalf("mode %d: homing frame not detected", mode)
		}
		speech := randomFrames(mode, 1, uint32(7+mode))[0].data
		if homingFrameTest(mimeUnsort(speech, int16(mode)), int16(mode)) {
			t.Fatalf("mode %d: random frame detected as homing frame", mode)
		}

		d := NewDecoder(DecoderConfig{OctetAligned: true})
		decode := func(data []byte) []int16 {
			out, err := d.Decode(packPayloadOctet(noCMR, []frame{{ft: mode, q: true, data: data}}))
			if err != nil {
				t.Fatal(err)
			}
			return out
		}
		isPattern := func(pcm []int16) bool {
			for _, v := range pcm {
				if v != cEHF_MASK {
					return false
				}
			}
			return true
		}
		if !isPattern(decode(h)) {
			t.Fatalf("mode %d: first homing frame did not output the homing pattern", mode)
		}
		if isPattern(decode(speech)) {
			t.Fatalf("mode %d: speech frame output the homing pattern", mode)
		}
		if isPattern(decode(h)) {
			t.Fatalf("mode %d: homing frame after speech was not decoded", mode)
		}
		if !isPattern(decode(h)) {
			t.Fatalf("mode %d: homing frame after reset did not output the homing pattern", mode)
		}
	}
}

// Decoded PCM is 14-bit: the two LSBs are always zero.
func TestDecoderOutputIs14Bit(t *testing.T) {
	d := NewDecoder(DecoderConfig{OctetAligned: true})
	for _, f := range randomFrames(8, 20, 1) {
		out, err := d.Decode(packPayloadOctet(noCMR, []frame{{ft: f.ft, q: true, data: f.data}}))
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range out {
			if v&3 != 0 {
				t.Fatalf("sample %d = %d has nonzero LSBs", i, v)
			}
		}
	}
}
