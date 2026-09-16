package fieldsgen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Generate analyzes opts.Dir for pack models and writes opts.OutFile
// (inside opts.Dir) with generated Col/Rel field-and-relation helpers for
// each one.
func Generate(opts Options) (Result, error) {
	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return Result{}, fmt.Errorf("resolving %s: %w", opts.Dir, err)
	}

	ld, err := load(absDir, opts.OutFile)
	if err != nil {
		return Result{}, err
	}

	discovered := discoverModels(ld.pkg)
	if len(discovered) == 0 {
		return Result{}, fmt.Errorf(
			"generate fields: no pack models found in %s (looked for types embedding pack.Model[...] or implementing TableName() string)",
			opts.Dir,
		)
	}

	w := &walker{aux: ld.aux}

	var (
		infos []ModelInfo
		errs  []error
	)
	for _, m := range discovered {
		res := w.flatten(m.Name, m.Struct)
		errs = append(errs, res.Errs...)
		infos = append(infos, ModelInfo{
			Name:    m.Name,
			Cols:    res.Cols,
			Rels:    res.Rels,
			Skipped: res.Skipped,
		})
	}
	if len(errs) > 0 {
		return Result{}, fmt.Errorf("generate fields: %s: %w", opts.Dir, errors.Join(errs...))
	}

	src, err := render(ld.pkg.Types.Name(), ld.pkg.Types, infos)
	if err != nil {
		return Result{}, fmt.Errorf("generate fields: %w", err)
	}

	outPath := filepath.Join(absDir, opts.OutFile)
	if err := os.WriteFile(outPath, src, 0o644); err != nil {
		return Result{}, fmt.Errorf("generate fields: writing %s: %w", outPath, err)
	}

	return Result{ModelCount: len(infos), OutPath: outPath}, nil
}
