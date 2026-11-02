package amrwb

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"testing"
)

// TestFileWriterDiffAgainstCReference stores the public Encoder's payloads,
// in every payload format and one or three frames per payload, with a
// FileWriter, and requires the bytes of vo-amrwbenc's storage-format output
// (AMRWB_ENC) behind the AMR-WB magic number.
func TestFileWriterDiffAgainstCReference(t *testing.T) {
	sig := dtxSignal()
	sig = sig[:len(sig)/FrameSamples/3*3*FrameSamples] // whole groups of 3
	modeOf := func(f int) int { return (f/30)%9 | dtxFlag }
	want := append([]byte(fileMagic), encodeStorage(t, sig, modeOf)...)
	frames := len(sig) / FrameSamples
	for _, f := range allFormats {
		for _, k := range []int{1, 3} {
			enc, _ := NewEncoder(EncoderConfig{Mode: Mode0660, DTX: true, OctetAligned: f.pf.octetAligned, CRC: f.pf.crc, RobustSorting: f.pf.robustSorting})
			var file bytes.Buffer
			pf := PayloadFormat{OctetAligned: f.pf.octetAligned, CRC: f.pf.crc, RobustSorting: f.pf.robustSorting}
			fw, err := NewFileWriter(&file, pf)
			if err != nil {
				t.Fatal(err)
			}
			for g := 0; g+k <= frames; g += k {
				enc.SetMode(Mode(modeOf(g) &^ dtxFlag))
				p, err := enc.Encode(sig[g*FrameSamples : (g+k)*FrameSamples])
				if err != nil {
					t.Fatal(err)
				}
				n := 0
				if len(p) > 0 {
					if n, err = fw.WritePayload(p); err != nil {
						t.Fatal(err)
					}
				}
				for ; n < k; n++ {
					fw.WriteNoData()
				}
			}
			if !bytes.Equal(file.Bytes(), want) {
				t.Fatalf("%s, %d frames per payload: file differs from the reference", f.name, k)
			}
		}
	}
}

// TestFileRoundTrip writes payloads to a file, reads them back in every
// payload format, and requires the PCM of decoding the original payloads.
func TestFileRoundTrip(t *testing.T) {
	sig := dtxSignal()
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode1585, DTX: true, OctetAligned: true})
	ref := NewDecoder(DecoderConfig{OctetAligned: true})
	var file bytes.Buffer
	fw, _ := NewFileWriter(&file, PayloadFormat{OctetAligned: true})
	var want []int16
	pcm := make([]int16, FrameSamples)
	frames := len(sig) / FrameSamples
	for i := 0; i < frames; i++ {
		p, _ := enc.Encode(sig[i*FrameSamples : (i+1)*FrameSamples])
		if len(p) == 0 {
			fw.WriteNoData()
			ref.DecodeNoData(pcm)
		} else {
			fw.WritePayload(p)
			ref.DecodeTo(pcm, p)
		}
		want = append(want, pcm...)
	}
	for _, f := range allFormats {
		pf := PayloadFormat{OctetAligned: f.pf.octetAligned, CRC: f.pf.crc, RobustSorting: f.pf.robustSorting}
		fr, err := NewFileReader(bytes.NewReader(file.Bytes()), pf)
		if err != nil {
			t.Fatal(err)
		}
		dec := NewDecoder(DecoderConfig(pf))
		var got []int16
		dst := make([]byte, MaxPayloadBytes)
		for {
			n, err := fr.ReadPayload(dst)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: %v", f.name, err)
			}
			if n == 0 {
				dec.DecodeNoData(pcm)
			} else if _, err := dec.DecodeTo(pcm, dst[:n]); err != nil {
				t.Fatalf("%s: %v", f.name, err)
			}
			got = append(got, pcm...)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%s: %d samples read back, want %d, or PCM differs", f.name, len(got), len(want))
		}
	}
}

func TestFileReaderErrors(t *testing.T) {
	for _, c := range []struct {
		name string
		file string
		want error
	}{
		{"empty", "", errNotAWB},
		{"amr-nb", "#!AMR\n\x3c", errNotAWB},
		{"multi-channel", fileMagicMC + "\x00\x00\x00\x01", errMultiChannel},
	} {
		if _, err := NewFileReader(bytes.NewReader([]byte(c.file)), PayloadFormat{}); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	dst := make([]byte, MaxPayloadBytes)
	read := func(file string) error {
		fr, err := NewFileReader(bytes.NewReader([]byte(file)), PayloadFormat{})
		if err != nil {
			return err
		}
		_, err = fr.ReadPayload(dst)
		return err
	}
	if err := read(fileMagic); err != io.EOF {
		t.Errorf("no frames: %v, want io.EOF", err)
	}
	if err := read(fileMagic + "\x44\x00"); err != io.ErrUnexpectedEOF { // 6.60 cut short
		t.Errorf("truncated frame: %v, want io.ErrUnexpectedEOF", err)
	}
	if err := read(fileMagic + "\x54"); err != errFileFrameType { // FT 10
		t.Errorf("FT 10: %v, want errFileFrameType", err)
	}
	// SPEECH_LOST is read as a payload; NO_DATA as none.
	fr, _ := NewFileReader(bytes.NewReader([]byte(fileMagic+"\x74\x7c")), PayloadFormat{OctetAligned: true})
	if n, err := fr.ReadPayload(dst); err != nil || !bytes.Equal(dst[:n], []byte{0xF0, 0x74}) {
		t.Errorf("SPEECH_LOST: payload %x, %v", dst[:n], err)
	}
	if n, err := fr.ReadPayload(dst); err != nil || n != 0 {
		t.Errorf("NO_DATA: %d bytes, %v", n, err)
	}
}

func TestFileWriterRejectsEmptyPayload(t *testing.T) {
	fw, _ := NewFileWriter(io.Discard, PayloadFormat{})
	if _, err := fw.WritePayload(nil); err == nil {
		t.Error("empty payload accepted")
	}
}
