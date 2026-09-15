package cli

import (
	"os"

	"github.com/alecthomas/kong"
)

type CLI struct {
	Migrate MigrateCmd `cmd:"" help:"Manage DB migrations."`
}

func Execute() {
	var cli CLI
	k, err := kong.New(&cli,
		kong.Name("trails"),
		kong.Description("Trails CLI: a fast framework with opinions"))
	if err != nil {
		panic(err)
	}

	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"--help"}
	}

	kctx, err := k.Parse(args)
	kctx.FatalIfErrorf(err)

	err = kctx.Run()
	k.FatalIfErrorf(err)
}
