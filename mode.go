// Package amrwb implements the AMR-WB (ITU-T G.722.2 / 3GPP TS 26.190) wideband
// speech codec in pure Go, with RFC 4867 RTP payload framing.
//
// AMR-WB operates on 16 kHz mono audio in 20 ms frames (320 samples). It defines
// nine speech coding modes (0..8) spanning 6.60 to 23.85 kbit/s, plus a comfort
// noise (SID) frame used by DTX.
package amrwb

// SampleRate is the fixed AMR-WB sampling rate in Hz.
const SampleRate = 16000

// FrameSamples is the number of PCM samples in one 20 ms AMR-WB frame.
const FrameSamples = 320

// Mode identifies an AMR-WB speech coding mode (0..8).
type Mode int

const (
	Mode0660 Mode = iota // 6.60 kbit/s
	Mode0885             // 8.85 kbit/s
	Mode1265             // 12.65 kbit/s
	Mode1425             // 14.25 kbit/s
	Mode1585             // 15.85 kbit/s
	Mode1825             // 18.25 kbit/s
	Mode1985             // 19.85 kbit/s
	Mode2305             // 23.05 kbit/s
	Mode2385             // 23.85 kbit/s
	numSpeechModes
)

// Frame type (FT) values carried in the RFC 4867 ToC byte. Values 0..8 are the
// speech modes; the remainder describe comfort noise and no-data frames.
const (
	FTMode0660   = 0
	FTMode0885   = 1
	FTMode1265   = 2
	FTMode1425   = 3
	FTMode1585   = 4
	FTMode1825   = 5
	FTMode1985   = 6
	FTMode2305   = 7
	FTMode2385   = 8
	FTSID        = 9  // comfort noise (SID)
	FTSpeechLost = 14 // speech lost (RFC 4867)
	FTNoData     = 15 // no data
)

// modeBits is the number of speech bits per AMR-WB mode (0..8). These are the
// canonical class-A+B+C bit counts from 3GPP TS 26.201 Table 2.
var modeBits = [numSpeechModes]int{
	132, // 6.60
	177, // 8.85
	253, // 12.65
	285, // 14.25
	317, // 15.85
	365, // 18.25
	397, // 19.85
	461, // 23.05
	477, // 23.85
}

// sidBits is the number of bits in an AMR-WB SID (comfort noise) frame.
const sidBits = 40

// frameBitsByFT returns the number of payload bits for a given frame type and
// whether the frame type is a recognized one that carries bits. SID and speech
// modes carry bits; SPEECH_LOST and NO_DATA carry none.
func frameBitsByFT(ft int) (bits int, ok bool) {
	switch {
	case ft >= FTMode0660 && ft <= FTMode2385:
		return modeBits[ft], true
	case ft == FTSID:
		return sidBits, true
	case ft == FTSpeechLost, ft == FTNoData:
		return 0, true
	default:
		return 0, false
	}
}

// frameBytesByFT returns the number of octet-aligned bytes occupied by a frame
// of the given type (bits rounded up to whole octets).
func frameBytesByFT(ft int) (bytes int, ok bool) {
	bits, ok := frameBitsByFT(ft)
	if !ok {
		return 0, false
	}
	return (bits + 7) / 8, true
}

// Bits returns the number of speech bits produced by this mode.
func (m Mode) Bits() int {
	if m < 0 || m >= numSpeechModes {
		return 0
	}
	return modeBits[m]
}

// Bytes returns the octet-aligned byte count of a frame in this mode.
func (m Mode) Bytes() int { return (m.Bits() + 7) / 8 }

// FrameType returns the RFC 4867 frame-type value for this mode.
func (m Mode) FrameType() int { return int(m) }

// Valid reports whether m is a defined speech mode.
func (m Mode) Valid() bool { return m >= Mode0660 && m < numSpeechModes }

// Bitrate returns the nominal bit rate of the mode in bit/s.
func (m Mode) Bitrate() int {
	switch m {
	case Mode0660:
		return 6600
	case Mode0885:
		return 8850
	case Mode1265:
		return 12650
	case Mode1425:
		return 14250
	case Mode1585:
		return 15850
	case Mode1825:
		return 18250
	case Mode1985:
		return 19850
	case Mode2305:
		return 23050
	case Mode2385:
		return 23850
	default:
		return 0
	}
}
