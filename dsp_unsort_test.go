package amrwb

import "testing"

// Each sort table must be a permutation of [0, nbBits): every internal
// parameter index hit exactly once. This validates the converted tables
// (no duplicates, no out-of-range, no gaps) and that de-sorting is lossless.
func TestSortTablesArePermutations(t *testing.T) {
	for mode := 0; mode < len(sortTables); mode++ {
		st := sortTables[mode]
		n := int(unpacked_size[mode])
		if len(st) != n {
			t.Errorf("mode %d: sort table len %d != unpacked_size %d", mode, len(st), n)
			continue
		}
		seen := make([]bool, n)
		for k, idx := range st {
			if int(idx) < 0 || int(idx) >= n {
				t.Errorf("mode %d: sort[%d]=%d out of range [0,%d)", mode, k, idx, n)
				continue
			}
			if seen[idx] {
				t.Errorf("mode %d: index %d appears twice", mode, idx)
			}
			seen[idx] = true
		}
		for idx, s := range seen {
			if !s {
				t.Errorf("mode %d: internal index %d never mapped", mode, idx)
			}
		}
	}
}

// mimeUnsort must round-trip a known bit through the table: setting transmitted
// bit k yields BIT_1 at internal index sortTable[k].
func TestMimeUnsortScatters(t *testing.T) {
	mode := int16(2) // 12.65k
	st := sortTables[mode]
	// Only bits in whole processed bytes are mapped (see mimeUnsort).
	for _, k := range []int{0, 1, 50, 100, 200} {
		packed := make([]byte, (len(st)+7)/8)
		packed[k/8] |= 1 << uint(7-k%8)
		out := mimeUnsort(packed, mode)
		if out[st[k]] != cBIT_1 {
			t.Errorf("bit %d -> internal %d not set", k, st[k])
		}
		// Exactly one internal bit set.
		count := 0
		for _, v := range out {
			if v == cBIT_1 {
				count++
			}
		}
		if count != 1 {
			t.Errorf("bit %d: %d internal bits set, want 1", k, count)
		}
	}
}
