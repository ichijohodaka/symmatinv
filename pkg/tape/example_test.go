package tape_test

import (
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ichijohodaka/symmatinv/pkg/tape"
)

// quadratic は判別式 b² − 4ac を求める命令列を手で組み立てたもの。
//
//	入力 [a b c]
//	t0 = b*b        スロット 3
//	t1 = a*c        スロット 4
//	t2 = t1+t1      スロット 5   （定数を持たないので 2 倍は足し算で作る）
//	t3 = t2+t2      スロット 6   （4ac）
//	t4 = t0-t3      スロット 7
func quadratic() *tape.Tape {
	return &tape.Tape{
		NumVars:  3,
		VarNames: []string{"a", "b", "c"},
		Instrs: []tape.Instr{
			tape.Bin(tape.OpMul, 1, 1),
			tape.Bin(tape.OpMul, 0, 2),
			tape.Bin(tape.OpAdd, 4, 4),
			tape.Bin(tape.OpAdd, 5, 5),
			tape.Bin(tape.OpSub, 3, 6),
		},
		Outputs:  []int32{7},
		OutNames: []string{"D"},
	}
}

func ExampleEvalFloat64() {
	tp := quadratic()
	out := make([]float64, 1)

	// a=1, b=5, c=6 なら 25 − 24 = 1
	tape.EvalFloat64(tp, []float64{1, 5, 6}, tape.NewScratch[float64](tp), out)
	fmt.Println(out[0])
	// Output: 1
}

// 同じ命令列を複素数の上で動かす。命令列は数を1つも持たないので、どの体で
// 評価するかは呼ぶときに決まる。
func ExampleEval_complex() {
	tp := quadratic()
	out := make([]complex128, 1)

	// a=1, b=2i, c=1 なら (2i)² − 4 = −4 − 4 = −8
	vars := []complex128{1, 2i, 1}
	tape.Eval(tape.Complex128Field{}, tp, vars, tape.NewScratch[complex128](tp), out)
	fmt.Println(out[0])
	// Output: (-8+0i)
}

// 有理数の上で動かすと丸め誤差が出ない。
func ExampleEval_rationals() {
	tp := quadratic()
	out := make([]*big.Rat, 1)

	// a=1/3, b=1, c=1/2 なら 1 − 4·(1/6) = 1/3
	vars := []*big.Rat{big.NewRat(1, 3), big.NewRat(1, 1), big.NewRat(1, 2)}
	tape.Eval(tape.RatField{}, tp, vars, tape.NewScratch[*big.Rat](tp), out)
	fmt.Println(out[0])
	// Output: 1/3
}

func ExampleTape_String() {
	fmt.Print(quadratic())
	// Output:
	// t0 = b * b
	// t1 = a * c
	// t2 = t1 + t1
	// t3 = t2 + t2
	// t4 = t0 - t3
	// D := t4
}

func ExampleWriteJSON() {
	if err := tape.WriteJSON(os.Stdout, quadratic()); err != nil {
		panic(err)
	}
	// Output:
	// {
	//   "version": 1,
	//   "numVars": 3,
	//   "varNames": ["a","b","c"],
	//   "instrs": [
	//     ["mul",1,1],
	//     ["mul",0,2],
	//     ["add",4,4],
	//     ["add",5,5],
	//     ["sub",3,6]
	//   ],
	//   "outputs": [7],
	//   "outNames": ["D"]
	// }
}

func ExampleReadJSON() {
	src := `{
	  "version": 1,
	  "numVars": 2,
	  "instrs": [["add",0,1],["recip",2]],
	  "outputs": [3]
	}`
	tp, err := tape.ReadJSON(strings.NewReader(src))
	if err != nil {
		panic(err)
	}
	out := make([]float64, 1)
	tape.EvalFloat64(tp, []float64{1, 3}, tape.NewScratch[float64](tp), out)
	fmt.Println(out[0]) // 1/(1+3)
	// Output: 0.25
}

// Validate は外から読み込んだ命令列を評価する前に通す。読み出し元が自分より
// 後ろのスロットを指していれば、上から順に実行しても値が決まらない。
func ExampleTape_Validate() {
	bad := &tape.Tape{
		NumVars: 1,
		Instrs:  []tape.Instr{tape.Un(tape.OpNeg, 1)}, // 自分自身を読んでいる
		Outputs: []int32{1},
	}
	fmt.Println(bad.Validate())
	// Output:
	// tape: 命令 0 (neg): A=1 がスロット範囲 [0,1) の外
}

// Prune は出力に寄与しない命令を落とす。
func ExampleTape_Prune() {
	tp := quadratic()
	tp.Instrs = append(tp.Instrs, tape.Bin(tape.OpMul, 0, 1)) // 誰も読まない
	fmt.Println(tp.NumInstrs(), "->", tp.Prune().NumInstrs())
	// Output: 6 -> 5
}

func ExampleTape_OpCounts() {
	c := quadratic().OpCounts()
	fmt.Printf("add %d, mul %d, sub %d\n", c[tape.OpAdd], c[tape.OpMul], c[tape.OpSub])
	// Output: add 2, mul 2, sub 1
}
