package verify

import (
	"math/big"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/ichijohodaka/symmatinv/pkg/symmat"
	"github.com/ichijohodaka/symmatinv/pkg/tape"
)

func newRNG() *rand.Rand { return rand.New(rand.NewPCG(20260914, 4)) }

// corrupt は命令列を1か所だけ壊した複製を返す。
//
// symmat.Inverse が返す命令列は使い回されるので、複製せずに書き換えると
// 他のテストまで巻き添えになる。
func corrupt(t *testing.T, n int) *tape.Tape {
	t.Helper()
	inv, err := symmat.Inverse(n)
	if err != nil {
		t.Fatal(err)
	}
	src := inv.Tape()
	bad := &tape.Tape{
		NumVars:  src.NumVars,
		Instrs:   append([]tape.Instr(nil), src.Instrs...),
		Outputs:  append([]int32(nil), src.Outputs...),
		VarNames: src.VarNames,
		OutNames: src.OutNames,
	}
	// 最後の掛け算を足し算に変える。命令列としては正しいまま、答えだけが狂う。
	for i := len(bad.Instrs) - 1; i >= 0; i-- {
		if bad.Instrs[i].Op == tape.OpMul {
			bad.Instrs[i].Op = tape.OpAdd
			if err := bad.Validate(); err != nil {
				t.Fatalf("壊した命令列が Validate を通らない: %v", err)
			}
			return bad
		}
	}
	t.Fatal("掛け算が見つからない")
	return nil
}

// TestResidualCatchesWrongTape は、間違った命令列を渡したときに残差が
// はっきり大きくなることを確かめる。検算が検算として働いていることの担保で、
// これが通らないと「残差が小さい」という報告に意味がなくなる。
func TestResidualCatchesWrongTape(t *testing.T) {
	bad := corrupt(t, 3)
	res, err := Residual(3, bad, newRNG(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if res < 1e-3 {
		t.Errorf("壊した命令列なのに残差が %g しかない", res)
	}
}

// TestExactCatchesWrongTape は、間違った命令列を厳密検算が見逃さないことを
// 確かめる。
func TestExactCatchesWrongTape(t *testing.T) {
	bad := corrupt(t, 3)
	err := Exact(3, bad, newRNG(), 1)
	if err == nil {
		t.Fatal("壊した命令列なのにエラーにならない")
	}
	if !strings.Contains(err.Error(), "A·A⁻¹") {
		t.Errorf("どこが食い違ったか分かるエラーであるべき: %v", err)
	}
}

// TestChecksAcceptCorrectTape は、正しい命令列を誤って弾かないことを確かめる。
// 上の2つと合わせて、検算が両方向に効いていることになる。
func TestChecksAcceptCorrectTape(t *testing.T) {
	inv, err := symmat.Inverse(4)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Residual(4, inv.Tape(), newRNG(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if res > 1e-12 {
		t.Errorf("正しい命令列なのに残差 %g", res)
	}
	if err := Exact(4, inv.Tape(), newRNG(), 2); err != nil {
		t.Errorf("正しい命令列なのに弾かれた: %v", err)
	}
}

// TestShapeMismatch は、次数と食い違う命令列を渡したら断ることを確かめる。
func TestShapeMismatch(t *testing.T) {
	inv, err := symmat.Inverse(3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Residual(4, inv.Tape(), newRNG(), 1); err == nil {
		t.Error("Residual: 次数違いはエラーになるはず")
	}
	if err := Exact(4, inv.Tape(), newRNG(), 1); err == nil {
		t.Error("Exact: 次数違いはエラーになるはず")
	}
	if err := DetExact(3, inv.Tape(), newRNG(), 1); err == nil {
		t.Error("DetExact: 出力1個でなければエラーになるはず")
	}
}

// TestDetLaplaceKnown は、行列式の基準にしている余因子展開そのものを
// 手で計算できる行列で確かめる。基準が間違っていたら検算に意味がない。
func TestDetLaplaceKnown(t *testing.T) {
	rat := func(v ...int64) []*big.Rat {
		out := make([]*big.Rat, len(v))
		for i, x := range v {
			out[i] = big.NewRat(x, 1)
		}
		return out
	}
	cases := []struct {
		name string
		n    int
		a    []*big.Rat
		want int64
	}{
		{"1次", 1, rat(7), 7},
		{"2次", 2, rat(1, 2, 3, 4), -2},
		{"3次 上三角", 3, rat(2, 9, 9, 0, 3, 9, 0, 0, 4), 24},
		{"3次 一般", 3, rat(6, 1, 1, 4, -2, 5, 2, 8, 7), -306},
		{"特異", 3, rat(1, 2, 3, 2, 4, 6, 1, 0, 1), 0},
		{"4次 単位行列", 4, rat(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detLaplace(c.n, c.a)
			if got.Cmp(big.NewRat(c.want, 1)) != 0 {
				t.Errorf("detLaplace = %s, want %d", got, c.want)
			}
		})
	}
}

// TestDetExactCatchesWrongTape は行列式の検算も両方向に効くことを確かめる。
func TestDetExactCatchesWrongTape(t *testing.T) {
	tp, err := symmat.Determinant(3)
	if err != nil {
		t.Fatal(err)
	}
	if err := DetExact(3, tp, newRNG(), 2); err != nil {
		t.Fatalf("正しい命令列なのに弾かれた: %v", err)
	}

	bad := &tape.Tape{
		NumVars: tp.NumVars,
		Instrs:  append([]tape.Instr(nil), tp.Instrs...),
		Outputs: append([]int32(nil), tp.Outputs...),
	}
	bad.Instrs = append(bad.Instrs, tape.Un(tape.OpNeg, bad.Outputs[0]))
	bad.Outputs[0] = bad.ResultSlot(len(bad.Instrs) - 1) // 符号だけ反転させる
	if err := DetExact(3, bad, newRNG(), 1); err == nil {
		t.Error("符号を反転させたのにエラーにならない")
	}
}

// TestRandomSPD は、作られる行列が対称で対角優位（したがって正則）であることを
// 確かめる。ここが崩れると検算そのものが不安定になる。
func TestRandomSPD(t *testing.T) {
	rng := newRNG()
	for n := 1; n <= 8; n++ {
		a := RandomSPD(n, rng)
		if len(a) != symmat.NumEntries(n) {
			t.Fatalf("n=%d: 長さ %d, want %d", n, len(a), symmat.NumEntries(n))
		}
		full := make([]float64, n*n)
		symmat.ToFull(n, a, full)
		for i := 0; i < n; i++ {
			off := 0.0
			for j := 0; j < n; j++ {
				if full[i*n+j] != full[j*n+i] {
					t.Fatalf("n=%d: 対称でない (%d,%d)", n, i, j)
				}
				if i != j {
					off += abs(full[i*n+j])
				}
			}
			if d := abs(full[i*n+i]); d <= off {
				t.Errorf("n=%d 行 %d: 対角 %g が非対角の和 %g を超えていない", n, i, d, off)
			}
		}
	}
}

func TestRandomSPDRat(t *testing.T) {
	rng := newRNG()
	for n := 1; n <= 6; n++ {
		a := RandomSPDRat(n, rng)
		if len(a) != symmat.NumEntries(n) {
			t.Fatalf("n=%d: 長さ %d, want %d", n, len(a), symmat.NumEntries(n))
		}
		for i := 0; i < n; i++ {
			off := new(big.Rat)
			for j := 0; j < n; j++ {
				if i == j {
					continue
				}
				off.Add(off, new(big.Rat).Abs(a[symmat.Index(n, i, j)]))
			}
			d := new(big.Rat).Abs(a[symmat.Index(n, i, i)])
			if d.Cmp(off) <= 0 {
				t.Errorf("n=%d 行 %d: 対角 %s が非対角の和 %s を超えていない", n, i, d, off)
			}
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
