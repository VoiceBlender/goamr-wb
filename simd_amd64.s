//go:build amd64

#include "textflag.h"

// func firRawAVX2(dst []int32, x []int16, coef32 []int32)
// dst[o] = sum_t x[o+t]*coef32[t], vectorised across 8 outputs per iteration:
// lane k accumulates output og+k, so no per-output horizontal reduction.
TEXT ·firRawAVX2(SB), NOSPLIT, $0-72
	MOVQ dst_base+0(FP), DI
	MOVQ dst_len+8(FP), CX     // number of outputs
	MOVQ x_base+24(FP), SI     // &x[og] (group base)
	MOVQ coef32_base+48(FP), BX
	MOVQ coef32_len+56(FP), R9 // taps

	MOVQ CX, R10
	SHRQ $3, R10               // R10 = groups of 8 outputs
	JZ   firtail

grouploop:
	VPXOR Y0, Y0, Y0
	MOVQ  SI, R11              // x cursor for this group
	MOVQ  BX, R12              // coef cursor
	MOVQ  R9, R13             // tap counter

taploop:
	VPMOVSXWD    (R11), Y1     // 8 int16 x[og+t..og+t+7] -> 8 int32
	VPBROADCASTD (R12), Y2     // coef32[t] in all 8 lanes
	VPMULLD      Y2, Y1, Y3
	VPADDD       Y3, Y0, Y0
	ADDQ         $2, R11       // next tap shifts x window by 1 int16
	ADDQ         $4, R12
	DECQ         R13
	JNZ          taploop

	VMOVDQU Y0, (DI)
	ADDQ    $32, DI            // 8 int32 stored
	ADDQ    $16, SI            // next group: x base += 8 int16
	DECQ    R10
	JNZ     grouploop

firtail:
	ANDQ $7, CX                // remaining outputs (<8)
	JZ   firdone

tailout:
	MOVQ SI, R11
	MOVQ BX, R12
	MOVQ R9, R13
	XORL AX, AX

tailtap:
	MOVWLSX (R11), DX
	MOVL    (R12), R8
	IMULL   R8, DX
	ADDL    DX, AX
	ADDQ    $2, R11
	ADDQ    $4, R12
	DECQ    R13
	JNZ     tailtap

	MOVL AX, (DI)
	ADDQ $4, DI
	ADDQ $2, SI                // next single output: x base += 1 int16
	DECQ CX
	JNZ  tailout

firdone:
	VZEROUPPER
	RET

// func firDotAVX2(a, b []int16) int32
// Returns sum_i a[i]*b[i] (wrapping int32) over len(a) elements. VPMADDWD folds
// each int16 pair into an int32 (exact mod 2^32); lanes accumulate, then a
// horizontal sum + scalar tail. All adds wrap, so the order matches the scalar.
TEXT ·firDotAVX2(SB), NOSPLIT, $0-52
	MOVQ  a_base+0(FP), SI
	MOVQ  a_len+8(FP), CX
	MOVQ  b_base+24(FP), DI
	VPXOR Y0, Y0, Y0           // accumulator: 8 int32 lanes

	MOVQ CX, R10
	SHRQ $4, R10               // groups of 16 int16
	JZ   dtail

dloop:
	VMOVDQU  (SI), Y1          // 16 int16 a
	VMOVDQU  (DI), Y2          // 16 int16 b
	VPMADDWD Y2, Y1, Y3        // 8 int32: a[2k]*b[2k] + a[2k+1]*b[2k+1]
	VPADDD   Y3, Y0, Y0
	ADDQ     $32, SI
	ADDQ     $32, DI
	DECQ     R10
	JNZ      dloop

dtail:
	// horizontal sum of the 8 int32 lanes -> AX
	VEXTRACTI128 $1, Y0, X1
	VPADDD       X1, X0, X0    // 4 int32
	VPSHUFD      $0x4E, X0, X1 // [2,3,0,1]
	VPADDD       X1, X0, X0    // 2 int32
	VPSHUFD      $0xB1, X0, X1 // [1,0,3,2]
	VPADDD       X1, X0, X0    // 1 int32
	MOVD         X0, AX

	ANDQ $15, CX               // remaining elements (<16)
	JZ   ddone

dscalar:
	MOVWLSX (SI), DX
	MOVWLSX (DI), R8
	IMULL   R8, DX
	ADDL    DX, AX
	ADDQ    $2, SI
	ADDQ    $2, DI
	DECQ    CX
	JNZ     dscalar

ddone:
	VZEROUPPER
	MOVL AX, ret+48(FP)
	RET
