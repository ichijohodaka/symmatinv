package symmat_test

import (
	"strings"
	"testing"

	"github.com/ichijohodaka/symmatinv/pkg/symmat"
)

func TestDefaultPlanString(t *testing.T) {
	want := map[int]string{
		1:  "1",
		2:  "2",
		3:  "3=1+2",
		4:  "4=2+2",
		5:  "5=2+3[2,3=1+2]",
		6:  "6=3+3[3=1+2,3=1+2]",
		8:  "8=4+4[4=2+2,4=2+2]",
		12: "12=6+6[6=3+3[3=1+2,3=1+2],6=3+3[3=1+2,3=1+2]]",
	}
	for n, w := range want {
		if got := symmat.DefaultPlan(n).String(); got != w {
			t.Errorf("DefaultPlan(%d) = %q, want %q", n, got, w)
		}
	}
}

func TestPlanStages(t *testing.T) {
	want := map[int]int{1: 0, 2: 0, 3: 1, 4: 1, 5: 2, 6: 2, 8: 2, 16: 3}
	for n, w := range want {
		if got := symmat.DefaultPlan(n).Stages(); got != w {
			t.Errorf("DefaultPlan(%d).Stages() = %d, want %d", n, got, w)
		}
	}
}

// TestParsePlanRoundTrip は String が書いたものを ParsePlan が読み戻せることを
// 確かめる。計画を記録して再現するために必要な性質。
func TestParsePlanRoundTrip(t *testing.T) {
	for n := 1; n <= 16; n++ {
		s := symmat.DefaultPlan(n).String()
		got, err := symmat.ParsePlan(s)
		if err != nil {
			t.Fatalf("ParsePlan(%q): %v", s, err)
		}
		if got.String() != s {
			t.Errorf("往復で変わった: %q -> %q", s, got.String())
		}
		if got.N != n {
			t.Errorf("次数が %d になった（%d のはず）", got.N, n)
		}
	}
}

// TestParsePlanFillsDefaults は、最上段だけ書けば下は既定の分け方で埋まる
// ことを確かめる。
func TestParsePlanFillsDefaults(t *testing.T) {
	got, err := symmat.ParsePlan("6=3+3")
	if err != nil {
		t.Fatal(err)
	}
	if want := "6=3+3[3=1+2,3=1+2]"; got.String() != want {
		t.Errorf("= %q, want %q", got, want)
	}
}

func TestParsePlanRejects(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "数がない"},
		{"6", "分け方を書かないといけない"},
		{"6=3+4", "不正"},           // 3+4 ≠ 6
		{"6=3+3[3=1+2]", ", がない"}, // 下位計画が1つしかない
		{"6=3+3[3=1+2,3=1+2", "] がない"},
		{"6=3+3xyz", "余分"},
		{"6=3", "+ がない"},
		{"4=2+2[1,2]", "計画の次数"}, // 1 次の計画を 2 次のところに置いた
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			_, err := symmat.ParsePlan(c.in)
			if err == nil {
				t.Fatalf("エラーになるはずが nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("エラーに %q を含むはず: %v", c.want, err)
			}
		})
	}
}

// TestParsedPlanWorks は、読んだ計画で実際に逆行列が組み立てられることを
// 確かめる。
func TestParsedPlanWorks(t *testing.T) {
	p, err := symmat.ParsePlan("7=2+5[2,5=1+4[1,4=2+2]]")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := symmat.InverseWithPlan(7, p)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Plan().Stages() != 3 {
		t.Errorf("段数 = %d, want 3", inv.Plan().Stages())
	}
	if err := inv.Tape().Validate(); err != nil {
		t.Fatal(err)
	}
}
