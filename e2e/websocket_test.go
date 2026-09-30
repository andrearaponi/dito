package e2e

// WebSocket proxying. Security scenarios (authentication, origin, wss with a
// custom CA) come with the specs that fix their findings (C6).

import (
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocket_Echo(t *testing.T) {
	Run(t, func(s *S) {
		s.Backend("echo", websocketEcho())
		p := s.Proxy(`
  - path: "^/ws$"
    target_url: "{{ws "echo"}}/ws"
    enable_websocket: true
    replace_path: true`)
		conn, status, err := s.DialWebSocket(wsURL(p.URL)+"/ws", nil)
		require.NoError(s, err, "handshake status %d", status)
		for _, msg := range []string{"ping", "second message"} {
			require.NoError(s, conn.WriteMessage(websocket.TextMessage, []byte(msg)))
			_, got, err := conn.ReadMessage()
			require.NoError(s, err)
			assert.Equal(s, msg, string(got))
		}
	})
}
