package channels_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/asimmons91/trails/channels"
	"github.com/asimmons91/trails/channels/backend/memory"
)

// --- command endpoint (POST /cable/command) ---

func doCommand(t *testing.T, mount http.Handler, connID, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/cable/command", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if connID != "" {
		req.Header.Set(channels.HeaderConnectionID, connID)
	}
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)
	return rec
}

func TestHandleCommand_Subscribe_Success(t *testing.T) {
	stub := &stubChannel{}
	reg := channels.NewRegistry()
	reg.Register("stub", func() channels.Channel { return stub })
	hub := channels.NewHub(nopBroadcaster{}, reg)
	conn := hub.Connect()

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, conn.ID(), `{"command":"subscribe","channel":"stub","params":{"room_id":"42"}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, stub.subscribedCalls, 1)
	require.Equal(t, "42", stub.subscribedCalls[0]["room_id"])
}

func TestHandleCommand_Subscribe_UnknownConnection(t *testing.T) {
	reg := channels.NewRegistry()
	reg.Register("stub", func() channels.Channel { return &stubChannel{} })
	hub := channels.NewHub(nopBroadcaster{}, reg)

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, "nope", `{"command":"subscribe","channel":"stub"}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleCommand_Subscribe_UnknownChannel(t *testing.T) {
	hub := channels.NewHub(nopBroadcaster{}, channels.NewRegistry())
	conn := hub.Connect()

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, conn.ID(), `{"command":"subscribe","channel":"missing"}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleCommand_Subscribe_AlreadySubscribed(t *testing.T) {
	reg := channels.NewRegistry()
	reg.Register("stub", func() channels.Channel { return &stubChannel{} })
	hub := channels.NewHub(nopBroadcaster{}, reg)
	conn := hub.Connect()
	require.NoError(t, hub.Subscribe(t.Context(), conn.ID(), "stub", nil))

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, conn.ID(), `{"command":"subscribe","channel":"stub"}`)

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestHandleCommand_Subscribe_ChannelSubscribedError(t *testing.T) {
	stub := &stubChannel{subscribeErr: errors.New("boom")}
	reg := channels.NewRegistry()
	reg.Register("stub", func() channels.Channel { return stub })
	hub := channels.NewHub(nopBroadcaster{}, reg)
	conn := hub.Connect()

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, conn.ID(), `{"command":"subscribe","channel":"stub"}`)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleCommand_Unsubscribe_Success(t *testing.T) {
	stub := &stubChannel{}
	reg := channels.NewRegistry()
	reg.Register("stub", func() channels.Channel { return stub })
	hub := channels.NewHub(nopBroadcaster{}, reg)
	conn := hub.Connect()
	require.NoError(t, hub.Subscribe(t.Context(), conn.ID(), "stub", nil))

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, conn.ID(), `{"command":"unsubscribe","channel":"stub"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, stub.unsubscribedCalls)
}

func TestHandleCommand_Unsubscribe_NotSubscribed(t *testing.T) {
	reg := channels.NewRegistry()
	reg.Register("stub", func() channels.Channel { return &stubChannel{} })
	hub := channels.NewHub(nopBroadcaster{}, reg)
	conn := hub.Connect()

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, conn.ID(), `{"command":"unsubscribe","channel":"stub"}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleCommand_Message_Success(t *testing.T) {
	stub := &stubChannel{}
	reg := channels.NewRegistry()
	reg.Register("stub", func() channels.Channel { return stub })
	hub := channels.NewHub(nopBroadcaster{}, reg)
	conn := hub.Connect()
	require.NoError(t, hub.Subscribe(t.Context(), conn.ID(), "stub", nil))

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, conn.ID(), `{"command":"message","channel":"stub","data":{"body":"hi"}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, stub.receivedCalls, 1)
	require.JSONEq(t, `{"body":"hi"}`, string(stub.receivedCalls[0]))
}

func TestHandleCommand_Message_NotSubscribed(t *testing.T) {
	reg := channels.NewRegistry()
	reg.Register("stub", func() channels.Channel { return &stubChannel{} })
	hub := channels.NewHub(nopBroadcaster{}, reg)
	conn := hub.Connect()

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, conn.ID(), `{"command":"message","channel":"stub","data":{}}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleCommand_MissingConnectionIDHeader(t *testing.T) {
	b := channels.New(newTestHub())
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, "", `{"command":"subscribe","channel":"stub"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCommand_UnknownCommandValue(t *testing.T) {
	hub := newTestHub()
	conn := hub.Connect()

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, conn.ID(), `{"command":"bogus","channel":"stub"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCommand_MalformedJSONBody(t *testing.T) {
	hub := newTestHub()
	conn := hub.Connect()

	b := channels.New(hub)
	mount := mountBackend(t, "/cable", b)

	rec := doCommand(t, mount, conn.ID(), `{not json`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- stream endpoint (GET /cable) ---
//
// These need a real listener rather than httptest.NewRecorder(): the
// recorder's Flush is a no-op with no concurrent reader, so it can't
// observe incremental delivery (multiple flushes arriving over time, a
// heartbeat firing mid-test, or a client disconnect propagating back
// through the request context) the way a live connection can.

func newTestServer(t *testing.T, hub *channels.Hub, opts ...channels.Option) (*httptest.Server, string) {
	t.Helper()

	b := channels.New(hub, opts...)
	mount := mountBackend(t, "/cable", b)
	srv := httptest.NewServer(mount)
	t.Cleanup(srv.Close)

	return srv, srv.URL + "/cable"
}

func openStream(t *testing.T, url string) (*http.Response, *bufio.Reader) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	return resp, bufio.NewReader(resp.Body)
}

func connectionID(t *testing.T, r *bufio.Reader) string {
	t.Helper()

	event, data, isComment := readSSEFrame(t, r)
	require.False(t, isComment)
	require.Equal(t, "connected", event)

	var welcome struct {
		ConnectionID string `json:"connection_id"`
	}
	require.NoError(t, json.Unmarshal(data, &welcome))
	require.NotEmpty(t, welcome.ConnectionID)

	return welcome.ConnectionID
}

func TestHandleConnect_SendsConnectedEventWithUsableConnID(t *testing.T) {
	hub := newTestHub()
	srv, url := newTestServer(t, hub)

	_, r := openStream(t, url)
	connID := connectionID(t, r)

	rec := doCommand(t, srv.Config.Handler, connID, `{"command":"subscribe","channel":"missing"}`)
	// 404 (unknown channel), not "unknown connection" -> proves the id round-trips live.
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleConnect_DeliversBroadcastMessageAsSSEFrame(t *testing.T) {
	stub := &stubChannel{streamTopic: "room_42"}
	reg := channels.NewRegistry()
	reg.Register("chat", func() channels.Channel { return stub })
	hub := channels.NewHub(memory.New(), reg)

	srv, url := newTestServer(t, hub)

	_, r := openStream(t, url)
	connID := connectionID(t, r)

	rec := doCommand(t, srv.Config.Handler, connID, `{"command":"subscribe","channel":"chat"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, hub.Broadcast(t.Context(), "room_42", []byte(`{"body":"hi"}`)))

	event, data, isComment := readSSEFrame(t, r)
	require.False(t, isComment)
	require.Equal(t, "chat", event)
	require.JSONEq(t, `{"body":"hi"}`, string(data))
}

func TestHandleConnect_HeartbeatKeepsConnectionAlive(t *testing.T) {
	hub := newTestHub()
	_, url := newTestServer(t, hub, channels.WithHeartbeat(20*time.Millisecond))

	_, r := openStream(t, url)
	connectionID(t, r) // drain the welcome frame

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, _, isComment := readSSEFrame(t, r)
		if isComment {
			return
		}
	}
	t.Fatal("expected at least one heartbeat comment line")
}

func TestHandleConnect_ClientDisconnectCallsHubDisconnect(t *testing.T) {
	hub := newTestHub()
	srv, url := newTestServer(t, hub)

	resp, r := openStream(t, url)
	connID := connectionID(t, r)
	require.NoError(t, resp.Body.Close())

	require.Eventually(t, func() bool {
		rec := doCommand(t, srv.Config.Handler, connID, `{"command":"subscribe","channel":"missing"}`)
		return rec.Code == http.StatusNotFound && strings.Contains(rec.Body.String(), "unknown connection")
	}, 2*time.Second, 10*time.Millisecond, "expected Hub.Disconnect to run after client disconnect")
}

func TestHandleConnect_FlusherUnsupported(t *testing.T) {
	b := channels.New(newTestHub())
	mount := mountBackend(t, "/cable", b)

	req := httptest.NewRequest(http.MethodGet, "/cable", nil)
	rec := newNonFlushingRecorder()
	mount.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.rec.Code)
}

// nonFlushingRecorder wraps httptest.ResponseRecorder without embedding it,
// so its http.Flusher implementation is NOT promoted — this deliberately
// fails handleConnect's pre-stream capability check. Embedding
// *httptest.ResponseRecorder directly would promote Flush and defeat the
// point of this type.
type nonFlushingRecorder struct {
	rec *httptest.ResponseRecorder
}

func newNonFlushingRecorder() *nonFlushingRecorder {
	return &nonFlushingRecorder{rec: httptest.NewRecorder()}
}

func (w *nonFlushingRecorder) Header() http.Header         { return w.rec.Header() }
func (w *nonFlushingRecorder) Write(b []byte) (int, error) { return w.rec.Write(b) }
func (w *nonFlushingRecorder) WriteHeader(code int)        { w.rec.WriteHeader(code) }

var _ http.ResponseWriter = (*nonFlushingRecorder)(nil)

// --- test doubles ---

// nopBroadcaster is a channels.Broadcaster whose Subscribe is never
// expected to be called by the command-endpoint tests above (none of them
// exercise StreamFrom), so it only needs to satisfy the interface.
type nopBroadcaster struct{}

func (nopBroadcaster) Publish(ctx context.Context, topic string, payload []byte) error { return nil }
func (nopBroadcaster) Subscribe(ctx context.Context, topic string) (channels.Subscription, error) {
	return nil, errors.New("nopBroadcaster: Subscribe not supported")
}
