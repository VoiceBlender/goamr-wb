package amrwb

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"testing"
)

// refFrame is one frame of a differential test stream: a frame type and its
// left-justified speech bits (empty for SPEECH_LOST / NO_DATA).
type refFrame struct {
	ft   int
	data []byte
}

// TestDiffAgainstCReference decodes identical AMR-WB streams with the public
// Decoder (both payload formats) and the stock Apache-2.0 opencore-amrwb
// library (D_IF_decode), requiring identical PCM. Set AMRWB_DIFF to the
// harness built by `make harness` (testdata/refharness/amrwbdec.c).
//
// Not covered: SID frames (comfort-noise synthesis is not ported) and Q=0
// frames (the reference wrapper ignores the Q bit; this decoder treats such
// frames as bad, per RFC 4867).
func TestDiffAgainstCReference(t *testing.T) {
	bin := os.Getenv("AMRWB_DIFF")
	if bin == "" {
		t.Skip("set AMRWB_DIFF to the opencore-amrwb reference harness to run (make harness)")
	}

	for mode := 0; mode <= 8; mode++ {
		t.Run("random/"+modeNames[mode], func(t *testing.T) {
			compareWithRef(t, bin, randomFrames(mode, 50, uint32(0x12345+mode*7)))
		})
		t.Run("speech/"+modeNames[mode], func(t *testing.T) {
			compareWithRef(t, bin, encodeFrames(t, testSignal(), func(int) int { return mode }))
		})
		t.Run("lost/"+modeNames[mode], func(t *testing.T) {
			compareWithRef(t, bin, dropFrames(encodeFrames(t, testSignal(), func(int) int { return mode }), uint32(mode)))
		})
		t.Run("homing/"+modeNames[mode], func(t *testing.T) {
			compareWithRef(t, bin, withHomingFrames(t, mode))
		})
		t.Run("stress/"+modeNames[mode], func(t *testing.T) {
			compareWithRef(t, bin, encodeFrames(t, stressSignal(), func(int) int { return mode }))
		})
	}
	t.Run("modeswitch", func(t *testing.T) {
		seed := uint32(99)
		compareWithRef(t, bin, encodeFrames(t, testSignal(), func(f int) int {
			if f%3 == 0 {
				seed = seed*1664525 + 1013904223
			}
			return int(seed>>16) % 9
		}))
	})
	t.Run("modeswitch-lost", func(t *testing.T) {
		compareWithRef(t, bin, dropFrames(encodeFrames(t, testSignal(), func(f int) int { return (f / 7) % 9 }), 7))
	})
}

// compareWithRef decodes frames with the C reference and with the public Go
// Decoder in both payload formats, failing on the first differing sample.
func compareWithRef(t *testing.T, bin string, frames []refFrame) {
	t.Helper()
	var stream []byte
	for _, f := range frames {
		stream = binary.LittleEndian.AppendUint16(stream, uint16(1+len(f.data)))
		stream = append(stream, byte(f.ft<<3|0x04))
		stream = append(stream, f.data...)
	}
	cmd := exec.Command(bin)
	cmd.Stdin = bytes.NewReader(stream)
	var cout, cerr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &cout, &cerr
	if err := cmd.Run(); err != nil {
		t.Fatalf("C harness: %v: %s", err, cerr.String())
	}
	want := make([]int16, cout.Len()/2)
	binary.Read(&cout, binary.LittleEndian, want)
	if len(want) != len(frames)*FrameSamples {
		t.Fatalf("C produced %d samples, want %d", len(want), len(frames)*FrameSamples)
	}

	for _, octet := range []bool{true, false} {
		d := NewDecoder(DecoderConfig{OctetAligned: octet})
		got := make([]int16, FrameSamples)
		for i, f := range frames {
			fr := []frame{{ft: f.ft, q: true, data: f.data}}
			var p []byte
			if octet {
				p = packPayloadOctet(noCMR, fr)
			} else {
				p = packPayloadBandwidthEfficient(noCMR, fr)
			}
			n, err := d.DecodeTo(got, p)
			if err != nil || n != FrameSamples {
				t.Fatalf("octet=%v frame %d (ft %d): DecodeTo = %d, %v", octet, i, f.ft, n, err)
			}
			ref := want[i*FrameSamples : (i+1)*FrameSamples]
			for j := range got {
				if got[j] != ref[j] {
					var prev []int
					for k := max(0, i-4); k < i; k++ {
						prev = append(prev, frames[k].ft)
					}
					t.Fatalf("octet=%v frame %d (ft %d, previous fts %v) sample %d: go=%d c=%d",
						octet, i, f.ft, prev, j, got[j], ref[j])
				}
			}
		}
	}
}

// randomFrames returns n frames of uniformly random speech bits for mode.
func randomFrames(mode, n int, seed uint32) []refFrame {
	frames := make([]refFrame, n)
	nbits := modeBits[mode]
	for i := range frames {
		data := make([]byte, (nbits+7)/8)
		for j := range data {
			seed = seed*1664525 + 1013904223
			data[j] = byte(seed >> 16)
		}
		if r := nbits % 8; r != 0 {
			data[len(data)-1] &= 0xFF << uint(8-r)
		}
		frames[i] = refFrame{ft: mode, data: data}
	}
	return frames
}

// encodeFrames encodes pcm with the vo-amrwbenc reference encoder (AMRWB_ENC),
// choosing each frame's mode with modeOf, and returns the frames. The
// reference encoder is used so these tests exercise the decoder on valid
// streams, VAD flags included, independently of the Go encoder.
func encodeFrames(t *testing.T, pcm []int16, modeOf func(frame int) int) []refFrame {
	t.Helper()
	bin := os.Getenv("AMRWB_ENC")
	if bin == "" {
		t.Skip("set AMRWB_ENC to the vo-amrwbenc reference harness to run (make harness)")
	}
	var in []byte
	for f := 0; (f+1)*FrameSamples <= len(pcm); f++ {
		in = append(in, byte(modeOf(f)))
		for _, v := range pcm[f*FrameSamples : (f+1)*FrameSamples] {
			in = binary.LittleEndian.AppendUint16(in, uint16(v))
		}
	}
	cmd := exec.Command(bin)
	cmd.Stdin = bytes.NewReader(in)
	var cout, cerr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &cout, &cerr
	if err := cmd.Run(); err != nil {
		t.Fatalf("C encoder harness: %v: %s", err, cerr.String())
	}
	var frames []refFrame
	for b := cout.Bytes(); len(b) > 0; {
		ft := int(b[0]>>3) & 0x0F
		n, ok := frameBytesByFT(ft)
		if !ok || len(b) < 1+n {
			t.Fatalf("C encoder: bad storage frame (ft %d) at frame %d", ft, len(frames))
		}
		frames = append(frames, refFrame{ft: ft, data: b[1 : 1+n]})
		b = b[1+n:]
	}
	if len(frames) != len(pcm)/FrameSamples {
		t.Fatalf("C encoder produced %d frames, want %d", len(frames), len(pcm)/FrameSamples)
	}
	return frames
}

// dropFrames replaces frames with SPEECH_LOST / NO_DATA: the first frame,
// bursts of 6 and 20, and scattered single losses.
func dropFrames(frames []refFrame, seed uint32) []refFrame {
	out := append([]refFrame(nil), frames...)
	lost := func(i int) {
		seed = seed*1664525 + 1013904223
		if seed>>31 == 0 {
			out[i] = refFrame{ft: FTSpeechLost}
		} else {
			out[i] = refFrame{ft: FTNoData}
		}
	}
	lost(0)
	for i := 40; i < 46 && i < len(out); i++ {
		lost(i)
	}
	for i := 300; i < 320 && i < len(out); i++ { // long burst: deep concealment
		lost(i)
	}
	for i := 1; i < len(out); i++ {
		seed = seed*1664525 + 1013904223
		if seed>>16%10 == 0 {
			lost(i)
		}
	}
	return out
}

// homingFrame returns the decoder homing frame of mode in payload bit order.
func homingFrame(mode int) []byte {
	nbits := int(unpacked_size[mode])
	dhf := dhfTables[mode]
	prms := make([]int16, nbits)
	for i := range prms {
		prms[i] = cBIT_0
		if w := i / 15; w < len(dhf) && dhf[w]>>uint(14-i%15)&1 != 0 {
			prms[i] = cBIT_1
		}
	}
	return packFrame(prms, int16(mode))
}

// withHomingFrames returns speech with decoder homing frames at the start, in
// the middle, and twice in a row.
func withHomingFrames(t *testing.T, mode int) []refFrame {
	speech := encodeFrames(t, testSignal()[:60*FrameSamples], func(int) int { return mode })
	h := refFrame{ft: mode, data: homingFrame(mode)}
	var out []refFrame
	out = append(out, h, h, h)
	out = append(out, speech[:20]...)
	out = append(out, h)
	out = append(out, speech[20:40]...)
	out = append(out, h, h)
	out = append(out, speech[40:]...)
	return out
}

// testSignal returns a deterministic speech-like signal: voiced segments with a
// gliding pitch and moving formants, noise bursts and silence, at levels from
// quiet to clipped.
func testSignal() []int16 {
	const sr = 16000.0
	seed := uint32(1)
	noise := func() float64 {
		seed = seed*1664525 + 1013904223
		return float64(int32(seed)) / (1 << 31)
	}
	var sig []float64

	// One syllable: pulse train through two resonators, with an envelope.
	type res struct{ y1, y2 float64 }
	resonate := func(r *res, x, f, bw float64) float64 {
		a := math.Exp(-math.Pi * bw / sr)
		b1 := 2 * a * math.Cos(2*math.Pi*f/sr)
		y := x + b1*r.y1 - a*a*r.y2
		r.y2, r.y1 = r.y1, y
		return y
	}
	for syl := 0; syl < 12; syl++ {
		n := int(0.18 * sr)
		f0 := 110.0 + 15*float64(syl%5)
		f1, f2 := 500.0+60*float64(syl%4), 1500.0+200*float64(syl%3)
		var r1, r2 res
		phase := 0.0
		for i := 0; i < n; i++ {
			pos := float64(i) / float64(n)
			f := f0 * (1 + 0.2*pos)
			phase += f / sr
			x := 0.0
			if phase >= 1 {
				phase--
				x = 1
			}
			x += 0.02 * noise()
			y := resonate(&r1, x, f1+300*pos, 90) + 0.5*resonate(&r2, x, f2-200*pos, 120)
			sig = append(sig, y*math.Sin(math.Pi*pos))
		}
		if syl%4 == 3 { // fricative
			for i := 0; i < int(0.08*sr); i++ {
				sig = append(sig, 0.3*noise())
			}
		}
		if syl%6 == 5 { // pause
			sig = append(sig, make([]float64, int(0.1*sr))...)
		}
	}
	peak := 0.0
	for _, v := range sig {
		peak = math.Max(peak, math.Abs(v))
	}

	// Repeat at increasing peak levels; the last is driven into clipping.
	var out []int16
	for _, level := range []float64{0.003, 0.05, 0.3, 0.9, 2.0} {
		for _, v := range sig {
			s := v / peak * level * 32767
			out = append(out, int16(math.Max(-32768, math.Min(32767, s))))
		}
	}
	return out[:len(out)/FrameSamples*FrameSamples]
}

// stressSignal returns deterministic pathological input: stationary noise at
// several levels (VAD noise tracking), silence, full-scale and near-full-scale
// tones (pitch-gain clipping, unstable LP filters), a square wave, an impulse
// train, a DC step and a frequency sweep.
func stressSignal() []int16 {
	const sr = 16000.0
	seed := uint32(7)
	noise := func() float64 {
		seed = seed*1664525 + 1013904223
		return float64(int32(seed)) / (1 << 31)
	}
	var out []int16
	add := func(n int, f func(i int) float64) {
		for i := 0; i < n; i++ {
			v := f(i)
			out = append(out, int16(math.Max(-32768, math.Min(32767, v))))
		}
	}
	sec := int(sr)
	add(3*sec, func(int) float64 { return 1000 * noise() }) // stationary noise
	add(sec/2, func(int) float64 { return 0 })              // silence
	add(2*sec, func(int) float64 { return 100 * noise() })  // quiet noise
	add(sec, func(i int) float64 { return 32767 * math.Sin(2*math.Pi*1000*float64(i)/sr) })
	add(sec, func(i int) float64 { return 29000 * math.Sin(2*math.Pi*200*float64(i)/sr) })
	add(sec, func(i int) float64 { return 20000 * math.Sin(2*math.Pi*3500*float64(i)/sr) })
	add(sec, func(i int) float64 { // square wave, 100 Hz, full scale
		if (i/80)%2 == 0 {
			return 32767
		}
		return -32768
	})
	add(sec, func(i int) float64 { // impulse train, 125 Hz
		if i%128 == 0 {
			return 32767
		}
		return 0
	})
	add(sec/2, func(int) float64 { return 20000 }) // DC step
	add(sec/2, func(int) float64 { return 0 })
	add(2*sec, func(i int) float64 { // sweep 50 Hz .. 7 kHz
		t := float64(i) / sr
		return 25000 * math.Sin(2*math.Pi*(50*t+(7000-50)/(2*2.0)*t*t))
	})
	add(2*sec, func(int) float64 { return 30000 * noise() }) // loud noise
	return out[:len(out)/FrameSamples*FrameSamples]
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
