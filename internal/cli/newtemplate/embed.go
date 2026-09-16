// Package newtemplate holds the embedded starter-app template tree used by
// `trails new`.
package newtemplate

import "embed"

//go:embed all:templates
var FS embed.FS
