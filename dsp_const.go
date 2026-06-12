package amrwb

// Decoder constants mirrored from the opencore-amrwb reference
// (pvamrwbdecoder_cnst.h, qpisf_2s.cpp). AMR-WB codes at an internal 12.8 kHz
// sampling rate (L_FRAME = 256 samples / 20 ms), then upsamples to 16 kHz.
const (
	cL_FRAME    = 256 // internal frame size at 12.8 kHz
	cL_SUBFR16k = 80
	cL_SUBFR    = 64 // subframe size
	cNB_SUBFR   = 4  // subframes per frame
	cL_NEXT     = 64
	cL_WINDOW   = 384
	cL_TOTAL    = 384
	cM          = 16 // LP filter order
	cM16k       = 20
	cL_FILT16k  = 15
	cL_FILT     = 12

	cGP_CLIP   = 15565
	cPIT_SHARP = 27853

	cPIT_MIN    = 34
	cPIT_FR2    = 128
	cPIT_FR1_9b = 160
	cPIT_FR1_8b = 92
	cPIT_MAX    = 231
	cL_INTERPOL = 16 + 1

	cOPL_DECIM = 2

	cPREEMPH_FAC = 22282
	cGAMMA1      = 30147
	cTILT_FAC    = 22282

	cQ_MAX = 8

	cRANDOM_INITSEED = 21845

	cL_MEANBUF       = 3
	cONE_PER_MEANBUF = 10923

	cORDER   = 16
	cISF_GAP = 128

	cMP1   = cM + 1    // 17: LP coefficient count
	cNC    = cM / 2    // 8
	cNC16k = cM16k / 2 // 10

	// ISF predictive-quantizer constants (qpisf_2s.cpp).
	cISF_MU        = 10923 // prediction factor 1/3 in Q15
	cISF_ALPHA     = 29491 // 0.9 in Q15
	cISF_ONE_ALPHA = 32768 - cISF_ALPHA
)

// DTX global states and RX frame types (dtx.h).
const (
	cSPEECH   = 0
	cDTX      = 1
	cDTX_MUTE = 2

	cRX_SPEECH_GOOD              = 0
	cRX_SPEECH_PROBABLY_DEGRADED = 1
	cRX_SPEECH_LOST              = 2
	cRX_SPEECH_BAD               = 3
	cRX_SID_FIRST                = 4
	cRX_SID_UPDATE               = 5
	cRX_SID_BAD                  = 6
	cRX_NO_DATA                  = 7
)

// AMR-WB mode indices used internally by the DSP (pvamrwbdecoder_cnst.h).
const (
	dMODE_7k = iota
	dMODE_9k
	dMODE_12k
	dMODE_14k
	dMODE_16k
	dMODE_18k
	dMODE_20k
	dMODE_23k
	dMODE_24k
	dMRDTX
)
