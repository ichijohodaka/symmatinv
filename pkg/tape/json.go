package tape

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// FormatVersion は命令列 JSON の版。読み書きの形式を変えたら上げる。
const FormatVersion = 1

// MarshalJSON は1命令を ["mul",0,3]（二項）または ["neg",6]（単項）という
// 配列にする。1行に収まるので、保存したファイルが人にも diff にも読みやすい。
func (in Instr) MarshalJSON() ([]byte, error) {
	if !in.Op.Valid() {
		return nil, fmt.Errorf("tape: 未知の演算 %d", uint8(in.Op))
	}
	var b bytes.Buffer
	b.WriteString(`["`)
	b.WriteString(in.Op.String())
	b.WriteString(`",`)
	fmt.Fprintf(&b, "%d", in.A)
	if in.Op.Arity() == 2 {
		fmt.Fprintf(&b, ",%d", in.B)
	}
	b.WriteByte(']')
	return b.Bytes(), nil
}

// UnmarshalJSON は MarshalJSON の逆。単項演算の B は -1 にそろえる。
func (in *Instr) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("tape: 命令が配列でない: %w", err)
	}
	if len(raw) == 0 {
		return fmt.Errorf("tape: 命令が空")
	}
	var name string
	if err := json.Unmarshal(raw[0], &name); err != nil {
		return fmt.Errorf("tape: 命令の先頭が演算名でない: %w", err)
	}
	op, err := ParseOp(name)
	if err != nil {
		return err
	}
	want := op.Arity() + 1
	if len(raw) != want {
		return fmt.Errorf("tape: %s は引数 %d 個だが %d 個ある", op, op.Arity(), len(raw)-1)
	}
	operands := make([]int32, op.Arity())
	for i := range operands {
		if err := json.Unmarshal(raw[i+1], &operands[i]); err != nil {
			return fmt.Errorf("tape: %s の引数 %d がスロット番号でない: %w", op, i, err)
		}
	}
	in.Op = op
	in.A = operands[0]
	if op.Arity() == 2 {
		in.B = operands[1]
	} else {
		in.B = -1
	}
	return nil
}

// jsonTape はファイル上の形。Tape そのものを直接 encoding/json に渡さないのは、
// version を持たせるためと、内部表現を変えても形式を固定できるようにするため。
type jsonTape struct {
	Version  int      `json:"version"`
	NumVars  int      `json:"numVars"`
	VarNames []string `json:"varNames,omitempty"`
	Instrs   []Instr  `json:"instrs"`
	Outputs  []int32  `json:"outputs"`
	OutNames []string `json:"outNames,omitempty"`
}

// WriteJSON は命令列を JSON で書き出す。命令は1行に1つ並べる。
func WriteJSON(w io.Writer, t *Tape) error {
	if err := t.Validate(); err != nil {
		return err
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  \"version\": %d,\n", FormatVersion)
	fmt.Fprintf(&b, "  \"numVars\": %d,\n", t.NumVars)
	if len(t.VarNames) != 0 {
		names, err := json.Marshal(t.VarNames)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "  \"varNames\": %s,\n", names)
	}
	b.WriteString("  \"instrs\": [")
	for i, in := range t.Instrs {
		line, err := in.MarshalJSON()
		if err != nil {
			return fmt.Errorf("命令 %d: %w", i, err)
		}
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("\n    ")
		b.Write(line)
	}
	if len(t.Instrs) != 0 {
		b.WriteString("\n  ")
	}
	b.WriteString("],\n")
	outs, err := json.Marshal(t.Outputs)
	if err != nil {
		return err
	}
	if len(t.Outputs) == 0 {
		outs = []byte("[]") // nil を null にしない
	}
	fmt.Fprintf(&b, "  \"outputs\": %s", outs)
	if len(t.OutNames) != 0 {
		names, err := json.Marshal(t.OutNames)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, ",\n  \"outNames\": %s", names)
	}
	b.WriteString("\n}\n")
	_, err = w.Write(b.Bytes())
	return err
}

// ReadJSON は WriteJSON が書いたものを読む。版を照合し、Validate まで通す。
// ここを通った命令列は、そのまま Eval に渡してよい。
func ReadJSON(r io.Reader) (*Tape, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var jt jsonTape
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&jt); err != nil {
		return nil, fmt.Errorf("tape: JSON を読めない: %w", err)
	}
	if jt.Version != FormatVersion {
		return nil, fmt.Errorf("tape: 形式の版が違う (ファイル %d, このプログラム %d)", jt.Version, FormatVersion)
	}
	t := &Tape{
		NumVars:  jt.NumVars,
		Instrs:   jt.Instrs,
		Outputs:  jt.Outputs,
		VarNames: jt.VarNames,
		OutNames: jt.OutNames,
	}
	if t.Outputs == nil {
		t.Outputs = []int32{}
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}
