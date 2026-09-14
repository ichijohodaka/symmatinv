package tape

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestJSONRoundTrip(t *testing.T) {
	orig := inv2()
	var buf bytes.Buffer
	if err := WriteJSON(&buf, orig); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	got, err := ReadJSON(&buf)
	if err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("往復で変わった:\n元  %+v\n後  %+v", orig, got)
	}
}

func TestJSONLayout(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, inv2()); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	s := buf.String()
	for _, want := range []string{
		`"version": 1`,
		`"numVars": 3`,
		`"varNames": ["a00","a01","a11"]`,
		`["mul",0,2]`,
		`["recip",5]`, // 単項は要素2つ
		`["neg",8]`,
		`"outputs": [7,9,10]`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("出力に %q がない:\n%s", want, s)
		}
	}
	// 命令が1行に1つ並んでいること（diff が読みやすいことの担保）。
	if n := strings.Count(s, "],\n    ["); n != 7 {
		t.Errorf("命令の改行が想定と違う (%d):\n%s", n, s)
	}
}

func TestReadJSONRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"版が違う",
			`{"version":999,"numVars":1,"instrs":[],"outputs":[0]}`,
			"形式の版が違う",
		},
		{
			"知らないフィールド",
			`{"version":1,"numVars":1,"instrs":[],"outputs":[0],"nazo":1}`,
			"JSON を読めない",
		},
		{
			"前方参照",
			`{"version":1,"numVars":1,"instrs":[["neg",1]],"outputs":[1]}`,
			"スロット範囲",
		},
		{
			"単項に引数2つ",
			`{"version":1,"numVars":2,"instrs":[["neg",0,1]],"outputs":[2]}`,
			"引数",
		},
		{
			"二項に引数1つ",
			`{"version":1,"numVars":2,"instrs":[["add",0]],"outputs":[2]}`,
			"引数",
		},
		{
			"未知の演算",
			`{"version":1,"numVars":1,"instrs":[["pow",0]],"outputs":[1]}`,
			"未知の演算",
		},
		{
			"出力が範囲外",
			`{"version":1,"numVars":1,"instrs":[],"outputs":[5]}`,
			"範囲",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ReadJSON(strings.NewReader(c.in))
			if err == nil {
				t.Fatal("エラーになるはずが nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("エラーに %q を含むはず: %v", c.want, err)
			}
		})
	}
}

// TestJSONUnaryCanonical は、単項演算を B 付きで書いても、読み込み時に B が
// -1 にそろえられることを確かめる。
func TestJSONUnaryCanonical(t *testing.T) {
	got, err := ReadJSON(strings.NewReader(
		`{"version":1,"numVars":1,"instrs":[["neg",0]],"outputs":[1]}`))
	if err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if got.Instrs[0].B != -1 {
		t.Errorf("単項の B = %d, want -1", got.Instrs[0].B)
	}
}

// TestJSONEmptyTape は命令も出力もない命令列が往復することを確かめる
// （出力が null ではなく [] になること）。
func TestJSONEmptyTape(t *testing.T) {
	orig := &Tape{NumVars: 2, Instrs: nil, Outputs: []int32{}}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, orig); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if s := buf.String(); !strings.Contains(s, `"outputs": []`) {
		t.Errorf("空の出力が [] になっていない:\n%s", s)
	}
	if _, err := ReadJSON(&buf); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
}
