package trails

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"reflect"
	"strconv"
)

// MaxBodyBytes caps the size of JSON/XML request bodies BindBody will read.
// It defaults to 4MB. A body over the limit fails with an HTTPError (413
// Payload Too Large) instead of being read in full. It has no effect on
// form or multipart bodies — multipart/form-data uses its own fixed 32MB
// limit (net/http's ParseMultipartForm) instead.
var MaxBodyBytes int64 = 4 << 20

// Binder populates target from a request. DefaultBinder is used unless
// TrailOptions.Binder overrides it.
type Binder interface {
	Bind(c *Context, target any) error
}

// Unmarshaler lets a type customize how the path/query/form binding path
// (not JSON/XML) parses a single string value into it, consulted before
// falling back to the built-in scalar kinds (string/bool/int*/uint*/
// float*).
type Unmarshaler interface {
	UnmarshalParam(value string) error
}

type bindLookupFunc func(name string) ([]string, bool)

// DefaultBinder is the Binder trails uses unless TrailOptions.Binder
// overrides it.
type DefaultBinder struct{}

// Bind populates target from, in order, path params (BindPathParams),
// query params (BindQueryParams), and — only if the request has a body —
// the request body (BindBody). Each stage stops and returns immediately
// on error, so a later stage never overwrites a field an earlier stage
// already failed on; but if a field is tagged for more than one stage
// (e.g. both param and query), a later stage's value does silently
// overwrite an earlier one's on success.
func (d *DefaultBinder) Bind(c *Context, target any) error {
	if err := BindPathParams(c, target); err != nil {
		return err
	}

	if err := BindQueryParams(c, target); err != nil {
		return err
	}

	if c.Request().Body == nil || c.Request().ContentLength == 0 {
		return nil
	}

	return BindBody(c, target)
}

// BindPathParams sets each field of dst tagged `param:"name"` from
// name's path value (see net/http's ServeMux path patterns). A field
// whose path value is empty or absent is left unset.
func BindPathParams(c *Context, dst any) error {
	return bindTagSource(dst, "param", func(name string) ([]string, bool) {
		v := c.Request().PathValue(name)
		if v == "" {
			return nil, false
		}
		return []string{v}, true
	})
}

// BindQueryParams sets each field of dst tagged `query:"name"` from
// name's query parameter(s). A slice field collects every value for a
// repeated query key; a scalar field takes the first. A field whose
// query key is absent is left unset.
func BindQueryParams(c *Context, dst any) error {
	values := c.Request().URL.Query()
	return bindTagSource(dst, "query", func(name string) ([]string, bool) {
		v, ok := values[name]
		return v, ok && len(v) > 0
	})
}

// BindBody decodes the request body into dst according to its
// Content-Type: application/json and application/xml/text/xml are
// decoded directly via encoding/json/encoding/xml against dst's own
// json/xml tags (so nested structs and slices are fully supported), each
// capped at MaxBodyBytes; application/x-www-form-urlencoded and
// multipart/form-data are instead parsed into dst's `form:"name"`-tagged
// fields the same reflection-based way BindQueryParams uses — which only
// supports top-level scalar and slice-of-scalar fields (or a field
// implementing Unmarshaler), not nested structs. Any other Content-Type
// is an error.
func BindBody(c *Context, dst any) error {
	ctype := c.Request().Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(ctype)
	if err != nil {
		return fmt.Errorf("bind: invalid Content-Type %q: %w", ctype, err)
	}

	switch mediaType {
	case "application/json":
		body := http.MaxBytesReader(c.Response(), c.Request().Body, MaxBodyBytes)
		dec := json.NewDecoder(body)
		if err := dec.Decode(dst); err != nil {
			return bodyDecodeError("JSON", err)
		}

	case "application/xml", "text/xml":
		body := http.MaxBytesReader(c.Response(), c.Request().Body, MaxBodyBytes)
		dec := xml.NewDecoder(body)
		if err := dec.Decode(dst); err != nil {
			return bodyDecodeError("XML", err)
		}

	case "application/x-www-form-urlencoded":
		if err := c.Request().ParseForm(); err != nil {
			return fmt.Errorf("bind: parsing form: %w", err)
		}
		return bindTagSource(dst, "form", func(name string) ([]string, bool) {
			v, ok := c.Request().Form[name]
			return v, ok && len(v) > 0
		})

	case "multipart/form-data":
		if err := c.Request().ParseMultipartForm(32 << 20); err != nil {
			return fmt.Errorf("bind: parsing multipart form: %w", err)
		}
		return bindTagSource(dst, "form", func(name string) ([]string, bool) {
			v, ok := c.Request().MultipartForm.Value[name]
			return v, ok && len(v) > 0
		})
	default:
		return fmt.Errorf("bind: unsupported Content-Type %q", mediaType)
	}

	return nil
}

func bodyDecodeError(kind string, err error) error {
	var mbErr *http.MaxBytesError
	if errors.As(err, &mbErr) {
		return NewHTTPError(http.StatusRequestEntityTooLarge,
			fmt.Errorf("bind: %s body exceeds %d byte limit: %w", kind, MaxBodyBytes, err))
	}
	return fmt.Errorf("bind: decoding %s body: %w", kind, err)
}

func bindTagSource(dst any, tag string, lookup bindLookupFunc) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("bind: destination must be a non-nil pointer to a struct")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("bind: destination must point to a struct")
	}

	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}

		name, ok := sf.Tag.Lookup(tag)
		if !ok || name == "" || name == "-" {
			continue
		}

		values, found := lookup(name)
		if !found {
			continue
		}
		if err := setField(rv.Field(i), values); err != nil {
			return fmt.Errorf("bind: field %q (tag %s=%q): %w", sf.Name, tag, name, err)
		}
	}
	return nil
}

func setField(field reflect.Value, values []string) error {
	if !field.CanSet() || len(values) == 0 {
		return nil
	}

	if field.Kind() == reflect.Slice {
		out := reflect.MakeSlice(field.Type(), len(values), len(values))
		for i, v := range values {
			if err := setScalar(out.Index(i), v); err != nil {
				return err
			}
		}
		field.Set(out)
		return nil
	}

	return setScalar(field, values[0])
}

func setScalar(field reflect.Value, value string) error {
	if field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}

		return setScalar(field.Elem(), value)
	}

	if field.CanAddr() {
		if u, ok := field.Addr().Interface().(Unmarshaler); ok {
			return u.UnmarshalParam(value)
		}
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool %q: %w", value, err)
		}
		field.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("invalid integer %q: %w", value, err)
		}
		field.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("invalid unsigned integer %q: %w", value, err)
		}
		field.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(value, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("invalid float %q: %w", value, err)
		}
		field.SetFloat(n)
	default:
		return fmt.Errorf("unsupported field kind %s (implement trails.Unmarshaler for custom types)", field.Kind())
	}

	return nil
}
