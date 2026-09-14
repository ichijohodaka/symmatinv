package tape

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

// inv2 は 2 次対称行列の逆行列（上三角）を手で書いた命令列。
//
//	入力 [a00 a01 a11]
//	det = a00*a11 - a01*a01
//	r   = 1/det
//	出力 [b00 b01 b11] = [a11*r, -(a01*r), a00*r]
//
// このパッケージのテストはすべてこれを土台にする。中身が小さく、正解を
// 手で書けるため。
func inv2() *Tape {
	return &Tape{
		NumVars:  3,
		VarNames: []string{"a00", "a01", "a11"},
		Instrs: []Instr{
			Bin(OpMul, 0, 2), // t0 = a00*a11   slot 3
			Bin(OpMul, 1, 1), // t1 = a01*a01   slot 4
			Bin(OpSub, 3, 4), // t2 = det       slot 5
			Un(OpRecip, 5),   // t3 = 1/det     slot 6
			Bin(OpMul, 2, 6), // t4 = b00       slot 7
			Bin(OpMul, 1, 6), // t5 = a01*r     slot 8
			Un(OpNeg, 8),     // t6 = b01       slot 9
			Bin(OpMul, 0, 6), // t7 = b11       slot 10
		},
		Outputs:  []int32{7, 9, 10},
		OutNames: []string{"b00", "b01", "b11"},
	}
}

func TestValidateOK(t *testing.T) {
	if err := inv2().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateNG(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Tape)
		want string
	}{
		{"前方参照", func(tp *Tape) { tp.Instrs[0].A = 3 }, "スロット範囲"},
		{"負のスロット", func(tp *Tape) { tp.Instrs[0].A = -1 }, "スロット範囲"},
		{"単項の B が -1 でない", func(tp *Tape) { tp.Instrs[3].B = 0 }, "単項演算の B"},
		{"未知の演算", func(tp *Tape) { tp.Instrs[0].Op = Op(99) }, "未知の演算"},
		{"出力が範囲外", func(tp *Tape) { tp.Outputs[0] = 99 }, "範囲"},
		{"VarNames の長さ違い", func(tp *Tape) { tp.VarNames = []string{"a"} }, "VarNames"},
		{"OutNames の長さ違い", func(tp *Tape) { tp.OutNames = []string{"a"} }, "OutNames"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tp := inv2()
			c.mut(tp)
			err := tp.Validate()
			if err == nil {
				t.Fatalf("エラーになるはずが nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("エラーに %q を含むはず: %v", c.want, err)
			}
		})
	}
}

func TestEvalFloat64(t *testing.T) {
	tp := inv2()
	// A = [[4,1],[1,2]], det = 7, A^-1 = (1/7)[[2,-1],[-1,4]]
	vars := []float64{4, 1, 2}
	out := make([]float64, len(tp.Outputs))
	EvalFloat64(tp, vars, NewScratch[float64](tp), out)

	want := []float64{2.0 / 7, -1.0 / 7, 4.0 / 7}
	for i := range want {
		if math.Abs(out[i]-want[i]) > 1e-15 {
			t.Errorf("out[%d] = %g, want %g", i, out[i], want[i])
		}
	}
}

// TestEvalAgreesWithFast は、体を通す一般の Eval と float64 専用の
// EvalFloat64 が同じ値を返すことを確かめる。速い方だけを使っても安全だという
// 根拠になる。
func TestEvalAgreesWithFast(t *testing.T) {
	tp := inv2()
	vars := []float64{4, 1, 2}

	fast := make([]float64, 3)
	EvalFloat64(tp, vars, NewScratch[float64](tp), fast)

	slow := make([]float64, 3)
	Eval(Float64Field{}, tp, vars, NewScratch[float64](tp), slow)

	for i := range fast {
		if fast[i] != slow[i] {
			t.Errorf("out[%d]: EvalFloat64 %v, Eval %v", i, fast[i], slow[i])
		}
	}
}

// TestEvalRatExact は有理数体で評価し、丸め誤差なしに正解と一致することを
// 確かめる。浮動小数の「たまたま一致」を排除するための検算。
func TestEvalRatExact(t *testing.T) {
	tp := inv2()
	rat := func(a, b int64) *big.Rat { return big.NewRat(a, b) }

	vars := []*big.Rat{rat(4, 1), rat(1, 1), rat(2, 1)}
	out := make([]*big.Rat, 3)
	Eval(RatField{}, tp, vars, NewScratch[*big.Rat](tp), out)

	want := []*big.Rat{rat(2, 7), rat(-1, 7), rat(4, 7)}
	for i := range want {
		if out[i].Cmp(want[i]) != 0 {
			t.Errorf("out[%d] = %s, want %s", i, out[i], want[i])
		}
	}
}

// TestInverseIdentityRat は A·A^-1 = I を有理数で厳密に確かめる。
// 「逆行列である」という定義そのものに照らした検算。
func TestInverseIdentityRat(t *testing.T) {
	tp := inv2()
	a00, a01, a11 := big.NewRat(4, 1), big.NewRat(1, 1), big.NewRat(2, 1)

	out := make([]*big.Rat, 3)
	Eval(RatField{}, tp, []*big.Rat{a00, a01, a11}, NewScratch[*big.Rat](tp), out)
	b00, b01, b11 := out[0], out[1], out[2]

	// A·B の各成分。A も B も対称なので4成分すべてを書き下す。
	mul := func(x, y *big.Rat) *big.Rat { return new(big.Rat).Mul(x, y) }
	add := func(x, y *big.Rat) *big.Rat { return new(big.Rat).Add(x, y) }

	got := [2][2]*big.Rat{
		{add(mul(a00, b00), mul(a01, b01)), add(mul(a00, b01), mul(a01, b11))},
		{add(mul(a01, b00), mul(a11, b01)), add(mul(a01, b01), mul(a11, b11))},
	}
	for i := range got {
		for j := range got[i] {
			want := big.NewRat(0, 1)
			if i == j {
				want = big.NewRat(1, 1)
			}
			if got[i][j].Cmp(want) != 0 {
				t.Errorf("(A·B)[%d][%d] = %s, want %s", i, j, got[i][j], want)
			}
		}
	}
}

func TestRecipZeroPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RatField.Recip(0) は panic するはず")
		}
	}()
	RatField{}.Recip(big.NewRat(0, 1))
}

func TestOpCounts(t *testing.T) {
	c := inv2().OpCounts()
	want := map[Op]int{OpMul: 5, OpSub: 1, OpRecip: 1, OpNeg: 1}
	for op, n := range want {
		if c[op] != n {
			t.Errorf("%s: %d, want %d", op, c[op], n)
		}
	}
	if got := inv2().NumInstrs(); got != 8 {
		t.Errorf("NumInstrs = %d, want 8", got)
	}
}

func TestParseOpRoundTrip(t *testing.T) {
	for _, op := range []Op{OpAdd, OpSub, OpMul, OpNeg, OpRecip} {
		got, err := ParseOp(op.String())
		if err != nil {
			t.Fatalf("ParseOp(%q): %v", op, err)
		}
		if got != op {
			t.Errorf("ParseOp(%q) = %v", op, got)
		}
	}
	if _, err := ParseOp("pow"); err == nil {
		t.Error("未知の演算名はエラーになるはず")
	}
}

func TestString(t *testing.T) {
	s := inv2().String()
	for _, want := range []string{"t0 = a00 * a11", "t3 = 1 / t2", "t6 = -t5", "b01 := t6"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() に %q がない:\n%s", want, s)
		}
	}
}
