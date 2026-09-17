package fieldsgen

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// auxTypes bundles the standard-library types needed to detect composite
// primary keys (isCompositeKeyCandidate mirrors
// pack/internal/schema.isCompositeKeyCandidate).
type auxTypes struct {
	timeType     *types.Named
	valuerIface  *types.Interface
	scannerIface *types.Interface
}

// loaded is the result of type-checking the target directory.
type loaded struct {
	pkg *packages.Package
	aux auxTypes
}

const (
	stdTime        = "time"
	stdDatabaseSQL = "database/sql"
	stdDatabaseDrv = "database/sql/driver"
)

// load type-checks the target directory (plus the handful of standard-
// library packages needed for composite-key detection) in one
// packages.Load call, so every resolved type shares one *types.Package
// identity per import path.
func load(absDir, outFile string) (*loaded, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedTypesSizes | packages.NeedSyntax | packages.NeedTypesInfo,
		Dir: absDir,
	}

	// A stale generated file from a prior run can reference fields that
	// have since been renamed or removed, which would make the package
	// fail to type-check and deadlock regeneration. Overlay it in-memory
	// with an empty file (correct package clause only) before loading.
	outPath := filepath.Join(absDir, outFile)
	if pkgName, ok := packageNameFor(absDir, outPath); ok {
		cfg.Overlay = map[string][]byte{
			outPath: []byte("package " + pkgName + "\n"),
		}
	}

	pkgs, err := packages.Load(cfg, ".", stdDatabaseSQL, stdDatabaseDrv, stdTime)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", absDir, err)
	}

	var (
		target                     *packages.Package
		sqlPkg, driverPkg, timePkg *packages.Package
		errs                       []error
	)
	for _, p := range pkgs {
		switch p.PkgPath {
		case stdDatabaseSQL:
			sqlPkg = p
		case stdDatabaseDrv:
			driverPkg = p
		case stdTime:
			timePkg = p
		default:
			target = p
		}
		errs = append(errs, packageErrors(p)...)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s does not compile: %w", absDir, errors.Join(errs...))
	}
	if target == nil {
		return nil, fmt.Errorf("loading %s: target package not found among loaded packages", absDir)
	}

	aux, err := buildAuxTypes(sqlPkg, driverPkg, timePkg)
	if err != nil {
		return nil, err
	}

	return &loaded{pkg: target, aux: aux}, nil
}

func packageErrors(p *packages.Package) []error {
	if len(p.Errors) == 0 {
		return nil
	}
	errs := make([]error, len(p.Errors))
	for i, e := range p.Errors {
		errs[i] = e
	}
	return errs
}

func buildAuxTypes(sqlPkg, driverPkg, timePkg *packages.Package) (auxTypes, error) {
	timeNamed, err := lookupNamed(timePkg, "Time")
	if err != nil {
		return auxTypes{}, err
	}
	valuerNamed, err := lookupNamed(driverPkg, "Valuer")
	if err != nil {
		return auxTypes{}, err
	}
	scannerNamed, err := lookupNamed(sqlPkg, "Scanner")
	if err != nil {
		return auxTypes{}, err
	}

	valuerIface, ok := valuerNamed.Underlying().(*types.Interface)
	if !ok {
		return auxTypes{}, fmt.Errorf("database/sql/driver.Valuer is not an interface type")
	}
	scannerIface, ok := scannerNamed.Underlying().(*types.Interface)
	if !ok {
		return auxTypes{}, fmt.Errorf("database/sql.Scanner is not an interface type")
	}

	return auxTypes{timeType: timeNamed, valuerIface: valuerIface, scannerIface: scannerIface}, nil
}

func lookupNamed(pkg *packages.Package, name string) (*types.Named, error) {
	if pkg == nil || pkg.Types == nil {
		return nil, fmt.Errorf("fieldsgen: %s not loaded", name)
	}
	obj := pkg.Types.Scope().Lookup(name)
	if obj == nil {
		return nil, fmt.Errorf("fieldsgen: %s.%s not found", pkg.PkgPath, name)
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return nil, fmt.Errorf("fieldsgen: %s.%s is not a named type", pkg.PkgPath, name)
	}
	return named, nil
}

// packageNameFor reports the package clause to use for an in-memory
// overlay placeholder at outPath, or false if outPath doesn't exist yet
// (a fresh generation needs no overlay). If outPath exists but its own
// package clause can't be parsed, falls back to a sibling .go file's.
func packageNameFor(dir, outPath string) (string, bool) {
	if _, err := os.Stat(outPath); err != nil {
		return "", false
	}

	if name, ok := packageClause(outPath); ok {
		return name, true
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if name, ok := packageClause(filepath.Join(dir, e.Name())); ok {
			return name, true
		}
	}
	return "", false
}

func packageClause(path string) (string, bool) {
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
	if err != nil {
		return "", false
	}
	return f.Name.Name, true
}
