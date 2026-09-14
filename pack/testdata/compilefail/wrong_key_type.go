package main

import (
	"context"

	"github.com/asimmons91/trails/pack"
)

type user struct {
	pack.Model[int64] `db:"table:users"`
}

func main() {
	_, _ = pack.ByID[user](context.Background(), nil, "abc")
}
