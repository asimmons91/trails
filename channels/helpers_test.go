package channels_test

import (
	"bufio"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/channels"
)

// mountBackend builds a minimal, fully working *trails.Trail with b's
// routes mounted at prefix, so handler tests exercise the same
// request/response path (including trails' Bind machinery) a real client
// would, without needing real templates or assets. Mirrors
// cloudtask_test's mountBackend helper.
func mountBackend(t *testing.T, prefix string, b *channels.Backend) http.Handler {
	t.Helper()

	trail, err := trails.New(trails.WithDefaultOptions(&trails.TrailOptions{
		AssetsFS:       fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte("{}")}},
		ConfigFS:       fstest.MapFS{"importmap.toml": &fstest.MapFile{Data: []byte("")}},
		ViewFS:         fstest.MapFS{},
		AssetsStrategy: trails.AssetsStrategyNone,
		RouteBuilder: func(r *trails.Router) {
			r.WithGroup(prefix, b.Routes)
		},
	}))
	require.NoError(t, err)

	return trail
}

// readSSEFrame reads one blank-line-terminated SSE frame off r. isComment
// reports whether the frame was a `:`-prefixed comment line (e.g. a
// heartbeat) rather than a real event/data frame.
func readSSEFrame(t *testing.T, r *bufio.Reader) (event string, data []byte, isComment bool) {
	t.Helper()

	for {
		line, err := r.ReadString('\n')
		require.NoError(t, err, "reading SSE frame")

		switch {
		case line == "\n":
			// Blank line: end of frame. If we only ever saw a comment
			// line, isComment is already true and event/data are empty.
			return event, data, isComment
		case len(line) > 0 && line[0] == ':':
			isComment = true
		case len(line) > len("event: ") && line[:len("event: ")] == "event: ":
			event = line[len("event: ") : len(line)-1]
		case len(line) > len("data: ") && line[:len("data: ")] == "data: ":
			if data != nil {
				data = append(data, '\n')
			}
			data = append(data, []byte(line[len("data: "):len(line)-1])...)
		}
	}
}
