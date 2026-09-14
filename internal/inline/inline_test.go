package inline_test

import (
	"errors"
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/ichijohodaka/symmatinv/internal/inline"
	"github.com/ichijohodaka/symmatinv/internal/verify"
	"github.com/ichijohodaka/symmatinv/pkg/symmat"
	"github.com/ichijohodaka/symmatinv/pkg/tape"
)

func newRNG() *rand.Rand { return rand.New(rand.NewPCG(20260914, 2)) }

// TestSizeMatchesExpand は、数えた大きさと実際に展開した命令数が一致することを
// 確かめる。Size は展開せずに数えるので、これが合っていないと意味がない。
func TestSizeMatchesExpand(t *testing.T) {
	for n := 1; n <= 5; n++ {
		inv, err := symmat.Inverse(n)
		if err != nil {
			t.Fatal(err)
		}
		want := inline.Size(inv.Tape())
		got, err := inline.Expand(inv.Tape(), 1<<24)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		if big.NewInt(int64(got.NumInstrs())).Cmp(want) != 0 {
			t.Errorf("n=%d: Size %s, 実際 %d", n, want, got.NumInstrs())
		}
	}
}

// TestExpandSameValue は、展開しても答えが変わらないことを確かめる。
// 方法2 と方法3 は同じものを別の順序で計算しているだけ。
func TestExpandSameValue(t *testing.T) {
	rng := newRNG()
	for n := 1; n <= 5; n++ {
		inv, err := symmat.Inverse(n)
		if err != nil {
			t.Fatal(err)
		}
		shared := inv.Tape()
		flat, err := inline.Expand(shared, 1<<24)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		if err := flat.Validate(); err != nil {
			t.Fatalf("n=%d: 展開後が不正: %v", n, err)
		}

		ne := symmat.NumEntries(n)
		o1 := make([]float64, ne)
		o2 := make([]float64, ne)
		s1 := tape.NewScratch[float64](shared)
		s2 := tape.NewScratch[float64](flat)
		for trial := 0; trial < 5; trial++ {
			a := verify.RandomSPD(n, rng)
			tape.EvalFloat64(shared, a, s1, o1)
			tape.EvalFloat64(flat, a, s2, o2)
			for i := range o1 {
				if math.Abs(o1[i]-o2[i]) > 1e-12*(1+math.Abs(o1[i])) {
					t.Errorf("n=%d [%d]: 方法2 %g, 方法3 %g", n, i, o1[i], o2[i])
				}
			}
		}
	}
}

// TestExpandIsATree は、展開後はどの命令の結果もちょうど一度しか読まれない
// ことを確かめる。つまり共有が本当になくなっている。
func TestExpandIsATree(t *testing.T) {
	inv, err := symmat.Inverse(4)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := inline.Expand(inv.Tape(), 1<<24)
	if err != nil {
		t.Fatal(err)
	}
	uses := make([]int, flat.NumSlots())
	for _, in := range flat.Instrs {
		uses[in.A]++
		if in.Op.Arity() == 2 {
			uses[in.B]++
		}
	}
	for _, s := range flat.Outputs {
		uses[s]++
	}
	for i := range flat.Instrs {
		if u := uses[flat.ResultSlot(i)]; u != 1 {
			t.Fatalf("命令 %d の結果が %d 回読まれている（木なら1回）", i, u)
		}
	}
}

func TestExpandTooBig(t *testing.T) {
	inv, err := symmat.Inverse(6)
	if err != nil {
		t.Fatal(err)
	}
	_, err = inline.Expand(inv.Tape(), 10)
	var tooBig *inline.ErrTooBig
	if !errors.As(err, &tooBig) {
		t.Fatalf("ErrTooBig になるはず: %v", err)
	}
	if tooBig.Limit != 10 {
		t.Errorf("Limit = %d, want 10", tooBig.Limit)
	}
}

// TestMethod2vs3Table は方法2 と方法3 の命令数を並べて出す。
// この差がそのまま「同じ部分式を何度も計算する無駄」の大きさ。
func TestMethod2vs3Table(t *testing.T) {
	t.Log("  n   方法2（共有あり）   方法3（すべて代入）        倍率")
	for n := 1; n <= 12; n++ {
		inv, err := symmat.Inverse(n)
		if err != nil {
			t.Fatal(err)
		}
		shared := inv.NumInstrs()
		flat := inline.Size(inv.Tape())
		ratio := new(big.Float).Quo(new(big.Float).SetInt(flat), big.NewFloat(float64(shared)))
		t.Logf("%3d %12d %22s %11s", n, shared, flat.String(), ratio.Text('g', 4))
	}
}
