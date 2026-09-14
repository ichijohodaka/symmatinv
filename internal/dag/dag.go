// Package dag は式をグラフとして組み立て、命令列に変換する。
//
// ここで使うグラフは有向非巡回グラフ（directed acyclic graph、略して DAG）で、
// 矢印に向きがあり、たどっても元の節点に戻ってこないグラフのこと。
// パッケージ名はその英語の略称に合わせてあるが、日本語のコメントでは
// 「式グラフ」と呼ぶ。
//
// # なぜ木ではなくグラフか
//
// 対称行列の逆行列を matrix inversion lemma（逆行列補題）で表すと、同じ部分式が
// 何度も現れる。たとえば A11 の逆行列は、B11 の中にも B12 の中にも出てくる。
// これを木で持つと同じものを何度も評価することになり、代入を 10^7 回まわす
// 用途では致命的になる。
//
// そこで節点を作るたびに「同じものが既にないか」を表で調べ、あれば作らずに
// 使い回す（英語では hash consing と呼ぶ手法）。こうすると同じ部分式は自動的に
// 1つの節点にまとまるので、同じ計算を繰り返さないための工夫が「あとから
// 最適化する工程」ではなく、組み立ての副作用としてただで手に入る。
//
// # 節点番号とスロット番号
//
// 変数を先に節点 0..numVars-1 として作るので、節点番号がそのまま命令列の
// スロット番号（値の置き場の番地）になる。Compile で番号を振り直す必要がない。
package dag

import (
	"fmt"

	"github.com/ichijohodaka/symmatinv/pkg/tape"
)

// ID は式グラフの節点を指す。
type ID int32

// opVar は変数の節点を表す内部用の印。tape.Op と混ざらないよう大きな値にする。
const opVar uint8 = 255

// node は比較可能な構造体。これをそのままハッシュ表の鍵にする。
type node struct {
	op   uint8
	a, b ID
}

// Builder は式を組み立てる。ゼロ値は使えないので New で作ること。
type Builder struct {
	numVars int
	nodes   []node
	memo    map[node]ID
}

// New は変数 numVars 個を持つ Builder を作る。変数の節点番号は 0..numVars-1。
func New(numVars int) *Builder {
	if numVars < 0 {
		panic("dag: numVars が負")
	}
	b := &Builder{
		numVars: numVars,
		nodes:   make([]node, numVars),
		memo:    make(map[node]ID),
	}
	for i := range b.nodes {
		b.nodes[i] = node{op: opVar, a: ID(i), b: -1}
	}
	return b
}

// NumVars は変数の個数。
func (b *Builder) NumVars() int { return b.numVars }

// NumNodes は今までに作られた節点の総数（変数を含む）。同じ部分式の共有が
// どれだけ効いたかを見るのに使う。
func (b *Builder) NumNodes() int { return len(b.nodes) }

// Var は i 番の変数の節点を返す。
func (b *Builder) Var(i int) ID {
	if i < 0 || i >= b.numVars {
		panic(fmt.Sprintf("dag: 変数 %d が範囲 [0,%d) の外", i, b.numVars))
	}
	return ID(i)
}

// intern は節点を1つ確保する。同じものが既にあればそれを返す。
func (b *Builder) intern(n node) ID {
	if id, ok := b.memo[n]; ok {
		return id
	}
	id := ID(len(b.nodes))
	b.nodes = append(b.nodes, n)
	b.memo[n] = id
	return id
}

// Add は x + y。
func (b *Builder) Add(x, y ID) ID { return b.commutative(tape.OpAdd, x, y) }

// Mul は x * y。
func (b *Builder) Mul(x, y ID) ID { return b.commutative(tape.OpMul, x, y) }

// commutative は可換な演算の引数を番号順に並べてから確保する。こうしないと
// x+y と y+x が別の節点になり、まとめそこねる。
func (b *Builder) commutative(op tape.Op, x, y ID) ID {
	b.check(x)
	b.check(y)
	if x > y {
		x, y = y, x
	}
	return b.intern(node{op: uint8(op), a: x, b: y})
}

// Sub は x - y。
func (b *Builder) Sub(x, y ID) ID {
	b.check(x)
	b.check(y)
	return b.intern(node{op: uint8(tape.OpSub), a: x, b: y})
}

// Neg は -x。
//
// -(-x) = x は体でつねに成り立つので、その場で畳んで節点を1つ減らす。
func (b *Builder) Neg(x ID) ID {
	b.check(x)
	if n := b.nodes[x]; n.op == uint8(tape.OpNeg) {
		return n.a
	}
	return b.intern(node{op: uint8(tape.OpNeg), a: x, b: -1})
}

// Recip は 1/x。
//
// 1/(1/x) = x は x が可逆であるかぎり成り立つ。逆行列の計算では逆数をとる
// 対象は可逆であることが前提なので、その場で畳んでよい。
func (b *Builder) Recip(x ID) ID {
	b.check(x)
	if n := b.nodes[x]; n.op == uint8(tape.OpRecip) {
		return n.a
	}
	return b.intern(node{op: uint8(tape.OpRecip), a: x, b: -1})
}

func (b *Builder) check(x ID) {
	if x < 0 || int(x) >= len(b.nodes) {
		panic(fmt.Sprintf("dag: 節点 %d が範囲 [0,%d) の外", x, len(b.nodes)))
	}
}

// Compile は outputs を出力とする命令列を作る。出力に寄与しない節点は落とす。
//
// 節点番号がそのままスロット番号なので、変換は節点を順に命令へ書き写すだけ。
func (b *Builder) Compile(outputs []ID) *tape.Tape {
	instrs := make([]tape.Instr, 0, len(b.nodes)-b.numVars)
	for _, n := range b.nodes[b.numVars:] {
		op := tape.Op(n.op)
		if !op.Valid() {
			panic(fmt.Sprintf("dag: 変換できない節点 (op=%d)", n.op))
		}
		if op.Arity() == 2 {
			instrs = append(instrs, tape.Bin(op, int32(n.a), int32(n.b)))
		} else {
			instrs = append(instrs, tape.Un(op, int32(n.a)))
		}
	}
	outs := make([]int32, len(outputs))
	for i, id := range outputs {
		b.check(id)
		outs[i] = int32(id)
	}
	t := &tape.Tape{NumVars: b.numVars, Instrs: instrs, Outputs: outs}
	return t.Prune()
}
