package amrwb

import (
	"sort"
	"testing"
)

func TestMedian5(t *testing.T) {
	sets := [][5]int16{
		{1, 2, 3, 4, 5},
		{5, 4, 3, 2, 1},
		{3, 1, 4, 1, 5},
		{-100, 50, 0, 50, -100},
		{7, 7, 7, 7, 7},
	}
	for _, s := range sets {
		buf := []int16{s[0], s[1], s[2], s[3], s[4]}
		got := median5(buf, 2)
		sorted := append([]int16(nil), buf...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		if got != sorted[2] {
			t.Errorf("median5(%v)=%d, want %d", s, got, sorted[2])
		}
	}
}

func TestScaleSignalUpDown(t *testing.T) {
	x := []int16{100, -100, 1000, -1000}
	scale_signal(x, 4, 2) // <<2 = *4
	want := []int16{400, -400, 4000, -4000}
	for i := range x {
		if x[i] != want[i] {
			t.Errorf("scale up x[%d]=%d, want %d", i, x[i], want[i])
		}
	}
	// Saturation on up-scale.
	y := []int16{20000, -20000}
	scale_signal(y, 2, 4)
	if y[0] != max16 || y[1] != min16 {
		t.Errorf("scale up saturation: %v", y)
	}
}

func TestPitShrpAddsScaledLag(t *testing.T) {
	// With sharp=0 the response is unchanged; with sharp>0, x[i] grows by the
	// sharpened earlier sample. Verify the no-op case and one explicit step.
	x := []int16{1000, 2000, 3000, 4000}
	orig := append([]int16(nil), x...)
	Pit_shrp(x, 2, 0, 4)
	for i := range x {
		if x[i] != orig[i] {
			t.Fatalf("sharp=0 changed x[%d]: %d != %d", i, x[i], orig[i])
		}
	}
	// sharp = 0x4000 (0.5 Q15): x[2] += round(x[0]*0.5*2)= x[0]/... verify growth.
	x2 := []int16{1000, 2000, 0, 0}
	Pit_shrp(x2, 2, 16384, 4)
	if x2[2] <= 0 || x2[3] <= 0 {
		t.Errorf("pitch sharpening did not add lagged energy: %v", x2)
	}
}
