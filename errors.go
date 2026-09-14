package trails

import "net/http"

type ErrorHandlerFunc func(c *Context, err HTTPError)

type HTTPError interface {
	error
	StatusCode() int
}

type statusError struct {
	code int
	err  error
}

func (s *statusError) Error() string   { return s.err.Error() }
func (s *statusError) StatusCode() int { return s.code }
func (s *statusError) Unwrap() error   { return s.err }

func NewHTTPError(code int, err error) HTTPError {
	return &statusError{code, err}
}

func defaultErrorHandler(c *Context, err HTTPError) {
	http.Error(c.Response(), err.Error(), err.StatusCode())
}
