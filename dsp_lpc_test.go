package amrwb

import "testing"

func TestIsfIspCosMapping(t *testing.T) {
	// isf all zero -> every isp maps through cosTable[0] = 32767.
	isf := make([]int16, cM)
	isp := make([]int16, cM)
	Isf_isp(isf, isp, cM)
	for i, v := range isp {
		if v != 32767 {
			t.Fatalf("isp[%d]=%d, want 32767 for zero isf", i, v)
		}
	}
	// isf[0] = 128 selects cosTable index 1 exactly (offset 0).
	isf[0] = 128
	Isf_isp(isf, isp, cM)
	if isp[0] != cosTable[1] {
		t.Errorf("isp[0]=%d, want %d (cosTable[1])", isp[0], cosTable[1])
	}
}

func TestIspAzLeadingCoeff(t *testing.T) {
	// Isp_Az must always emit a[0] = 4096 (Q12 1.0) and fill MP1 coefficients.
	isp := make([]int16, cM)
	for i := range isp {
		// Evenly spaced cosine-domain values (descending), a valid ISP set.
		isp[i] = int16(30000 - i*3500)
	}
	a := make([]int16, cMP1)
	Isp_Az(isp, a, cM, 0)
	if a[0] != 4096 {
		t.Errorf("a[0]=%d, want 4096", a[0])
	}
}

func TestReorderIsfEnforcesGap(t *testing.T) {
	isf := []int16{500, 400, 600, 100, 2000}
	Reorder_isf(isf, cISF_GAP, int16(len(isf)))
	for i := 1; i < len(isf); i++ {
		if isf[i]-isf[i-1] < 0 {
			t.Errorf("isf not non-decreasing at %d: %v", i, isf)
		}
	}
}

func TestInterpolateIspFillsFourSubframes(t *testing.T) {
	ispOld := make([]int16, cM)
	ispNew := make([]int16, cM)
	for i := range ispOld {
		ispOld[i] = int16(30000 - i*3500)
		ispNew[i] = int16(29000 - i*3400)
	}
	frac := []int16{8192, 16384, 24576}
	az := make([]int16, 4*cMP1)
	interpolate_isp(ispOld, ispNew, frac, az)
	for sf := 0; sf < 4; sf++ {
		if az[sf*cMP1] != 4096 {
			t.Errorf("subframe %d a[0]=%d, want 4096", sf, az[sf*cMP1])
		}
	}
}
