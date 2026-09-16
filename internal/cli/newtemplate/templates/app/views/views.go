package views

import "embed"

//go:embed all:*/*.gohtml
var ViewsFS embed.FS
