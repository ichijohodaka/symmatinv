package tape

import (
	"fmt"
	"math/big"
)

// Field は命令列を評価するのに必要な演算をひとまとめにしたもの。体の演算
// そのものなので、実数・複素数・有理数・有限体、どれでも実装できる。
//
// Recip には 0 が渡されうる。どう振る舞うかは実装に任せる（Float64Field は
// +Inf を返し、RatField は panic する）。
type Field[T any] interface {
	Add(a, b T) T
	Sub(a, b T) T
	Mul(a, b T) T
	Neg(a T) T
	Recip(a T) T
}

// Eval は体 f の上で命令列を実行する。
//
// vars は長さ NumVars 以上、scratch は長さ NumSlots 以上、out は長さ
// len(Outputs) 以上でなければならない。scratch を呼び出し側が持つのは、
// 同じ命令列を何度も評価するときに確保をやり直さないため。
//
// t は Validate を通っていることを前提とする。通っていない命令列を渡すと
// 範囲外アクセスで panic する。
func Eval[T any](f Field[T], t *Tape, vars, scratch, out []T) {
	if len(scratch) < t.NumSlots() {
		panic(fmt.Sprintf("tape: scratch が短い (%d < %d)", len(scratch), t.NumSlots()))
	}
	if len(vars) < t.NumVars {
		panic(fmt.Sprintf("tape: vars が短い (%d < %d)", len(vars), t.NumVars))
	}
	if len(out) < len(t.Outputs) {
		panic(fmt.Sprintf("tape: out が短い (%d < %d)", len(out), len(t.Outputs)))
	}
	copy(scratch[:t.NumVars], vars[:t.NumVars])
	base := t.NumVars
	for i := range t.Instrs {
		in := &t.Instrs[i]
		a := scratch[in.A]
		var r T
		switch in.Op {
		case OpAdd:
			r = f.Add(a, scratch[in.B])
		case OpSub:
			r = f.Sub(a, scratch[in.B])
		case OpMul:
			r = f.Mul(a, scratch[in.B])
		case OpNeg:
			r = f.Neg(a)
		case OpRecip:
			r = f.Recip(a)
		default:
			panic(fmt.Sprintf("tape: 未知の演算 %d", uint8(in.Op)))
		}
		scratch[base+i] = r
	}
	for i, s := range t.Outputs {
		out[i] = scratch[s]
	}
}

// EvalFloat64 は float64 に特化した評価。Eval[float64](Float64Field{}, ...) と
// 同じ結果を返すが、演算がインライン展開されるので速い。10^7 回まわす用途では
// こちらを使う。
//
// t は Validate を通っていることを前提とする。
func EvalFloat64(t *Tape, vars, scratch, out []float64) {
	if len(scratch) < t.NumSlots() {
		panic(fmt.Sprintf("tape: scratch が短い (%d < %d)", len(scratch), t.NumSlots()))
	}
	if len(vars) < t.NumVars {
		panic(fmt.Sprintf("tape: vars が短い (%d < %d)", len(vars), t.NumVars))
	}
	if len(out) < len(t.Outputs) {
		panic(fmt.Sprintf("tape: out が短い (%d < %d)", len(out), len(t.Outputs)))
	}
	copy(scratch[:t.NumVars], vars[:t.NumVars])
	s := scratch
	base := t.NumVars
	for i := range t.Instrs {
		in := &t.Instrs[i]
		switch in.Op {
		case OpAdd:
			s[base+i] = s[in.A] + s[in.B]
		case OpSub:
			s[base+i] = s[in.A] - s[in.B]
		case OpMul:
			s[base+i] = s[in.A] * s[in.B]
		case OpNeg:
			s[base+i] = -s[in.A]
		case OpRecip:
			s[base+i] = 1 / s[in.A]
		default:
			panic(fmt.Sprintf("tape: 未知の演算 %d", uint8(in.Op)))
		}
	}
	for i, slot := range t.Outputs {
		out[i] = s[slot]
	}
}

// NewScratch は t の評価に使う作業用配列を確保する。
func NewScratch[T any](t *Tape) []T { return make([]T, t.NumSlots()) }

// Float64Field は float64 の体。Recip(0) は +Inf を返す（IEEE 754 のまま）。
type Float64Field struct{}

func (Float64Field) Add(a, b float64) float64 { return a + b }
func (Float64Field) Sub(a, b float64) float64 { return a - b }
func (Float64Field) Mul(a, b float64) float64 { return a * b }
func (Float64Field) Neg(a float64) float64    { return -a }
func (Float64Field) Recip(a float64) float64  { return 1 / a }

// Complex128Field は complex128 の体。このパッケージ自身は複素数を前提にしないが、
// 素直な実装の例として置いてある。
type Complex128Field struct{}

func (Complex128Field) Add(a, b complex128) complex128 { return a + b }
func (Complex128Field) Sub(a, b complex128) complex128 { return a - b }
func (Complex128Field) Mul(a, b complex128) complex128 { return a * b }
func (Complex128Field) Neg(a complex128) complex128    { return -a }
func (Complex128Field) Recip(a complex128) complex128  { return 1 / a }

// RatField は有理数の体。丸め誤差がないので、命令列が正しいことを厳密に
// 確かめるのに使う（浮動小数の「たまたま一致」を排除できる）。
//
// 値は *big.Rat で、演算のたびに新しい値を作る。遅いので検算専用。
// Recip(0) は panic する。
type RatField struct{}

func (RatField) Add(a, b *big.Rat) *big.Rat { return new(big.Rat).Add(a, b) }
func (RatField) Sub(a, b *big.Rat) *big.Rat { return new(big.Rat).Sub(a, b) }
func (RatField) Mul(a, b *big.Rat) *big.Rat { return new(big.Rat).Mul(a, b) }
func (RatField) Neg(a *big.Rat) *big.Rat    { return new(big.Rat).Neg(a) }

func (RatField) Recip(a *big.Rat) *big.Rat {
	if a.Sign() == 0 {
		panic("tape: RatField.Recip(0): 有理数体では 0 の逆数がとれない")
	}
	return new(big.Rat).Inv(a)
}

// 各体が Field を満たしていることをコンパイル時に確かめる。
var (
	_ Field[float64]    = Float64Field{}
	_ Field[complex128] = Complex128Field{}
	_ Field[*big.Rat]   = RatField{}
)
