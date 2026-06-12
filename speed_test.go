package amrwb

// Speed benchmarks for the AMR-WB codec.
//
//   - BenchmarkEncode / BenchmarkDecode are standard Go micro-benchmarks (per
//     mode, no external dependencies). Each reports an "xRT" metric: how many
//     times faster than real time one core runs (a 20 ms frame's budget is
//     20 ms, so xRT = 20e6 / ns-per-frame).
//   - TestEncSpeedVsCReference / TestDecSpeedVsCReference compare the pure-Go
//     codec against the Apache-2.0 C reference. They reuse the same locally
//     built harnesses as the differential tests and SKIP unless AMRWB_ENC /
//     AMRWB_DIFF is set.
//
// Methodology for the C comparison: a subprocess run of n frames costs
// T(n) ≈ fixed + n·perFrame, where fixed = process start + IO setup. Timing two
// sizes n1 (small) and n2 (large) and taking perFrame = (T(n2)-T(n1))/(n2-n1)
// cancels fixed entirely; each size is timed `reps` times and the MIN wall-clock
// is used (least scheduler noise). Caveat: the C number still crosses a pipe to
// a separate process, so the residual per-frame stdin/stdout copy — inherent to
// driving an external binary, and absent from the in-process Go number — makes
// this a throughput-style comparison that modestly favors Go. Single core,
// deterministic synthetic speech. Tune the frame count with AMRWB_BENCH_FRAMES
// (default 4000) and repetitions with AMRWB_BENCH_REPS (default 3).

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// benchSink keeps the optimizer from eliminating encode/decode work.
var benchSink int

func benchEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// benchGenFrames builds n deterministic speech-like frames (sum of tones),
// matching the generator the differential tests use.
func benchGenFrames(n int) [][]int16 {
	out := make([][]int16, n)
	phase := 0.0
	for f := 0; f < n; f++ {
		s := make([]int16, FrameSamples)
		for i := range s {
			v := 5000*sinApprox(phase) + 2000*sinApprox(phase*2.7)
			s[i] = int16(v)
			phase += 2 * 3.14159265 * 300 / 16000
		}
		out[f] = s
	}
	return out
}

func benchPCMBytes(frames [][]int16) []byte {
	buf := make([]byte, 0, len(frames)*FrameSamples*2)
	for _, fr := range frames {
		for _, s := range fr {
			buf = append(buf, byte(s), byte(s>>8))
		}
	}
	return buf
}

// benchEncodeFrames Go-encodes every frame and returns the per-frame packed
// speech bytes (bit-exact, so valid input for the C decoder harness).
func benchEncodeFrames(mode int, frames [][]int16) [][]byte {
	var enc encoderState
	enc.reset()
	out := make([][]byte, len(frames))
	for i, fr := range frames {
		p, _ := enc.encodeFrame(Mode(mode), fr)
		cp := make([]byte, len(p))
		copy(cp, p)
		out[i] = cp
	}
	return out
}

func reportXRT(b *testing.B) {
	if b.N == 0 {
		return
	}
	nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	if nsPerOp > 0 {
		b.ReportMetric(20e6/nsPerOp, "xRT")
	}
}

func BenchmarkEncode(b *testing.B) {
	const ring = 50
	frames := benchGenFrames(ring)
	for mode := 0; mode <= 8; mode++ {
		mode := mode
		b.Run(modeNames[mode], func(b *testing.B) {
			var enc encoderState
			enc.reset()
			b.ReportAllocs()
			b.SetBytes(int64(FrameSamples * 2))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p, _ := enc.encodeFrame(Mode(mode), frames[i%ring])
				benchSink += len(p)
			}
			b.StopTimer()
			reportXRT(b)
		})
	}
}

func BenchmarkDecode(b *testing.B) {
	const ring = 50
	frames := benchGenFrames(ring)
	for mode := 0; mode <= 8; mode++ {
		mode := mode
		b.Run(modeNames[mode], func(b *testing.B) {
			packed := benchEncodeFrames(mode, frames)
			var st decoderState
			st.reset()
			synth := make([]int16, cAMR_WB_PCM_FRAME)
			b.ReportAllocs()
			b.SetBytes(int64(FrameSamples * 2))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				params := mimeUnsort(packed[i%ring], int16(mode))
				st.dsp.decodeAmrWb(int16(mode), params, cRX_SPEECH_GOOD, synth)
				benchSink += int(synth[0])
			}
			b.StopTimer()
			reportXRT(b)
		})
	}
}

// --- Go-vs-C comparison ---

func benchMinHarness(tb testing.TB, bin string, mode int, input []byte, reps int) time.Duration {
	best := time.Duration(1) << 62
	for r := 0; r < reps; r++ {
		cmd := exec.Command(bin, itoa(mode))
		cmd.Stdin = bytes.NewReader(input)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		start := time.Now()
		if err := cmd.Run(); err != nil {
			tb.Fatalf("C harness mode %d (%d frames): %v", mode, len(input), err)
		}
		if d := time.Since(start); d < best {
			best = d
		}
	}
	return best
}

// benchCPerFrameNs returns the C reference's per-frame compute (ns) via the
// two-point slope method that cancels fixed process/IO overhead.
func benchCPerFrameNs(tb testing.TB, bin string, mode, reps, n1, n2 int, makeInput func(n int) []byte) float64 {
	t1 := benchMinHarness(tb, bin, mode, makeInput(n1), reps)
	t2 := benchMinHarness(tb, bin, mode, makeInput(n2), reps)
	return float64((t2 - t1).Nanoseconds()) / float64(n2-n1)
}

func benchGoEncodeNs(mode int, frames [][]int16, reps int) float64 {
	best := time.Duration(1) << 62
	for r := 0; r < reps; r++ {
		var enc encoderState
		enc.reset()
		start := time.Now()
		for _, fr := range frames {
			p, _ := enc.encodeFrame(Mode(mode), fr)
			benchSink += len(p)
		}
		if d := time.Since(start); d < best {
			best = d
		}
	}
	return float64(best.Nanoseconds()) / float64(len(frames))
}

func benchGoDecodeNs(mode int, packed [][]byte, reps int) float64 {
	best := time.Duration(1) << 62
	synth := make([]int16, cAMR_WB_PCM_FRAME)
	for r := 0; r < reps; r++ {
		var st decoderState
		st.reset()
		start := time.Now()
		for _, pk := range packed {
			params := mimeUnsort(pk, int16(mode))
			st.dsp.decodeAmrWb(int16(mode), params, cRX_SPEECH_GOOD, synth)
			benchSink += int(synth[0])
		}
		if d := time.Since(start); d < best {
			best = d
		}
	}
	return float64(best.Nanoseconds()) / float64(len(packed))
}

func benchFrameCounts() (n1, n2, reps int) {
	n2 = benchEnvInt("AMRWB_BENCH_FRAMES", 4000)
	n1 = n2 / 20
	if n1 < 50 {
		n1 = 50
	}
	reps = benchEnvInt("AMRWB_BENCH_REPS", 3)
	return
}

func TestEncSpeedVsCReference(t *testing.T) {
	bin := os.Getenv("AMRWB_ENC")
	if bin == "" {
		t.Skip("set AMRWB_ENC to the vo-amrwbenc reference harness to run")
	}
	n1, n2, reps := benchFrameCounts()
	frames := benchGenFrames(n2)
	pcm := benchPCMBytes(frames) // n2 frames, little-endian int16

	t.Logf("encode speed: n1=%d n2=%d reps=%d (C per-frame = two-point slope, min wall)", n1, n2, reps)
	t.Logf("%-7s %12s %12s %7s %9s %9s", "mode", "go ns/f", "c ns/f", "go/c", "go xRT", "c xRT")
	for mode := 0; mode <= 8; mode++ {
		cNs := benchCPerFrameNs(t, bin, mode, reps, n1, n2, func(n int) []byte {
			return pcm[:n*FrameSamples*2]
		})
		goNs := benchGoEncodeNs(mode, frames, reps)
		t.Logf("%-7s %12.0f %12.0f %7.2f %8.1fx %8.1fx",
			modeNames[mode], goNs, cNs, ratio(goNs, cNs), xrt(goNs), xrt(cNs))
	}
}

func TestDecSpeedVsCReference(t *testing.T) {
	bin := os.Getenv("AMRWB_DIFF")
	if bin == "" {
		t.Skip("set AMRWB_DIFF to the opencore-amrwb reference harness to run")
	}
	n1, n2, reps := benchFrameCounts()
	frames := benchGenFrames(n2)

	t.Logf("decode speed: n1=%d n2=%d reps=%d (C per-frame = two-point slope, min wall)", n1, n2, reps)
	t.Logf("%-7s %12s %12s %7s %9s %9s", "mode", "go ns/f", "c ns/f", "go/c", "go xRT", "c xRT")
	for mode := 0; mode <= 8; mode++ {
		packed := benchEncodeFrames(mode, frames) // n2 valid bitstreams
		per := len(packed[0])
		raw := make([]byte, 0, n2*per)
		for _, pk := range packed {
			raw = append(raw, pk...)
		}
		cNs := benchCPerFrameNs(t, bin, mode, reps, n1, n2, func(n int) []byte {
			return raw[:n*per]
		})
		goNs := benchGoDecodeNs(mode, packed, reps)
		t.Logf("%-7s %12.0f %12.0f %7.2f %8.1fx %8.1fx",
			modeNames[mode], goNs, cNs, ratio(goNs, cNs), xrt(goNs), xrt(cNs))
	}
}

func ratio(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func xrt(nsPerFrame float64) float64 {
	if nsPerFrame == 0 {
		return 0
	}
	return 20e6 / nsPerFrame
}
