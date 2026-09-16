package cli

import (
	"fmt"
	"log/slog"

	"github.com/asimmons91/trails/pack/fieldsgen"
)

type GenerateCmd struct {
	Fields FieldsCmd `cmd:"" help:"Generate pack Col/Rel field and relation helpers for models."`
}

type FieldsCmd struct {
	Dir string `help:"Directory containing model source files." default:"app/models"`
	Out string `help:"Output filename, written inside --dir." default:"trails_fields_gen.go"`
}

func (f *FieldsCmd) Run() error {
	logger := slog.Default()

	res, err := fieldsgen.Generate(fieldsgen.Options{Dir: f.Dir, OutFile: f.Out})
	if err != nil {
		return fmt.Errorf("generate fields: %w", err)
	}

	logger.Info("generated field/relation helpers", "dir", f.Dir, "out", f.Out, "models", res.ModelCount)
	return nil
}
