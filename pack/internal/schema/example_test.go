package schema_test

import (
	"fmt"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/schema"
)

type post struct {
	ID    int64  `db:"id,pk,auto_increment"`
	Title string `db:"title"`
}

func (post) TableName() string { return "posts" }

type author struct {
	ID    int64   `db:"id,pk,auto_increment"`
	Name  string  `db:"name"`
	Posts []*post `db:"rel:has_many"`
}

func (author) TableName() string { return "authors" }

// Example shows how a db struct tag maps a Go field to a Table's Field or
// Relation: schema.For reflects author into a Table naming its columns and
// relations, deriving what a bare tag leaves implicit — the column name
// from the Go field name, and the has_many relation's FK from the owning
// struct's name.
func Example() {
	tbl, err := schema.For(reflect.TypeFor[author]())
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("table:", tbl.Name)
	fmt.Println("column:", tbl.Fields[1].Column)
	fmt.Println("relation kind:", tbl.Relations[0].Kind)
	fmt.Println("relation fk:", tbl.Relations[0].FK)
	// Output:
	// table: authors
	// column: name
	// relation kind: has_many
	// relation fk: author_id
}
