package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	trails "github.com/asimmons91/trails"
)

// handleConnect opens the SSE stream for one client connection (GET
// /cable). Once headers are written, this handler never returns a non-nil
// error — the router's errorHandler would try to write a second status
// line, which is invalid after the stream has started — so every failure
// past that point is logged and ends the loop by returning nil.
func (b *Backend) handleConnect(c *trails.Context) error {
	flusher, ok := c.Response().(http.Flusher)
	if !ok {
		return trails.NewHTTPError(http.StatusInternalServerError, errors.New("channels: response writer does not support flushing"))
	}

	conn := b.hub.Connect()
	ctx := c.Request().Context()

	header := c.Response().Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no") // nginx: disable proxy buffering of the stream
	c.Response().WriteHeader(http.StatusOK)

	// conn.ID() is always lowercase hex (see newConnID in hub.go), so it
	// never needs JSON escaping.
	welcome := fmt.Sprintf(`{"connection_id":"%s"}`, conn.ID())
	if err := writeSSEEvent(c.Response(), "connected", []byte(welcome)); err != nil {
		b.hub.Disconnect(context.WithoutCancel(ctx), conn.ID())
		return nil
	}
	flusher.Flush()

	ticker := time.NewTicker(b.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.hub.Disconnect(context.WithoutCancel(ctx), conn.ID())
			return nil

		case msg := <-conn.Outbox():
			if err := writeSSEEvent(c.Response(), msg.Channel, msg.Payload); err != nil {
				b.logger.Warn("channels: writing SSE frame failed, disconnecting", "conn_id", conn.ID(), "error", err)
				b.hub.Disconnect(context.WithoutCancel(ctx), conn.ID())
				return nil
			}
			flusher.Flush()

		case <-ticker.C:
			if err := writeSSEComment(c.Response(), "ping"); err != nil {
				b.logger.Warn("channels: writing SSE heartbeat failed, disconnecting", "conn_id", conn.ID(), "error", err)
				b.hub.Disconnect(context.WithoutCancel(ctx), conn.ID())
				return nil
			}
			flusher.Flush()
		}
	}
}

// writeSSEEvent writes one `event: <event>\ndata: <line>...\n\n` frame.
// data is split on '\n' defensively per SSE framing rules, even though
// Message payloads are expected to be single-line JSON.
func writeSSEEvent(w io.Writer, event string, data []byte) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// writeSSEComment writes a `: <comment>\n\n` line — invisible to
// EventSource's onmessage, used for heartbeats.
func writeSSEComment(w io.Writer, comment string) error {
	_, err := fmt.Fprintf(w, ": %s\n\n", comment)
	return err
}

type commandBody struct {
	Command string            `json:"command"`
	Channel string            `json:"channel"`
	Params  map[string]string `json:"params,omitempty"`
	Data    json.RawMessage   `json:"data,omitempty"`
}

// handleCommand is the upstream leg (POST /cable/command): subscribe,
// unsubscribe, and client-to-channel messages all arrive here, dispatched
// on body.Command, correlated to a live connection via HeaderConnectionID.
func (b *Backend) handleCommand(c *trails.Context) error {
	connID := c.Request().Header.Get(HeaderConnectionID)
	if connID == "" {
		return c.String(http.StatusBadRequest, "missing "+HeaderConnectionID+" header")
	}

	var body commandBody
	if err := c.Bind(&body); err != nil {
		return c.String(http.StatusBadRequest, "bad request")
	}

	ctx := c.Request().Context()

	var err error
	switch body.Command {
	case "subscribe":
		err = b.hub.Subscribe(ctx, connID, body.Channel, body.Params)
	case "unsubscribe":
		err = b.hub.Unsubscribe(ctx, connID, body.Channel)
	case "message":
		err = b.hub.Receive(ctx, connID, body.Channel, body.Data)
	default:
		return c.String(http.StatusBadRequest, "unknown command")
	}

	if err != nil {
		b.logger.Warn("channels: command failed", "command", body.Command, "channel", body.Channel, "error", err)
		return c.String(statusFor(err), err.Error())
	}

	return c.String(http.StatusOK, "ok")
}

// statusFor maps a Hub error to an HTTP status code. Any error that isn't
// one of Hub's own sentinels (i.e. an app-defined Channel.Subscribed or
// Channel.Receive error) falls through to 500 — the transport has no way
// to tell a caller-fault error apart from a server bug for those today.
func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrUnknownConnection), errors.Is(err, ErrUnknownChannel), errors.Is(err, ErrNotSubscribed):
		return http.StatusNotFound
	case errors.Is(err, ErrAlreadySubscribed):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
