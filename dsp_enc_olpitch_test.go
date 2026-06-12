package amrwb

import "testing"

func TestHpWspRejectsDC(t *testing.T) {
	const lg = 128
	wsp := make([]int16, lg)
	for i := range wsp {
		wsp[i] = 4000
	}
	out := make([]int16, lg)
	mem := make([]int16, 9)
	Hp_wsp(wsp, out, lg, mem)
	for i := 40; i < lg; i++ {
		if out[i] < -200 || out[i] > 200 {
			t.Fatalf("Hp_wsp leaked DC at %d: %d", i, out[i])
		}
	}
}

func TestMedOlag(t *testing.T) {
	hist := []int16{0, 0, 0, 0, 0}
	for _, lag := range []int16{50, 52, 51, 100, 49} {
		medOlag(lag, hist)
	}
	// history now holds 49,100,51,52,50 -> median = 51.
	if got := medOlag(51, hist); got == 0 {
		t.Errorf("medOlag returned 0, expected a real median (hist=%v)", hist)
	}
}

func TestPitchMedOlFindsPeriod(t *testing.T) {
	const period = 60
	const lFrame = 128
	const lMax = 115
	const wspOff = 130
	wsp := make([]int16, wspOff+lFrame)
	// Impulse train at `period` — a clean open-loop pitch proxy.
	for i := range wsp {
		if i%period == 0 {
			wsp[i] = 10000
		}
	}
	oldHpWsp := make([]int16, lMax+lFrame)
	hpMem := make([]int16, 9)
	var olGain int16
	tm := Pitch_med_ol(wsp, wspOff, 0, &olGain, hpMem, oldHpWsp, 0, lFrame)
	if tm < period-3 || tm > period+3 {
		t.Errorf("Pitch_med_ol found lag %d, want ~%d", tm, period)
	}
}
