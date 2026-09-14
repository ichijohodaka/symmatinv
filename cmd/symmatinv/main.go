// symmatinv は n 次対称行列の逆行列を、要素を文字のまま求めるコマンド。
//
// 詳しくは各サブコマンドの -h を見ること。
package main

import (
	"fmt"
	"os"
)

const usage = `symmatinv — n 次対称行列の逆行列を、要素を文字のまま求める

使い方:
  symmatinv <サブコマンド> [オプション]

サブコマンド:
  show     計算手順を人が読める形で表示する
  gen      Go のソースか JSON に書き出す
  verify   本当に逆行列になっているかを検算する
  plan     分け方の候補を並べて演算回数を比べる
  bench    方法1・2・3 の速さを比べる

共通のオプション:
  -n N        行列の次数（必須）
  -plan P     分け方。例 "6=3+3[3=1+2,3=1+2]"。省略すると半分ずつ

例:
  symmatinv show -n 3
  symmatinv gen -n 6 -lang go -pkg mypkg -func Inv6 -o inv6.go
  symmatinv verify -n 6 -exact
  symmatinv plan -n 8
  symmatinv bench -n 6 -sets 1000000
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "show":
		err = cmdShow(os.Args[2:])
	case "gen":
		err = cmdGen(os.Args[2:])
	case "verify":
		err = cmdVerify(os.Args[2:])
	case "plan":
		err = cmdPlan(os.Args[2:])
	case "bench":
		err = cmdBench(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "symmatinv: 知らないサブコマンド %q\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "symmatinv: %v\n", err)
		os.Exit(1)
	}
}
