package amrwb

import "errors"

// RFC 4867 RTP payload framing for AMR-WB. Two formats are supported:
//
//   - octet-aligned (octet-align=1): every field starts on an octet boundary.
//     Layout: [CMR(1 byte)] [ToC byte per frame] [speech bytes per frame].
//   - bandwidth-efficient: fields are bit-packed with no padding.
//     Layout: CMR(4 bits) then, per frame, ToC(6 bits) then the speech bits.
//
// Only the fields VoiceBlender needs are modelled. Interleaving and the
// CRC/robust-sorting options are not used (they are negotiated off by default
// and we never offer them).

// noCMR is the "no mode request" CMR value (15) emitted when we have no
// preference to signal to the peer.
const noCMR = 15

// frame is one decoded ToC entry plus its raw speech bits.
type frame struct {
	ft   int    // frame type (FT)
	q    bool   // quality bit (Q): true = good frame
	data []byte // speech bits, octet-aligned, left-justified in the final byte
}

var (
	errShortPayload = errors.New("amrwb: payload too short")
	errBadFrameType = errors.New("amrwb: unknown frame type")
)

// packPayloadOctet builds an octet-aligned payload carrying the given frames
// with the supplied CMR (use noCMR for none).
func packPayloadOctet(cmr int, frames []frame) []byte {
	out := []byte{byte(cmr&0x0F) << 4}
	for i, f := range frames {
		last := i == len(frames)-1
		toc := byte(f.ft&0x0F) << 3
		if !last {
			toc |= 0x80 // F bit: more frames follow
		}
		if f.q {
			toc |= 0x04 // Q bit
		}
		out = append(out, toc)
	}
	for _, f := range frames {
		nb, _ := frameBytesByFT(f.ft)
		buf := make([]byte, nb)
		copy(buf, f.data)
		out = append(out, buf...)
	}
	return out
}

// unpackPayloadOctet parses an octet-aligned payload into its frames and CMR.
func unpackPayloadOctet(p []byte) (cmr int, frames []frame, err error) {
	if len(p) < 1 {
		return 0, nil, errShortPayload
	}
	cmr = int(p[0] >> 4)
	idx := 1
	// Read ToC bytes until the F bit clears.
	var tocs []byte
	for {
		if idx >= len(p) {
			return 0, nil, errShortPayload
		}
		toc := p[idx]
		idx++
		tocs = append(tocs, toc)
		if toc&0x80 == 0 {
			break
		}
	}
	for _, toc := range tocs {
		ft := int(toc>>3) & 0x0F
		q := toc&0x04 != 0
		nb, ok := frameBytesByFT(ft)
		if !ok {
			return 0, nil, errBadFrameType
		}
		if idx+nb > len(p) {
			return 0, nil, errShortPayload
		}
		data := make([]byte, nb)
		copy(data, p[idx:idx+nb])
		idx += nb
		frames = append(frames, frame{ft: ft, q: q, data: data})
	}
	return cmr, frames, nil
}

// packPayloadBandwidthEfficient builds a bandwidth-efficient payload.
func packPayloadBandwidthEfficient(cmr int, frames []frame) []byte {
	w := &bitWriter{}
	w.writeBits(uint32(cmr&0x0F), 4)
	for i, f := range frames {
		last := i == len(frames)-1
		fBit := uint32(0)
		if !last {
			fBit = 1
		}
		w.writeBits(fBit, 1)
		w.writeBits(uint32(f.ft&0x0F), 4)
		q := uint32(0)
		if f.q {
			q = 1
		}
		w.writeBits(q, 1)
	}
	for _, f := range frames {
		bits, _ := frameBitsByFT(f.ft)
		w.writeBitsFromBytes(f.data, bits)
	}
	return w.bytes()
}

// unpackPayloadBandwidthEfficient parses a bandwidth-efficient payload.
func unpackPayloadBandwidthEfficient(p []byte) (cmr int, frames []frame, err error) {
	r := &bitReader{data: p}
	c, ok := r.readBits(4)
	if !ok {
		return 0, nil, errShortPayload
	}
	cmr = int(c)
	type toc struct {
		ft int
		q  bool
	}
	var tocs []toc
	for {
		fBit, ok := r.readBits(1)
		if !ok {
			return 0, nil, errShortPayload
		}
		ft, ok := r.readBits(4)
		if !ok {
			return 0, nil, errShortPayload
		}
		qBit, ok := r.readBits(1)
		if !ok {
			return 0, nil, errShortPayload
		}
		if _, valid := frameBitsByFT(int(ft)); !valid {
			return 0, nil, errBadFrameType
		}
		tocs = append(tocs, toc{ft: int(ft), q: qBit == 1})
		if fBit == 0 {
			break
		}
	}
	for _, t := range tocs {
		bits, _ := frameBitsByFT(t.ft)
		data, ok := r.readBitsToBytes(bits)
		if !ok {
			return 0, nil, errShortPayload
		}
		frames = append(frames, frame{ft: t.ft, q: t.q, data: data})
	}
	return cmr, frames, nil
}

// payloadParser parses RFC 4867 payloads into frames without allocating once
// its scratch buffers have grown to the largest payload seen. Octet-aligned
// frame data aliases the payload; bandwidth-efficient frame data aliases the
// parser's bit buffer. Frames are valid until the next parse.
type payloadParser struct {
	frames []frame
	fts    []int
	qs     []bool
	bits   []byte
}

func (pp *payloadParser) parseOctet(p []byte) ([]frame, error) {
	if len(p) < 1 {
		return nil, errShortPayload
	}
	idx := 1
	tocStart := idx
	for {
		if idx >= len(p) {
			return nil, errShortPayload
		}
		toc := p[idx]
		idx++
		if toc&0x80 == 0 {
			break
		}
	}
	frames := pp.frames[:0]
	for _, toc := range p[tocStart:idx] {
		ft := int(toc>>3) & 0x0F
		nb, ok := frameBytesByFT(ft)
		if !ok {
			return nil, errBadFrameType
		}
		if idx+nb > len(p) {
			return nil, errShortPayload
		}
		frames = append(frames, frame{ft: ft, q: toc&0x04 != 0, data: p[idx : idx+nb : idx+nb]})
		idx += nb
	}
	pp.frames = frames
	return frames, nil
}

func (pp *payloadParser) parseBandwidthEfficient(p []byte) ([]frame, error) {
	r := bitReader{data: p}
	if _, ok := r.readBits(4); !ok {
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
	return frames, nil
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
