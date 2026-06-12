package amrwb

import "testing"

func TestQGain2(t *testing.T) {
	xn := make([]int16, cL_SUBFR)
	y1 := make([]int16, cL_SUBFR)
	y2 := make([]int16, cL_SUBFR)
	code := make([]int16, cL_SUBFR)
	for i := range xn {
		xn[i] = int16(1500 - i*20)
		y1[i] = int16(1400 - i*19) // adaptive cb close to target -> nonzero pitch gain
		y2[i] = int16((i * 11) % 300)
		code[i] = int16((i*7)%200 - 100)
	}
	gCoeff := make([]int16, 5)
	G_pitch(xn, y1, gCoeff, cL_SUBFR)

	mem := make([]int16, 4)
	initQGain2(mem)
	var gainPit int16 = 8192
	var gainCod int32
	index := Q_gain2(xn, y1, 0, y2, code, gCoeff, cL_SUBFR, 7, &gainPit, &gainCod, 0, mem)

	if index < 0 || index >= cNB_QUA_GAIN7B {
		t.Fatalf("index %d out of range [0,%d)", index, cNB_QUA_GAIN7B)
	}
	// gain_pit must equal the table entry for the chosen index.
	if gainPit != t_qua_gain7b[index*2] {
		t.Errorf("gainPit=%d != table[%d]=%d", gainPit, index, t_qua_gain7b[index*2])
	}
	if gainPit < 0 || gainPit > 19661 {
		t.Errorf("gainPit=%d out of Q14 range [0,1.2]", gainPit)
	}
	if gainCod <= 0 {
		t.Errorf("gainCod=%d, want > 0", gainCod)
	}
	// predictor memory updated away from init.
	if mem[0] == -14336 {
		t.Error("Q_gain2 did not update predictor memory")
	}
}
