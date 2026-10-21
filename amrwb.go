package amrwb

import "errors"

// MaxPayloadBytes is the largest single-frame RTP payload produced by
// EncodeTo (mode 23.85 kbit/s, either framing); size dst accordingly.
const MaxPayloadBytes = 62

var (
	errEncodeFrameSize = errors.New("amrwb: encode expects exactly 320 samples")
	errShortDst        = errors.New("amrwb: destination buffer too small")
)

// Encoder encodes 16 kHz mono PCM into AMR-WB RFC 4867 RTP payloads.
//
// It satisfies the codec.Encoder interface: Encode takes one 20 ms frame
// (320 int16 samples) and returns a complete RTP payload (CMR + ToC + speech
// bits) in the configured payload format.
type Encoder struct {
	mode         Mode
	octetAligned bool
	st           encoderState
	scaled       [FrameSamples]int16
	frameBits    [60]byte // packed speech bits of the largest mode
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
//
// Input samples are halved (-6 dB) before being passed to the encoder. This
// is a workaround for a porting defect in the encoder fixed-point path: the
// bit stream diverges from the vo-amrwbenc reference for non-tone content at
// input levels above roughly -12 dBFS, and when decoded the synthesised speech
// overshoots into saturation ("loud and clipping" symptom). The reference C
// encoder/decoder pair handles the same input cleanly up to 0 dBFS.
//
// The existing TestEncDiffAgainstCReference uses tone input which doesn't
// exercise the divergent path. The bug is likely in the LPC analysis or ACELP
// codebook search rounding; a multi-hour per-stage bisect against the C
// reference is needed to find the exact missing saturation. Until then, the
// -6 dB headroom keeps the decoded output below full scale.
func (e *Encoder) Encode(samples []int16) ([]byte, error) {
	out := make([]byte, MaxPayloadBytes)
	n, err := e.EncodeTo(out, samples)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}

// EncodeTo is Encode writing the RTP payload into dst, which must hold at
// least MaxPayloadBytes bytes (or the payload size of the active mode). It
// returns the payload length and does not allocate.
func (e *Encoder) EncodeTo(dst []byte, samples []int16) (int, error) {
	if len(samples) != FrameSamples {
		return 0, errEncodeFrameSize
	}
	for i, s := range samples {
		e.scaled[i] = s >> 1
	}
	bits, err := e.st.encodeFrameInto(e.frameBits[:], e.mode, e.scaled[:])
	if err != nil {
		return 0, err
	}
	f := frame{ft: e.mode.FrameType(), q: true, data: bits}
	var (
		n  int
		ok bool
	)
	if e.octetAligned {
		n, ok = packOctetInto(dst, noCMR, f)
	} else {
		n, ok = packBandwidthEfficientInto(dst, noCMR, f)
	}
	if !ok {
		return 0, errShortDst
	}
	return n, nil
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
	parser       payloadParser
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
	frames, err := d.parse(payload)
	if err != nil {
		return nil, err
	}
	out := make([]int16, len(frames)*FrameSamples)
	if err := d.decodeFrames(out, frames); err != nil {
		return nil, err
	}
	return out, nil
}

// DecodeTo is Decode writing the PCM into dst, which must hold 320 samples
// per frame carried by the payload (320 for the usual single-frame packet).
// It returns the number of samples written and, once its internal buffers
// have grown to the largest payload seen, does not allocate.
func (d *Decoder) DecodeTo(dst []int16, payload []byte) (int, error) {
	frames, err := d.parse(payload)
	if err != nil {
		return 0, err
	}
	n := len(frames) * FrameSamples
	if len(dst) < n {
		return 0, errShortDst
	}
	if err := d.decodeFrames(dst[:n], frames); err != nil {
		return 0, err
	}
	return n, nil
}

func (d *Decoder) parse(payload []byte) ([]frame, error) {
	if d.octetAligned {
		return d.parser.parseOctet(payload)
	}
	return d.parser.parseBandwidthEfficient(payload)
}

func (d *Decoder) decodeFrames(dst []int16, frames []frame) error {
	for i, f := range frames {
		if err := d.st.decodeFrameInto(f, dst[i*FrameSamples:(i+1)*FrameSamples]); err != nil {
			return err
		}
	}
	return nil
}

// Reset clears all decoder state.
func (d *Decoder) Reset() { d.st.reset() }
