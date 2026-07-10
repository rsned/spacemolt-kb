package patch

// applyTectonicBase (layer 0) initializes the heightmap from the
// cropped crust BaseHeight — the patch analog of rocky.go's crust
// raft init.
func applyTectonicBase(ctx *Context, st *State) *State {
	ns := *st
	ns.Height = ctx.Fields.BaseHeight.Clone()
	return &ns
}
