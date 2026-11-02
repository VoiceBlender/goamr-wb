package amrwb

import "errors"

// RFC 4867 RTP payload framing for single-channel AMR-WB. Two formats:
//
//   - octet-aligned (octet-align=1): every field starts on an octet boundary.
//     Layout: [CMR byte] [ToC byte per frame] [CRC byte per frame with data,
//     if crc=1] [speech bytes per frame, in robust sorting order if
//     robust-sorting=1].
//   - bandwidth-efficient: fields are bit-packed with no padding.
//     Layout: CMR(4 bits) then, per frame, ToC(6 bits), then the speech bits.
//
// Frame interleaving (interleaving=N) is not supported: never offer it.

// noCMR is the "no mode request" CMR value (15).
const noCMR = 15

// frame is one ToC entry plus its speech bits.
type frame struct {
	ft   int    // frame type (FT)
	q    bool   // quality bit (Q): true = good frame
	data []byte // speech bits, octet-aligned, left-justified in the final byte
}

var (
	errShortPayload = errors.New("amrwb: payload too short")
	errBadFrameType = errors.New("amrwb: unknown frame type")
)

// payloadFormat is the payload format negotiated for a session.
type payloadFormat struct {
	octetAligned  bool
	crc           bool // frame CRCs (crc=1); octet-aligned only
	robustSorting bool // robust sorting order (robust-sorting=1); octet-aligned only
}

// newPayloadFormat returns the format for the given SDP parameters; crc=1 and
// robust-sorting=1 imply octet-align=1 (RFC 4867, section 8.2).
func newPayloadFormat(octetAligned, crc, robustSorting bool) payloadFormat {
	return payloadFormat{octetAligned: octetAligned || crc || robustSorting, crc: crc, robustSorting: robustSorting}
}

// classABits is the number of class A bits of each frame type with data,
// over which frame CRCs are computed: TS 26.201 Table 2, and 40 for SID
// (RFC 4867, section 4.4.2.1).
var classABits = [FTSID + 1]int{54, 64, 72, 72, 72, 72, 72, 72, 72, 40}

// frameCRC returns the frame CRC of RFC 4867, section 4.4.2.1, over the
// first n bits of data (d(0) first, MSB-first): C(x) = 1 + x^2 + x^3 + x^4 +
// x^8, register cleared, shifted right, its LSB XORed with each bit.
func frameCRC(data []byte, n int) byte {
	var crc byte
	for i := 0; i < n; i++ {
		bit := data[i/8] >> uint(7-i%8) & 1
		fb := (crc ^ bit) & 1
		crc >>= 1
		if fb != 0 {
			crc ^= 0xB8
		}
	}
	return crc
}

// frameByte returns octet k of a frame of nb octets and the given bit count,
// with the padding bits of its last octet cleared.
func frameByte(f frame, k, nb, bits int) byte {
	if k >= len(f.data) {
		return 0
	}
	b := f.data[k]
	if k == nb-1 && bits%8 != 0 {
		b &= 0xFF << uint(8-bits%8)
	}
	return b
}

// size returns the length of the payload carrying frames.
func (pf payloadFormat) size(frames []frame) int {
	if !pf.octetAligned {
		bits := 4 + 6*len(frames)
		for _, f := range frames {
			nb, _ := frameBitsByFT(f.ft)
			bits += nb
		}
		return (bits + 7) / 8
	}
	n := 1 + len(frames)
	for _, f := range frames {
		nb, _ := frameBytesByFT(f.ft)
		n += nb
		if pf.crc && nb > 0 {
			n++
		}
	}
	return n
}

// packInto writes a payload carrying frames, in order, into dst and returns
// its length, or false if dst is too small or there are no frames. Frames of
// type SPEECH_LOST and NO_DATA get a ToC entry and nothing else.
func (pf payloadFormat) packInto(dst []byte, cmr int, frames []frame) (int, bool) {
	n := pf.size(frames)
	if len(dst) < n || len(frames) == 0 {
		return 0, false
	}
	if !pf.octetAligned {
		w := bitWriter{buf: dst[:0:n]}
		w.writeBits(uint32(cmr&0x0F), 4)
		for i, f := range frames {
			var more, q uint32
			if i < len(frames)-1 {
				more = 1
			}
			if f.q {
				q = 1
			}
			w.writeBits(more, 1)
			w.writeBits(uint32(f.ft&0x0F), 4)
			w.writeBits(q, 1)
		}
		for _, f := range frames {
			bits, _ := frameBitsByFT(f.ft)
			w.writeBitsFromBytes(f.data, bits)
		}
		return len(w.bytes()), true
	}

	dst = dst[:n]
	dst[0] = byte(cmr&0x0F) << 4
	idx := 1
	for i, f := range frames {
		toc := byte(f.ft&0x0F) << 3
		if i < len(frames)-1 {
			toc |= 0x80 // F: more frames follow
		}
		if f.q {
			toc |= 0x04
		}
		dst[idx] = toc
		idx++
	}
	if pf.crc {
		for _, f := range frames {
			if bits, _ := frameBitsByFT(f.ft); bits > 0 {
				dst[idx] = frameCRC(f.data, classABits[f.ft])
				idx++
			}
		}
	}
	if !pf.robustSorting {
		for _, f := range frames {
			bits, _ := frameBitsByFT(f.ft)
			nb := (bits + 7) / 8
			for k := 0; k < nb; k++ {
				dst[idx] = frameByte(f, k, nb, bits)
				idx++
			}
		}
		return n, true
	}
	// Robust sorting: octet k of every frame that has one, for k = 0, 1, ...
	for k := 0; idx < n; k++ {
		for _, f := range frames {
			bits, _ := frameBitsByFT(f.ft)
			if nb := (bits + 7) / 8; k < nb {
				dst[idx] = frameByte(f, k, nb, bits)
				idx++
			}
		}
	}
	return n, true
}

// payloadParser parses RFC 4867 payloads into frames without allocating once
// its scratch buffers have grown to the largest payload seen. Frame data may
// alias the payload or the parser's buffer, and is valid until the next
// parse.
type payloadParser struct {
	cmr    int // CMR of the last payload parsed without error
	frames []frame
	fts    []int
	qs     []bool
	crcs   []byte
	bits   []byte
}

// parse parses a payload in format pf. With frame CRCs, a frame whose class A
// bits fail the CRC check gets Q=0 (RFC 4867, section 4.4.2.1).
func (pp *payloadParser) parse(p []byte, pf payloadFormat) ([]frame, error) {
	if pf.octetAligned {
		return pp.parseOctet(p, pf)
	}
	return pp.parseBandwidthEfficient(p)
}

func (pp *payloadParser) parseOctet(p []byte, pf payloadFormat) ([]frame, error) {
	if len(p) < 1 {
		return nil, errShortPayload
	}
	cmr := int(p[0] >> 4)
	idx := 1
	fts, qs := pp.fts[:0], pp.qs[:0]
	total, withData := 0, 0
	for {
		if idx >= len(p) {
			return nil, errShortPayload
		}
		toc := p[idx]
		idx++
		ft := int(toc>>3) & 0x0F
		nb, ok := frameBytesByFT(ft)
		if !ok {
			return nil, errBadFrameType
		}
		fts, qs = append(fts, ft), append(qs, toc&0x04 != 0)
		total += nb
		if nb > 0 {
			withData++
		}
		if toc&0x80 == 0 {
			break
		}
	}
	pp.fts, pp.qs = fts, qs
	crcs := pp.crcs[:0]
	if pf.crc {
		if idx+withData > len(p) {
			return nil, errShortPayload
		}
		crcs = append(crcs, p[idx:idx+withData]...)
		idx += withData
		pp.crcs = crcs
	}
	if idx+total > len(p) {
		return nil, errShortPayload
	}

	var data []byte
	if !pf.robustSorting {
		data = p[idx : idx+total]
	} else {
		if cap(pp.bits) < total {
			pp.bits = make([]byte, total)
		}
		data = pp.bits[:total]
		// Undo robust sorting: octet k of every frame that has one, in turn.
		for k, src := 0, idx; src < idx+total; k++ {
			off := 0
			for _, ft := range fts {
				nb, _ := frameBytesByFT(ft)
				if k < nb {
					data[off+k] = p[src]
					src++
				}
				off += nb
			}
		}
	}

	frames := pp.frames[:0]
	off, c := 0, 0
	for i, ft := range fts {
		nb, _ := frameBytesByFT(ft)
		f := frame{ft: ft, q: qs[i], data: data[off : off+nb : off+nb]}
		off += nb
		if pf.crc && nb > 0 {
			if frameCRC(f.data, classABits[ft]) != crcs[c] {
				f.q = false
			}
			c++
		}
		frames = append(frames, f)
	}
	pp.frames = frames
	pp.cmr = cmr
	return frames, nil
}

func (pp *payloadParser) parseBandwidthEfficient(p []byte) ([]frame, error) {
	r := bitReader{data: p}
	cmr, ok := r.readBits(4)
	if !ok {
		return nil, errShortPayload
	}
	fts, qs := pp.fts[:0], pp.qs[:0]
	total := 0
	for {
		fBit, ok := r.readBits(1)
		if !ok {
			return nil, errShortPayload
		}
		ft, ok := r.readBits(4)
		if !ok {
			return nil, errShortPayload
		}
		qBit, ok := r.readBits(1)
		if !ok {
			return nil, errShortPayload
		}
		bits, valid := frameBitsByFT(int(ft))
		if !valid {
			return nil, errBadFrameType
		}
		fts, qs = append(fts, int(ft)), append(qs, qBit == 1)
		total += (bits + 7) / 8
		if fBit == 0 {
			break
		}
	}
	pp.fts, pp.qs = fts, qs
	if cap(pp.bits) < total {
		pp.bits = make([]byte, total)
	}
	buf := pp.bits[:total]
	frames := pp.frames[:0]
	off := 0
	for i, ft := range fts {
		bits, _ := frameBitsByFT(ft)
		nb := (bits + 7) / 8
		data := buf[off : off+nb : off+nb]
		if !r.readBitsInto(data, bits) {
			return nil, errShortPayload
		}
		frames = append(frames, frame{ft: ft, q: qs[i], data: data})
		off += nb
	}
	pp.frames = frames
	pp.cmr = int(cmr)
	return frames, nil
}

// Allocating helpers, used by tests.

// packPayloadOctet builds an octet-aligned payload carrying frames.
func packPayloadOctet(cmr int, frames []frame) []byte {
	return packPayload(payloadFormat{octetAligned: true}, cmr, frames)
}

// packPayloadBandwidthEfficient builds a bandwidth-efficient payload.
func packPayloadBandwidthEfficient(cmr int, frames []frame) []byte {
	return packPayload(payloadFormat{}, cmr, frames)
}

func packPayload(pf payloadFormat, cmr int, frames []frame) []byte {
	out := make([]byte, pf.size(frames))
	n, _ := pf.packInto(out, cmr, frames)
	return out[:n]
}

// unpackPayloadOctet parses an octet-aligned payload into its CMR and frames.
func unpackPayloadOctet(p []byte) (cmr int, frames []frame, err error) {
	return unpackPayload(p, payloadFormat{octetAligned: true})
}

// unpackPayloadBandwidthEfficient parses a bandwidth-efficient payload.
func unpackPayloadBandwidthEfficient(p []byte) (cmr int, frames []frame, err error) {
	return unpackPayload(p, payloadFormat{})
}

func unpackPayload(p []byte, pf payloadFormat) (cmr int, frames []frame, err error) {
	var pp payloadParser
	frames, err = pp.parse(p, pf)
	if err != nil {
		return 0, nil, err
	}
	return pp.cmr, frames, nil
}

// bitWriter accumulates bits MSB-first into a byte slice.
type bitWriter struct {
	buf  []byte
	cur  byte
	nimb int // number of bits filled in cur
}

func (w *bitWriter) writeBit(b uint32) {
	w.cur |= byte(b&1) << uint(7-w.nimb)
	w.nimb++
	if w.nimb == 8 {
		w.buf = append(w.buf, w.cur)
		w.cur = 0
		w.nimb = 0
	}
}

func (w *bitWriter) writeBits(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		w.writeBit(v >> uint(i))
	}
}

// writeBitsFromBytes writes the first n bits of data, MSB-first.
func (w *bitWriter) writeBitsFromBytes(data []byte, n int) {
	for i := 0; i < n; i++ {
		bit := uint32(0)
		if byteIdx := i / 8; byteIdx < len(data) {
			bit = uint32(data[byteIdx]>>uint(7-i%8)) & 1
		}
		w.writeBit(bit)
	}
}

func (w *bitWriter) bytes() []byte {
	out := w.buf
	if w.nimb > 0 {
		out = append(out, w.cur)
	}
	return out
}

// bitReader reads bits MSB-first from a byte slice.
type bitReader struct {
	data []byte
	pos  int // bit position
}

func (r *bitReader) readBits(n int) (uint32, bool) {
	var v uint32
	for i := 0; i < n; i++ {
		if r.pos >= len(r.data)*8 {
			return 0, false
		}
		bit := uint32(r.data[r.pos/8]>>uint(7-r.pos%8)) & 1
		v = (v << 1) | bit
		r.pos++
	}
	return v, true
}

// readBitsToBytes reads n bits and returns them left-justified in a byte slice.
func (r *bitReader) readBitsToBytes(n int) ([]byte, bool) {
	out := make([]byte, (n+7)/8)
	if !r.readBitsInto(out, n) {
		return nil, false
	}
	return out, true
}

// readBitsInto reads n bits left-justified into out, which must hold
// (n+7)/8 bytes; it overwrites out.
func (r *bitReader) readBitsInto(out []byte, n int) bool {
	for i := range out {
		out[i] = 0
	}
	for i := range n {
		if r.pos >= len(r.data)*8 {
			return false
		}
		bit := r.data[r.pos/8] >> uint(7-r.pos%8) & 1
		out[i/8] |= bit << uint(7-i%8)
		r.pos++
	}
	return true
}

// packOctetInto writes an octet-aligned single-frame payload into dst and
// returns the number of bytes written, or false if dst is too small.
func packOctetInto(dst []byte, cmr int, f frame) (int, bool) {
	nb, _ := frameBytesByFT(f.ft)
	n := 2 + nb
	if len(dst) < n {
		return 0, false
	}
	dst[0] = byte(cmr&0x0F) << 4
	toc := byte(f.ft&0x0F) << 3
	if f.q {
		toc |= 0x04
	}
	dst[1] = toc
	clear(dst[2:n])
	copy(dst[2:n], f.data)
	return n, true
}

// packBandwidthEfficientInto writes a bandwidth-efficient single-frame
// payload into dst and returns the number of bytes written, or false if dst
// is too small.
func packBandwidthEfficientInto(dst []byte, cmr int, f frame) (int, bool) {
	bits, _ := frameBitsByFT(f.ft)
	n := (4 + 6 + bits + 7) / 8
	if len(dst) < n {
		return 0, false
	}
	w := bitWriter{buf: dst[:0:n]}
	w.writeBits(uint32(cmr&0x0F), 4)
	w.writeBits(0, 1) // F: last frame
	w.writeBits(uint32(f.ft&0x0F), 4)
	q := uint32(0)
	if f.q {
		q = 1
	}
	w.writeBits(q, 1)
	w.writeBitsFromBytes(f.data, bits)
	return len(w.bytes()), true
}
