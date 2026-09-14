package main

import (
	"context"

	"github.com/asimmons91/trails/pack"
)

type summary struct {
	Total int `db:"total"`
}

func (summary) TableName() string { return "summaries" }

func main() {
	_, _ = pack.ByID[summary](context.Background(), nil, 1)
}
