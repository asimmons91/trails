package sqlbuild

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

type renderer struct {
	d    dialect.Dialect
	args []any
}

func newRenderer(d dialect.Dialect) *renderer {
	return &renderer{d: d, args: []any{}}
}

func (r *renderer) bind(v any) string {
	r.args = append(r.args, v)
	return r.d.Placeholder(len(r.args))
}

func (r *renderer) quoteColumn(c Column) string {
	if c.Table != "" {
		return r.d.QuoteIdent(c.Table) + "." + r.d.QuoteIdent(c.Name)
	}

	return r.d.QuoteIdent(c.Name)
}

func (r *renderer) quoteTable(t Table) string {
	if t.Alias != "" {
		return r.d.QuoteIdent(t.Name) + " AS " + r.d.QuoteIdent(t.Alias)
	}

	return r.d.QuoteIdent(t.Name)
}

var rawPlaceholderRE = regexp.MustCompile(`\$([0-9]+)`)

// renderRawFragment rewrites fragment's own "$1", "$2", ... placeholders
// (always this syntax, regardless of dialect) into the dialect's actual
// placeholder syntax and position — offset by whatever's already been
// bound via r.bind — then appends rawArgs to r.args in order. It errors
// with ErrRawPlaceholderOutOfRange if fragment references a placeholder
// index with no matching rawArgs entry.
func (r *renderer) renderRawFragment(fragment string, rawArgs []any) (string, error) {
	base := len(r.args)

	var outerErr error
	out := rawPlaceholderRE.ReplaceAllStringFunc(fragment, func(match string) string {
		if outerErr != nil {
			return match
		}

		k, err := strconv.Atoi(match[1:])
		if err != nil || k < 1 || k > len(rawArgs) {
			outerErr = &ErrRawPlaceholderOutOfRange{Fragment: fragment, Index: k, NumArgs: len(rawArgs)}
			return match
		}
		return r.d.Placeholder(base + k)
	})
	if outerErr != nil {
		return "", outerErr
	}

	r.args = append(r.args, rawArgs...)
	return out, nil
}

func (r *renderer) renderPredicate(p Predicate) (string, error) {
	switch p.kind {
	case predNone:
		return "TRUE", nil
	case predEq:
		return r.quoteColumn(p.col) + " = " + r.bind(p.args[0]), nil
	case predNe:
		return r.quoteColumn(p.col) + " <> " + r.bind(p.args[0]), nil
	case predGt:
		return r.quoteColumn(p.col) + " > " + r.bind(p.args[0]), nil
	case predGte:
		return r.quoteColumn(p.col) + " >= " + r.bind(p.args[0]), nil
	case predLt:
		return r.quoteColumn(p.col) + " < " + r.bind(p.args[0]), nil
	case predLte:
		return r.quoteColumn(p.col) + " <= " + r.bind(p.args[0]), nil
	case predIn:
		if len(p.args) == 0 {
			return "FALSE", nil
		}
		return r.quoteColumn(p.col) + " IN (" + r.bindList(p.args) + ")", nil
	case predNotIn:
		if len(p.args) == 0 {
			return "TRUE", nil
		}
		return r.quoteColumn(p.col) + " NOT IN (" + r.bindList(p.args) + ")", nil
	case predIsNull:
		return r.quoteColumn(p.col) + " IS NULL", nil
	case predIsNotNull:
		return r.quoteColumn(p.col) + " IS NOT NULL", nil
	case predBetween:
		return r.quoteColumn(p.col) + " BETWEEN " + r.bind(p.args[0]) + " AND " + r.bind(p.args[1]), nil
	case predLike:
		return r.quoteColumn(p.col) + " LIKE " + r.bind(p.args[0]), nil
	case predILike:
		if !r.d.SupportsILike() {
			return "", &ErrILikeUnsupportedByDialect{Dialect: r.d.Name()}
		}
		return r.quoteColumn(p.col) + " ILIKE " + r.bind(p.args[0]), nil
	case predNotLike:
		return r.quoteColumn(p.col) + " NOT LIKE " + r.bind(p.args[0]), nil
	case predAnd:
		if len(p.children) == 0 {
			return "TRUE", nil
		}
		parts := make([]string, len(p.children))
		for i, c := range p.children {
			s, err := r.renderChild(c)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return strings.Join(parts, " AND "), nil
	case predOr:
		if len(p.children) == 0 {
			return "FALSE", nil
		}
		parts := make([]string, len(p.children))
		for i, c := range p.children {
			s, err := r.renderChild(c)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return strings.Join(parts, " OR "), nil
	case predNot:
		s, err := r.renderChild(p.children[0])
		if err != nil {
			return "", err
		}
		return "NOT " + s, nil
	case predRaw:
		return r.renderRawFragment(p.rawSQL, p.args)
	case predEqCol:
		return r.quoteColumn(p.col) + " = " + r.quoteColumn(p.rightCol), nil
	default:
		return "TRUE", nil
	}
}

func (r *renderer) renderChild(p Predicate) (string, error) {
	s, err := r.renderPredicate(p)
	if err != nil {
		return "", err
	}
	if p.kind == predAnd || p.kind == predOr || p.kind == predNot {
		return "(" + s + ")", nil
	}

	return s, nil
}

func (r *renderer) bindList(vs []any) string {
	parts := make([]string, len(vs))

	for i, v := range vs {
		parts[i] = r.bind(v)
	}

	return strings.Join(parts, ", ")
}

func (r *renderer) renderAssignment(a Assignment) (string, error) {
	target := r.d.QuoteIdent(a.Col.Name)
	if a.isRaw {
		expr, err := r.renderRawFragment(a.rawSQL, a.rawArgs)
		if err != nil {
			return "", err
		}
		return target + " = " + expr, nil
	}

	return target + " = " + r.bind(a.val), nil
}

func (r *renderer) renderJoin(j Join) (string, error) {
	var sb strings.Builder
	if j.kind == LeftJoin {
		sb.WriteString(" LEFT JOIN ")
	} else {
		sb.WriteString(" JOIN ")
	}

	sb.WriteString(r.quoteTable(j.table))
	sb.WriteString(" ON ")
	onSQL, err := r.renderPredicate(j.on)
	if err != nil {
		return "", err
	}
	sb.WriteString(onSQL)

	return sb.String(), nil
}

func (r *renderer) renderOrderTerm(o OrderTerm) string {
	if o.raw != "" {
		return o.raw
	}
	if o.desc {
		return r.quoteColumn(o.col) + " DESC"
	}

	return r.quoteColumn(o.col) + " ASC"
}
