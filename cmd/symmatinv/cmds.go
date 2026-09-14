package main

import (
	"errors"
	"flag"
	"fmt"
	"math/big"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/ichijohodaka/symmatinv/internal/inline"
	"github.com/ichijohodaka/symmatinv/internal/numeric"
	"github.com/ichijohodaka/symmatinv/internal/verify"
	"github.com/ichijohodaka/symmatinv/pkg/symmat"
	"github.com/ichijohodaka/symmatinv/pkg/tape"
)

// common は全サブコマンドに共通の指定（次数と分け方）をまとめる。
type common struct {
	n    int
	plan string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.IntVar(&c.n, "n", 0, "行列の次数（必須）")
	fs.StringVar(&c.plan, "plan", "", `分け方。例 "6=3+3[3=1+2,3=1+2]"。省略すると半分ずつ`)
}

// resolve は次数と分け方を確定させる。
func (c *common) resolve() (*symmat.Plan, error) {
	if c.n < 1 {
		return nil, errors.New("-n に 1 以上の次数を指定してください")
	}
	if c.plan == "" {
		return symmat.DefaultPlan(c.n), nil
	}
	p, err := symmat.ParsePlan(c.plan)
	if err != nil {
		return nil, err
	}
	if p.N != c.n {
		return nil, fmt.Errorf("分け方 %q は %d 次だが -n は %d", c.plan, p.N, c.n)
	}
	return p, nil
}

// inverse は次数と分け方から計算手順を作る。
func (c *common) inverse() (*symmat.Inv, error) {
	p, err := c.resolve()
	if err != nil {
		return nil, err
	}
	return symmat.InverseWithPlan(c.n, p)
}

func newRNG(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, 0x5eed)) }

// ---- show ----

func cmdShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	var c common
	c.register(fs)
	det := fs.Bool("det", false, "逆行列ではなく行列式の計算手順を表示する")
	if err := fs.Parse(args); err != nil {
		return err
	}

	p, err := c.resolve()
	if err != nil {
		return err
	}
	var tp *tape.Tape
	if *det {
		tp, err = symmat.DeterminantWithPlan(c.n, p)
	} else {
		var inv *symmat.Inv
		inv, err = symmat.InverseWithPlan(c.n, p)
		if err == nil {
			tp = inv.Tape()
		}
	}
	if err != nil {
		return err
	}

	fmt.Printf("# %d 次対称行列の%s\n", c.n, map[bool]string{true: "行列式", false: "逆行列"}[*det])
	fmt.Printf("# 分け方 %s（matrix inversion lemma %d 段）\n", p, p.Stages())
	fmt.Printf("# 入力 %d 個, 出力 %d 個, 演算 %d 回%s\n\n",
		tp.NumVars, len(tp.Outputs), tp.NumInstrs(), opBreakdown(tp))
	fmt.Print(tp)
	return nil
}

func opBreakdown(tp *tape.Tape) string {
	c := tp.OpCounts()
	return fmt.Sprintf(" (add %d, sub %d, mul %d, neg %d, recip %d)",
		c[tape.OpAdd], c[tape.OpSub], c[tape.OpMul], c[tape.OpNeg], c[tape.OpRecip])
}

// ---- gen ----

func cmdGen(args []string) error {
	fs := flag.NewFlagSet("gen", flag.ExitOnError)
	var c common
	c.register(fs)
	lang := fs.String("lang", "go", "書き出す形式。go か json")
	out := fs.String("o", "", "書き出し先のファイル。省略すると標準出力")
	pkg := fs.String("pkg", "symmatgen", "Go のパッケージ名")
	fn := fs.String("func", "", "Go の関数名。省略すると Inv<n>")
	det := fs.Bool("det", false, "逆行列ではなく行列式を書き出す")
	if err := fs.Parse(args); err != nil {
		return err
	}

	p, err := c.resolve()
	if err != nil {
		return err
	}
	var tp *tape.Tape
	kind := "逆行列"
	if *det {
		kind = "行列式"
		tp, err = symmat.DeterminantWithPlan(c.n, p)
	} else {
		var inv *symmat.Inv
		inv, err = symmat.InverseWithPlan(c.n, p)
		if err == nil {
			tp = inv.Tape()
		}
	}
	if err != nil {
		return err
	}

	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}

	switch *lang {
	case "json":
		if err := tape.WriteJSON(w, tp); err != nil {
			return err
		}
	case "go":
		name := *fn
		if name == "" {
			name = fmt.Sprintf("Inv%d", c.n)
			if *det {
				name = fmt.Sprintf("Det%d", c.n)
			}
		}
		doc := fmt.Sprintf("%s は %d 次対称行列の%sを求める。\n", name, c.n, kind)
		doc += fmt.Sprintf("\n入力 v は上三角を行優先で並べたもの（%s）。",
			strings.Join(symmat.Names(c.n, "a"), " "))
		if !*det {
			doc += "\n出力 out も同じ並び（逆行列もまた対称なので）。"
		}
		src := fmt.Sprintf("生成元: symmatinv gen -n %d -plan %q", c.n, p.String())
		if err := tape.GenGo(w, tp, tape.GoOptions{
			Package: *pkg, Func: name, Doc: doc, Source: src,
		}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("-lang は go か json（%q が指定された）", *lang)
	}

	if *out != "" {
		fmt.Fprintf(os.Stderr, "%s に書き出しました（%d 次の%s、演算 %d 回）\n",
			*out, c.n, kind, tp.NumInstrs())
	}
	return nil
}

// ---- verify ----

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	var c common
	c.register(fs)
	trials := fs.Int("trials", 100, "試す乱数の組の数")
	exact := fs.Bool("exact", false, "有理数で厳密に確かめる（遅いが丸め誤差がない）")
	seed := fs.Uint64("seed", 20260914, "乱数の種")
	if err := fs.Parse(args); err != nil {
		return err
	}

	inv, err := c.inverse()
	if err != nil {
		return err
	}
	tp := inv.Tape()
	fmt.Printf("%d 次、分け方 %s、演算 %d 回\n", c.n, inv.Plan(), tp.NumInstrs())

	st := time.Now()
	res, err := verify.Residual(c.n, tp, newRNG(*seed), *trials)
	if err != nil {
		return err
	}
	fmt.Printf("残差   A·A⁻¹ − I の最大絶対値 %.3e（乱数 %d 組, %v）\n",
		res, *trials, time.Since(st).Round(time.Millisecond))
	if res > 1e-10 {
		return fmt.Errorf("残差が大きすぎる（%.3e）", res)
	}

	if *exact {
		n := *trials
		if n > 10 {
			n = 10 // 有理数は重いので控えめに
		}
		st = time.Now()
		if err := verify.Exact(c.n, tp, newRNG(*seed), n); err != nil {
			return err
		}
		fmt.Printf("厳密   A·A⁻¹ = I が有理数で成立（%d 組, %v）\n",
			n, time.Since(st).Round(time.Millisecond))
	}

	// 独立に書いた数値 LU とも突き合わせる。
	st = time.Now()
	if err := compareWithLU(c.n, tp, newRNG(*seed+1), *trials); err != nil {
		return err
	}
	fmt.Printf("LU     部分ピボット付き LU と一致（%d 組, %v）\n",
		*trials, time.Since(st).Round(time.Millisecond))
	fmt.Println("すべて通りました。")
	return nil
}

func compareWithLU(n int, tp *tape.Tape, rng *rand.Rand, trials int) error {
	ne := symmat.NumEntries(n)
	scratch := tape.NewScratch[float64](tp)
	out := make([]float64, ne)
	af := make([]float64, n*n)
	bf := make([]float64, n*n)
	lu := numeric.NewLU(n)

	for trial := 0; trial < trials; trial++ {
		a := verify.RandomSPD(n, rng)
		tape.EvalFloat64(tp, a, scratch, out)
		symmat.ToFull(n, a, af)
		if err := lu.Inverse(af, bf); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			for j := i; j < n; j++ {
				got, want := out[symmat.Index(n, i, j)], bf[i*n+j]
				if d := got - want; d > 1e-9*(1+abs(want)) || -d > 1e-9*(1+abs(want)) {
					return fmt.Errorf("%d 組目 (%d,%d): 記号 %g, LU %g", trial, i, j, got, want)
				}
			}
		}
	}
	return nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ---- plan ----

func cmdPlan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	n := fs.Int("n", 0, "行列の次数（必須）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *n < 3 {
		return errors.New("-n に 3 以上の次数を指定してください（1 次と 2 次は分けられません）")
	}

	def := symmat.DefaultPlan(*n)
	fmt.Printf("%d 次の分け方と演算回数（最上段だけ変えて比べる）\n\n", *n)
	fmt.Printf("  %-28s %8s %8s  %s\n", "分け方", "演算", "うち逆数", "")
	best, bestPlan := -1, ""
	for p := 1; p < *n; p++ {
		plan, err := symmat.SplitPlan(*n, p)
		if err != nil {
			return err
		}
		inv, err := symmat.InverseWithPlan(*n, plan)
		if err != nil {
			return err
		}
		c := inv.Tape().OpCounts()
		mark := ""
		if plan.String() == def.String() {
			mark = "← 既定"
		}
		fmt.Printf("  %-28s %8d %8d  %s\n", plan, inv.NumInstrs(), c[tape.OpRecip], mark)
		if best < 0 || inv.NumInstrs() < best {
			best, bestPlan = inv.NumInstrs(), plan.String()
		}
	}
	fmt.Printf("\n最小は %s の %d 回。\n", bestPlan, best)
	return nil
}

// ---- bench ----

func cmdBench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	var c common
	c.register(fs)
	sets := fs.Int("sets", 1_000_000, "代入するパラメータの組の数")
	pool := fs.Int("pool", 4096, "あらかじめ用意しておく乱数行列の数")
	limit := fs.Duration("timeout", 10*time.Minute, "1方式あたりの打ち切り時間")
	max3 := fs.Int("max3", 1<<22, "方法3 を実際に組み立てる命令数の上限")
	seed := fs.Uint64("seed", 20260914, "乱数の種")
	if err := fs.Parse(args); err != nil {
		return err
	}

	inv, err := c.inverse()
	if err != nil {
		return err
	}
	n, tp := c.n, inv.Tape()
	ne := symmat.NumEntries(n)

	// 乱数行列をあらかじめ作っておき、3方式で同じものを使う。乱数生成の時間が
	// 比較に混ざらないようにするため。
	rng := newRNG(*seed)
	mats := make([][]float64, *pool)
	full := make([][]float64, *pool)
	for i := range mats {
		mats[i] = verify.RandomSPD(n, rng)
		full[i] = make([]float64, n*n)
		symmat.ToFull(n, mats[i], full[i])
	}

	fmt.Printf("%d 次、分け方 %s、%d 組を代入\n\n", n, inv.Plan(), *sets)
	fmt.Printf("  %-34s %10s %14s %12s\n", "方式", "演算回数", "時間", "1組あたり")

	// 方法1: 先に数値を代入して毎回 LU
	lu := numeric.NewLU(n)
	bf := make([]float64, n*n)
	report("方法1 先に代入して毎回 LU", luOps(n), run(*sets, *limit, func(i int) {
		_ = lu.Inverse(full[i%*pool], bf)
	}))

	// 方法2: 中間表現を保った命令列
	scratch := tape.NewScratch[float64](tp)
	out := make([]float64, ne)
	report("方法2 命令列（中間表現を保つ）", tp.NumInstrs(), run(*sets, *limit, func(i int) {
		tape.EvalFloat64(tp, mats[i%*pool], scratch, out)
	}))

	// 方法3: すべて代入して a_ij の式にしたもの
	size := inline.Size(tp)
	if !size.IsInt64() || size.Int64() > int64(*max3) {
		fmt.Printf("  %-34s %10s %14s\n",
			"方法3 すべて代入", size.String(), "組み立てず（大きすぎ）")
		fmt.Printf("\n方法3 は %s 命令になる（方法2 の %s 倍）ので、-max3 %d では組み立てません。\n",
			size, ratio(size, tp.NumInstrs()), *max3)
		return nil
	}
	// 方法3 は演算回数が方法2 の何桁も上になることがある。始める前に、どれだけ
	// かかりそうかを先に見せておく（途中で止めたくなることがあるので）。
	fmt.Printf("  %-34s %10s   方法2 の %s 倍。組み立て中…\n",
		"方法3 すべて代入（共有なし）", size.String(), ratio(size, tp.NumInstrs()))
	flat, err := inline.Expand(tp, *max3)
	if err != nil {
		return err
	}
	fscratch := tape.NewScratch[float64](flat)
	report("方法3 すべて代入（共有なし）", flat.NumInstrs(), run(*sets, *limit, func(i int) {
		tape.EvalFloat64(flat, mats[i%*pool], fscratch, out)
	}))
	fmt.Printf("\n方法3 は方法2 の %s 倍の演算をしている。これが「同じ部分式を何度も計算する無駄」。\n",
		ratio(size, tp.NumInstrs()))
	return nil
}

// luOps は LU による逆行列の演算回数のおおよその目安（n³ + n·n² ≒ 2n³/3 + n³）。
// 正確な内訳ではなく、桁を比べるための数。
func luOps(n int) int { return 2*n*n*n/3 + n*n*n }

type result struct {
	done int
	took time.Duration
	cut  bool
}

// run は f を sets 回まわし、limit を超えたら打ち切る。
//
// 時計を見る回数を減らすため何回かまとめて実行するが、そのまとめる数は
// 実測した1回あたりの時間から決め直す。方法3 のように1回が数ミリ秒かかる
// 場合でも、決め打ちのまとまりだと打ち切りが何分も遅れてしまうため。
func run(sets int, limit time.Duration, f func(i int)) result {
	const watch = 50 * time.Millisecond // 見回りの間隔の目安
	st := time.Now()
	chunk, i := 1, 0
	for i < sets {
		end := min(i+chunk, sets)
		for ; i < end; i++ {
			f(i)
		}
		el := time.Since(st)
		if el > limit {
			return result{done: i, took: el, cut: true}
		}
		if per := el / time.Duration(i); per > 0 {
			chunk = min(max(int(watch/per), 1), 1<<20)
		} else {
			chunk = min(chunk*4, 1<<20)
		}
	}
	return result{done: sets, took: time.Since(st)}
}

func report(name string, ops int, r result) {
	per := r.took / time.Duration(max(r.done, 1))
	note := ""
	if r.cut {
		note = fmt.Sprintf("（%d 組で打ち切り）", r.done)
	}
	fmt.Printf("  %-34s %10d %14v %12v %s\n",
		name, ops, r.took.Round(time.Millisecond), per.Round(time.Nanosecond), note)
}

func ratio(size *big.Int, base int) string {
	r := new(big.Float).Quo(new(big.Float).SetInt(size), big.NewFloat(float64(base)))
	return r.Text('g', 4)
}
