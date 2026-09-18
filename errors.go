package trails

import "net/http"

// ErrorHandlerFunc handles an error returned from a HandlerFunc or
// MiddlewareFunc after the response has not yet been written. The
// default (see WithDefaultOptions) writes err's message and StatusCode
// via http.Error.
type ErrorHandlerFunc func(c *Context, err HTTPError)

// HTTPError is an error with an HTTP status code attached. NewHTTPError
// constructs one; any error a handler returns that isn't already an
// HTTPError (checked via errors.As, so a wrapped HTTPError is still
// found) is converted to one with a 500 status before reaching
// TrailOptions.ErrorHandler.
type HTTPError interface {
	error
	// StatusCode is the HTTP status code to respond with.
	StatusCode() int
}

type statusError struct {
	code int
	err  error
}

func (s *statusError) Error() string   { return s.err.Error() }
func (s *statusError) StatusCode() int { return s.code }
func (s *statusError) Unwrap() error   { return s.err }

// NewHTTPError wraps err as an HTTPError reporting code. Error() and
// Unwrap() delegate to err unchanged — code is carried alongside it, not
// mixed into the message.
func NewHTTPError(code int, err error) HTTPError {
	return &statusError{code, err}
}

func defaultErrorHandler(c *Context, err HTTPError) {
	http.Error(c.Response(), err.Error(), err.StatusCode())
}
