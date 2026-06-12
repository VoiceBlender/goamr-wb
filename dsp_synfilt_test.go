package amrwb

import "testing"

func zeroAz() []int16 {
	a := make([]int16, cM+1)
	a[0] = 4096 // Q12 1.0; a[1..M]=0 -> A(z)=1
	return a
}

func TestSynFilt32ZeroInput(t *testing.T) {
	a := zeroAz()
	exc := make([]int16, cL_SUBFR)
	sigHi := make([]int16, cM+cL_SUBFR)
	sigLo := make([]int16, cM+cL_SUBFR)
	Syn_filt_32(a, cM, exc, 1, sigHi, sigLo, cM, cL_SUBFR)
	for i := cM; i < cM+cL_SUBFR; i++ {
		if sigHi[i] != 0 || sigLo[i] != 0 {
			t.Fatalf("zero excitation produced nonzero synthesis at %d: hi=%d lo=%d", i, sigHi[i], sigLo[i])
		}
	}
}

func TestSynFilt32ProducesEnergy(t *testing.T) {
	a := zeroAz()
	exc := make([]int16, cL_SUBFR)
	for i := range exc {
		exc[i] = 200
	}
	sigHi := make([]int16, cM+cL_SUBFR)
	sigLo := make([]int16, cM+cL_SUBFR)
	Syn_filt_32(a, cM, exc, 1, sigHi, sigLo, cM, cL_SUBFR)
	var energy int64
	for i := cM; i < cM+cL_SUBFR; i++ {
		energy += int64(sigHi[i]) * int64(sigHi[i])
	}
	if energy == 0 {
		t.Fatal("nonzero excitation produced zero synthesis energy")
	}
}

func TestWbSynFiltZeroInput(t *testing.T) {
	a := zeroAz()
	x := make([]int16, cL_SUBFR)
	y := make([]int16, cL_SUBFR)
	mem := make([]int16, cM)
	yBuf := make([]int16, cM+cL_SUBFR)
	wb_syn_filt(a, cM, x, y, cL_SUBFR, mem, 1, yBuf)
	for i, v := range y {
		if v != 0 {
			t.Fatalf("zero input produced nonzero output at %d: %d", i, v)
		}
	}
	for i, v := range mem {
		if v != 0 {
			t.Fatalf("memory updated to nonzero at %d: %d", i, v)
		}
	}
}

func TestWbSynFiltProducesEnergy(t *testing.T) {
	a := zeroAz()
	x := make([]int16, cL_SUBFR)
	for i := range x {
		x[i] = 500
	}
	y := make([]int16, cL_SUBFR)
	mem := make([]int16, cM)
	yBuf := make([]int16, cM+cL_SUBFR)
	wb_syn_filt(a, cM, x, y, cL_SUBFR, mem, 1, yBuf)
	var energy int64
	for _, v := range y {
		energy += int64(v) * int64(v)
	}
	if energy == 0 {
		t.Fatal("nonzero input produced zero output energy")
	}
}
