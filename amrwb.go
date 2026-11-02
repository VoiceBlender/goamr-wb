package amrwb

import "errors"

// MaxPayloadBytes is the largest payload EncodeTo produces from one 20 ms
// frame (mode 23.85 kbit/s, octet-aligned with a frame CRC); size dst
// accordingly. See MaxPayloadBytesFor for several frames per packet.
const MaxPayloadBytes = 63

// MaxPayloadBytesFor returns the largest payload EncodeTo produces from n
// frames (n*320 samples) in any payload format.
func MaxPayloadBytesFor(n int) int { return 1 + n*(1+1+maxFrameBytes) }

// maxFrameBytes is the length of the largest frame, 23.85 kbit/s, in octets.
const maxFrameBytes = 60

var (
	errEncodeFrameSize = errors.New("amrwb: encode expects a positive multiple of 320 samples")
	errShortDst        = errors.New("amrwb: destination buffer too small")
)

// Encoder encodes 16 kHz mono PCM into AMR-WB RFC 4867 RTP payloads.
//
// Encode takes one or more 20 ms frames (320 int16 samples each) and returns
// a complete RTP payload (CMR + ToC + speech bits) carrying them, in the
// configured payload format.
type Encoder struct {
	mode           Mode   // mode of the next frame
	target         Mode   // mode set by SetMode, reached as the mode-change rules allow
	modeSet        uint16 // allowed modes, bit m for mode m
	changePeriod   uint   // mode-change-period: 1 or 2
	changeNeighbor bool   // mode-change-neighbor
	frameNo        uint   // frame-blocks encoded, for the mode-change period
	format         payloadFormat
	cmr            Mode // codec mode request sent to the peer
	st             encoderState
	lastFT         int     // frame type of the last encoded frame
	talkspurt      bool    // the last payload starts with the first frame of a talkspurt
	frames         []frame // frames of the packet being built
	frameBits      []byte  // their packed speech bits, maxFrameBytes per frame
}

// EncoderConfig configures a new AMR-WB encoder. The fields after DTX mirror
// the RFC 4867 SDP parameters of the same names and default to their absence.
type EncoderConfig struct {
	Mode         Mode // initial speech mode (0..8)
	OctetAligned bool // true: octet-aligned framing; false: bandwidth-efficient

	// DTX enables discontinuous transmission: non-speech frames are coded as
	// SID frames or, between SID updates, as NO_DATA frames, which Encode and
	// EncodeTo report as an empty payload.
	DTX bool

	// CRC adds a CRC over the class A bits of each frame (crc=1). It implies
	// octet-aligned framing.
	CRC bool
	// RobustSorting interleaves the octets of the frames in a payload
	// (robust-sorting=1). It implies octet-aligned framing.
	RobustSorting bool

	// ModeSet restricts the modes the encoder uses and requests (mode-set);
	// empty means all. Mode must be in it.
	ModeSet []Mode
	// ModeChangePeriod is 2 to change modes only every second frame
	// (mode-change-period=2); 0 or 1 allows a change at any frame.
	ModeChangePeriod int
	// ModeChangeNeighbor makes mode changes step through the neighboring
	// modes of the mode set, one per change (mode-change-neighbor=1).
	ModeChangeNeighbor bool
}

// NewEncoder creates an AMR-WB encoder for the given mode and payload format.
func NewEncoder(cfg EncoderConfig) (*Encoder, error) {
	if !cfg.Mode.Valid() {
		return nil, errors.New("amrwb: invalid encoder mode")
	}
	e := &Encoder{
		mode:           cfg.Mode,
		target:         cfg.Mode,
		changePeriod:   1,
		changeNeighbor: cfg.ModeChangeNeighbor,
		format:         newPayloadFormat(cfg.OctetAligned, cfg.CRC, cfg.RobustSorting),
		cmr:            NoModeRequest,
		lastFT:         FTNoData,
	}
	for _, m := range cfg.ModeSet {
		if !m.Valid() {
			return nil, errors.New("amrwb: invalid mode in mode set")
		}
		e.modeSet |= 1 << m
	}
	if e.modeSet == 0 {
		e.modeSet = 1<<numSpeechModes - 1
	}
	if !e.allowed(cfg.Mode) {
		return nil, errors.New("amrwb: encoder mode not in mode set")
	}
	switch cfg.ModeChangePeriod {
	case 0, 1:
	case 2:
		e.changePeriod = 2
	default:
		return nil, errors.New("amrwb: mode change period must be 1 or 2")
	}
	e.st.dtx = cfg.DTX
	e.st.reset()
	return e, nil
}

// Encode encodes one or more 20 ms frames (a multiple of 320 samples) into an
// RTP payload. With DTX on, NO_DATA frames at the end of the payload are left
// out, and the payload is empty when every frame is NO_DATA: send nothing
// (RFC 4867, section 4.3.2). Either way, advance the RTP timestamp by
// len(samples).
func (e *Encoder) Encode(samples []int16) ([]byte, error) {
	out := make([]byte, MaxPayloadBytesFor(len(samples)/FrameSamples))
	n, err := e.EncodeTo(out, samples)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}

// EncodeTo is Encode writing the RTP payload into dst, which must hold at
// least MaxPayloadBytesFor(len(samples)/320) bytes (or the size of the
// payload). It returns the payload length, 0 when every frame is NO_DATA, and
// does not allocate once it has encoded a payload of as many frames.
func (e *Encoder) EncodeTo(dst []byte, samples []int16) (int, error) {
	k := len(samples) / FrameSamples
	if k == 0 || len(samples)%FrameSamples != 0 {
		return 0, errEncodeFrameSize
	}
	if cap(e.frames) < k {
		e.frames = make([]frame, 0, k)
		e.frameBits = make([]byte, k*maxFrameBytes)
	}
	frames := e.frames[:0]
	last := -1 // last frame with data
	for i := 0; i < k; i++ {
		e.stepMode()
		f, err := e.st.encodeFrameInto(e.frameBits[i*maxFrameBytes:(i+1)*maxFrameBytes], e.mode, samples[i*FrameSamples:(i+1)*FrameSamples])
		if err != nil {
			return 0, err
		}
		if i == 0 {
			e.talkspurt = f.ft <= FTMode2385 && e.lastFT > FTMode2385
		}
		e.lastFT = f.ft
		if f.ft != FTNoData {
			last = i
		}
		frames = append(frames, f)
	}
	e.frames = frames
	if last < 0 {
		return 0, nil
	}
	n, ok := e.format.packInto(dst, int(e.cmr), frames[:last+1])
	if !ok {
		return 0, errShortDst
	}
	return n, nil
}

// stepMode moves the mode toward the target before a frame, as the mode-change
// period and neighbor rules allow.
func (e *Encoder) stepMode() {
	if e.mode != e.target && e.frameNo%e.changePeriod == 0 {
		if e.changeNeighbor {
			e.mode = e.neighbor(e.mode, e.target)
		} else {
			e.mode = e.target
		}
	}
	e.frameNo++
}

// neighbor returns the allowed mode next to from in the direction of to.
func (e *Encoder) neighbor(from, to Mode) Mode {
	step := Mode(1)
	if to < from {
		step = -1
	}
	for m := from + step; m != to; m += step {
		if e.allowed(m) {
			return m
		}
	}
	return to
}

func (e *Encoder) allowed(m Mode) bool { return m.Valid() && e.modeSet&(1<<m) != 0 }

// nearest returns the highest allowed mode not above m, or else the lowest
// allowed mode.
func (e *Encoder) nearest(m Mode) Mode {
	for c := m; c >= Mode0660; c-- {
		if e.allowed(c) {
			return c
		}
	}
	for c := Mode0660; c < numSpeechModes; c++ {
		if e.allowed(c) {
			return c
		}
	}
	return m
}

// SetMode changes the speech mode, e.g. to the one the peer requests (see
// Decoder.ModeRequest). A mode outside the mode set is replaced by the highest
// allowed mode below it, or else the lowest allowed mode, and an invalid mode
// is ignored. With a mode-change period or neighbor rule, the encoder reaches
// the mode over the next frames.
func (e *Encoder) SetMode(m Mode) {
	if m.Valid() {
		e.target = e.nearest(m)
	}
}

// Mode returns the mode of the next frame, before any change still due from
// SetMode.
func (e *Encoder) Mode() Mode { return e.mode }

// SetModeRequest sets the codec mode request (CMR) carried by every payload
// from now on, asking the peer to encode in mode m (RFC 4867, section 4.3.1).
// A mode outside the mode set is replaced as in SetMode. NoModeRequest, the
// default, or any invalid mode asks for none. With DTX on, the peer sees it
// only once a SID or speech frame is sent.
func (e *Encoder) SetModeRequest(m Mode) {
	if !m.Valid() {
		e.cmr = NoModeRequest
		return
	}
	e.cmr = e.nearest(m)
}

// Talkspurt reports whether the last payload starts with a speech frame that
// is the first frame or the first after SID/NO_DATA frames, so that the RTP
// marker bit of its packet must be set (RFC 4867, section 4.1).
func (e *Encoder) Talkspurt() bool { return e.talkspurt }

// Reset clears all encoder state. The mode and mode request are kept.
func (e *Encoder) Reset() {
	e.st.reset()
	e.lastFT = FTNoData
	e.talkspurt = false
	e.frameNo = 0
}

// Decoder decodes AMR-WB RFC 4867 RTP payloads into 16 kHz mono PCM.
type Decoder struct {
	format payloadFormat
	cmr    Mode // codec mode request of the last payload decoded
	st     decoderState
	parser payloadParser
}

// DecoderConfig configures a new AMR-WB decoder. Its fields must match the
// negotiated payload format and mirror the RFC 4867 SDP parameters of the same
// names. Frame interleaving is not supported.
type DecoderConfig struct {
	OctetAligned bool
	// CRC: payloads carry frame CRCs (crc=1). A frame whose CRC does not
	// match is decoded as a bad frame. It implies octet-aligned framing.
	CRC bool
	// RobustSorting: payloads use robust sorting order (robust-sorting=1). It
	// implies octet-aligned framing.
	RobustSorting bool
}

// NewDecoder creates an AMR-WB decoder for the given payload format.
func NewDecoder(cfg DecoderConfig) *Decoder {
	d := &Decoder{format: newPayloadFormat(cfg.OctetAligned, cfg.CRC, cfg.RobustSorting), cmr: NoModeRequest}
	d.st.reset()
	return d
}

// Decode decodes one RTP payload, returning concatenated PCM for all frames it
// carries (one 320-sample frame per ToC entry). A payload whose trailing
// NO_DATA frames were left out covers fewer 20 ms intervals than the packet
// interval: call DecodeNoData for the rest.
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
	frames, err := d.parser.parse(payload, d.format)
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
