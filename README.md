# goamr-wb

A **pure-Go AMR-WB (ITU-T G.722.2 / 3GPP TS 26.190) codec** — no cgo. Encoder and
decoder for all nine speech modes (6.60–23.85 kbit/s) with DTX and comfort noise, plus
RFC 4867 RTP payload framing (octet-aligned and bandwidth-efficient, several frames per
packet, frame CRCs, robust sorting, mode-set and mode-change rules) and the `.awb` file
format. Frame interleaving is not supported.

Every Go AMR-WB option on pkg.go.dev is a cgo wrapper around the C `opencore-amrwb` /
`vo-amrwbenc` libraries. This package is a hand-port of that fixed-point reference to Go,
so it builds with `CGO_ENABLED=0` and cross-compiles anywhere Go does.

The decoder is **bit-exact** with `opencore-amrwb`'s `D_IF_decode` in all nine modes for
speech frames, lost frames (concealment), mode switches, decoder homing frames and comfort
noise, including its 14-bit output. Known difference:

- **Q bit:** frames with Q=0 are treated as bad frames (RFC 4867). `D_IF_decode` ignores
  the Q bit, so bad speech and SID frames have no reference to compare against.

The encoder is **bit-exact** with `vo-amrwbenc`'s `E_IF_encode`, with DTX on or off, in all
nine modes, from quiet to clipped input and across mode switches, including the VAD flag,
SID frames and encoder homing frames.

Both claims are checked by the differential tests below.

## Install

```bash
go get github.com/VoiceBlender/goamr-wb
```

## Usage

```go
import amrwb "github.com/VoiceBlender/goamr-wb"

enc, _ := amrwb.NewEncoder(amrwb.EncoderConfig{Mode: amrwb.Mode2385, OctetAligned: true})
dec := amrwb.NewDecoder(amrwb.DecoderConfig{OctetAligned: true})

// One 20 ms frame = 320 samples of 16 kHz mono PCM (int16).
payload, _ := enc.Encode(frame)   // RFC 4867 RTP payload (CMR + ToC + speech bits)
pcm, _ := dec.Decode(payload)     // back to 320 PCM samples
```

`Encode` takes one or more 20 ms frames, a multiple of `amrwb.FrameSamples` (320) samples,
and returns one payload carrying them all (e.g. 960 samples for `ptime:60`); `Decode`
accepts a full RFC 4867 payload and returns concatenated PCM for every frame it carries.
`EncodeTo` and `DecodeTo` do the same without allocating; size the payload buffer with
`amrwb.MaxPayloadBytesFor(frames)`.

### Payload format (SDP parameters)

The config fields mirror the RFC 4867 `fmtp` parameters, and leaving them unset matches
their absence. Both sides must match what the session negotiated:

| SDP parameter | `EncoderConfig` | `DecoderConfig` |
|---|---|---|
| `octet-align=1` | `OctetAligned` | `OctetAligned` |
| `crc=1` | `CRC` | `CRC`: a frame failing its CRC is decoded as a bad frame |
| `robust-sorting=1` | `RobustSorting` | `RobustSorting` |
| `mode-set=0,2,5` | `ModeSet: []amrwb.Mode{0, 2, 5}` | (any mode is decoded) |
| `mode-change-period=2` | `ModeChangePeriod: 2` | |
| `mode-change-neighbor=1` | `ModeChangeNeighbor: true` | |

`crc` and `robust-sorting` imply octet-aligned framing. With a mode set, `SetMode` and
`SetModeRequest` pick the highest allowed mode at or below the one given. With a
mode-change period or neighbor rule, `SetMode` sets a target, and the encoder reaches it
over the next frames as the rules allow. `interleaving` is not supported, so don't offer
or accept it.

### Mode requests (CMR)

Each payload carries a codec mode request: the mode its sender would like to receive.
`Decoder.ModeRequest` returns the peer's latest request, or `amrwb.NoModeRequest`, and
`Encoder.SetModeRequest` sets the one this side sends (none by default):

```go
dec.DecodeTo(pcm, pkt.Payload)
if m := dec.ModeRequest(); m != amrwb.NoModeRequest {
    enc.SetMode(m) // honour the peer's request; SetMode keeps to EncoderConfig.ModeSet
}
enc.SetModeRequest(amrwb.Mode1265) // ask the peer for 12.65 kbit/s
```

### DTX

With `EncoderConfig{DTX: true}` the encoder stops sending speech during silence. It sends
a short SID frame, usually every 8th frame, and an empty payload for the frames in between.
The sender has to skip those, while keeping the RTP clock running:

```go
payload, _ := enc.Encode(frames)
if len(payload) > 0 { // empty: NO_DATA, send nothing (RFC 4867, section 4.3.2)
    send(payload, timestamp, enc.Talkspurt()) // Talkspurt: the RTP marker bit
}
timestamp += uint32(len(frames)) // 320 per 20 ms frame
```

With several frames per packet, NO_DATA frames at the end of a payload are left out, so a
payload may carry fewer frames than the packet interval.

Any peer may use DTX, whether or not this encoder does, so a receiver has to run the decoder
every 20 ms, not only when a packet arrives. Otherwise each SID frame plays as a 20 ms burst
of noise instead of continuous comfort noise:

```go
if pkt != nil { // the packet due for this 20 ms interval
    dec.DecodeTo(pcm, pkt.Payload)
} else {
    dec.DecodeNoData(pcm) // comfort noise in DTX, concealment otherwise
}
```

With several frames per packet, call `DecodeNoData` for each 20 ms interval that the packet
covers but its payload doesn't (`DecodeTo` returns 320 samples per frame).

### `.awb` files

`FileWriter` stores RTP payloads in the single-channel AMR-WB storage format (RFC 4867,
section 5), for example to record a call without transcoding. `FileReader` reads one back as
single-frame payloads:

```go
fw, _ := amrwb.NewFileWriter(f, amrwb.PayloadFormat{OctetAligned: true})
n, _ := fw.WritePayload(payload) // stores its n frames
fw.WriteNoData()                 // once per 20 ms not covered by a payload

fr, _ := amrwb.NewFileReader(f, amrwb.PayloadFormat{OctetAligned: true})
for {
    n, err := fr.ReadPayload(buf) // buf: amrwb.MaxPayloadBytes
    if err == io.EOF {
        break
    }
    if n == 0 {
        dec.DecodeNoData(pcm) // a NO_DATA frame
    } else {
        dec.DecodeTo(pcm, buf[:n])
    }
}
```

## Performance

Both directions run **far faster than real time** on a single core (a 20 ms frame's budget is
20 ms). The hot FIR/correlation kernels (open-loop pitch, autocorrelation, normalized
correlation, residual, and adaptive-codebook interpolation) use AVX2 assembly on `amd64` —
`firRaw` (sliding correlation) and `firDot` (horizontal dot product) — falling back to a
bit-identical pure-Go path elsewhere (or on `amd64` with `-tags purego`). An encoder pass
vectorized the open-loop pitch / autocorrelation / `normCorr` kernels and made `L_shl` O(1) (**encode 8–16 %** faster); a later
pass routed the decoder's `Pred_lt4` adaptive-codebook interpolation through `firDot`
(**decode ~6–9 %** faster, halving the gap to C). A third pass put the encoder's codebook
correlations (`corHVec012`, `corHVec30`), the 12.8 kHz decimation and `Convolve` on the same
kernels and removed bounds checks from `searchIxiy` (**encode 13–30 %** faster). Every kernel
is fuzz-tested bit-identical to its scalar fallback or to the verbatim port it replaced (see
the bit-exactness notes above).

Measured on an **AMD Ryzen 9 7900 (Zen 4, AVX2), Go 1.26.3, linux/amd64, single core**.
`Go ×RT` = how many times faster than real time one core runs (so also ≈ concurrent streams
per core). `C` is the Apache-2.0 `vo-amrwbenc` / `opencore-amrwb` reference, measured across a
subprocess with the two-point-slope method (4000 frames) to cancel process startup; the
residual per-frame pipe copy modestly favors the in-process Go figure. Each table below is a
single back-to-back run, so Go and C are measured under identical conditions and the `Go/C`
ratio is apples-to-apples. **Caveat:** absolute figures still depend on CPU boost state and
system load, so treat `Go ×RT` as order-of-magnitude and expect the C subprocess timing to
drift between sessions. Deterministic synthetic speech. Reproduce with `make bench` (Go-only)
and `make bench-vs-c` (needs the C harnesses).

**Encode** (µs per 20 ms frame):

| Mode (kbit/s) | Go | C | Go/C | Go ×RT |
|---|---|---|---|---|
| 6.60  |  77.1 |  65.8 | 1.17 | 259× |
| 8.85  |  94.2 |  77.1 | 1.22 | 212× |
| 12.65 | 108.6 |  90.2 | 1.20 | 184× |
| 14.25 | 118.1 |  97.5 | 1.21 | 169× |
| 15.85 | 119.5 |  99.4 | 1.20 | 167× |
| 18.25 | 123.4 | 101.4 | 1.22 | 162× |
| 19.85 | 127.1 | 105.2 | 1.21 | 157× |
| 23.05 | 126.9 | 105.6 | 1.20 | 158× |
| 23.85 | 128.5 |  97.2 | 1.32 | 156× |

**Decode** (µs per 20 ms frame):

| Mode (kbit/s) | Go | C | Go/C | Go ×RT |
|---|---|---|---|---|
| 6.60  | 30.2 | 25.8 | 1.17 | 662× |
| 8.85  | 26.7 | 23.1 | 1.16 | 749× |
| 12.65 | 24.6 | 21.8 | 1.13 | 813× |
| 14.25 | 24.5 | 21.9 | 1.12 | 817× |
| 15.85 | 24.8 | 22.0 | 1.13 | 807× |
| 18.25 | 25.2 | 22.5 | 1.12 | 794× |
| 19.85 | 25.4 | 22.5 | 1.13 | 788× |
| 23.05 | 26.2 | 23.1 | 1.13 | 765× |
| 23.85 | 26.9 | 25.8 | 1.04 | 744× |

Most of the remaining encode time is the algebraic codebook search: `searchIxiy` is a running
maximum whose every comparison depends on the previous one (already branch-free), and the
depth-first search around it is scalar in the C reference too.

## Tests & benchmarks

```bash
# Unit tests (pure Go, no external dependencies)
go test .          # or: make test

# Go micro-benchmarks (encode/decode ns/op + xRT real-time factor, per mode)
go test -bench 'BenchmarkEncode|BenchmarkDecode' -benchmem .   # or: make bench
```

### Differential & speed-vs-C tests (optional, need the C reference)

The bit-exact differential tests and the Go-vs-C speed comparison drive the stock
Apache-2.0 `opencore-amrwb` (decoder) and `vo-amrwbenc` (encoder) shared libraries through
small harnesses in `testdata/refharness`. They **skip** unless pointed at those harnesses.
CI runs them on amd64 and arm64.

```bash
# Debian/Ubuntu: the runtime libraries are enough (the harnesses declare the API).
sudo apt-get install libopencore-amrwb0 libvo-amrwbenc0

make harness   # builds testdata/refharness/bin/{amrwbdec,amrwbenc}
make diff      # all differential tests (decoder and encoder, via the public API)
GOFLAGS=-tags=purego make diff # the same, using the portable (non-AVX2) kernels
make bench-vs-c                # Go-vs-C speed comparison
# On macOS with Homebrew (brew install opencore-amr vo-amrwbenc):
make harness AMRWB_DEC_LIBS="-L/opt/homebrew/lib -lopencore-amrwb" \
             AMRWB_ENC_LIBS="-L/opt/homebrew/lib -lvo-amrwbenc"
```

Use Ubuntu 24.04's `libvo-amrwbenc0`: `speech/23.05k` fails against Ubuntu 26.04's, a rebuild
of the same source with GCC 15 and LTO. `search_ixiy` in `c4t64fx.c` overflows a signed
integer, which C leaves undefined, and that build compiles the comparison after it differently,
so the search can pick another pulse. This package wraps, as `vo-amrwbenc` does when built with
`-fwrapv`, and the 26.04 output is only a different choice of pulses: its frames still decode.

The decoder test feeds identical streams to the public `Decoder` (both payload formats) and
to `D_IF_decode`, and requires identical PCM. The streams cover random bits, reference-encoded
speech from quiet to clipped, mode switches, lost frames (including bursts and a lost first
frame), decoder homing frames, and DTX: reference streams with and without losses, comfort
noise muting, and SID frames with random parameters in orders no encoder produces. The
encoder tests compare the same speech signal and a two-tone signal, through the public
`Encoder` with mode switches and both payload formats, and with DTX on, switched during a
stream, and against noise that varies in level and spectrum, and with encoder homing
frames, alone and back to back, in speech and in DTX. Further tests pack 2, 3 and 5 frames
per payload in every payload format and require the reference's frames in each one, and
require a `FileWriter` file to match the reference's `.awb` output byte for byte.
To run a test directly, set `AMRWB_DIFF` (decoder harness) and `AMRWB_ENC` (encoder harness)
to the built binaries. `AMRWB_BENCH_FRAMES` tunes the speed comparison's frame count
(default 4000).

## License

Apache-2.0. This is a derivative of the Apache-2.0 `opencore-amrwb` / `vo-amrwbenc`
references — see [NOTICE](NOTICE) for attribution and [LICENSE](LICENSE) for terms.
