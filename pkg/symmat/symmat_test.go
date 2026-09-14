package symmat_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/ichijohodaka/symmatinv/internal/numeric"
	"github.com/ichijohodaka/symmatinv/internal/verify"
	"github.com/ichijohodaka/symmatinv/pkg/symmat"
	"github.com/ichijohodaka/symmatinv/pkg/tape"
)

func newRNG() *rand.Rand { return rand.New(rand.NewPCG(20260914, 1)) }

func TestIndexLayout(t *testing.T) {
	// n = 3 の並びが [a00 a01 a02 a11 a12 a22] であること。
	want := map[[2]int]int{
		{0, 0}: 0, {0, 1}: 1, {0, 2}: 2,
		{1, 1}: 3, {1, 2}: 4,
		{2, 2}: 5,
	}
	for k, v := range want {
		if got := symmat.Index(3, k[0], k[1]); got != v {
			t.Errorf("Index(3,%d,%d) = %d, want %d", k[0], k[1], got, v)
		}
		// 添字を入れ替えても同じ番号になること。
		if got := symmat.Index(3, k[1], k[0]); got != v {
			t.Errorf("Index(3,%d,%d) = %d, want %d", k[1], k[0], got, v)
		}
	}
	for n := 1; n <= 8; n++ {
		if got := symmat.NumEntries(n); got != n*(n+1)/2 {
			t.Errorf("NumEntries(%d) = %d", n, got)
		}
		// 全要素がちょうど一度ずつ使われること。
		seen := make([]bool, symmat.NumEntries(n))
		for i := 0; i < n; i++ {
			for j := i; j < n; j++ {
				idx := symmat.Index(n, i, j)
				if seen[idx] {
					t.Fatalf("n=%d: 番号 %d が重複", n, idx)
				}
				seen[idx] = true
			}
		}
	}
}

func TestNames(t *testing.T) {
	got := symmat.Names(3, "a")
	want := []string{"a00", "a01", "a02", "a11", "a12", "a22"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Names(3,\"a\") = %v, want %v", got, want)
	}
}

func TestToFullFromFull(t *testing.T) {
	upper := []float64{1, 2, 3, 4, 5, 6}
	full := make([]float64, 9)
	symmat.ToFull(3, upper, full)
	want := []float64{1, 2, 3, 2, 4, 5, 3, 5, 6}
	for i := range want {
		if full[i] != want[i] {
			t.Fatalf("ToFull = %v, want %v", full, want)
		}
	}
	back := make([]float64, 6)
	symmat.FromFull(3, full, back)
	for i := range upper {
		if back[i] != upper[i] {
			t.Fatalf("FromFull = %v, want %v", back, upper)
		}
	}
}

// TestInverseExact は、組み立てた命令列が本当に逆行列を計算していることを
// 有理数で厳密に確かめる。丸め誤差がないので「たまたま合った」がありえない。
func TestInverseExact(t *testing.T) {
	for n := 1; n <= 6; n++ {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			inv, err := symmat.Inverse(n)
			if err != nil {
				t.Fatalf("Inverse(%d): %v", n, err)
			}
			if err := verify.Exact(n, inv.Tape(), newRNG(), 3); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestInverseResidual は乱数を代入して A·A⁻¹ − I の大きさを見る。
// 大きい n まで速く回せるので、こちらは 8 次まで確かめる。
func TestInverseResidual(t *testing.T) {
	for n := 1; n <= 8; n++ {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			inv, err := symmat.Inverse(n)
			if err != nil {
				t.Fatalf("Inverse(%d): %v", n, err)
			}
			res, err := verify.Residual(n, inv.Tape(), newRNG(), 20)
			if err != nil {
				t.Fatal(err)
			}
			if res > 1e-12 {
				t.Errorf("残差 %g が大きすぎる", res)
			}
			t.Logf("n=%d 命令数 %3d 残差 %.2e", n, inv.NumInstrs(), res)
		})
	}
}

// TestAgainstLU は記号計算の結果を、独立に書いた数値 LU（方法1）と突き合わせる。
func TestAgainstLU(t *testing.T) {
	rng := newRNG()
	for n := 1; n <= 8; n++ {
		inv, err := symmat.Inverse(n)
		if err != nil {
			t.Fatalf("Inverse(%d): %v", n, err)
		}
		tp := inv.Tape()
		scratch := tape.NewScratch[float64](tp)
		out := make([]float64, symmat.NumEntries(n))
		af := make([]float64, n*n)
		bf := make([]float64, n*n)
		lu := numeric.NewLU(n)

		for trial := 0; trial < 10; trial++ {
			a := verify.RandomSPD(n, rng)
			tape.EvalFloat64(tp, a, scratch, out)
			symmat.ToFull(n, a, af)
			if err := lu.Inverse(af, bf); err != nil {
				t.Fatalf("n=%d: LU: %v", n, err)
			}
			for i := 0; i < n; i++ {
				for j := i; j < n; j++ {
					got := out[symmat.Index(n, i, j)]
					want := bf[i*n+j]
					if math.Abs(got-want) > 1e-10*(1+math.Abs(want)) {
						t.Errorf("n=%d (%d,%d): 記号 %g, LU %g", n, i, j, got, want)
					}
				}
			}
		}
	}
}

// TestDeterminant は行列式の命令列を、余因子展開（厳密）と数値 LU の両方と
// 突き合わせる。
func TestDeterminant(t *testing.T) {
	for n := 1; n <= 5; n++ {
		tp, err := symmat.Determinant(n)
		if err != nil {
			t.Fatalf("Determinant(%d): %v", n, err)
		}
		if err := verify.DetExact(n, tp, newRNG(), 3); err != nil {
			t.Fatal(err)
		}
		t.Logf("n=%d 行列式の命令数 %d", n, tp.NumInstrs())
	}

	// 数値でも一致すること。
	rng := newRNG()
	for n := 1; n <= 8; n++ {
		tp, err := symmat.Determinant(n)
		if err != nil {
			t.Fatal(err)
		}
		scratch := tape.NewScratch[float64](tp)
		out := make([]float64, 1)
		af := make([]float64, n*n)
		a := verify.RandomSPD(n, rng)
		tape.EvalFloat64(tp, a, scratch, out)
		symmat.ToFull(n, a, af)
		want, err := numeric.Det(n, af)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(out[0]-want) > 1e-9*math.Abs(want) {
			t.Errorf("n=%d: det 記号 %g, LU %g", n, out[0], want)
		}
	}
}

// TestPlanAffectsCountNotValue は、分け方を変えても答えは同じで、演算回数だけが
// 変わることを確かめる。分割の選択が「速さの話であって正しさの話ではない」
// ことの担保。
func TestPlanAffectsCountNotValue(t *testing.T) {
	const n = 6
	rng := newRNG()
	a := verify.RandomSPD(n, rng)

	var first []float64
	for p := 1; p < n; p++ {
		plan, err := symmat.SplitPlan(n, p)
		if err != nil {
			t.Fatal(err)
		}
		inv, err := symmat.InverseWithPlan(n, plan)
		if err != nil {
			t.Fatalf("p=%d: %v", p, err)
		}
		tp := inv.Tape()
		out := make([]float64, symmat.NumEntries(n))
		tape.EvalFloat64(tp, a, tape.NewScratch[float64](tp), out)

		if first == nil {
			first = append([]float64(nil), out...)
		} else {
			for i := range out {
				if math.Abs(out[i]-first[i]) > 1e-10*(1+math.Abs(first[i])) {
					t.Errorf("分割 %s で値が違う: [%d] %g vs %g", plan, i, out[i], first[i])
				}
			}
		}
		t.Logf("%-14s 命令数 %d", plan.String(), tp.NumInstrs())
	}
}

// TestSharingBeatsExpansion は「各段を代入せずに中間表現のまま保つ」ことが
// 効いていることを確かめる。共有が壊れていれば命令数は急激に増える。
//
// 目安として、n 次の逆行列は素朴な余因子展開なら n! 規模の項数になる。
// 6 次で 720 項を超えるようなら共有が効いていない。
func TestSharingBeatsExpansion(t *testing.T) {
	for n := 2; n <= 8; n++ {
		inv, err := symmat.Inverse(n)
		if err != nil {
			t.Fatal(err)
		}
		limit := 40 * n * n * n // 経験的な上限。桁が変わったら気づけるようにする
		if got := inv.NumInstrs(); got > limit {
			t.Errorf("n=%d: 命令数 %d が想定 (%d) を大きく超えた。共有が効いていない可能性",
				n, got, limit)
		}
	}
}

// TestInstrCountTable は n と命令数の対応を表にして出す。同梱する n の上限を
// 決めるための材料（計画書 Q9）。
func TestInstrCountTable(t *testing.T) {
	t.Log("  n  要素数   命令数   内訳")
	for n := 1; n <= 10; n++ {
		inv, err := symmat.Inverse(n)
		if err != nil {
			t.Fatalf("Inverse(%d): %v", n, err)
		}
		tp := inv.Tape()
		c := tp.OpCounts()
		t.Logf("%3d %5d %8d   add %d, sub %d, mul %d, neg %d, recip %d  [%s]",
			n, symmat.NumEntries(n), tp.NumInstrs(),
			c[tape.OpAdd], c[tape.OpSub], c[tape.OpMul], c[tape.OpNeg], c[tape.OpRecip],
			inv.Plan())
	}
}

func TestInverseRejects(t *testing.T) {
	if _, err := symmat.Inverse(0); err == nil {
		t.Error("n=0 はエラーになるはず")
	}
	if _, err := symmat.Inverse(-1); err == nil {
		t.Error("n=-1 はエラーになるはず")
	}
	bad := &symmat.Plan{N: 4, Kind: symmat.KindMIL, P: 2, Q: 3}
	if _, err := symmat.InverseWithPlan(4, bad); err == nil {
		t.Error("2+3≠4 の計画はエラーになるはず")
	}
}

// TestMemoReturnsSame は、同じ (n, 計画) を2回求めたら同じものが返ることを
// 確かめる（プロセス内の覚え書き）。
func TestMemoReturnsSame(t *testing.T) {
	a, err := symmat.Inverse(5)
	if err != nil {
		t.Fatal(err)
	}
	b, err := symmat.Inverse(5)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("2回目が作り直されている")
	}
}

func BenchmarkInverseTape6(b *testing.B) {
	inv, err := symmat.Inverse(6)
	if err != nil {
		b.Fatal(err)
	}
	tp := inv.Tape()
	a := verify.RandomSPD(6, newRNG())
	scratch := tape.NewScratch[float64](tp)
	out := make([]float64, symmat.NumEntries(6))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tape.EvalFloat64(tp, a, scratch, out)
	}
}

// BenchmarkInverseLU6 は方法1（先に数値を代入して毎回 LU）の基準。
func BenchmarkInverseLU6(b *testing.B) {
	const n = 6
	a := verify.RandomSPD(n, newRNG())
	af := make([]float64, n*n)
	bf := make([]float64, n*n)
	symmat.ToFull(n, a, af)
	lu := numeric.NewLU(n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := lu.Inverse(af, bf); err != nil {
			b.Fatal(err)
		}
	}
}
