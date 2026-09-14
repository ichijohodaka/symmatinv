// Package inline は「方法3」、つまり各段の結果をすべて代入してしまったら
// どうなるかを調べる。
//
// pkg/symmat が作る命令列では、W = A11⁻¹ のような中間結果が1つの値として
// 置かれ、それを参照する箇所はみな同じ値を読む。方法3 はこれをやめて、参照の
// たびに中身をその場に書き下す。結果として、最初の行列の要素 a_ij だけからなる
// 1本の巨大な式になる。
//
// ここでするのは「代入」だけで、積の分配（展開）はしない。文字どおり
// 「2段階の計算をすべて代入し、a_ij で表す」に対応する。
//
// # なぜ測る価値があるか
//
// 方法2 と方法3 の差は、そのまま「同じ部分式を何度も計算する無駄」の大きさに
// なる。中間表現を保つことが本当に得策かどうかは、これを測ってはじめて言える。
//
// # 大きさは先に数えられる
//
// 展開後の命令数は、実際に展開しなくても数えられる（Size）。各命令が何回
// 参照されるかを出口から逆にたどって足し合わせるだけでよい。展開すると
// 天文学的な大きさになることがあるので、作る前に必ず数えること。
package inline

import (
	"fmt"
	"math/big"

	"github.com/ichijohodaka/symmatinv/pkg/tape"
)

// Size は命令列を共有なしの木に展開したときの命令数を返す。
//
// 実際に展開しないので、結果が巨大でも安全に求まる。戻り値が big.Int なのは
// 段数が増えると int64 に収まらなくなるため。
func Size(t *tape.Tape) *big.Int {
	n := t.NumSlots()
	mult := make([]*big.Int, n)
	for i := range mult {
		mult[i] = new(big.Int)
	}
	one := big.NewInt(1)
	for _, s := range t.Outputs {
		mult[s].Add(mult[s], one)
	}
	// 命令は必ず自分より前のスロットしか読まないので、後ろから1回なめれば
	// 参照回数が確定する。
	total := new(big.Int)
	for i := len(t.Instrs) - 1; i >= 0; i-- {
		m := mult[t.ResultSlot(i)]
		if m.Sign() == 0 {
			continue
		}
		total.Add(total, m)
		in := t.Instrs[i]
		mult[in.A].Add(mult[in.A], m)
		if in.Op.Arity() == 2 {
			mult[in.B].Add(mult[in.B], m)
		}
	}
	return total
}

// ErrTooBig は展開後の大きさが limit を超えたことを表す。
type ErrTooBig struct {
	Size  *big.Int
	Limit int
}

func (e *ErrTooBig) Error() string {
	return fmt.Sprintf("inline: 展開すると %s 命令になり、上限 %d を超える", e.Size, e.Limit)
}

// Expand は共有をやめた等価な命令列を作る。どの命令の結果もちょうど一度しか
// 読まれない、つまり木になっている命令列が返る。
//
// 展開後の命令数が limit を超える場合は *ErrTooBig を返し、何も作らない。
func Expand(t *tape.Tape, limit int) (*tape.Tape, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	size := Size(t)
	if size.IsInt64() && size.Int64() <= int64(limit) {
		// 作れる大きさ。下で展開する。
	} else {
		return nil, &ErrTooBig{Size: size, Limit: limit}
	}

	e := &expander{src: t, instrs: make([]tape.Instr, 0, size.Int64())}
	outs := make([]int32, len(t.Outputs))
	for i, s := range t.Outputs {
		outs[i] = e.emit(s)
	}
	out := &tape.Tape{
		NumVars: t.NumVars,
		Instrs:  e.instrs,
		Outputs: outs,
	}
	if len(t.VarNames) != 0 {
		out.VarNames = append([]string(nil), t.VarNames...)
	}
	if len(t.OutNames) != 0 {
		out.OutNames = append([]string(nil), t.OutNames...)
	}
	return out, nil
}

type expander struct {
	src    *tape.Tape
	instrs []tape.Instr
}

// emit は元のスロット s を計算する命令をまるごと書き下し、新しい命令列での
// スロット番号を返す。同じ s を2回渡せば、2組の命令が別々にできる。
// これが「共有をやめる」ということ。
func (e *expander) emit(s int32) int32 {
	if int(s) < e.src.NumVars {
		return s // 変数は写すまでもない
	}
	in := e.src.Instrs[int(s)-e.src.NumVars]
	a := e.emit(in.A)
	b := int32(-1)
	if in.Op.Arity() == 2 {
		b = e.emit(in.B)
	}
	e.instrs = append(e.instrs, tape.Instr{Op: in.Op, A: a, B: b})
	return int32(e.src.NumVars + len(e.instrs) - 1)
}
