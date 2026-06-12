.PHONY: test bench bench-vs-c bench-vs-c-enc bench-vs-c-dec vet fmt

# Paths to the locally built Apache-2.0 reference harnesses, and the comparison
# workload size. Override on the command line, e.g.
#   make bench-vs-c AMRWB_ENC=/path/to/enc AMRWB_DIFF=/path/to/dec
# or via the environment. The ?= defaults assume the harnesses live in /tmp.
AMRWB_ENC ?= /tmp/amrwbenc
AMRWB_DIFF ?= /tmp/amrwbdiff
AMRWB_BENCH_FRAMES ?= 4000

# Unit tests (pure Go, no external dependencies).
test:
	go test .

# Go micro-benchmarks: encode/decode ns/op + xRT real-time factor, per mode.
bench:
	go test -bench 'BenchmarkEncode|BenchmarkDecode' -benchmem .

# Go-vs-C speed comparison (both directions). Each sub-test SKIPs unless its
# harness env var is set, so this prints whichever harnesses are available.
bench-vs-c: bench-vs-c-enc bench-vs-c-dec

# Encoder Go-vs-vo-amrwbenc per-mode table.
bench-vs-c-enc:
	AMRWB_ENC=$(AMRWB_ENC) AMRWB_BENCH_FRAMES=$(AMRWB_BENCH_FRAMES) \
		go test -run TestEncSpeedVsCReference -v .

# Decoder Go-vs-opencore-amrwb per-mode table.
bench-vs-c-dec:
	AMRWB_DIFF=$(AMRWB_DIFF) AMRWB_BENCH_FRAMES=$(AMRWB_BENCH_FRAMES) \
		go test -run TestDecSpeedVsCReference -v .

vet:
	go vet ./...

fmt:
	gofmt -l .
