package amrwb

import "testing"

func TestDecGain2Init(t *testing.T) {
	mem := make([]int16, 23)
	decGain2Init(mem)
	for i := 0; i < 4; i++ {
		if mem[i] != -14336 {
			t.Errorf("mem[%d]=%d, want -14336", i, mem[i])
		}
	}
	if mem[22] != 21845 {
		t.Errorf("mem[22]=%d, want 21845", mem[22])
	}
}

func TestDecGain2GoodFrame(t *testing.T) {
	mem := make([]int16, 23)
	decGain2Init(mem)
	code := make([]int16, cL_SUBFR)
	for i := range code {
		code[i] = int16(50 - i)
	}
	var gainPit int16
	var gainCod int32
	const index = 10
	dec_gain2_amr_wb(index, 6, code, cL_SUBFR, &gainPit, &gainCod,
		0, 0, 0, 0, 5, mem)

	// In a good frame the pitch gain is taken directly from the table (Q14).
	if gainPit != t_qua_gain6b[index<<1] {
		t.Errorf("gainPit=%d, want %d", gainPit, t_qua_gain6b[index<<1])
	}
	if gainCod <= 0 {
		t.Errorf("gainCod=%d, want > 0", gainCod)
	}
	// Past energy memory must have been updated away from the init value.
	if mem[0] == -14336 {
		t.Errorf("past_qua_en[0] not updated: %d", mem[0])
	}
}
