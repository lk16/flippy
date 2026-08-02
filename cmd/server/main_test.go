package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/stretchr/testify/require"
)

// TestLogRequests_SupportsWebSocketUpgrade guards against the statusRecorder
// wrapper hiding the underlying http.Hijacker: a WebSocket upgrade routed
// through logRequests must still complete (it previously returned 501 because
// coder/websocket's Accept type-asserts the ResponseWriter to http.Hijacker).
func TestLogRequests_SupportsWebSocketUpgrade(t *testing.T) {
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		var msg string
		if err := wsjson.Read(r.Context(), conn, &msg); err != nil {
			return
		}
		_ = wsjson.Write(r.Context(), conn, msg)
	})

	srv := httptest.NewServer(logRequests(echo))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	require.NoError(t, err, "WebSocket upgrade must succeed through logRequests")
	defer func() { _ = conn.CloseNow() }()

	require.NoError(t, wsjson.Write(context.Background(), conn, "ping"))

	var got string
	require.NoError(t, wsjson.Read(context.Background(), conn, &got))
	require.Equal(t, "ping", got)
}

// TestStatusRecorder_ImplementsHijacker is a compile-time-style guard that the
// wrapper advertises http.Hijacker.
func TestStatusRecorder_ImplementsHijacker(t *testing.T) {
	var _ http.Hijacker = (*statusRecorder)(nil)
}
