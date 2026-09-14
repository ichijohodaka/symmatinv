// Package tape は、分岐もループもない代入文の並び（命令列）を表す。
//
// 命令列は上から順に実行するだけで答えが出る。1命令がちょうど四則演算1回に
// 対応するので、命令数がそのまま「1回の代入にかかる演算回数」になる。
//
// 英語では straight-line program、あるいは Wengert list や tape と呼ばれる。
// パッケージ名と型名はその英語に合わせてあるが、日本語のコメントでは
// 一貫して「命令列」と呼ぶ。
//
// # どの体の上でも動く
//
// 命令は加算・減算・乗算・符号反転・逆数の5つだけで、これは体（四則演算が
// できる数の集まり。実数・複素数・有理数など）が備えている演算そのものである。
// したがって同じ命令列を float64 でも complex128 でも有理数でも評価できる。
// 命令列自身は数を1つも持たない（定数表がない）ので、どの体の上で動かすかは
// 評価するときに決まる。
//
// 対称行列の逆行列を matrix inversion lemma（逆行列補題）で表すと、途中に
// 数値定数が現れない（すべて入力要素どうしの演算になる）ため、定数表を
// 持たない設計で足りている。
//
// # 値の置き場（スロット）
//
// 評価は float64 などの平坦な配列の上で行う。計算の途中結果を入れておく
// 配列の番地をスロットと呼ぶ。配列の使い方は
//
//	[0, NumVars)                      入力変数
//	[NumVars, NumVars+len(Instrs))    命令の結果
//
// で、i 番の命令は必ずスロット NumVars+i に書く。書き込み先が命令の位置から
// 一意に決まるので、命令は演算の種類と読み出し元だけを持てばよい。
//
// 命令の読み出し元は必ず自分より前のスロットを指す（Validate が確かめる）。
// この制約があるので、上から順に実行するだけで未定義の値を読むことがない。
package tape

import (
	"fmt"
	"strings"
)

// Op は命令の種類。体が備えている演算に1対1で対応する。
type Op uint8

const (
	OpAdd   Op = iota // a + b
	OpSub             // a - b
	OpMul             // a * b
	OpNeg             // -a
	OpRecip           // 1 / a
)

var opNames = [...]string{"add", "sub", "mul", "neg", "recip"}

// String は JSON 表現にも使う小文字の名前を返す。
func (o Op) String() string {
	if int(o) >= len(opNames) {
		return fmt.Sprintf("Op(%d)", uint8(o))
	}
	return opNames[o]
}

// Arity は演算がとる引数の個数（2 か 1）を返す。
func (o Op) Arity() int {
	switch o {
	case OpAdd, OpSub, OpMul:
		return 2
	case OpNeg, OpRecip:
		return 1
	}
	return 0
}

// Valid は o が定義済みの演算かどうかを返す。
func (o Op) Valid() bool { return int(o) < len(opNames) }

// ParseOp は String の逆。
func ParseOp(s string) (Op, error) {
	for i, name := range opNames {
		if name == s {
			return Op(i), nil
		}
	}
	return 0, fmt.Errorf("tape: 未知の演算 %q", s)
}

// Instr は1命令。A と B は読み出し元のスロット番号で、書き込み先は命令の位置から
// 決まる（パッケージのドキュメント参照）。単項演算では B は使わず、決まった形として
// -1 を入れる。
type Instr struct {
	Op   Op
	A, B int32
}

// Bin は二項演算の命令を作る。
func Bin(op Op, a, b int32) Instr { return Instr{Op: op, A: a, B: b} }

// Un は単項演算の命令を作る。
func Un(op Op, a int32) Instr { return Instr{Op: op, A: a, B: -1} }

// Tape は一本道の計算手順。
//
// VarNames と OutNames は説明のためだけのもので、評価には使わない。空でもよいが、
// 空でなければ長さが NumVars / len(Outputs) と一致していなければならない。
type Tape struct {
	NumVars  int     // 入力変数の個数
	Instrs   []Instr // 上から順に実行する命令
	Outputs  []int32 // 出力として読み出すスロット番号
	VarNames []string
	OutNames []string
}

// NumSlots は評価に必要な作業用配列の長さ。
func (t *Tape) NumSlots() int { return t.NumVars + len(t.Instrs) }

// ResultSlot は i 番の命令が書き込むスロット番号。
func (t *Tape) ResultSlot(i int) int32 { return int32(t.NumVars + i) }

// NumInstrs は1回の評価で行う演算の回数。方式どうしを比べるときの指標になる。
func (t *Tape) NumInstrs() int { return len(t.Instrs) }

// OpCounts は演算の種類ごとの回数を返す。逆数は乗算よりずっと重いので、
// 総数だけでなく内訳を見たいことが多い。
func (t *Tape) OpCounts() map[Op]int {
	m := make(map[Op]int, len(opNames))
	for _, in := range t.Instrs {
		m[in.Op]++
	}
	return m
}

// Validate は命令列が自己整合しているかを調べる。外から読み込んだ命令列
// （JSON など）は、評価する前に必ずこれを通すこと。
//
// 確かめるのは次の4点。
//
//	演算が定義済みであること
//	読み出し元が範囲内で、かつ自分より前のスロットであること
//	単項演算の B が -1（決まった形）であること
//	出力のスロット番号が範囲内であること
func (t *Tape) Validate() error {
	if t.NumVars < 0 {
		return fmt.Errorf("tape: NumVars が負 (%d)", t.NumVars)
	}
	if n := len(t.VarNames); n != 0 && n != t.NumVars {
		return fmt.Errorf("tape: VarNames の長さ %d が NumVars %d と合わない", n, t.NumVars)
	}
	for i, in := range t.Instrs {
		if !in.Op.Valid() {
			return fmt.Errorf("tape: 命令 %d: 未知の演算 %d", i, uint8(in.Op))
		}
		limit := t.ResultSlot(i) // 自分より前しか読めない
		if in.A < 0 || in.A >= limit {
			return fmt.Errorf("tape: 命令 %d (%s): A=%d がスロット範囲 [0,%d) の外", i, in.Op, in.A, limit)
		}
		if in.Op.Arity() == 2 {
			if in.B < 0 || in.B >= limit {
				return fmt.Errorf("tape: 命令 %d (%s): B=%d がスロット範囲 [0,%d) の外", i, in.Op, in.B, limit)
			}
		} else if in.B != -1 {
			return fmt.Errorf("tape: 命令 %d (%s): 単項演算の B は -1 でなければならない (B=%d)", i, in.Op, in.B)
		}
	}
	n := int32(t.NumSlots())
	for i, s := range t.Outputs {
		if s < 0 || s >= n {
			return fmt.Errorf("tape: 出力 %d: スロット %d が範囲 [0,%d) の外", i, s, n)
		}
	}
	if m := len(t.OutNames); m != 0 && m != len(t.Outputs) {
		return fmt.Errorf("tape: OutNames の長さ %d が出力数 %d と合わない", m, len(t.Outputs))
	}
	return nil
}

// slotName はスロット番号を人が読める名前にする。説明・コード生成用。
func (t *Tape) slotName(s int32) string {
	if int(s) < t.NumVars {
		if len(t.VarNames) != 0 {
			return t.VarNames[s]
		}
		return fmt.Sprintf("v%d", s)
	}
	return fmt.Sprintf("t%d", int(s)-t.NumVars)
}

// String は命令列を人が読める形に並べる。デバッグと show コマンド用で、
// 保存には使わない（保存は WriteJSON）。
func (t *Tape) String() string {
	var b strings.Builder
	for i, in := range t.Instrs {
		dst := t.slotName(t.ResultSlot(i))
		a := t.slotName(in.A)
		switch in.Op {
		case OpAdd:
			fmt.Fprintf(&b, "%s = %s + %s\n", dst, a, t.slotName(in.B))
		case OpSub:
			fmt.Fprintf(&b, "%s = %s - %s\n", dst, a, t.slotName(in.B))
		case OpMul:
			fmt.Fprintf(&b, "%s = %s * %s\n", dst, a, t.slotName(in.B))
		case OpNeg:
			fmt.Fprintf(&b, "%s = -%s\n", dst, a)
		case OpRecip:
			fmt.Fprintf(&b, "%s = 1 / %s\n", dst, a)
		default:
			fmt.Fprintf(&b, "%s = %s(%s)\n", dst, in.Op, a)
		}
	}
	for i, s := range t.Outputs {
		name := fmt.Sprintf("out%d", i)
		if len(t.OutNames) != 0 {
			name = t.OutNames[i]
		}
		fmt.Fprintf(&b, "%s := %s\n", name, t.slotName(s))
	}
	return b.String()
}
