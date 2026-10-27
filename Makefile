.PHONY: test harness diff bench bench-vs-c bench-vs-c-enc bench-vs-c-dec vet fmt

# Reference harnesses around the stock Apache-2.0 opencore-amrwb (decoder) and
# vo-amrwbenc (encoder) shared libraries, built from testdata/refharness by
# `make harness`. Only the runtime libraries are needed (Debian/Ubuntu:
# libopencore-amrwb0 libvo-amrwbenc0). On other systems override the link
# flags, e.g. on macOS with Homebrew:
#   make harness CFLAGS=-I/opt/homebrew/include \
#     AMRWB_DEC_LIBS="-L/opt/homebrew/lib -lopencore-amrwb" \
#     AMRWB_ENC_LIBS="-L/opt/homebrew/lib -lvo-amrwbenc"
HARNESS_DIR := testdata/refharness
AMRWB_DEC_LIBS ?= -l:libopencore-amrwb.so.0
AMRWB_ENC_LIBS ?= -l:libvo-amrwbenc.so.0
AMRWB_DIFF ?= $(HARNESS_DIR)/bin/amrwbdec
AMRWB_ENC ?= $(HARNESS_DIR)/bin/amrwbenc
AMRWB_BENCH_FRAMES ?= 4000

# Unit tests (pure Go, no external dependencies).
test:
	go test .

# Build the reference harnesses.
harness: $(HARNESS_DIR)/bin/amrwbdec $(HARNESS_DIR)/bin/amrwbenc

$(HARNESS_DIR)/bin/amrwbdec: $(HARNESS_DIR)/amrwbdec.c
	@mkdir -p $(@D)
	$(CC) $(CFLAGS) -O2 -o $@ $< $(AMRWB_DEC_LIBS)

$(HARNESS_DIR)/bin/amrwbenc: $(HARNESS_DIR)/amrwbenc.c
	@mkdir -p $(@D)
	$(CC) $(CFLAGS) -O2 -o $@ $< $(AMRWB_ENC_LIBS)

# Bit-exact differential tests against the reference harnesses.
diff: harness
	AMRWB_DIFF=$(abspath $(AMRWB_DIFF)) AMRWB_ENC=$(abspath $(AMRWB_ENC)) \
		go test -run 'DiffAgainstCReference' -v .

# Go micro-benchmarks: encode/decode ns/op + xRT real-time factor, per mode.
bench:
	go test -bench 'BenchmarkEncode|BenchmarkDecode' -benchmem .

# Go-vs-C speed comparison (both directions). Each sub-test SKIPs unless its
# harness env var is set, so this prints whichever harnesses are available.
bench-vs-c: harness bench-vs-c-enc bench-vs-c-dec

# Encoder Go-vs-vo-amrwbenc per-mode table.
bench-vs-c-enc:
	AMRWB_ENC=$(abspath $(AMRWB_ENC)) AMRWB_BENCH_FRAMES=$(AMRWB_BENCH_FRAMES) \
		go test -run TestEncSpeedVsCReference -v .

# Decoder Go-vs-opencore-amrwb per-mode table.
bench-vs-c-dec:
	AMRWB_DIFF=$(abspath $(AMRWB_DIFF)) AMRWB_BENCH_FRAMES=$(AMRWB_BENCH_FRAMES) \
		go test -run TestDecSpeedVsCReference -v .

vet:
	go vet ./...

fmt:
	gofmt -l .
