package sqlbuild_test

import (
	"fmt"

	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

// Example builds a multi-clause SELECT — a join, a WHERE, an ORDER BY, and
// a LIMIT — showing how SelectBuilder's chain methods compose and how
// Render turns the result into a dialect-specific SQL string plus its
// bind arguments in placeholder order.
func Example() {
	users := sqlbuild.Table{Name: "users", Alias: "u"}
	posts := sqlbuild.Table{Name: "posts", Alias: "p"}

	q := sqlbuild.Select(users).
		Columns(sqlbuild.QualifiedCol("u", "id"), sqlbuild.QualifiedCol("u", "email")).
		Join(posts, sqlbuild.EqCol(sqlbuild.QualifiedCol("p", "user_id"), sqlbuild.QualifiedCol("u", "id"))).
		Where(sqlbuild.Eq(sqlbuild.QualifiedCol("u", "active"), true)).
		OrderBy(sqlbuild.Desc(sqlbuild.QualifiedCol("u", "id"))).
		Limit(10)

	sql, args, err := q.Render(pgdialect.New())
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// SELECT "u"."id", "u"."email" FROM "users" AS "u" JOIN "posts" AS "p" ON "p"."user_id" = "u"."id" WHERE "u"."active" = $1 ORDER BY "u"."id" DESC LIMIT 10
	// [true]
}
