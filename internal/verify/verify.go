// Package verify は、組み立てた命令列が本当に逆行列を計算しているかを確かめる。
//
// 基準は「逆行列である」という定義そのもの、つまり A·A⁻¹ = I。数値 LU の結果と
// 比べるのではなく定義に照らすので、LU 側にも同じ誤りがあった場合に見逃す、
// ということが起きない。
//
// 確かめ方は2通りある。
//
//   - Residual は乱数（float64）を代入して A·A⁻¹ − I の最大絶対値を測る。速い。
//   - Exact は有理数を代入して A·A⁻¹ = I を厳密に確かめる。丸め誤差がないので
//     「浮動小数でたまたま一致した」という可能性を排除できる。遅い。
package verify

import (
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"

	"github.com/ichijohodaka/symmatinv/pkg/symmat"
	"github.com/ichijohodaka/symmatinv/pkg/tape"
)

// RandomSPD は対角優位な n 次対称行列を上三角で作る。
//
// 対角優位にするのは、逆行列が確実に存在し、かつ条件数が暴れないようにするため。
// 検算で見たいのは式が合っているかであって、悪条件での振る舞いではない。
func RandomSPD(n int, rng *rand.Rand) []float64 {
	a := make([]float64, symmat.NumEntries(n))
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			if i == j {
				a[symmat.Index(n, i, j)] = float64(n) + rng.Float64()
			} else {
				a[symmat.Index(n, i, j)] = 2*rng.Float64() - 1
			}
		}
	}
	return a
}

// RandomSPDRat は RandomSPD の有理数版。分母を揃えた小さい整数比にするので、
// big.Rat での計算が膨らみにくい。
func RandomSPDRat(n int, rng *rand.Rand) []*big.Rat {
	const den = 16
	a := make([]*big.Rat, symmat.NumEntries(n))
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			var num int64
			if i == j {
				num = int64(n)*den + rng.Int64N(den)
			} else {
				num = rng.Int64N(2*den) - den
			}
			a[symmat.Index(n, i, j)] = big.NewRat(num, den)
		}
	}
	return a
}

// Residual は乱数を trials 組だけ代入し、A·A⁻¹ − I の要素の最大絶対値を返す。
//
// t は symmat.Inverse が返す形の命令列（入力も出力も上三角 n(n+1)/2 個）で
// なければならない。
func Residual(n int, t *tape.Tape, rng *rand.Rand, trials int) (float64, error) {
	ne := symmat.NumEntries(n)
	if t.NumVars != ne || len(t.Outputs) != ne {
		return 0, fmt.Errorf("verify: %d 次なら入力も出力も %d 個のはず (入力 %d, 出力 %d)",
			n, ne, t.NumVars, len(t.Outputs))
	}
	scratch := tape.NewScratch[float64](t)
	out := make([]float64, ne)
	af := make([]float64, n*n)
	bf := make([]float64, n*n)

	worst := 0.0
	for trial := 0; trial < trials; trial++ {
		a := RandomSPD(n, rng)
		tape.EvalFloat64(t, a, scratch, out)
		symmat.ToFull(n, a, af)
		symmat.ToFull(n, out, bf)

		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				s := 0.0
				for k := 0; k < n; k++ {
					s += af[i*n+k] * bf[k*n+j]
				}
				want := 0.0
				if i == j {
					want = 1
				}
				if d := math.Abs(s - want); d > worst {
					worst = d
				}
			}
		}
	}
	return worst, nil
}

// Exact は有理数を trials 組だけ代入し、A·A⁻¹ = I が厳密に成り立つことを
// 確かめる。成り立たなければどの要素が食い違ったかを含むエラーを返す。
func Exact(n int, t *tape.Tape, rng *rand.Rand, trials int) error {
	ne := symmat.NumEntries(n)
	if t.NumVars != ne || len(t.Outputs) != ne {
		return fmt.Errorf("verify: %d 次なら入力も出力も %d 個のはず (入力 %d, 出力 %d)",
			n, ne, t.NumVars, len(t.Outputs))
	}
	scratch := tape.NewScratch[*big.Rat](t)
	out := make([]*big.Rat, ne)
	af := make([]*big.Rat, n*n)
	bf := make([]*big.Rat, n*n)
	one := big.NewRat(1, 1)
	zero := big.NewRat(0, 1)

	for trial := 0; trial < trials; trial++ {
		a := RandomSPDRat(n, rng)
		tape.Eval(tape.RatField{}, t, a, scratch, out)
		symmat.ToFull(n, a, af)
		symmat.ToFull(n, out, bf)

		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				s := new(big.Rat)
				term := new(big.Rat)
				for k := 0; k < n; k++ {
					s.Add(s, term.Mul(af[i*n+k], bf[k*n+j]))
				}
				want := zero
				if i == j {
					want = one
				}
				if s.Cmp(want) != 0 {
					return fmt.Errorf("verify: %d 組目、(A·A⁻¹)[%d][%d] = %s（%s のはず）",
						trial, i, j, s, want)
				}
			}
		}
	}
	return nil
}

// DetExact は行列式の命令列が正しいことを、余因子展開と突き合わせて厳密に
// 確かめる。n が小さいうちしか使えない（展開の手間が n! で増えるため）。
func DetExact(n int, t *tape.Tape, rng *rand.Rand, trials int) error {
	ne := symmat.NumEntries(n)
	if t.NumVars != ne || len(t.Outputs) != 1 {
		return fmt.Errorf("verify: %d 次の行列式なら入力 %d 個・出力 1 個のはず (入力 %d, 出力 %d)",
			n, ne, t.NumVars, len(t.Outputs))
	}
	scratch := tape.NewScratch[*big.Rat](t)
	out := make([]*big.Rat, 1)
	af := make([]*big.Rat, n*n)

	for trial := 0; trial < trials; trial++ {
		a := RandomSPDRat(n, rng)
		tape.Eval(tape.RatField{}, t, a, scratch, out)
		symmat.ToFull(n, a, af)
		want := detLaplace(n, af)
		if out[0].Cmp(want) != 0 {
			return fmt.Errorf("verify: %d 組目、det = %s（%s のはず）", trial, out[0], want)
		}
	}
	return nil
}

// detLaplace は余因子展開で行列式を厳密に求める。独立した基準にするため、
// 記号計算とは別の素朴な方法で書いてある。
func detLaplace(n int, a []*big.Rat) *big.Rat {
	if n == 1 {
		return new(big.Rat).Set(a[0])
	}
	det := new(big.Rat)
	minor := make([]*big.Rat, (n-1)*(n-1))
	for c := 0; c < n; c++ {
		k := 0
		for i := 1; i < n; i++ {
			for j := 0; j < n; j++ {
				if j == c {
					continue
				}
				minor[k] = a[i*n+j]
				k++
			}
		}
		term := new(big.Rat).Mul(a[c], detLaplace(n-1, minor))
		if c%2 == 0 {
			det.Add(det, term)
		} else {
			det.Sub(det, term)
		}
	}
	return det
}
