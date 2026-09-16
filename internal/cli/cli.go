package cli

import (
	"os"

	"github.com/alecthomas/kong"
)

type CLI struct {
	New         NewCmd         `cmd:"" help:"Generate a new trails application."`
	Migrate     MigrateCmd     `cmd:"" help:"Manage DB migrations."`
	Assets      AssetsCmd      `cmd:"" help:"Build frontend assets."`
	Importmap   ImportmapCmd   `cmd:"" help:"Manage the JS importmap."`
	Credentials CredentialsCmd `cmd:"" help:"Manage encrypted credentials."`
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
	k.FatalIfErrorf(err)

	err = kctx.Run()
	k.FatalIfErrorf(err)
}
