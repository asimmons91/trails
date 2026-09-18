package trails

import (
	"net/http"
	"net/url"
	"strings"
)

// SensitiveParams lists substrings (matched case-insensitively) that mark a
// request parameter as sensitive. FilterParams replaces the value of any
// key containing one of these with "[FILTERED]" — the same
// substring-matching approach Rails' config.filter_parameters uses, so a
// single "password" entry also catches "user_password" and
// "password_confirmation". Extend it in your app's init for any
// app-specific secret-shaped param names.
var SensitiveParams = []string{
	"password", "passwd", "secret", "token", "api_key", "apikey",
	"auth", "credit_card", "card_number", "cvv", "ssn",
}

// SensitiveHeaders lists header names (matched case-insensitively via
// http.CanonicalHeaderKey) whose entire value FilterHeader replaces with
// "[FILTERED]".
var SensitiveHeaders = []string{
	"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie",
}

// FilterParams returns a copy of values with the value of any key matching
// SensitiveParams replaced with "[FILTERED]". values is left untouched —
// call this before logging query/form params to avoid leaking secrets.
func FilterParams(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for k, v := range values {
		if isSensitiveParam(k) {
			out[k] = []string{"[FILTERED]"}
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}

// FilterHeader returns a copy of header with the value of any header listed
// in SensitiveHeaders replaced with "[FILTERED]". header is left untouched
// — call this before logging request/response headers to avoid leaking
// credentials or session cookies.
func FilterHeader(header http.Header) http.Header {
	out := make(http.Header, len(header))
	for k, v := range header {
		if isSensitiveHeader(k) {
			out[k] = []string{"[FILTERED]"}
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}

func isSensitiveParam(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range SensitiveParams {
		if strings.Contains(lower, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

func isSensitiveHeader(name string) bool {
	canon := http.CanonicalHeaderKey(name)
	for _, s := range SensitiveHeaders {
		if http.CanonicalHeaderKey(s) == canon {
			return true
		}
	}
	return false
}
