package dag

import (
	"math"
	"testing"

	"github.com/ichijohodaka/symmatinv/pkg/tape"
)

// TestHashConsing は同じ式を2回作っても節点が1つしかできないことを確かめる。
// これが共通部分式除去の実体。
func TestHashConsing(t *testing.T) {
	b := New(2)
	x, y := b.Var(0), b.Var(1)
	p := b.Mul(x, y)
	q := b.Mul(x, y)
	if p != q {
		t.Errorf("同じ式が別の節点になった: %d, %d", p, q)
	}
	if n := b.NumNodes(); n != 3 {
		t.Errorf("節点数 = %d, want 3（変数2 + 積1）", n)
	}
}

// TestCommutativeOrder は x*y と y*x が同じ節点になることを確かめる。
// 引数の順を揃えておかないと1つにまとめそこねる。
func TestCommutativeOrder(t *testing.T) {
	b := New(2)
	x, y := b.Var(0), b.Var(1)
	if b.Mul(x, y) != b.Mul(y, x) {
		t.Error("x*y と y*x が別の節点になった")
	}
	if b.Add(x, y) != b.Add(y, x) {
		t.Error("x+y と y+x が別の節点になった")
	}
	// 減算は可換でないので分かれること。
	if b.Sub(x, y) == b.Sub(y, x) {
		t.Error("x-y と y-x が同じ節点になった")
	}
}

func TestFoldNegNeg(t *testing.T) {
	b := New(1)
	x := b.Var(0)
	if got := b.Neg(b.Neg(x)); got != x {
		t.Errorf("-(-x) = %d, want %d", got, x)
	}
	if n := b.NumNodes(); n != 2 {
		t.Errorf("節点数 = %d, want 2（変数1 + Neg1）", n)
	}
}

func TestFoldRecipRecip(t *testing.T) {
	b := New(1)
	x := b.Var(0)
	if got := b.Recip(b.Recip(x)); got != x {
		t.Errorf("1/(1/x) = %d, want %d", got, x)
	}
}

// TestCompileInv2 は 2 次対称行列の逆行列を組み立て、値が正しいことを確かめる。
// pkg/tape の手書き命令列と同じ計算を、今度は式グラフ経由で作る。
func TestCompileInv2(t *testing.T) {
	b := New(3) // a00, a01, a11
	a00, a01, a11 := b.Var(0), b.Var(1), b.Var(2)

	det := b.Sub(b.Mul(a00, a11), b.Mul(a01, a01))
	r := b.Recip(det)
	b00 := b.Mul(a11, r)
	b01 := b.Neg(b.Mul(a01, r))
	b11 := b.Mul(a00, r)

	tp := b.Compile([]ID{b00, b01, b11})
	if err := tp.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if tp.NumInstrs() != 8 {
		t.Errorf("命令数 = %d, want 8:\n%s", tp.NumInstrs(), tp)
	}

	// A = [[4,1],[1,2]], det = 7
	out := make([]float64, 3)
	tape.EvalFloat64(tp, []float64{4, 1, 2}, tape.NewScratch[float64](tp), out)
	want := []float64{2.0 / 7, -1.0 / 7, 4.0 / 7}
	for i := range want {
		if math.Abs(out[i]-want[i]) > 1e-15 {
			t.Errorf("out[%d] = %g, want %g", i, out[i], want[i])
		}
	}
}

// TestCompilePrunes は出力に寄与しない節点が落ちることを確かめる。
func TestCompilePrunes(t *testing.T) {
	b := New(2)
	x, y := b.Var(0), b.Var(1)
	keep := b.Add(x, y)
	b.Mul(x, y) // 使わない

	tp := b.Compile([]ID{keep})
	if tp.NumInstrs() != 1 {
		t.Errorf("命令数 = %d, want 1:\n%s", tp.NumInstrs(), tp)
	}
}

// TestSharingSavesWork は、同じ部分式を何度も参照しても命令が増えないことを
// 確かめる。同じ部分式の共有が効いていることの、いちばん直接的な証拠。
func TestSharingSavesWork(t *testing.T) {
	b := New(2)
	x, y := b.Var(0), b.Var(1)
	s := b.Add(x, y)
	// s を5回使う式を作る。
	acc := s
	for i := 0; i < 4; i++ {
		acc = b.Mul(acc, s)
	}
	tp := b.Compile([]ID{acc})
	// s が1命令、掛け算が4命令。s を毎回作り直していれば命令数が増える。
	if tp.NumInstrs() != 5 {
		t.Errorf("命令数 = %d, want 5:\n%s", tp.NumInstrs(), tp)
	}
}

func TestVarOutOfRangePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("範囲外の変数は panic するはず")
		}
	}()
	New(2).Var(2)
}
