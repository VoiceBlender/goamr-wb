package amrwb

// This file holds the AMR-WB signal-processing state and the entry points the
// public Encoder/Decoder call per 20 ms frame. The fixed-point DSP itself
// (LP analysis/ISF quantization, open/closed-loop pitch, algebraic codebook
// search, gain quantization, synthesis and post-filtering, HF band, DTX) is a
// large bit-exact port of 3GPP TS 26.190 — see opencore-amrwb (decoder) and
// vo-amrwbenc (encoder), both Apache-2.0.
//
// The structs below define where that ported state lives. The encodeFrame /
// decodeFrame methods are the single integration seam where the per-block DSP
// functions are orchestrated per 20 ms frame.

// encoderState holds the persistent encoder analysis state across frames.
type encoderState struct {
	st      coderState
	started bool
	inBuf   [cAMR_WB_PCM_FRAME]int16 // masked-input scratch, reused per frame
	prmsBuf [cNBBITS_24k]int16       // serial-parameter scratch, reused per frame
}

func (s *encoderState) reset() {
	s.st.reset(true)
	s.started = true
}

// encodeFrame analyses one 320-sample frame in the given mode and returns the
// RFC 4867 frame payload bytes (the sorted/packed speech bits, without the
// CMR/ToC header which the caller adds).
//
// All nine speech modes (6.60-23.85 kbit/s) are bit-exact against the
// vo-amrwbenc reference for active speech (see TestEncDiffAgainstCReference).
// The VAD is stubbed to always-active, so the vad_flag bit and DTX/SID encoding
// are not reference-accurate on silence/noise; the decoder ignores vad_flag.
func (s *encoderState) encodeFrame(mode Mode, samples []int16) ([]byte, error) {
	if !s.started {
		s.reset()
	}
	// The reference encoder masks the 2 LSBs of every input sample (14-bit
	// input convention) before analysis; replicate on a (reused) copy.
	in := s.inBuf[:len(samples)]
	for i, v := range samples {
		in[i] = v &^ 3
	}
	// coder fills exactly amrwbCompressed[mode] serial bits; clear the reused
	// buffer first so any unwritten tail matches the original zero-filled slice.
	prms := s.prmsBuf[:amrwbCompressed[mode]]
	for i := range prms {
		prms[i] = 0
	}
	m := int16(mode)
	s.st.coder(&m, in, prms, 0)
	return packFrame(prms, int16(mode)), nil
}

// decoderState holds the persistent decoder synthesis state across frames.
type decoderState struct {
	dsp      decoderStateDSP
	lastMode Mode
	started  bool
}

func (s *decoderState) reset() {
	s.dsp.reset(true)
	s.lastMode = Mode2385
	s.started = true
}

// decodeFrame decodes one parsed AMR-WB frame into 320 PCM samples. Speech
// frames are fully decoded; SID/NO_DATA/SPEECH_LOST currently emit silence /
// concealment (CNG synthesis not yet ported).
//
// NOTE: the speech bits are expanded MSB-first directly into the decoder's
// parameter order. AMR-WB's RTP payload bit order (RFC 4867) must match the
// reference Serial_parm order; this is validated against the 3GPP TS 26.174
// test vectors before AMR-WB is offered in SDP.
func (s *decoderState) decodeFrame(f frame) ([]int16, error) {
	if !s.started {
		s.reset()
	}
	synth := make([]int16, cAMR_WB_PCM_FRAME)

	var mode Mode
	var frameType int16
	switch {
	case f.ft <= FTMode2385: // speech
		mode = Mode(f.ft)
		s.lastMode = mode
		if f.q {
			frameType = cRX_SPEECH_GOOD
		} else {
			frameType = cRX_SPEECH_BAD
		}
	case f.ft == FTSID:
		mode = Mode(dMRDTX)
		frameType = cRX_SID_UPDATE
	case f.ft == FTSpeechLost:
		mode = s.lastMode
		frameType = cRX_SPEECH_LOST
	default: // FTNoData and anything else
		mode = s.lastMode
		frameType = cRX_NO_DATA
	}

	// Build the per-bit parameter array via RFC 4867 de-sorting. For frames
	// without usable bits, a zero-filled array keeps the bit cursor in sync
	// (values are ignored by the concealment paths).
	var params []int16
	switch frameType {
	case cRX_SPEECH_GOOD, cRX_SPEECH_BAD:
		params = mimeUnsort(f.data, int16(mode))
	case cRX_SID_UPDATE:
		params = mimeUnsort(f.data, int16(dMRDTX))
	default:
		params = make([]int16, amrwbCompressed[mode])
	}

	s.dsp.decodeAmrWb(int16(mode), params, frameType, synth)
	return synth, nil
}
