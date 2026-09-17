package main

import (
	"github.com/asimmons91/trails/internal/cli"
)

var version = "dev"

func main() {
	cli.Execute(version)
}
