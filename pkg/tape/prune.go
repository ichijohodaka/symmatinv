package tape

// Prune は出力に寄与しない命令を落とした新しい命令列を返す。t は変更しない。
//
// 入力変数は使われていなくても必ず残す。変数の並びは命令列の外向きの取り決め
// （何番目に何を入れるか）であって、中で使うかどうかとは別の話だから。
//
// 式グラフから素直に組み立てた命令列には普通そのような命令は残らないが、外から
// 読み込んだ命令列や、途中結果を捨てた命令列には残りうる。Go のコード生成では
// 使われない局所変数がコンパイルエラーになるため、生成の前に必ず通す。
func (t *Tape) Prune() *Tape {
	n := t.NumSlots()
	live := make([]bool, n)
	for _, s := range t.Outputs {
		live[s] = true
	}
	// 命令は必ず自分より前のスロットしか読まないので、後ろから1回なめれば
	// 生存判定が完結する。
	for i := len(t.Instrs) - 1; i >= 0; i-- {
		if !live[t.ResultSlot(i)] {
			continue
		}
		in := t.Instrs[i]
		live[in.A] = true
		if in.Op.Arity() == 2 {
			live[in.B] = true
		}
	}

	// 旧スロット番号 → 新スロット番号。変数はそのまま。
	remap := make([]int32, n)
	for i := 0; i < t.NumVars; i++ {
		remap[i] = int32(i)
	}
	instrs := make([]Instr, 0, len(t.Instrs))
	next := int32(t.NumVars)
	for i, in := range t.Instrs {
		old := t.ResultSlot(i)
		if !live[old] {
			continue
		}
		in.A = remap[in.A]
		if in.Op.Arity() == 2 {
			in.B = remap[in.B]
		}
		instrs = append(instrs, in)
		remap[old] = next
		next++
	}
	outputs := make([]int32, len(t.Outputs))
	for i, s := range t.Outputs {
		outputs[i] = remap[s]
	}

	out := &Tape{
		NumVars: t.NumVars,
		Instrs:  instrs,
		Outputs: outputs,
	}
	if len(t.VarNames) != 0 {
		out.VarNames = append([]string(nil), t.VarNames...)
	}
	if len(t.OutNames) != 0 {
		out.OutNames = append([]string(nil), t.OutNames...)
	}
	return out
}
