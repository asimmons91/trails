package fieldsgen

import (
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
)

// packModelPkgPath is compared against by import-path string, not by
// importing the pack package directly: the target package's type-checking
// session creates its own *types.Package instance for
// "github.com/asimmons91/trails/pack", distinct from any package object
// fieldsgen itself might import, so identity can only be established by
// path string, not object equality.
const packModelPkgPath = "github.com/asimmons91/trails/pack"

// discoveredModel is a struct type found to be a pack model, along with
// its flattened field/method shape.
type discoveredModel struct {
	Name   string
	Struct *types.Struct
}

// discoverModels finds every struct type in pkg that is a pack model: one
// with a directly embedded pack.Model[...] field, or one that implements
// TableName() string (value or pointer receiver). Results are sorted by
// name for deterministic generated output.
func discoverModels(pkg *packages.Package) []discoveredModel {
	scope := pkg.Types.Scope()
	names := scope.Names()
	sort.Strings(names)

	tableNamer := tableNamerInterface(pkg.Types)

	var out []discoveredModel
	for _, name := range names {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}

		if embedsPackModel(st) || implementsTableName(named, tableNamer) {
			out = append(out, discoveredModel{Name: name, Struct: st})
		}
	}

	return out
}

func embedsPackModel(st *types.Struct) bool {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Embedded() {
			continue
		}

		named, ok := f.Type().(*types.Named)
		if !ok || named.Obj().Pkg() == nil {
			continue
		}

		if named.Obj().Pkg().Path() == packModelPkgPath && named.Obj().Name() == "Model" &&
			named.TypeArgs() != nil && named.TypeArgs().Len() == 1 {
			return true
		}
	}

	return false
}

func tableNamerInterface(pkg *types.Package) *types.Interface {
	sig := types.NewSignatureType(nil, nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String])), false)
	fn := types.NewFunc(token.NoPos, pkg, "TableName", sig)
	iface := types.NewInterfaceType([]*types.Func{fn}, nil)
	iface.Complete()
	return iface
}

func implementsTableName(named *types.Named, iface *types.Interface) bool {
	return types.Implements(named, iface) || types.Implements(types.NewPointer(named), iface)
}
