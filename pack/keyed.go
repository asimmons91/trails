package pack

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

func ByID[T Entity[ID], ID comparable](ctx context.Context, db *DB, id ID) (T, error) {
	var zero T
	t := reflect.TypeFor[T]()
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack: ByID[%s]: %v", t.Name(), err))
	}

	p, err := pkPredicate(table, id, table.Alias)
	if err != nil {
		return zero, err
	}

	return Of[T](db).Where(Predicate{p: p}).First(ctx)
}

func Update[T Entity[ID], ID comparable](ctx context.Context, db *DB, row *T) error {
	t := reflect.TypeFor[T]()
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack: Update[%s]: %v", t.Name(), err))
	}

	if table.Hooks.BeforeUpdate {
		if bu, ok := any(row).(BeforeUpdater); ok {
			if err := bu.BeforeUpdate(ctx); err != nil {
				return fmt.Errorf("pack: BeforeUpdate hook on %s: %w", table.GoType.Name(), err)
			}
		}
	}

	rv := reflect.ValueOf(row).Elem()
	assignments, err := writeAllAssignments(table.GoType.Name(), table, rv)
	if err != nil {
		return err
	}

	p, err := pkPredicate(table, (*row).PK(), "")
	if err != nil {
		return err
	}

	sqlText, args, err := sqlbuild.Update(sqlbuild.Table{Name: table.Name}).
		Set(assignments...).
		Where(p).
		Render(db.dialect)
	if err != nil {
		return err
	}

	if _, err := db.execContext(ctx, "Update", table.GoType.Name(), sqlText, args); err != nil {
		return err
	}

	if table.Hooks.AfterUpdate {
		if au, ok := any(row).(AfterUpdater); ok {
			if err := au.AfterUpdate(ctx); err != nil {
				return fmt.Errorf("pack: AfterUpdate hook on %s: %w", table.GoType.Name(), err)
			}
		}
	}

	return nil
}

func Save[T Entity[ID], ID comparable](ctx context.Context, db *DB, row *T) error {
	var zeroID ID
	if (*row).PK() == zeroID {
		return Create[T](ctx, db, row)
	}

	return Update[T, ID](ctx, db, row)
}

func Delete[T Entity[ID], ID comparable](ctx context.Context, db *DB, id ID) error {
	t := reflect.TypeFor[T]()
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack: Delete[%s]: %v", t.Name(), err))
	}

	var row *T
	if table.Hooks.BeforeDelete || table.Hooks.AfterDelete {
		fetched, err := ByID[T, ID](ctx, db, id)
		if err != nil {
			if errors.Is(err, ErrNoRows) {
				return nil
			}
			return err
		}
		row = &fetched
	}

	if row != nil && table.Hooks.BeforeDelete {
		if bd, ok := any(row).(BeforeDeleter); ok {
			if err := bd.BeforeDelete(ctx); err != nil {
				return fmt.Errorf("pack: BeforeDelete hook on %s: %w", table.GoType.Name(), err)
			}
		}
	}

	p, err := pkPredicate(table, id, "")
	if err != nil {
		return err
	}

	sqlText, args, err := sqlbuild.Delete(sqlbuild.Table{Name: table.Name}).Where(p).Render(db.dialect)
	if err != nil {
		return err
	}
	if _, err := db.execContext(ctx, "Delete", table.GoType.Name(), sqlText, args); err != nil {
		return err
	}

	if row != nil && table.Hooks.AfterDelete {
		if ad, ok := any(row).(AfterDeleter); ok {
			if err := ad.AfterDelete(ctx); err != nil {
				return fmt.Errorf("pack: AfterDelete hook on %s: %w", table.GoType.Name(), err)
			}
		}
	}

	return nil
}
