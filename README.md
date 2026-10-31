# goamr-wb

A **pure-Go AMR-WB (ITU-T G.722.2 / 3GPP TS 26.190) codec** — no cgo. Encoder and
decoder for all nine speech modes (6.60–23.85 kbit/s) with DTX and comfort noise, plus
RFC 4867 RTP payload framing in both octet-aligned and bandwidth-efficient formats.

Every Go AMR-WB option on pkg.go.dev is a cgo wrapper around the C `opencore-amrwb` /
`vo-amrwbenc` libraries. This package is a hand-port of that fixed-point reference to Go,
so it builds with `CGO_ENABLED=0` and cross-compiles anywhere Go does.

The decoder is **bit-exact** with `opencore-amrwb`'s `D_IF_decode` in all nine modes for
speech frames, lost frames (concealment), mode switches, decoder homing frames and comfort
noise, including its 14-bit output. Known difference:

- **Q bit:** frames with Q=0 are treated as bad frames (RFC 4867). `D_IF_decode` ignores
  the Q bit, so bad speech and SID frames have no reference to compare against.

The encoder is **bit-exact** with `vo-amrwbenc`'s `E_IF_encode`, with DTX on or off, in all
nine modes, from quiet to clipped input and across mode switches, including the VAD flag
and SID frames. Known gap:

- **Encoder homing:** an input frame of the encoder homing pattern does not reset the
  encoder, as it does in `E_IF_encode`, so the output differs after such a frame.

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

`Encode` takes exactly `amrwb.FrameSamples` (320) samples; `Decode` accepts a full RFC 4867
payload and returns concatenated PCM for every frame it carries.

### DTX

With `EncoderConfig{DTX: true}` the encoder stops sending speech during silence. It sends
a short SID frame, usually every 8th frame, and an empty payload for the frames in between.
The sender has to skip those, while keeping the RTP clock running:

```go
payload, _ := enc.Encode(frame)
if len(payload) > 0 { // empty: NO_DATA, send nothing (RFC 4867, section 4.3.2)
    send(payload, timestamp, enc.Talkspurt()) // Talkspurt: the RTP marker bit
}
timestamp += amrwb.FrameSamples
```

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

## Performance

Both directions run **far faster than real time** on a single core (a 20 ms frame's budget is
20 ms). The hot FIR/correlation kernels (open-loop pitch, autocorrelation, normalized
correlation, residual, and adaptive-codebook interpolation) use AVX2 assembly on `amd64` —
`firRaw` (sliding correlation) and `firDot` (horizontal dot product) — falling back to a
bit-identical pure-Go path elsewhere (or on `amd64` with `-tags purego`). An encoder pass
vectorized the open-loop pitch / autocorrelation / `normCorr` kernels and made `L_shl` O(1) (**encode 8–16 %** faster); a later
pass routed the decoder's `Pred_lt4` adaptive-codebook interpolation through `firDot`
(**decode ~6–9 %** faster, halving the gap to C). Every kernel is fuzz-tested bit-identical to
its scalar fallback (see the bit-exactness notes above).

Measured on an **AMD Ryzen 9 7900 (Zen 4, AVX2), Go 1.26.3, linux/amd64, single core**.
`Go ×RT` = how many times faster than real time one core runs (so also ≈ concurrent streams
per core). `C` is the Apache-2.0 `vo-amrwbenc` / `opencore-amrwb` reference, measured across a
subprocess with the two-point-slope method (4000 frames) to cancel process startup; the
residual per-frame pipe copy modestly favors the in-process Go figure. The tables below are a
single back-to-back run, so Go and C are measured under identical conditions and the `Go/C`
ratio is apples-to-apples. **Caveat:** absolute figures still depend on CPU boost state and
system load, so treat `Go ×RT` as order-of-magnitude and expect the C subprocess timing to
drift between sessions. Deterministic synthetic speech. Reproduce with `make bench` (Go-only)
and `make bench-vs-c` (needs the C harnesses).

**Encode** (µs per 20 ms frame):

| Mode (kbit/s) | Go | C | Go/C | Go ×RT |
|---|---|---|---|---|
| 6.60  |  87.8 |  66.6 | 1.32 | 228× |
| 8.85  | 119.2 |  77.7 | 1.53 | 168× |
| 12.65 | 143.9 |  90.9 | 1.58 | 139× |
| 14.25 | 157.6 |  98.6 | 1.60 | 127× |
| 15.85 | 159.3 |  98.9 | 1.61 | 126× |
| 18.25 | 168.0 | 102.8 | 1.63 | 119× |
| 19.85 | 174.7 | 107.2 | 1.63 | 115× |
| 23.05 | 172.8 | 106.1 | 1.63 | 116× |
| 23.85 | 157.8 |  98.5 | 1.60 | 127× |

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

The remaining encode gap is the algebraic codebook search (`corHVec012`, `searchIxiy`,
`ACELP_4t64`) — a branchy depth-first search that's scalar in the C reference too, so it
doesn't vectorize.

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

The decoder test feeds identical streams to the public `Decoder` (both payload formats) and
to `D_IF_decode`, and requires identical PCM. The streams cover random bits, reference-encoded
speech from quiet to clipped, mode switches, lost frames (including bursts and a lost first
frame), decoder homing frames, and DTX: reference streams with and without losses, comfort
noise muting, and SID frames with random parameters in orders no encoder produces. The
encoder tests compare the same speech signal and a two-tone signal, through the public
`Encoder` with mode switches and both payload formats, and with DTX on, switched during a
stream, and against noise that varies in level and spectrum.
To run a test directly, set `AMRWB_DIFF` (decoder harness) and `AMRWB_ENC` (encoder harness)
to the built binaries. `AMRWB_BENCH_FRAMES` tunes the speed comparison's frame count
(default 4000).

## License

Apache-2.0. This is a derivative of the Apache-2.0 `opencore-amrwb` / `vo-amrwbenc`
references — see [NOTICE](NOTICE) for attribution and [LICENSE](LICENSE) for terms.
