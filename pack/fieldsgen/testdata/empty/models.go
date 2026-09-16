// Package models has no pack models at all — used to verify Generate
// reports a clear error instead of silently writing an empty file.
package models

type NotAModel struct {
	Name string
}
