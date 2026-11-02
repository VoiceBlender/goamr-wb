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
	cmr          Mode // codec mode request sent to the peer
	st           encoderState
	lastFT       int      // frame type of the last encoded frame
	talkspurt    bool     // the last encoded frame starts a talkspurt
	frameBits    [60]byte // packed speech bits of the largest mode
}

// EncoderConfig configures a new AMR-WB encoder.
type EncoderConfig struct {
	Mode         Mode // active speech mode (0..8)
	OctetAligned bool // true: octet-aligned framing; false: bandwidth-efficient

	// DTX enables discontinuous transmission: non-speech frames are coded as
	// SID frames or, between SID updates, as NO_DATA frames, which Encode and
	// EncodeTo report as an empty payload.
	DTX bool
}

// NewEncoder creates an AMR-WB encoder for the given mode and payload format.
func NewEncoder(cfg EncoderConfig) (*Encoder, error) {
	if !cfg.Mode.Valid() {
		return nil, errors.New("amrwb: invalid encoder mode")
	}
	e := &Encoder{mode: cfg.Mode, octetAligned: cfg.OctetAligned, cmr: NoModeRequest, lastFT: FTNoData}
	e.st.dtx = cfg.DTX
	e.st.reset()
	return e, nil
}

// Encode encodes one 20 ms frame (320 samples) into an RTP payload. With DTX on
// the payload is empty for a NO_DATA frame: send nothing (RFC 4867, section
// 4.3.2), but still advance the RTP timestamp by 320.
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
// returns the payload length, 0 for a NO_DATA frame, and does not allocate.
func (e *Encoder) EncodeTo(dst []byte, samples []int16) (int, error) {
	if len(samples) != FrameSamples {
		return 0, errEncodeFrameSize
	}
	f, err := e.st.encodeFrameInto(e.frameBits[:], e.mode, samples)
	if err != nil {
		return 0, err
	}
	e.talkspurt = f.ft <= FTMode2385 && e.lastFT > FTMode2385
	e.lastFT = f.ft
	if f.ft == FTNoData {
		return 0, nil
	}
	var (
		n  int
		ok bool
	)
	if e.octetAligned {
		n, ok = packOctetInto(dst, int(e.cmr), f)
	} else {
		n, ok = packBandwidthEfficientInto(dst, int(e.cmr), f)
	}
	if !ok {
		return 0, errShortDst
	}
	return n, nil
}

// SetMode changes the active speech mode, e.g. to the one the peer requests
// (see Decoder.ModeRequest). An invalid mode is ignored.
func (e *Encoder) SetMode(m Mode) {
	if m.Valid() {
		e.mode = m
	}
}

// SetModeRequest sets the codec mode request (CMR) carried by every payload
// from now on, asking the peer to encode in mode m (RFC 4867, section 4.3.1).
// NoModeRequest, the default, or any invalid mode asks for none. With DTX on,
// the peer sees it only once a SID or speech frame is sent.
func (e *Encoder) SetModeRequest(m Mode) {
	if !m.Valid() {
		m = NoModeRequest
	}
	e.cmr = m
}

// Talkspurt reports whether the last encoded frame is speech and is either the
// first frame or the first after SID/NO_DATA frames, so that the RTP marker bit
// of its packet must be set (RFC 4867, section 4.1).
func (e *Encoder) Talkspurt() bool { return e.talkspurt }

// Reset clears all encoder state. The mode and mode request are kept.
func (e *Encoder) Reset() {
	e.st.reset()
	e.lastFT = FTNoData
	e.talkspurt = false
}

// Decoder decodes AMR-WB RFC 4867 RTP payloads into 16 kHz mono PCM.
type Decoder struct {
	octetAligned bool
	cmr          Mode // codec mode request of the last payload decoded
	st           decoderState
	parser       payloadParser
}

// DecoderConfig configures a new AMR-WB decoder.
type DecoderConfig struct {
	OctetAligned bool // must match the negotiated payload format
}

// NewDecoder creates an AMR-WB decoder for the given payload format.
func NewDecoder(cfg DecoderConfig) *Decoder {
	d := &Decoder{octetAligned: cfg.OctetAligned, cmr: NoModeRequest}
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

// DecodeNoData writes one frame (320 samples) to dst for a 20 ms interval
// without a packet. Receivers must call it for every such interval, because DTX
// senders transmit nothing between SID updates: it produces comfort noise while
// in DTX, and concealment otherwise. It returns the number of samples written
// and does not allocate.
func (d *Decoder) DecodeNoData(dst []int16) (int, error) {
	if len(dst) < FrameSamples {
		return 0, errShortDst
	}
	if err := d.st.decodeFrameInto(frame{ft: FTNoData, q: true}, dst[:FrameSamples]); err != nil {
		return 0, err
	}
	return FrameSamples, nil
}

// ModeRequest returns the codec mode request (CMR) of the last payload Decode
// or DecodeTo accepted: the mode the peer asks this side to encode in, to pass
// to Encoder.SetMode (RFC 4867, section 4.3.1). It returns NoModeRequest when
// the peer asks for none, sends a reserved value, or before any payload. The
// request stands until a payload carries another, so DecodeNoData keeps it.
func (d *Decoder) ModeRequest() Mode { return d.cmr }

func (d *Decoder) parse(payload []byte) ([]frame, error) {
	var (
		frames []frame
		err    error
	)
	if d.octetAligned {
		frames, err = d.parser.parseOctet(payload)
	} else {
		frames, err = d.parser.parseBandwidthEfficient(payload)
	}
	if err != nil {
		return nil, err
	}
	d.cmr = Mode(d.parser.cmr)
	if !d.cmr.Valid() {
		d.cmr = NoModeRequest
	}
	return frames, nil
}

func (d *Decoder) decodeFrames(dst []int16, frames []frame) error {
	for i, f := range frames {
		if err := d.st.decodeFrameInto(f, dst[i*FrameSamples:(i+1)*FrameSamples]); err != nil {
			return err
		}
	}
	return nil
}

// Reset clears all decoder state, including the last mode request.
func (d *Decoder) Reset() {
	d.st.reset()
	d.cmr = NoModeRequest
}
