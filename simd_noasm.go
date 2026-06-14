//go:build !amd64

package amrwb

// firRaw falls back to the portable implementation on non-amd64 targets.
func firRaw(dst []int32, x []int16, coef []int16) { firRawGeneric(dst, x, coef) }

// firDot falls back to the portable implementation on non-amd64 targets.
func firDot(a, b []int16) int32 { return firDotGeneric(a, b) }
