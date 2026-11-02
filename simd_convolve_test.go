package amrwb

import "testing"

// TestConvolveFIRMatchesGeneric fuzzes convolveFIR against the scalar loop,
// including full-scale inputs where the int32 sum wraps.
func TestConvolveFIRMatchesGeneric(t *testing.T) {
	seed := uint32(1)
	rnd := func() int16 { seed = seed*1664525 + 1013904223; return int16(seed >> 16) }
	var x, h [cL_SUBFR]int16
	var a, b [cL_SUBFR]int16
	for iter := 0; iter < 2000; iter++ {
		for i := range x {
			switch iter % 4 {
			case 0:
				x[i], h[i] = -32768, -32768
			case 1:
				x[i], h[i] = 32767, -32768
			default:
				x[i], h[i] = rnd(), rnd()
			}
		}
		convolveFIR(x[:], h[:], a[:])
		convolveGeneric(x[:], h[:], b[:])
		if a != b {
			t.Fatalf("iteration %d: convolveFIR %v, generic %v", iter, a, b)
		}
	}
}

func BenchmarkConvolve(b *testing.B) {
	var x, h, y [cL_SUBFR]int16
	for i := range x {
		x[i], h[i] = int16(i*731), int16(4096-i*61)
	}
	b.Run("generic", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			convolveGeneric(x[:], h[:], y[:])
		}
	})
	b.Run("fir", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			convolveFIR(x[:], h[:], y[:])
		}
	})
}
