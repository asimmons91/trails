package public

import (
	"embed"
	"io/fs"
)

//go:embed all:assets
var assetsFS embed.FS
var AssetsFS, _ = fs.Sub(assetsFS, "assets")

//go:embed all:static
var staticFS embed.FS
var StaticFS, _ = fs.Sub(staticFS, "static")
