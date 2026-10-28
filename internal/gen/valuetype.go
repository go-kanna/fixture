package gen

import (
	"go/types"
	"strings"
)

// valueType is a value type as kanna-value generates one: a struct holding a
// value in its one unexported field, read with the accessor named after the
// value's type and made with the constructor beside it. It is known by that
// shape, as kanna-mapper knows it, since the struct may be declared in a
// package of which only the types are loaded.
type valueType struct {
	named *types.Named
	held  types.Type

	// ctor is the constructor, NewX(T) (X, error).
	ctor *types.Func
}

// valueTypeOf returns t as a value type the fixtures, in a package of their
// own, can make, if it is one.
func valueTypeOf(t *types.Named) (valueType, bool) {
	st, ok := t.Underlying().(*types.Struct)
	if !ok || t.TypeParams().Len() > 0 || !t.Obj().Exported() ||
		st.NumFields() != 1 || st.Field(0).Exported() || st.Field(0).Embedded() {
		return valueType{}, false
	}
	held := st.Field(0).Type()

	var name string
	switch h := types.Unalias(held).(type) {
	case *types.Basic:
		name = h.Name()
	case *types.Named:
		name = h.Obj().Name()
	default:
		return valueType{}, false
	}
	m, _, _ := types.LookupFieldOrMethod(t, false, nil, strings.ToUpper(name[:1])+name[1:])
	accessor, ok := m.(*types.Func)
	if !ok || accessor.Signature().Params().Len() != 0 || accessor.Signature().Results().Len() != 1 ||
		!types.Identical(accessor.Signature().Results().At(0).Type(), held) {
		return valueType{}, false
	}

	ctor, ok := t.Obj().Pkg().Scope().Lookup("New" + t.Obj().Name()).(*types.Func)
	if !ok {
		return valueType{}, false
	}
	sig := ctor.Signature()
	if sig.Params().Len() != 1 || sig.Results().Len() != 2 ||
		!types.Identical(sig.Params().At(0).Type(), held) ||
		!types.Identical(sig.Results().At(0).Type(), t) ||
		!types.Identical(sig.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return valueType{}, false
	}
	return valueType{named: t, held: held, ctor: ctor}, true
}
