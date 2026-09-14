package schema

import (
	"reflect"
	"testing"
)

type benchUser struct {
	fakeModel[int64]
	Email     string `db:"email"`
	Age       int    `db:"age"`
	CreatedAt string `db:",null_zero"`
}

func (benchUser) TableName() string { return "bench_users" }

// BenchmarkFor_CachedResolution measures .For's steady-state hot path.
// The cache, backed by sync.Map/sync.OnceValues, is what every
// query actually pays per call once a type has been resolved once at
// package init — a cold-build benchmark would measure a path no
// request-serving code ever takes twice.
func BenchmarkFor_CachedResolution(b *testing.B) {
	t := reflect.TypeFor[benchUser]()
	if _, err := For(t); err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		if _, err := For(t); err != nil {
			b.Fatal(err)
		}
	}
}
