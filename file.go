package amrwb

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// The single-channel AMR-WB storage format of RFC 4867, section 5 (".awb"
// files): the magic number, then one frame per 20 ms, each a header octet
// (P, FT, Q, P, P) followed by the frame's speech bits, padded to an octet.

const (
	fileMagic   = "#!AMR-WB\n"
	fileMagicMC = "#!AMR-WB_MC1.0\n"
)

var (
	errNotAWB        = errors.New("amrwb: not an AMR-WB file")
	errMultiChannel  = errors.New("amrwb: multi-channel AMR-WB files are not supported")
	errEmptyPayload  = errors.New("amrwb: empty payload")
	errFileFrameType = errors.New("amrwb: file frame with unknown frame type")
)

// PayloadFormat describes RTP payloads for FileReader and FileWriter. Its
// fields mean the same as those of DecoderConfig.
type PayloadFormat struct {
	OctetAligned  bool
	CRC           bool
	RobustSorting bool
}

func (f PayloadFormat) format() payloadFormat {
	return newPayloadFormat(f.OctetAligned, f.CRC, f.RobustSorting)
}

// FileWriter writes the AMR-WB storage format (RFC 4867, section 5; ".awb"
// files) from RTP payloads, e.g. to record a call without transcoding, or
// to store the output of an Encoder.
type FileWriter struct {
	w      io.Writer
	format payloadFormat
	parser payloadParser
	buf    []byte
}

// NewFileWriter writes the header of a single-channel AMR-WB file to w and
// returns a writer for payloads in format f.
func NewFileWriter(w io.Writer, f PayloadFormat) (*FileWriter, error) {
	if _, err := io.WriteString(w, fileMagic); err != nil {
		return nil, err
	}
	return &FileWriter{w: w, format: f.format()}, nil
}

// WritePayload stores every frame of an RTP payload, and returns how many
// 20 ms frames it stored. Call WriteNoData for each 20 ms interval that no
// payload covers: an empty payload from Encoder.Encode, a lost packet, or the
// trailing NO_DATA frames a multi-frame payload leaves out. Frames whose CRC
// fails are stored with Q=0.
func (fw *FileWriter) WritePayload(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, errEmptyPayload
	}
	frames, err := fw.parser.parse(p, fw.format)
	if err != nil {
		return 0, err
	}
	buf := fw.buf[:0]
	for _, f := range frames {
		bits, _ := frameBitsByFT(f.ft)
		nb := (bits + 7) / 8
		hdr := byte(f.ft) << 3
		if f.q {
			hdr |= 0x04
		}
		buf = append(buf, hdr)
		for k := 0; k < nb; k++ {
			buf = append(buf, frameByte(f, k, nb, bits))
		}
	}
	fw.buf = buf
	if _, err := fw.w.Write(buf); err != nil {
		return 0, err
	}
	return len(frames), nil
}

// WriteNoData stores a NO_DATA frame for a 20 ms interval without data.
func (fw *FileWriter) WriteNoData() error {
	_, err := fw.w.Write([]byte{FTNoData<<3 | 0x04})
	return err
}

// FileReader reads the AMR-WB storage format (RFC 4867, section 5; ".awb"
// files) as RTP payloads, one per 20 ms frame, to pass to a Decoder or send.
type FileReader struct {
	r      *bufio.Reader
	format payloadFormat
	data   [maxFrameBytes]byte
}

// NewFileReader reads and checks the header of a single-channel AMR-WB file
// from r, and returns a reader whose payloads are in format f.
func NewFileReader(r io.Reader, f PayloadFormat) (*FileReader, error) {
	br := bufio.NewReader(r)
	magic, err := br.Peek(len(fileMagicMC))
	if err != nil && len(magic) < len(fileMagic) {
		return nil, errNotAWB
	}
	switch {
	case bytes.HasPrefix(magic, []byte(fileMagic)):
		br.Discard(len(fileMagic))
	case bytes.Equal(magic, []byte(fileMagicMC)):
		return nil, errMultiChannel
	default:
		return nil, errNotAWB
	}
	return &FileReader{r: br, format: f.format()}, nil
}

// ReadPayload reads the next 20 ms frame and writes it into dst, which must
// hold MaxPayloadBytes bytes, as a single-frame RTP payload. It returns 0 for
// a NO_DATA frame: decode it with Decoder.DecodeNoData, and send nothing. At
// the end of the file it returns io.EOF, and io.ErrUnexpectedEOF if the file
// ends inside a frame.
func (fr *FileReader) ReadPayload(dst []byte) (int, error) {
	hdr, err := fr.r.ReadByte()
	if err != nil {
		return 0, err // io.EOF at a frame boundary
	}
	ft := int(hdr>>3) & 0x0F
	bits, ok := frameBitsByFT(ft)
	if !ok {
		return 0, errFileFrameType
	}
	data := fr.data[:(bits+7)/8]
	if _, err := io.ReadFull(fr.r, data); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return 0, err
	}
	if ft == FTNoData {
		return 0, nil
	}
	n, ok := fr.format.packInto(dst, noCMR, []frame{{ft: ft, q: hdr&0x04 != 0, data: data}})
	if !ok {
		return 0, errShortDst
	}
	return n, nil
}
