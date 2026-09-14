// Package numeric は数値線形代数の基準実装。
//
// 役割はふたつある。
//
// ひとつは「方法1」、つまり先に数値を代入してから毎回数値的に逆行列を求める
// やり方そのもの。記号計算で作った命令列が本当に速いのかは、これと比べて
// はじめて言える。
//
// もうひとつは正しさの基準。記号計算の結果が合っているかどうかを、独立に
// 書かれたこちらの結果と突き合わせて確かめる。
//
// 行列は n×n の行優先（row-major）の平坦な配列で受け渡す。対称行列を上三角
// だけで持つ流儀はこのパッケージには持ち込まない。ここは一般の正方行列だけを
// 扱う。
package numeric

import (
	"errors"
	"fmt"
	"math"
)

// ErrSingular はピボットが 0 になり、逆行列が求まらないことを表す。
var ErrSingular = errors.New("numeric: 行列が特異で逆行列が求まらない")

// LU は部分ピボット付き LU 分解の作業場。同じ大きさの行列を何度も分解するとき、
// 確保をやり直さないために使い回す。
//
// 並行に使ってはいけない。ゴルーチンごとに1つ作ること。
type LU struct {
	n    int
	m    []float64 // 分解後の L と U を重ねて置く
	piv  []int     // 行の入れ替えの記録
	sign float64   // 入れ替えの回数の偶奇（行列式の符号）
	y    []float64 // 前進代入の途中結果
}

// NewLU は n 次の行列用の作業場を作る。
func NewLU(n int) *LU {
	if n < 1 {
		panic(fmt.Sprintf("numeric: n が小さすぎる (%d)", n))
	}
	return &LU{
		n:   n,
		m:   make([]float64, n*n),
		piv: make([]int, n),
		y:   make([]float64, n),
	}
}

// N は行列の次数。
func (l *LU) N() int { return l.n }

// Factor は a を分解する。a は n*n の行優先。a 自体は変更しない。
func (l *LU) Factor(a []float64) error {
	n := l.n
	if len(a) < n*n {
		return fmt.Errorf("numeric: 行列が短い (%d < %d)", len(a), n*n)
	}
	copy(l.m, a[:n*n])
	m := l.m
	for i := range l.piv {
		l.piv[i] = i
	}
	l.sign = 1

	for k := 0; k < n; k++ {
		// 部分ピボット選択。絶対値が最大の行を対角に持ってくる。
		best, p := math.Abs(m[k*n+k]), k
		for i := k + 1; i < n; i++ {
			if v := math.Abs(m[i*n+k]); v > best {
				best, p = v, i
			}
		}
		if best == 0 {
			return ErrSingular
		}
		if p != k {
			for j := 0; j < n; j++ {
				m[k*n+j], m[p*n+j] = m[p*n+j], m[k*n+j]
			}
			l.piv[k], l.piv[p] = l.piv[p], l.piv[k]
			l.sign = -l.sign
		}
		d := m[k*n+k]
		for i := k + 1; i < n; i++ {
			f := m[i*n+k] / d
			m[i*n+k] = f // L の要素としてその場に残す
			if f == 0 {
				continue
			}
			for j := k + 1; j < n; j++ {
				m[i*n+j] -= f * m[k*n+j]
			}
		}
	}
	return nil
}

// solve は Factor 済みの分解を使って A·x = b を解き、x を out に書く。
// b は入れ替え前の並びで渡す。
func (l *LU) solve(b, out []float64) {
	n, m, y := l.n, l.m, l.y
	for i := 0; i < n; i++ {
		s := b[l.piv[i]]
		for j := 0; j < i; j++ {
			s -= m[i*n+j] * y[j]
		}
		y[i] = s
	}
	for i := n - 1; i >= 0; i-- {
		s := y[i]
		for j := i + 1; j < n; j++ {
			s -= m[i*n+j] * out[j]
		}
		out[i] = s / m[i*n+i]
	}
}

// Det は Factor 済みの行列式を返す。
func (l *LU) Det() float64 {
	d := l.sign
	for i := 0; i < l.n; i++ {
		d *= l.m[i*l.n+i]
	}
	return d
}

// Inverse は a の逆行列を求めて out に書く。どちらも n*n の行優先。
//
// 単位行列の各列を右辺として n 回解く、素直なやり方。
func (l *LU) Inverse(a, out []float64) error {
	n := l.n
	if len(out) < n*n {
		return fmt.Errorf("numeric: out が短い (%d < %d)", len(out), n*n)
	}
	if err := l.Factor(a); err != nil {
		return err
	}
	e := make([]float64, n)
	x := make([]float64, n)
	for j := 0; j < n; j++ {
		for i := range e {
			e[i] = 0
		}
		e[j] = 1
		l.solve(e, x)
		for i := 0; i < n; i++ {
			out[i*n+j] = x[i]
		}
	}
	return nil
}

// Inverse は使い捨ての逆行列計算。作業場を使い回したいときは NewLU を使う。
func Inverse(n int, a, out []float64) error { return NewLU(n).Inverse(a, out) }

// Det は使い捨ての行列式計算。
func Det(n int, a []float64) (float64, error) {
	l := NewLU(n)
	if err := l.Factor(a); err != nil {
		return 0, err
	}
	return l.Det(), nil
}
