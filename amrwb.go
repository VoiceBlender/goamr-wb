package amrwb

import "errors"

// Encoder encodes 16 kHz mono PCM into AMR-WB RFC 4867 RTP payloads.
//
// It satisfies the codec.Encoder interface: Encode takes one 20 ms frame
// (320 int16 samples) and returns a complete RTP payload (CMR + ToC + speech
// bits) in the configured payload format.
type Encoder struct {
	mode         Mode
	octetAligned bool
	st           encoderState
}

// EncoderConfig configures a new AMR-WB encoder.
type EncoderConfig struct {
	Mode         Mode // active speech mode (0..8)
	OctetAligned bool // true: octet-aligned framing; false: bandwidth-efficient
}

// NewEncoder creates an AMR-WB encoder for the given mode and payload format.
func NewEncoder(cfg EncoderConfig) (*Encoder, error) {
	if !cfg.Mode.Valid() {
		return nil, errors.New("amrwb: invalid encoder mode")
	}
	e := &Encoder{mode: cfg.Mode, octetAligned: cfg.OctetAligned}
	e.st.reset()
	return e, nil
}

// Encode encodes one 20 ms frame (320 samples) into an RTP payload.
func (e *Encoder) Encode(samples []int16) ([]byte, error) {
	if len(samples) != FrameSamples {
		return nil, errors.New("amrwb: encode expects exactly 320 samples")
	}
	bits, err := e.st.encodeFrame(e.mode, samples)
	if err != nil {
		return nil, err
	}
	f := frame{ft: e.mode.FrameType(), q: true, data: bits}
	if e.octetAligned {
		return packPayloadOctet(noCMR, []frame{f}), nil
	}
	return packPayloadBandwidthEfficient(noCMR, []frame{f}), nil
}

// SetMode changes the active speech mode (e.g. in response to a peer CMR).
func (e *Encoder) SetMode(m Mode) {
	if m.Valid() {
		e.mode = m
	}
}

// Reset clears all encoder state.
func (e *Encoder) Reset() { e.st.reset() }

// Decoder decodes AMR-WB RFC 4867 RTP payloads into 16 kHz mono PCM.
type Decoder struct {
	octetAligned bool
	st           decoderState
}

// DecoderConfig configures a new AMR-WB decoder.
type DecoderConfig struct {
	OctetAligned bool // must match the negotiated payload format
}

// NewDecoder creates an AMR-WB decoder for the given payload format.
func NewDecoder(cfg DecoderConfig) *Decoder {
	d := &Decoder{octetAligned: cfg.OctetAligned}
	d.st.reset()
	return d
}

// Decode decodes one RTP payload, returning concatenated PCM for all frames it
// carries (one 320-sample frame per speech/SID/no-data ToC entry).
func (d *Decoder) Decode(payload []byte) ([]int16, error) {
	var (
		frames []frame
		err    error
	)
	if d.octetAligned {
		_, frames, err = unpackPayloadOctet(payload)
	} else {
		_, frames, err = unpackPayloadBandwidthEfficient(payload)
	}
	if err != nil {
		return nil, err
	}
	out := make([]int16, 0, len(frames)*FrameSamples)
	for _, f := range frames {
		pcm, derr := d.st.decodeFrame(f)
		if derr != nil {
			return nil, derr
		}
		out = append(out, pcm...)
	}
	return out, nil
}

// Reset clears all decoder state.
func (d *Decoder) Reset() { d.st.reset() }
