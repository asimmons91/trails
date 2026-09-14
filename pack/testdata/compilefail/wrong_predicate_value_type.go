package main

import "github.com/asimmons91/trails/pack"

type user struct {
	Age int `db:"age"`
}

func (user) TableName() string { return "users" }

var ageCol = pack.Field(func(u *user) *int { return &u.Age })

func main() {
	_ = ageCol.Eq("eighteen")
}
