package trails

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewHTTPErrorErrorAndStatusCode(t *testing.T) {
	underlying := errors.New("boom")
	httpErr := NewHTTPError(http.StatusTeapot, underlying)

	require.Equal(t, "boom", httpErr.Error())
	require.Equal(t, http.StatusTeapot, httpErr.StatusCode())
}

func TestNewHTTPErrorUnwrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	httpErr := NewHTTPError(http.StatusBadRequest, sentinel)

	require.True(t, errors.Is(httpErr, sentinel))
	require.Equal(t, sentinel, errors.Unwrap(httpErr))
}

func TestDefaultErrorHandler(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w, nil, nil)

	defaultErrorHandler(c, NewHTTPError(http.StatusBadGateway, errors.New("boom")))

	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Equal(t, "boom\n", w.Body.String())
}
