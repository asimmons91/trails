package trails

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterParamsRedactsSensitiveKeys(t *testing.T) {
	values := url.Values{
		"password":      {"hunter2"},
		"user_password": {"hunter3"},
		"token":         {"abc123"},
		"name":          {"bob"},
	}

	out := FilterParams(values)

	require.Equal(t, []string{"[FILTERED]"}, out["password"])
	require.Equal(t, []string{"[FILTERED]"}, out["user_password"])
	require.Equal(t, []string{"[FILTERED]"}, out["token"])
	require.Equal(t, []string{"bob"}, out["name"])
}

func TestFilterParamsCaseInsensitive(t *testing.T) {
	values := url.Values{"PASSWORD": {"hunter2"}}

	out := FilterParams(values)

	require.Equal(t, []string{"[FILTERED]"}, out["PASSWORD"])
}

func TestFilterParamsDoesNotMutateInput(t *testing.T) {
	values := url.Values{"password": {"hunter2"}}

	FilterParams(values)

	require.Equal(t, []string{"hunter2"}, values["password"])
}

func TestFilterHeaderRedactsSensitiveHeaders(t *testing.T) {
	header := http.Header{}
	header.Set("Authorization", "Bearer abc123")
	header.Set("Cookie", "session=abc123")
	header.Set("Set-Cookie", "session=abc123")
	header.Set("Content-Type", "application/json")

	out := FilterHeader(header)

	require.Equal(t, []string{"[FILTERED]"}, out["Authorization"])
	require.Equal(t, []string{"[FILTERED]"}, out["Cookie"])
	require.Equal(t, []string{"[FILTERED]"}, out["Set-Cookie"])
	require.Equal(t, []string{"application/json"}, out["Content-Type"])
}

func TestFilterHeaderDoesNotMutateInput(t *testing.T) {
	header := http.Header{}
	header.Set("Authorization", "Bearer abc123")

	FilterHeader(header)

	require.Equal(t, "Bearer abc123", header.Get("Authorization"))
}

func TestFilterParamsPreventsSecretsInLogOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	values := url.Values{"password": {"hunter2"}, "name": {"bob"}}
	logger.Info("request", "params", FilterParams(values))

	out := buf.String()
	require.NotContains(t, out, "hunter2")
	require.Contains(t, out, "bob")
	require.Contains(t, out, "[FILTERED]")
}
