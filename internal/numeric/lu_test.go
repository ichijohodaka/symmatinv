package numeric

import (
	"errors"
	"math"
	"math/rand/v2"
	"testing"
)

// このパッケージは記号計算が正しいことの「基準」に使われる。基準そのものが
// 間違っていたら意味がないので、ここでは記号計算をいっさい使わずに確かめる。

func newRNG() *rand.Rand { return rand.New(rand.NewPCG(20260914, 3)) }

// randomMatrix は対角優位な n×n 行列を作る。逆行列が確実に存在し、条件数も
// 暴れないので、式が合っているかどうかだけを見られる。
func randomMatrix(n int, rng *rand.Rand) []float64 {
	a := make([]float64, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				a[i*n+j] = float64(n) + rng.Float64()
			} else {
				a[i*n+j] = 2*rng.Float64() - 1
			}
		}
	}
	return a
}

// TestInverseIsInverse は A·A⁻¹ = I と A⁻¹·A = I の両方を確かめる。
// 逆行列の定義そのもの。左右どちらも見るのは、行の入れ替えの扱いを
// 取り違えていると片方しか合わないことがあるため。
func TestInverseIsInverse(t *testing.T) {
	rng := newRNG()
	for n := 1; n <= 8; n++ {
		for trial := 0; trial < 10; trial++ {
			a := randomMatrix(n, rng)
			b := make([]float64, n*n)
			if err := Inverse(n, a, b); err != nil {
				t.Fatalf("n=%d: %v", n, err)
			}
			for _, side := range []struct {
				name string
				x, y []float64
			}{
				{"A·A⁻¹", a, b},
				{"A⁻¹·A", b, a},
			} {
				for i := 0; i < n; i++ {
					for j := 0; j < n; j++ {
						s := 0.0
						for k := 0; k < n; k++ {
							s += side.x[i*n+k] * side.y[k*n+j]
						}
						want := 0.0
						if i == j {
							want = 1
						}
						if math.Abs(s-want) > 1e-12 {
							t.Fatalf("n=%d: (%s)[%d][%d] = %g, want %g", n, side.name, i, j, s, want)
						}
					}
				}
			}
		}
	}
}

// TestDetKnown は行列式を手で計算できる行列で確かめる。
func TestDetKnown(t *testing.T) {
	cases := []struct {
		name string
		n    int
		a    []float64
		want float64
	}{
		{"1次", 1, []float64{3}, 3},
		{"2次", 2, []float64{1, 2, 3, 4}, -2},
		{"3次 上三角", 3, []float64{2, 9, 9, 0, 3, 9, 0, 0, 4}, 24},
		{"3次 一般", 3, []float64{6, 1, 1, 4, -2, 5, 2, 8, 7}, -306},
		{"単位行列", 4, []float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Det(c.n, c.a)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-c.want) > 1e-9*(1+math.Abs(c.want)) {
				t.Errorf("Det = %g, want %g", got, c.want)
			}
		})
	}
}

// TestDetSignFromRowSwap は、行を1回入れ替えると行列式の符号が反転することを
// 確かめる。部分ピボット選択で入れ替えた回数を数え違えていると、ここで落ちる。
func TestDetSignFromRowSwap(t *testing.T) {
	rng := newRNG()
	const n = 5
	a := randomMatrix(n, rng)
	d1, err := Det(n, a)
	if err != nil {
		t.Fatal(err)
	}

	swapped := append([]float64(nil), a...)
	for j := 0; j < n; j++ {
		swapped[0*n+j], swapped[1*n+j] = swapped[1*n+j], swapped[0*n+j]
	}
	d2, err := Det(n, swapped)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(d1+d2) > 1e-9*(1+math.Abs(d1)) {
		t.Errorf("行を入れ替えても符号が反転していない: %g -> %g", d1, d2)
	}
}

// TestInverseNeedsPivoting は、部分ピボット選択がないと解けない行列を渡す。
// 左上が 0 なので、ピボット選択を省いた実装は 0 で割って壊れる。
func TestInverseNeedsPivoting(t *testing.T) {
	const n = 2
	a := []float64{0, 1, 1, 0} // 行を入れ替えれば単位行列
	b := make([]float64, n*n)
	if err := Inverse(n, a, b); err != nil {
		t.Fatalf("Inverse: %v", err)
	}
	// この行列は自分自身が逆行列。
	for i, want := range a {
		if math.Abs(b[i]-want) > 1e-15 {
			t.Errorf("b[%d] = %g, want %g", i, b[i], want)
		}
	}
}

func TestSingular(t *testing.T) {
	cases := []struct {
		name string
		n    int
		a    []float64
	}{
		{"零行列", 2, []float64{0, 0, 0, 0}},
		{"行が重複", 2, []float64{1, 2, 1, 2}},
		{"行が従属", 3, []float64{1, 2, 3, 2, 4, 6, 1, 0, 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := make([]float64, c.n*c.n)
			err := Inverse(c.n, c.a, b)
			if !errors.Is(err, ErrSingular) {
				t.Fatalf("ErrSingular になるはず: %v", err)
			}
		})
	}
}

// TestLUReuse は作業場を使い回しても結果が変わらないことを確かめる。
// 前回の分解が残っていると、2回目以降が狂う。
func TestLUReuse(t *testing.T) {
	rng := newRNG()
	const n = 4
	lu := NewLU(n)
	mats := make([][]float64, 5)
	first := make([][]float64, 5)
	for i := range mats {
		mats[i] = randomMatrix(n, rng)
		first[i] = make([]float64, n*n)
		if err := lu.Inverse(mats[i], first[i]); err != nil {
			t.Fatal(err)
		}
	}
	// 同じ作業場でもう一度、順序を変えて解く。
	got := make([]float64, n*n)
	for i := len(mats) - 1; i >= 0; i-- {
		if err := lu.Inverse(mats[i], got); err != nil {
			t.Fatal(err)
		}
		for k := range got {
			if got[k] != first[i][k] {
				t.Fatalf("%d 番目の行列: 使い回すと結果が変わった [%d] %g -> %g",
					i, k, first[i][k], got[k])
			}
		}
	}
}

// TestInverseDoesNotModifyInput は入力の行列を壊さないことを確かめる。
func TestInverseDoesNotModifyInput(t *testing.T) {
	const n = 4
	a := randomMatrix(n, newRNG())
	keep := append([]float64(nil), a...)
	b := make([]float64, n*n)
	if err := Inverse(n, a, b); err != nil {
		t.Fatal(err)
	}
	for i := range a {
		if a[i] != keep[i] {
			t.Fatalf("入力が書き換えられた [%d] %g -> %g", i, keep[i], a[i])
		}
	}
}

func TestShortSlices(t *testing.T) {
	lu := NewLU(3)
	if err := lu.Factor(make([]float64, 8)); err == nil {
		t.Error("行列が短ければエラーになるはず")
	}
	if err := lu.Inverse(make([]float64, 9), make([]float64, 8)); err == nil {
		t.Error("out が短ければエラーになるはず")
	}
}

func TestNewLURejectsSmallN(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("n < 1 は panic するはず")
		}
	}()
	NewLU(0)
}
