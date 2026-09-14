package symmat

import (
	"fmt"
	"strconv"
	"strings"
)

// 分割の種類。
const (
	KindDirect1 = "direct1" // 1 次。逆数をとるだけ
	KindDirect2 = "direct2" // 2 次。逆行列の公式を直接使う
	KindMIL     = "mil"     // matrix inversion lemma で p + q に分ける
)

// Plan は逆行列をどう分割して計算するかの計画。
//
// matrix inversion lemma（逆行列補題）は n 次の逆行列を、p 次の A11 の逆行列と
// q 次の Schur 補元 S の逆行列という2つの小さい問題に帰着させる。その2つを
// さらにどう分けるかを Sub が持つ、という入れ子になっている。
//
// 同じ n でも分け方によって演算回数が変わるので、どう分けたかは結果と一緒に
// 記録しておく（再現性のため）。
type Plan struct {
	N    int     `json:"n"`
	Kind string  `json:"kind"`
	P    int     `json:"p,omitempty"`   // Kind が mil のときの A11 の次数
	Q    int     `json:"q,omitempty"`   // Kind が mil のときの Schur 補元の次数
	Sub  []*Plan `json:"sub,omitempty"` // Kind が mil のとき [A11 の計画, S の計画]
}

// DefaultPlan は既定の分割、つまり半分ずつに割る計画を作る。
//
// n = 1, 2 は公式を直接使う。それ以上は p = ⌊n/2⌋ と q = n − p に分け、
// 両方を再帰的に同じ方針で分ける。
func DefaultPlan(n int) *Plan {
	switch {
	case n <= 0:
		return nil
	case n == 1:
		return &Plan{N: 1, Kind: KindDirect1}
	case n == 2:
		return &Plan{N: 2, Kind: KindDirect2}
	}
	p := n / 2
	q := n - p
	return &Plan{
		N: n, Kind: KindMIL, P: p, Q: q,
		Sub: []*Plan{DefaultPlan(p), DefaultPlan(q)},
	}
}

// SplitPlan は最上段だけを n = p + q に分け、下は既定の分け方にする計画を作る。
func SplitPlan(n, p int) (*Plan, error) {
	if n < 3 {
		return nil, fmt.Errorf("symmat: %d 次は分割できない（公式を直接使う）", n)
	}
	if p < 1 || p >= n {
		return nil, fmt.Errorf("symmat: 分割 %d = %d + %d が不正", n, p, n-p)
	}
	q := n - p
	return &Plan{N: n, Kind: KindMIL, P: p, Q: q,
		Sub: []*Plan{DefaultPlan(p), DefaultPlan(q)}}, nil
}

// Validate は計画が n 次の行列に対して筋が通っているかを調べる。
func (p *Plan) Validate(n int) error {
	if p == nil {
		return fmt.Errorf("symmat: 計画が nil")
	}
	if p.N != n {
		return fmt.Errorf("symmat: 計画の次数 %d が %d と合わない", p.N, n)
	}
	switch p.Kind {
	case KindDirect1:
		if n != 1 {
			return fmt.Errorf("symmat: %s は 1 次専用だが %d 次に使われている", p.Kind, n)
		}
	case KindDirect2:
		if n != 2 {
			return fmt.Errorf("symmat: %s は 2 次専用だが %d 次に使われている", p.Kind, n)
		}
	case KindMIL:
		if p.P < 1 || p.Q < 1 || p.P+p.Q != n {
			return fmt.Errorf("symmat: 分割 %d = %d + %d が不正", n, p.P, p.Q)
		}
		if len(p.Sub) != 2 {
			return fmt.Errorf("symmat: %d 次の分割に下位計画が %d 個（2 個必要）", n, len(p.Sub))
		}
		if err := p.Sub[0].Validate(p.P); err != nil {
			return err
		}
		if err := p.Sub[1].Validate(p.Q); err != nil {
			return err
		}
	default:
		return fmt.Errorf("symmat: 未知の分割の種類 %q", p.Kind)
	}
	return nil
}

// ParsePlan は String が書いた形の文字列を読む。
//
//	"6"                      分けない（1 次・2 次のみ）
//	"6=3+3"                  最上段だけ指定し、下は既定の分け方
//	"6=3+3[3=1+2,3=1+2]"     下まで明示
//
// 空白は無視する。
func ParsePlan(s string) (*Plan, error) {
	p := &planParser{src: strings.Join(strings.Fields(s), "")}
	plan, err := p.plan()
	if err != nil {
		return nil, err
	}
	if p.i != len(p.src) {
		return nil, fmt.Errorf("symmat: 計画 %q の %d 文字目から先が余分", s, p.i)
	}
	if err := plan.Validate(plan.N); err != nil {
		return nil, err
	}
	return plan, nil
}

type planParser struct {
	src string
	i   int
}

func (p *planParser) plan() (*Plan, error) {
	n, err := p.number()
	if err != nil {
		return nil, err
	}
	if !p.accept('=') {
		// 分けない段。次数から種類が決まる。
		switch n {
		case 1:
			return &Plan{N: 1, Kind: KindDirect1}, nil
		case 2:
			return &Plan{N: 2, Kind: KindDirect2}, nil
		}
		return nil, fmt.Errorf("symmat: %d 次は分け方を書かないといけない（1 次と 2 次だけが省略可）", n)
	}
	np, err := p.number()
	if err != nil {
		return nil, err
	}
	if !p.accept('+') {
		return nil, fmt.Errorf("symmat: 計画の %d 文字目に + がない", p.i)
	}
	nq, err := p.number()
	if err != nil {
		return nil, err
	}
	plan := &Plan{N: n, Kind: KindMIL, P: np, Q: nq}
	if p.accept('[') {
		sub1, err := p.plan()
		if err != nil {
			return nil, err
		}
		if !p.accept(',') {
			return nil, fmt.Errorf("symmat: 計画の %d 文字目に , がない", p.i)
		}
		sub2, err := p.plan()
		if err != nil {
			return nil, err
		}
		if !p.accept(']') {
			return nil, fmt.Errorf("symmat: 計画の %d 文字目に ] がない", p.i)
		}
		plan.Sub = []*Plan{sub1, sub2}
	} else {
		if np < 1 || nq < 1 {
			return nil, fmt.Errorf("symmat: 分割 %d = %d + %d が不正", n, np, nq)
		}
		plan.Sub = []*Plan{DefaultPlan(np), DefaultPlan(nq)}
	}
	return plan, nil
}

func (p *planParser) accept(c byte) bool {
	if p.i < len(p.src) && p.src[p.i] == c {
		p.i++
		return true
	}
	return false
}

func (p *planParser) number() (int, error) {
	start := p.i
	for p.i < len(p.src) && p.src[p.i] >= '0' && p.src[p.i] <= '9' {
		p.i++
	}
	if start == p.i {
		return 0, fmt.Errorf("symmat: 計画の %d 文字目に数がない", start)
	}
	n, err := strconv.Atoi(p.src[start:p.i])
	if err != nil {
		return 0, fmt.Errorf("symmat: 計画の数を読めない: %w", err)
	}
	return n, nil
}

// Stages は matrix inversion lemma を何段重ねたかを返す。
func (p *Plan) Stages() int {
	if p == nil || p.Kind != KindMIL {
		return 0
	}
	best := 0
	for _, s := range p.Sub {
		if d := s.Stages(); d > best {
			best = d
		}
	}
	return 1 + best
}

// String は分け方を "6=3+3[3=2+1,3=2+1]" の形で表す。
// 分けない段（1 次・2 次）は次数だけを書く。
func (p *Plan) String() string {
	if p == nil {
		return ""
	}
	if p.Kind != KindMIL {
		return fmt.Sprintf("%d", p.N)
	}
	subs := make([]string, 0, 2)
	nested := false
	for _, s := range p.Sub {
		subs = append(subs, s.String())
		if s.Kind == KindMIL {
			nested = true
		}
	}
	head := fmt.Sprintf("%d=%d+%d", p.N, p.P, p.Q)
	if !nested {
		return head
	}
	return head + "[" + strings.Join(subs, ",") + "]"
}
