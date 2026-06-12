package amrwb

import "testing"

// Build a stable LPC vector from a test signal, then check the analysis chain
// and an Az_isp -> Isp_Az round-trip (Isp_Az is the bit-exact decoder path).
func lpcFromSignal(t *testing.T) []int16 {
	t.Helper()
	sig := make([]int16, cL_WINDOW)
	// Two sinusoids so the autocorrelation is well-conditioned.
	for i := range sig {
		v := 8000.0*sinApprox(float64(i)*0.10) + 4000.0*sinApprox(float64(i)*0.31)
		sig[i] = int16(v)
	}
	rH := make([]int16, cM+1)
	rL := make([]int16, cM+1)
	Autocorr(sig, cM, rH, rL)
	Lag_window(rH, rL)
	a := make([]int16, cM+1)
	rc := make([]int16, cM)
	mem := make([]int16, 18)
	Levinson(rH, rL, a, rc, mem)
	if a[0] != 4096 {
		t.Fatalf("Levinson a[0]=%d, want 4096", a[0])
	}
	return a
}

func TestLpcAnalysisRoundTrip(t *testing.T) {
	a := lpcFromSignal(t)
	isp := make([]int16, cM)
	oldIsp := make([]int16, cM)
	copy(oldIsp, ispInit[:])
	Az_isp(a, isp, oldIsp)

	// The 15 ISP roots (cosine domain) must be strictly decreasing; isp[M-1] is
	// the separate a[M]<<3 term, not part of the root sequence.
	for i := 1; i < cM-1; i++ {
		if isp[i] >= isp[i-1] {
			t.Fatalf("ISPs not decreasing at %d: %d >= %d", i, isp[i], isp[i-1])
		}
	}

	// Round-trip ISP -> LPC via the validated decoder path, compare to a[].
	a2 := make([]int16, cM+1)
	Isp_Az(isp, a2, cM, 0)
	for i := 0; i <= cM; i++ {
		d := int(a[i]) - int(a2[i])
		if d < -8 || d > 8 {
			t.Errorf("LPC round-trip a[%d]=%d a2=%d (diff %d)", i, a[i], a2[i], d)
		}
	}
}

// sinApprox is a small Taylor range-reduced sine to avoid importing math in a
// codec test (values need only be deterministic, not precise).
func sinApprox(x float64) float64 {
	// reduce to [-pi, pi]
	const pi = 3.14159265358979
	for x > pi {
		x -= 2 * pi
	}
	for x < -pi {
		x += 2 * pi
	}
	x2 := x * x
	return x * (1 - x2/6*(1-x2/20*(1-x2/42)))
}
