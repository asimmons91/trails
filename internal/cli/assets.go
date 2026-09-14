package cli

import (
	"log/slog"

	"github.com/asimmons91/trails/assets"
)

type AssetsCmd struct {
	Build BuildCmd `cmd:"" help:"Run configured build scripts"`
}

type BuildCmd struct {
	Source  string   `help:"Directory the bundler writes output to." default:"app/frontend/builds"`
	Output  string   `help:"Digested asset output directory." default:"public/assets"`
	Prefix  string   `help:"URL prefix used when rewriting asset references." default:"/assets"`
	Dir     string   `help:"Working directory containing package.json." default:"."`
	Scripts []string `help:"package.json scripts to run before fingerprinting." default:"build"`
}

func (b *BuildCmd) Run() error {
	logger := slog.Default()

	if err := assets.RunBundlerScripts(
		assets.BundlerConfig{Dir: b.Dir, Scripts: b.Scripts}); err != nil {
		return err
	}

	if _, err := assets.Compile(b.Source, b.Output, b.Prefix); err != nil {
		return err
	}

	logger.Info("assets built", "source", b.Source, "output", b.Output)
	return nil
}
