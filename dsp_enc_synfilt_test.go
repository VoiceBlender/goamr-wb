package amrwb

import "testing"

func TestSynFiltIdentity(t *testing.T) {
	// A(z)=1 (a[0]=4096, rest 0): output = a0*x with a0=a[0]/2 -> ~x/2.
	a := make([]int16, 17)
	a[0] = 4096
	x := make([]int16, cL_SUBFR)
	for i := range x {
		x[i] = int16(1000 + i*5)
	}
	y := make([]int16, cL_SUBFR)
	mem := make([]int16, 16)
	Syn_filt(a, x, y, cL_SUBFR, mem, 1)
	for i := range y {
		want := int(x[i]) / 2
		if d := int(y[i]) - want; d < -2 || d > 2 {
			t.Fatalf("Syn_filt identity y[%d]=%d, want ~%d", i, y[i], want)
		}
	}
}

func TestLPDecim2(t *testing.T) {
	const l = 128
	const C = 5000
	x := make([]int16, l)
	for i := range x {
		x[i] = C
	}
	mem := make([]int16, 3)
	LP_Decim2(x, l, mem)
	// Output occupies first l/2 samples; DC ~preserved (unity-gain FIR).
	for i := 5; i < l/2; i++ {
		if d := int(x[i]) - C; d < -30 || d > 30 {
			t.Fatalf("LP_Decim2 DC x[%d]=%d, want ~%d", i, x[i], C)
		}
	}
}

func TestRandomDeterministic(t *testing.T) {
	a := int16(21845)
	b := int16(21845)
	for i := 0; i < 50; i++ {
		if Random(&a) != Random(&b) {
			t.Fatal("Random not deterministic for equal seeds")
		}
	}
	s := int16(21845)
	if Random(&s) == 21845 {
		t.Error("Random did not advance")
	}
}
