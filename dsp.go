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
	tx      txState
	dtx     bool // allow_dtx: emit SID and NO_DATA frames on non-speech
	started bool
	inBuf   [cAMR_WB_PCM_FRAME]int16 // masked-input scratch, reused per frame
	prmsBuf [cNBBITS_24k]int16       // serial-parameter scratch, reused per frame
}

func (s *encoderState) reset() {
	s.st.reset(true)
	s.tx.reset()
	s.started = true
}

// encodeFrame analyses one 320-sample frame in the given mode and returns the
// RFC 4867 frame payload bytes (the sorted/packed speech bits, without the
// CMR/ToC header which the caller adds).
//
// Bit-exactness against vo-amrwbenc is checked by TestEncDiffAgainstCReference,
// TestEncSpeechDiffAgainstCReference and TestEncDTXDiffAgainstCReference.
func (s *encoderState) encodeFrame(mode Mode, samples []int16) ([]byte, error) {
	out := make([]byte, packed_size[mode])
	f, err := s.encodeFrameInto(out, mode, samples)
	return f.data, err
}

// encodeFrameInto is encodeFrame writing the packed frame into dst, which must
// hold at least packed_size[mode] bytes. It returns the frame, which with DTX
// on may be a SID or a NO_DATA frame, and does not allocate.
func (s *encoderState) encodeFrameInto(dst []byte, mode Mode, samples []int16) (frame, error) {
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
	var allowDtx int16
	if s.dtx {
		allowDtx = 1
	}
	m := int16(mode)
	s.st.coder(&m, in, prms, allowDtx)
	switch txType := s.tx.frameType(m); txType {
	case cTX_SPEECH:
		return frame{ft: mode.FrameType(), q: true, data: packFrameInto(dst, prms, m)}, nil
	case cTX_NO_DATA:
		return frame{ft: FTNoData, q: true}, nil
	default:
		return frame{ft: FTSID, q: true, data: packSIDInto(dst, prms, txType, int16(mode))}, nil
	}
}

// decoderState holds the persistent decoder synthesis state across frames,
// plus the frame-handling state of the reference D_IF_decode wrapper.
type decoderState struct {
	dsp          decoderStateDSP
	prevMode     Mode // last mode decoded; reused for SID/SPEECH_LOST/NO_DATA
	resetFlag    bool // current frame is a decoder homing frame
	resetFlagOld bool // previous frame was a decoder homing frame (or fresh state)
	started      bool
	params       [cNBBITS_24k]int16 // de-sorted parameter scratch, reused per frame
}

func (s *decoderState) reset() {
	s.dsp.reset(true)
	s.prevMode = Mode0660
	s.resetFlag = false
	s.resetFlagOld = true
	s.started = true
}

// decodeFrame decodes one parsed AMR-WB frame into 320 PCM samples.
func (s *decoderState) decodeFrame(f frame) ([]int16, error) {
	synth := make([]int16, cAMR_WB_PCM_FRAME)
	if err := s.decodeFrameInto(f, synth); err != nil {
		return nil, err
	}
	return synth, nil
}

// decodeFrameInto is decodeFrame writing the 320 samples into synth, which
// must hold at least cAMR_WB_PCM_FRAME samples. It does not allocate.
//
// It mirrors D_IF_decode (opencore-amrwb wrapper.cpp) and mime_unsorting
// (mime_io.cpp): decoder homing frames, 14-bit output, and the mode used for
// frames without speech bits. One deliberate difference: the reference wrapper
// ignores the ToC Q bit, while this decoder treats Q=0 frames as bad frames
// (RFC 4867, section 4.3.2).
func (s *decoderState) decodeFrameInto(f frame, synth []int16) error {
	if !s.started {
		s.reset()
	}
	synth = synth[:cAMR_WB_PCM_FRAME]

	// Frames without bits leave the parameter buffer as it was, as in the
	// reference; the concealment paths do not use those values.
	params := s.params[:]
	mode := s.prevMode
	var frameType int16
	switch {
	case f.ft <= FTMode2385: // speech
		mode = Mode(f.ft)
		mimeUnsortInto(params, f.data, int16(mode))
		if f.q {
			frameType = cRX_SPEECH_GOOD
		} else {
			frameType = cRX_SPEECH_BAD
		}
	case f.ft == FTSID:
		mimeUnsortInto(params, f.data, int16(dMRDTX))
		switch {
		case !f.q:
			frameType = cRX_SID_BAD
		case len(f.data) > 4 && f.data[4]&0x10 != 0: // STI bit
			frameType = cRX_SID_UPDATE
		default:
			frameType = cRX_SID_FIRST
		}
	case f.ft == FTSpeechLost:
		frameType = cRX_SPEECH_LOST
	default: // FTNoData and anything else
		frameType = cRX_NO_DATA
	}
	s.prevMode = mode

	if frameType == cRX_NO_DATA || frameType == cRX_SPEECH_LOST {
		s.resetFlag = false
	} else if s.resetFlagOld {
		// Homed: check whether this frame is another homing frame (first
		// subframe only).
		s.resetFlag = homingFrameTestFirst(params, int16(mode))
	}

	if s.resetFlag && s.resetFlagOld {
		// Homed and still receiving homing frames: output the encoder homing
		// frame without decoding.
		for i := range synth {
			synth[i] = cEHF_MASK
		}
	} else {
		s.dsp.decodeAmrWb(int16(mode), params, frameType, synth)
	}

	for i := range synth { // delete the 2 LSBs (14-bit output)
		synth[i] &^= 3
	}

	if !s.resetFlagOld {
		// Not homed: check whether the whole frame is a homing frame.
		s.resetFlag = homingFrameTest(params, int16(mode))
	}
	if s.resetFlag {
		s.dsp.reset(true)
	}
	s.resetFlagOld = s.resetFlag
	return nil
}
