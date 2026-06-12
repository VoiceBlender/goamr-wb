# goamr-wb

A **pure-Go AMR-WB (ITU-T G.722.2 / 3GPP TS 26.190) codec** — no cgo. Encoder and
decoder for all nine speech modes (6.60–23.85 kbit/s), plus RFC 4867 RTP payload
framing in both octet-aligned and bandwidth-efficient formats.

Every Go AMR-WB option on pkg.go.dev is a cgo wrapper around the C `opencore-amrwb` /
`vo-amrwbenc` libraries. This package is a hand-port of that fixed-point reference to Go,
so it builds with `CGO_ENABLED=0` and cross-compiles anywhere Go does. The DSP is
**bit-exact** against the C reference for all nine modes (see the differential tests).

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

## Performance

Both directions run **far faster than real time** on a single core (a 20 ms frame's budget is
20 ms), and the pure-Go codec is **within ~7–42 % of the optimized scalar C reference** —
mode-8 decode is essentially at parity (1.07×). The hot FIR/correlation kernels use an AVX2
assembly path on `amd64`; other architectures fall back to a bit-identical pure-Go
implementation (correct, but not SIMD-accelerated).

Measured on an **AMD Ryzen 9 7900 (Zen 4, AVX2), Go 1.26.3, linux/amd64, single core**.
`Go ×RT` = how many times faster than real time one core runs (so also ≈ concurrent streams
per core). `C` is the Apache-2.0 `vo-amrwbenc` / `opencore-amrwb` reference, measured across a
subprocess with the two-point slope method (3000 frames) to cancel process/I-O overhead; the
residual per-frame pipe copy modestly favors the in-process Go figure. Deterministic synthetic
speech. Reproduce with `make bench` (Go-only) and `make bench-vs-c` (needs the C harnesses).

**Encode** (µs per 20 ms frame):

| Mode (kbit/s) | Go | C | Go/C | Go ×RT |
|---|---|---|---|---|
| 6.60  | 102.8 |  94.3 | 1.09 | 195× |
| 8.85  | 133.2 | 104.4 | 1.28 | 150× |
| 12.65 | 157.1 | 117.4 | 1.34 | 127× |
| 14.25 | 172.7 | 124.7 | 1.39 | 116× |
| 15.85 | 174.0 | 127.1 | 1.37 | 115× |
| 18.25 | 179.4 | 128.7 | 1.39 | 112× |
| 19.85 | 187.4 | 133.5 | 1.40 | 107× |
| 23.05 | 186.4 | 131.0 | 1.42 | 107× |
| 23.85 | 171.8 | 129.1 | 1.33 | 116× |

**Decode** (µs per 20 ms frame):

| Mode (kbit/s) | Go | C | Go/C | Go ×RT |
|---|---|---|---|---|
| 6.60  | 32.1 | 26.2 | 1.23 | 623× |
| 8.85  | 28.2 | 23.7 | 1.19 | 710× |
| 12.65 | 26.4 | 21.8 | 1.21 | 757× |
| 14.25 | 25.5 | 21.4 | 1.19 | 783× |
| 15.85 | 25.7 | 22.0 | 1.17 | 778× |
| 18.25 | 26.1 | 22.3 | 1.18 | 765× |
| 19.85 | 26.5 | 22.6 | 1.17 | 755× |
| 23.05 | 27.0 | 23.3 | 1.16 | 741× |
| 23.85 | 27.8 | 26.0 | 1.07 | 720× |

## Tests & benchmarks

```bash
# Unit tests (pure Go, no external dependencies)
go test .          # or: make test

# Go micro-benchmarks (encode/decode ns/op + xRT real-time factor, per mode)
go test -bench 'BenchmarkEncode|BenchmarkDecode' -benchmem .   # or: make bench
```

### Differential & speed-vs-C tests (optional, need the C reference)

The bit-exact differential tests and the Go-vs-C speed comparison run a locally built
harness around the Apache-2.0 `vo-amrwbenc` (encoder) / `opencore-amrwb` (decoder) C
sources. They **skip** unless pointed at that harness:

```bash
# Encoder: all 9 modes bit-exact vs vo-amrwbenc
AMRWB_ENC=/path/to/enc-harness go test -run TestEncDiffAgainstCReference .
# Decoder: bit-exact PCM vs opencore-amrwb
AMRWB_DIFF=/path/to/dec-harness go test -run TestDiffAgainstCReference .

# Go-vs-C speed comparison (per-mode ns/frame, ratio, xRealtime)
AMRWB_ENC=/path/to/enc-harness  go test -run TestEncSpeedVsCReference -v .
AMRWB_DIFF=/path/to/dec-harness go test -run TestDecSpeedVsCReference -v .
# or, via the Makefile (defaults to harnesses in /tmp; override as shown):
make bench-vs-c AMRWB_ENC=/path/to/enc-harness AMRWB_DIFF=/path/to/dec-harness
```

The encoder harness reads 320 int16 LE per frame on stdin (mode as argv[1]) and writes
`[ToC byte][packed speech bytes]` per frame; the decoder harness reads speech bytes per
frame on stdin and writes 320 int16 LE per frame. `AMRWB_BENCH_FRAMES` tunes the
comparison's frame count (default 4000).

## License

Apache-2.0. This is a derivative of the Apache-2.0 `opencore-amrwb` / `vo-amrwbenc`
references — see [NOTICE](NOTICE) for attribution and [LICENSE](LICENSE) for terms.
