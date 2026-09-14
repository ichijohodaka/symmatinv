// Package symmat は n 次対称行列の逆行列を、要素を文字のまま求める。
//
// 結果は命令列（分岐もループもない代入文の並び）として返る。あとは数値を
// 代入して上から順に実行するだけで逆行列が出る。同じ形の行列に対して
// 10^6〜10^8 組のパラメータを入れたい、という用途のためのもの。
//
// # やり方
//
// matrix inversion lemma（逆行列補題）を使って、n 次の逆行列を p 次と q 次
// （p + q = n）の2つの小さい逆行列の問題に帰着させる。
//
//	A = [ A11  A12 ]      W = A11⁻¹
//	    [ A12ᵀ A22 ]      S = A22 − A12ᵀ·W·A12   （Schur 補元）
//
//	A⁻¹ = [ W + M·T·Mᵀ   −M·T ]      M = W·A12
//	      [ (−M·T)ᵀ        T   ]      T = S⁻¹
//
// W と T をさらに同じやり方で分けていき、1 次（逆数をとるだけ）と 2 次
// （逆行列の公式）まで下りたら止める。
//
// 肝心なのは、各段の結果をその下の段の式で置き換えてしまわないこと。
// W は B11 の中にも B12 の中にも現れるが、式グラフの上では同じ1つの節点なので、
// 数値を代入するときも1度しか計算されない。全部を a_ij まで展開してしまうと、
// 同じ部分式を何度も計算することになる。
//
// # 受け渡しの約束
//
// 対称行列は上三角だけを行優先で並べる。n = 3 なら [a00 a01 a02 a11 a12 a22]。
// 逆行列も対称なので、出力も同じ並びの n(n+1)/2 個。
package symmat

import (
	"fmt"
	"sync"

	"github.com/ichijohodaka/symmatinv/internal/dag"
	"github.com/ichijohodaka/symmatinv/pkg/tape"
)

// Inv は逆行列の計算手順。Inverse か InverseWithPlan で作る。
//
// 中身は読み取り専用として扱うこと。同じ (n, 計画) に対しては同じ値が
// 使い回されるので、書き換えると他の呼び出し元にも影響する。
type Inv struct {
	n    int
	plan *Plan
	tp   *tape.Tape
}

// N は行列の次数。
func (v *Inv) N() int { return v.n }

// Plan はどう分割して計算したかを返す。
func (v *Inv) Plan() *Plan { return v.plan }

// Tape は逆行列を求める命令列を返す。入力は上三角の n(n+1)/2 個、
// 出力も同じ並びの n(n+1)/2 個。
func (v *Inv) Tape() *tape.Tape { return v.tp }

// NumInstrs は1回の代入にかかる演算回数。
func (v *Inv) NumInstrs() int { return v.tp.NumInstrs() }

// memo は同じものを2度計算しないためのプロセス内の覚え書き。
// ディスクにもネットワークにも触れない（保存はこのライブラリの仕事ではない）。
var memo sync.Map // key: string（次数と計画） -> *Inv

// Inverse は n 次対称行列の逆行列の計算手順を返す。分割は既定（半分ずつ）。
func Inverse(n int) (*Inv, error) { return InverseWithPlan(n, DefaultPlan(n)) }

// InverseWithPlan は分割のしかたを指定して計算手順を返す。
func InverseWithPlan(n int, p *Plan) (*Inv, error) {
	if n < 1 {
		return nil, fmt.Errorf("symmat: 次数は 1 以上でなければならない (n=%d)", n)
	}
	if err := p.Validate(n); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("inv/%d/%s", n, p)
	if v, ok := memo.Load(key); ok {
		return v.(*Inv), nil
	}

	b := dag.New(NumEntries(n))
	a := make([]dag.ID, NumEntries(n))
	for i := range a {
		a[i] = b.Var(i)
	}
	inv, _ := build(b, n, a, p)

	tp := b.Compile(inv)
	tp.VarNames = Names(n, "a")
	tp.OutNames = Names(n, "b")
	if err := tp.Validate(); err != nil {
		return nil, fmt.Errorf("symmat: 組み立てた命令列が不正: %w", err)
	}

	v := &Inv{n: n, plan: p, tp: tp}
	actual, _ := memo.LoadOrStore(key, v)
	return actual.(*Inv), nil
}

// Determinant は n 次対称行列の行列式を求める命令列を返す。
//
// matrix inversion lemma は det A = det(A11)·det(S) を途中で持っているので、
// 逆行列を組み立てるのと同じ手順からただで取り出せる。入力は上三角の
// n(n+1)/2 個、出力は1個。
func Determinant(n int) (*tape.Tape, error) { return DeterminantWithPlan(n, DefaultPlan(n)) }

// DeterminantWithPlan は分割のしかたを指定して行列式の命令列を返す。
func DeterminantWithPlan(n int, p *Plan) (*tape.Tape, error) {
	if n < 1 {
		return nil, fmt.Errorf("symmat: 次数は 1 以上でなければならない (n=%d)", n)
	}
	if err := p.Validate(n); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("det/%d/%s", n, p)
	if v, ok := memo.Load(key); ok {
		return v.(*tape.Tape), nil
	}

	b := dag.New(NumEntries(n))
	a := make([]dag.ID, NumEntries(n))
	for i := range a {
		a[i] = b.Var(i)
	}
	_, det := build(b, n, a, p)

	tp := b.Compile([]dag.ID{det})
	tp.VarNames = Names(n, "a")
	tp.OutNames = []string{"det"}
	if err := tp.Validate(); err != nil {
		return nil, fmt.Errorf("symmat: 組み立てた命令列が不正: %w", err)
	}
	actual, _ := memo.LoadOrStore(key, tp)
	return actual.(*tape.Tape), nil
}

// at は上三角の並び a から (i, j) 要素を取り出す。
func at(n int, a []dag.ID, i, j int) dag.ID { return a[Index(n, i, j)] }

// build は上三角で与えた n 次対称行列 a の逆行列（上三角）と行列式を組み立てる。
//
// 行列式は逆行列と同じ部分式から出るので、いつも一緒に作る。要らないほうは
// Compile のときに落ちるので、作っておいても損はしない。
func build(b *dag.Builder, n int, a []dag.ID, p *Plan) (inv []dag.ID, det dag.ID) {
	switch p.Kind {
	case KindDirect1:
		// A = [a]、A⁻¹ = [1/a]、det = a
		return []dag.ID{b.Recip(a[0])}, a[0]

	case KindDirect2:
		// a = [a00 a01 a11]
		det = b.Sub(b.Mul(a[0], a[2]), b.Mul(a[1], a[1]))
		r := b.Recip(det)
		return []dag.ID{
			b.Mul(a[2], r),        // b00 =  a11/det
			b.Neg(b.Mul(a[1], r)), // b01 = −a01/det
			b.Mul(a[0], r),        // b11 =  a00/det
		}, det
	}

	// ここからが matrix inversion lemma による分割。
	np, nq := p.P, p.Q

	// A11（上三角）、A12（np×nq、行優先）、A22（上三角）に切り分ける。
	a11 := make([]dag.ID, NumEntries(np))
	for i := 0; i < np; i++ {
		for j := i; j < np; j++ {
			a11[Index(np, i, j)] = at(n, a, i, j)
		}
	}
	a12 := make([]dag.ID, np*nq)
	for i := 0; i < np; i++ {
		for j := 0; j < nq; j++ {
			a12[i*nq+j] = at(n, a, i, np+j)
		}
	}
	a22 := make([]dag.ID, NumEntries(nq))
	for i := 0; i < nq; i++ {
		for j := i; j < nq; j++ {
			a22[Index(nq, i, j)] = at(n, a, np+i, np+j)
		}
	}

	// W = A11⁻¹（上三角）
	w, detW := build(b, np, a11, p.Sub[0])

	// M = W·A12（np×nq）
	m := make([]dag.ID, np*nq)
	for i := 0; i < np; i++ {
		for j := 0; j < nq; j++ {
			acc := b.Mul(at(np, w, i, 0), a12[0*nq+j])
			for k := 1; k < np; k++ {
				acc = b.Add(acc, b.Mul(at(np, w, i, k), a12[k*nq+j]))
			}
			m[i*nq+j] = acc
		}
	}

	// S = A22 − A12ᵀ·M（対称なので上三角だけ作る）
	s := make([]dag.ID, NumEntries(nq))
	for i := 0; i < nq; i++ {
		for j := i; j < nq; j++ {
			acc := b.Mul(a12[0*nq+i], m[0*nq+j])
			for k := 1; k < np; k++ {
				acc = b.Add(acc, b.Mul(a12[k*nq+i], m[k*nq+j]))
			}
			s[Index(nq, i, j)] = b.Sub(a22[Index(nq, i, j)], acc)
		}
	}

	// T = S⁻¹（上三角）。これがそのまま逆行列の右下ブロック B22 になる。
	t, detS := build(b, nq, s, p.Sub[1])

	// B12 = −M·T（np×nq）
	b12 := make([]dag.ID, np*nq)
	for i := 0; i < np; i++ {
		for j := 0; j < nq; j++ {
			acc := b.Mul(m[i*nq+0], at(nq, t, 0, j))
			for k := 1; k < nq; k++ {
				acc = b.Add(acc, b.Mul(m[i*nq+k], at(nq, t, k, j)))
			}
			b12[i*nq+j] = b.Neg(acc)
		}
	}

	// B11 = W + M·T·Mᵀ = W − B12·Mᵀ（対称なので上三角だけ作る）
	b11 := make([]dag.ID, NumEntries(np))
	for i := 0; i < np; i++ {
		for j := i; j < np; j++ {
			acc := b.Mul(b12[i*nq+0], m[j*nq+0])
			for k := 1; k < nq; k++ {
				acc = b.Add(acc, b.Mul(b12[i*nq+k], m[j*nq+k]))
			}
			b11[Index(np, i, j)] = b.Sub(at(np, w, i, j), acc)
		}
	}

	// 3つのブロックを n 次の上三角に並べ直す。
	inv = make([]dag.ID, NumEntries(n))
	for i := 0; i < np; i++ {
		for j := i; j < np; j++ {
			inv[Index(n, i, j)] = b11[Index(np, i, j)]
		}
	}
	for i := 0; i < np; i++ {
		for j := 0; j < nq; j++ {
			inv[Index(n, i, np+j)] = b12[i*nq+j]
		}
	}
	for i := 0; i < nq; i++ {
		for j := i; j < nq; j++ {
			inv[Index(n, np+i, np+j)] = t[Index(nq, i, j)]
		}
	}

	// det A = det(A11)·det(S)
	return inv, b.Mul(detW, detS)
}
