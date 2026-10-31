package amrwb

import (
	"math"
	"testing"
)

// dtxFlag, set in a mode byte of the encoder harness, turns DTX on for that
// frame (testdata/refharness/amrwbenc.c).
const dtxFlag = 0x80

// sidCNDithBit is the internal parameter bit of a SID frame that carries the
// noise stationarity flag: 6+6+6+5+5 ISF bits, then 6 energy bits.
const sidCNDithBit = 34

// dtxSignal returns a deterministic signal built to exercise DTX: speech bursts
// between long stationary noise (hangover, SID_FIRST, SID_UPDATE, NO_DATA),
// digital silence, and noise that keeps changing level or spectrum (comfort
// noise dithering).
func dtxSignal() []int16 {
	seed := uint32(11)
	noise := func() float64 {
		seed = seed*1664525 + 1013904223
		return float64(int32(seed)) / (1 << 31)
	}
	var out []int16
	add := func(frames int, f func(i int) float64) {
		for i := 0; i < frames*FrameSamples; i++ {
			out = append(out, int16(math.Max(-32768, math.Min(32767, f(i)))))
		}
	}
	white := func(amp float64) func(int) float64 {
		return func(int) float64 { return amp * noise() }
	}
	// Speech from the 0.3 level of testSignal, which comes third.
	sp := testSignal()
	sp = sp[2*len(sp)/5 : 3*len(sp)/5]
	speech := func(frames, start int) {
		add(frames, func(i int) float64 {
			return float64(sp[(start*FrameSamples+i)%len(sp)]) + 100*noise()
		})
	}

	add(120, white(300))
	speech(40, 0)
	add(160, white(300))
	speech(30, 40)
	add(120, func(int) float64 { return 0 }) // digital silence
	speech(30, 70)
	add(100, white(1000))
	// Noise level drifting across SID periods: energy dithering.
	var lvl float64
	add(120, func(i int) float64 {
		lvl = 600 + 500*math.Sin(2*math.Pi*float64(i)/(16*FrameSamples))
		return lvl * noise()
	})
	speech(30, 100)
	// Noise switching spectrum every 12 frames: ISF dithering.
	var lp, prev float64
	add(144, func(i int) float64 {
		x := 800 * noise()
		lp += 0.15 * (x - lp)
		h := x - prev
		prev = x
		if (i/(12*FrameSamples))%2 == 0 {
			return 5 * lp
		}
		return 0.7 * h
	})
	speech(20, 130)
	add(100, white(30)) // quiet noise
	add(100, white(4000))
	return out
}

// checkDTXCoverage fails unless frames, a reference-encoded DTX stream, reaches
// every frame type and both noise stationarity flags, so the DTX tests cannot
// quietly degrade into all-speech streams.
func checkDTXCoverage(t *testing.T, frames []refFrame) {
	t.Helper()
	var speech, sidFirst, sidUpdate, noData, stationary, nonStationary int
	for _, f := range frames {
		switch {
		case f.ft <= FTMode2385:
			speech++
		case f.ft == FTNoData:
			noData++
		case f.ft == FTSID && f.data[4]&0x10 == 0:
			sidFirst++
		case f.ft == FTSID:
			sidUpdate++
			if mimeUnsort(f.data, dMRDTX)[sidCNDithBit] == cBIT_1 {
				nonStationary++
			} else {
				stationary++
			}
		}
	}
	if speech == 0 || sidFirst == 0 || sidUpdate == 0 || noData == 0 ||
		stationary == 0 || nonStationary == 0 {
		t.Fatalf("poor DTX coverage: %d speech, %d SID_FIRST, %d SID_UPDATE (%d stationary, %d not), %d NO_DATA",
			speech, sidFirst, sidUpdate, stationary, nonStationary, noData)
	}
}

// dtxFrames encodes dtxSignal with DTX on using the reference encoder, choosing
// each frame's mode with modeOf, and checks the stream's coverage.
func dtxFrames(t *testing.T, modeOf func(frame int) int) []refFrame {
	t.Helper()
	frames := encodeFrames(t, dtxSignal(), func(f int) int { return modeOf(f) | dtxFlag })
	checkDTXCoverage(t, frames)
	return frames
}

// starveSID replaces the first 70 frames of the longest non-speech run with
// NO_DATA, so comfort noise mutes and any later SID frames of the run arrive
// while muted.
func starveSID(frames []refFrame) []refFrame {
	out := append([]refFrame(nil), frames...)
	start, length := 0, 0
	for i := 0; i < len(out); {
		j := i
		for j < len(out) && out[j].ft > FTMode2385 {
			j++
		}
		if j-i > length {
			start, length = i, j-i
		}
		i = j + 1
	}
	for i := start + 1; i < start+70 && i < len(out); i++ {
		out[i] = refFrame{ft: FTNoData}
	}
	return out
}

// sidFrame returns a SID frame with random parameter bits, carrying sti and the
// speech mode indication in its last byte.
func sidFrame(seed *uint32, sti bool, mode int) refFrame {
	data := make([]byte, 5)
	for i := range data {
		*seed = *seed*1664525 + 1013904223
		data[i] = byte(*seed >> 16)
	}
	data[4] = data[4]&0xE0 | byte(mode)
	if sti {
		data[4] |= 0x10
	}
	return refFrame{ft: FTSID, data: data}
}

// sidScenarios returns frame sequences no encoder produces in this order: SID
// as the first frame, SID_UPDATE straight after speech, repeated SID_FIRST,
// muting, updates while muted, speech straight after muting, and SID_FIRST
// after a long loss (negative mean log energy). The run of random updates
// reaches the low ISF clamp of the comfort noise dithering.
func sidScenarios(t *testing.T, mode int) []refFrame {
	speech := encodeFrames(t, testSignal()[:120*FrameSamples], func(int) int { return mode })
	seed := uint32(0xABCDE + mode)
	update := func() refFrame { return sidFrame(&seed, true, mode) }
	first := func() refFrame { return sidFrame(&seed, false, mode) }
	var out []refFrame
	add := func(f ...refFrame) { out = append(out, f...) }
	noData := func(n int) {
		for range n {
			add(refFrame{ft: FTNoData})
		}
	}

	add(update(), first())
	add(speech[:20]...)
	add(update())
	noData(5)
	add(first(), first(), first())
	add(speech[20:30]...)
	add(first())
	noData(7)
	for range 8 { // a run of updates with varying spacing
		add(update())
		noData(int(seed>>20) % 9)
	}
	add(speech[30:40]...)
	add(first())
	noData(60) // no update for too long: mute
	add(first(), refFrame{ft: FTSpeechLost})
	add(update()) // update while muted
	noData(55)
	add(speech[40:60]...) // speech straight after muting
	add(first())
	noData(30)
	for i := range 40 { // long interpolation lengths
		if i%5 == 4 {
			add(update())
		}
		noData(7)
	}
	add(speech[60:]...)
	for range 60 { // concealment fades the excitation out
		add(refFrame{ft: FTSpeechLost})
	}
	add(first())
	noData(10)
	for range 500 {
		add(update())
		noData(1)
	}
	return out
}
