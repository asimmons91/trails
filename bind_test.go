package trails

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func newBindContext(method, target string, body *bytes.Buffer, contentType string) *Context {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, target, body)
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	return newContext(w, r, nil)
}

type bindPathTarget struct {
	ID string `param:"id"`
}

type bindQueryTarget struct {
	Name string `query:"name"`
}

type bindJSONTarget struct {
	Name string `json:"name"`
}

func TestDefaultBinderBindReturnsPathParamError(t *testing.T) {
	c := newBindContext(http.MethodGet, "/", nil, "")
	err := (&DefaultBinder{}).Bind(c, "not-a-pointer")
	require.Error(t, err)
}

func TestDefaultBinderBindReturnsQueryParamError(t *testing.T) {
	type target struct {
		Age int `query:"age"`
	}
	c := newBindContext(http.MethodGet, "/?age=not-a-number", nil, "")

	err := (&DefaultBinder{}).Bind(c, &target{})
	require.Error(t, err)
}

func TestDefaultBinderBindSkipsBodyWhenNilOrEmpty(t *testing.T) {
	c := newBindContext(http.MethodGet, "/", nil, "application/json")
	c.Request().Body = nil
	c.Request().ContentLength = 0

	target := &bindJSONTarget{}
	err := (&DefaultBinder{}).Bind(c, target)
	require.NoError(t, err)
	require.Equal(t, "", target.Name)
}

func TestDefaultBinderBindBindsPathQueryAndBody(t *testing.T) {
	type target struct {
		ID   string `param:"id"`
		Name string `query:"name"`
		Age  int    `json:"age"`
	}

	body := bytes.NewBufferString(`{"age":42}`)
	c := newBindContext(http.MethodPost, "/?name=bob", body, "application/json")
	c.Request().SetPathValue("id", "7")
	c.Request().ContentLength = int64(body.Len())

	dst := &target{}
	err := (&DefaultBinder{}).Bind(c, dst)
	require.NoError(t, err)
	require.Equal(t, "7", dst.ID)
	require.Equal(t, "bob", dst.Name)
	require.Equal(t, 42, dst.Age)
}

func TestBindPathParamsBindsFoundValue(t *testing.T) {
	c := newBindContext(http.MethodGet, "/", nil, "")
	c.Request().SetPathValue("id", "42")

	dst := &bindPathTarget{}
	err := BindPathParams(c, dst)
	require.NoError(t, err)
	require.Equal(t, "42", dst.ID)
}

func TestBindPathParamsSkipsMissingValue(t *testing.T) {
	c := newBindContext(http.MethodGet, "/", nil, "")

	dst := &bindPathTarget{}
	err := BindPathParams(c, dst)
	require.NoError(t, err)
	require.Equal(t, "", dst.ID)
}

func TestBindQueryParamsBindsPresentValue(t *testing.T) {
	c := newBindContext(http.MethodGet, "/?name=alice", nil, "")

	dst := &bindQueryTarget{}
	err := BindQueryParams(c, dst)
	require.NoError(t, err)
	require.Equal(t, "alice", dst.Name)
}

func TestBindQueryParamsSkipsAbsentKey(t *testing.T) {
	c := newBindContext(http.MethodGet, "/", nil, "")

	dst := &bindQueryTarget{}
	err := BindQueryParams(c, dst)
	require.NoError(t, err)
	require.Equal(t, "", dst.Name)
}

func TestBindQueryParamsSkipsEmptyValue(t *testing.T) {
	c := newBindContext(http.MethodGet, "/?name=", nil, "")

	dst := &bindQueryTarget{}
	err := BindQueryParams(c, dst)
	require.NoError(t, err)
	require.Equal(t, "", dst.Name)
}

func TestBindBodyInvalidContentType(t *testing.T) {
	body := bytes.NewBufferString(`{}`)
	c := newBindContext(http.MethodPost, "/", body, ";;;not-valid;;;")

	err := BindBody(c, &bindJSONTarget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid Content-Type")
}

func TestBindBodyJSONSuccess(t *testing.T) {
	body := bytes.NewBufferString(`{"name":"bob"}`)
	c := newBindContext(http.MethodPost, "/", body, "application/json")

	dst := &bindJSONTarget{}
	err := BindBody(c, dst)
	require.NoError(t, err)
	require.Equal(t, "bob", dst.Name)
}

func TestBindBodyJSONDecodeError(t *testing.T) {
	body := bytes.NewBufferString(`{not-json`)
	c := newBindContext(http.MethodPost, "/", body, "application/json")

	err := BindBody(c, &bindJSONTarget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "decoding JSON body")
}

type bindXMLTarget struct {
	Name string `xml:"name"`
}

func TestBindBodyXMLSuccess(t *testing.T) {
	body := bytes.NewBufferString(`<bindXMLTarget><name>bob</name></bindXMLTarget>`)
	c := newBindContext(http.MethodPost, "/", body, "application/xml")

	dst := &bindXMLTarget{}
	err := BindBody(c, dst)
	require.NoError(t, err)
	require.Equal(t, "bob", dst.Name)
}

func TestBindBodyTextXMLSuccess(t *testing.T) {
	body := bytes.NewBufferString(`<bindXMLTarget><name>bob</name></bindXMLTarget>`)
	c := newBindContext(http.MethodPost, "/", body, "text/xml")

	dst := &bindXMLTarget{}
	err := BindBody(c, dst)
	require.NoError(t, err)
	require.Equal(t, "bob", dst.Name)
}

func TestBindBodyXMLDecodeError(t *testing.T) {
	body := bytes.NewBufferString(`<not-closed>`)
	c := newBindContext(http.MethodPost, "/", body, "application/xml")

	err := BindBody(c, &bindXMLTarget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "decoding XML body")
}

func withMaxBodyBytes(t *testing.T, limit int64) {
	t.Helper()
	original := MaxBodyBytes
	MaxBodyBytes = limit
	t.Cleanup(func() { MaxBodyBytes = original })
}

func TestBindBodyJSONExceedsMaxBodyBytes(t *testing.T) {
	withMaxBodyBytes(t, 10)
	body := bytes.NewBufferString(`{"name":"a-name-longer-than-ten-bytes"}`)
	c := newBindContext(http.MethodPost, "/", body, "application/json")

	err := BindBody(c, &bindJSONTarget{})
	require.Error(t, err)

	var httpErr HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusRequestEntityTooLarge, httpErr.StatusCode())
}

func TestBindBodyXMLExceedsMaxBodyBytes(t *testing.T) {
	withMaxBodyBytes(t, 10)
	body := bytes.NewBufferString(`<bindXMLTarget><name>a-name-longer-than-ten-bytes</name></bindXMLTarget>`)
	c := newBindContext(http.MethodPost, "/", body, "application/xml")

	err := BindBody(c, &bindXMLTarget{})
	require.Error(t, err)

	var httpErr HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusRequestEntityTooLarge, httpErr.StatusCode())
}

func TestBindBodyFormSuccess(t *testing.T) {
	form := url.Values{}
	form.Set("name", "bob")
	body := bytes.NewBufferString(form.Encode())
	c := newBindContext(http.MethodPost, "/", body, "application/x-www-form-urlencoded")

	type formTarget struct {
		Name string `form:"name"`
	}
	dst := &formTarget{}
	err := BindBody(c, dst)
	require.NoError(t, err)
	require.Equal(t, "bob", dst.Name)
}

func TestBindBodyFormParseError(t *testing.T) {
	body := bytes.NewBufferString("name=%")
	c := newBindContext(http.MethodPost, "/", body, "application/x-www-form-urlencoded")

	err := BindBody(c, &bindJSONTarget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing form")
}

func multipartBody(t *testing.T, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	for k, v := range fields {
		require.NoError(t, w.WriteField(k, v))
	}
	require.NoError(t, w.Close())
	return body, w.FormDataContentType()
}

func TestBindBodyMultipartSuccess(t *testing.T) {
	body, contentType := multipartBody(t, map[string]string{"name": "bob"})
	c := newBindContext(http.MethodPost, "/", body, contentType)

	type formTarget struct {
		Name string `form:"name"`
	}
	dst := &formTarget{}
	err := BindBody(c, dst)
	require.NoError(t, err)
	require.Equal(t, "bob", dst.Name)
}

func TestBindBodyMultipartParseError(t *testing.T) {
	body := bytes.NewBufferString("not-a-multipart-body")
	c := newBindContext(http.MethodPost, "/", body, "multipart/form-data; boundary=missing")

	err := BindBody(c, &bindJSONTarget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing multipart form")
}

func TestBindBodyUnsupportedContentType(t *testing.T) {
	body := bytes.NewBufferString("plain text")
	c := newBindContext(http.MethodPost, "/", body, "text/plain")

	err := BindBody(c, &bindJSONTarget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported Content-Type")
}

func TestBindTagSourceRejectsNonPointer(t *testing.T) {
	err := BindPathParams(newBindContext(http.MethodGet, "/", nil, ""), bindPathTarget{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-nil pointer")
}

func TestBindTagSourceRejectsNilPointer(t *testing.T) {
	var dst *bindPathTarget
	err := BindPathParams(newBindContext(http.MethodGet, "/", nil, ""), dst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-nil pointer")
}

func TestBindTagSourceRejectsPointerToNonStruct(t *testing.T) {
	s := "x"
	err := BindPathParams(newBindContext(http.MethodGet, "/", nil, ""), &s)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must point to a struct")
}

func TestBindTagSourceSkipsUnexportedField(t *testing.T) {
	type target struct {
		unexported string `param:"id"`
	}
	c := newBindContext(http.MethodGet, "/", nil, "")
	c.Request().SetPathValue("id", "42")

	dst := &target{}
	err := BindPathParams(c, dst)
	require.NoError(t, err)
	require.Equal(t, "", dst.unexported)
}

func TestBindTagSourceSkipsFieldsWithoutUsableTag(t *testing.T) {
	type target struct {
		NoTag    string
		EmptyTag string `param:""`
		DashTag  string `param:"-"`
	}
	c := newBindContext(http.MethodGet, "/", nil, "")
	c.Request().SetPathValue("NoTag", "a")
	c.Request().SetPathValue("EmptyTag", "b")
	c.Request().SetPathValue("DashTag", "c")

	dst := &target{}
	err := BindPathParams(c, dst)
	require.NoError(t, err)
	require.Equal(t, "", dst.NoTag)
	require.Equal(t, "", dst.EmptyTag)
	require.Equal(t, "", dst.DashTag)
}

func TestBindTagSourceWrapsSetFieldError(t *testing.T) {
	type target struct {
		Age int `param:"age"`
	}
	c := newBindContext(http.MethodGet, "/", nil, "")
	c.Request().SetPathValue("age", "not-a-number")

	dst := &target{}
	err := BindPathParams(c, dst)
	require.Error(t, err)
	require.Contains(t, err.Error(), `field "Age"`)
	require.Contains(t, err.Error(), `tag param="age"`)
}

func TestSetFieldBindsSliceValues(t *testing.T) {
	type target struct {
		Tags []string `query:"tag"`
	}
	c := newBindContext(http.MethodGet, "/?tag=a&tag=b&tag=c", nil, "")

	dst := &target{}
	err := BindQueryParams(c, dst)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, dst.Tags)
}

func TestSetFieldSliceStopsOnElementError(t *testing.T) {
	type target struct {
		Nums []int `query:"n"`
	}
	c := newBindContext(http.MethodGet, "/?n=1&n=bad&n=3", nil, "")

	dst := &target{}
	err := BindQueryParams(c, dst)
	require.Error(t, err)
}

func TestSetScalarAllocatesNilPointer(t *testing.T) {
	type target struct {
		Name *string `query:"name"`
	}
	c := newBindContext(http.MethodGet, "/?name=bob", nil, "")

	dst := &target{}
	err := BindQueryParams(c, dst)
	require.NoError(t, err)
	require.NotNil(t, dst.Name)
	require.Equal(t, "bob", *dst.Name)
}

type customUnmarshaler struct {
	value string
}

func (c *customUnmarshaler) UnmarshalParam(value string) error {
	c.value = "custom:" + value
	return nil
}

func TestSetScalarUsesUnmarshalerWhenImplemented(t *testing.T) {
	type target struct {
		Custom customUnmarshaler `query:"custom"`
	}
	c := newBindContext(http.MethodGet, "/?custom=hi", nil, "")

	dst := &target{}
	err := BindQueryParams(c, dst)
	require.NoError(t, err)
	require.Equal(t, "custom:hi", dst.Custom.value)
}

func TestSetScalarScalarKinds(t *testing.T) {
	type target struct {
		Str   string  `query:"str"`
		Bool  bool    `query:"bool"`
		Int   int     `query:"int"`
		Uint  uint    `query:"uint"`
		Float float64 `query:"float"`
	}

	c := newBindContext(http.MethodGet, "/?str=hi&bool=true&int=-5&uint=5&float=1.5", nil, "")
	dst := &target{}
	err := BindQueryParams(c, dst)
	require.NoError(t, err)
	require.Equal(t, "hi", dst.Str)
	require.Equal(t, true, dst.Bool)
	require.Equal(t, -5, dst.Int)
	require.Equal(t, uint(5), dst.Uint)
	require.Equal(t, 1.5, dst.Float)
}

func TestSetScalarParseErrors(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"bool", "bool=not-a-bool"},
		{"int", "int=not-an-int"},
		{"uint", "uint=not-a-uint"},
		{"float", "float=not-a-float"},
	}

	type target struct {
		Bool  bool    `query:"bool"`
		Int   int     `query:"int"`
		Uint  uint    `query:"uint"`
		Float float64 `query:"float"`
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newBindContext(http.MethodGet, "/?"+tc.query, nil, "")
			dst := &target{}
			err := BindQueryParams(c, dst)
			require.Error(t, err)
		})
	}
}

func TestSetScalarUnsupportedKind(t *testing.T) {
	type target struct {
		Nested struct{ A string } `query:"nested"`
	}
	c := newBindContext(http.MethodGet, "/?nested=x", nil, "")

	dst := &target{}
	err := BindQueryParams(c, dst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported field kind")
	require.Contains(t, err.Error(), "trails.Unmarshaler")
}

func TestSetFieldNoOpWhenNotSettable(t *testing.T) {
	// A value obtained via reflect.ValueOf on a non-pointer is never settable;
	// this branch can't be reached through the exported bind functions since
	// bindTagSource only ever passes addressable struct fields.
	rv := reflect.ValueOf("unsettable")
	err := setField(rv, []string{"x"})
	require.NoError(t, err)
}

func TestSetFieldNoOpWhenNoValues(t *testing.T) {
	var s string
	rv := reflect.ValueOf(&s).Elem()
	err := setField(rv, nil)
	require.NoError(t, err)
	require.Equal(t, "", s)
}
