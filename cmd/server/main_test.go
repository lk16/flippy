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

// A WebSocket upgrade through logRequests must complete: the statusRecorder wrapper must not
// hide the underlying http.Hijacker from websocket.Accept.
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
