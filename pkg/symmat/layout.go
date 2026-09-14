package symmat

import "fmt"

// 対称行列は上三角だけを行優先で並べて受け渡す。n = 3 なら
//
//	[a00 a01 a02 a11 a12 a22]
//
// の6個。対称行列の逆行列もまた対称なので、入力も出力も同じ並びになる。
// n² 個すべてを持つより演算がおよそ半分で済む。

// NumEntries は n 次対称行列の独立な要素の個数 n(n+1)/2 を返す。
func NumEntries(n int) int { return n * (n + 1) / 2 }

// Index は要素 (i, j) が上三角の並びの何番目かを返す。i > j でも（対称性から）
// (j, i) と同じ番号を返すので、添字の順を気にせず使える。
func Index(n, i, j int) int {
	if i > j {
		i, j = j, i
	}
	if i < 0 || j >= n {
		panic(fmt.Sprintf("symmat: 添字 (%d,%d) が %d 次の範囲外", i, j, n))
	}
	// i 行目の先頭までに i*n - i(i-1)/2 個ある。
	return i*n - i*(i-1)/2 + (j - i)
}

// Names は上三角の並びに対応する要素名 a00, a01, … を作る。
// prefix に "a" を渡せば入力用、"b" を渡せば出力用の名前になる。
func Names(n int, prefix string) []string {
	names := make([]string, NumEntries(n))
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			names[Index(n, i, j)] = fmt.Sprintf("%s%d%d", prefix, i, j)
		}
	}
	return names
}

// ToFull は上三角の並び upper を n×n の行優先の full に広げる。
// 値を写すだけなので、どんな型にも使える。
func ToFull[T any](n int, upper, full []T) {
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			v := upper[Index(n, i, j)]
			full[i*n+j] = v
			full[j*n+i] = v
		}
	}
}

// FromFull は n×n の行優先の full から上三角だけを取り出す。
// full が対称でない場合、上三角側の値が使われる。
func FromFull[T any](n int, full, upper []T) {
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			upper[Index(n, i, j)] = full[i*n+j]
		}
	}
}
