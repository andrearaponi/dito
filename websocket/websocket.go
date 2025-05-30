package websocket

import (
	"dito/logging"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// HandleWebSocketProxy handles the proxying of WebSocket connections between a client and a target server.
// It upgrades the HTTP connection to a WebSocket connection and forwards messages between the client and server.
//
// Parameters:
//   - w: The HTTP response writer.
//   - r: The HTTP request.
//   - targetURL: The URL of the target WebSocket server.
//   - logger: The logger instance.
func HandleWebSocketProxy(w http.ResponseWriter, r *http.Request, targetURL string, logger *slog.Logger) {
	url, err := url.Parse(targetURL)
	if err != nil {
		logger.Error("Invalid WebSocket target URL", slog.Any("details", err))
		http.Error(w, "Invalid WebSocket target URL", http.StatusInternalServerError)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Failed to upgrade to WebSocket", slog.Any("details", err))
		http.Error(w, "Failed to upgrade to WebSocket", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := clientConn.Close(); err != nil {
			logger.Error("Error closing client WebSocket connection", slog.Any("details", err))
		}
	}()

	serverConn, _, err := websocket.DefaultDialer.Dial(url.String(), nil)
	if err != nil {
		logger.Error("Failed to connect to target WebSocket server", slog.Any("details", err))
		clientConn.WriteMessage(websocket.TextMessage, []byte("Error: Unable to connect to WebSocket server"))
		return
	}
	defer func() {
		if err := serverConn.Close(); err != nil {
			logger.Error("Error closing server WebSocket connection", slog.Any("details", err))
		}
	}()

	go func() {
		if err := CopyWebSocketMessages(clientConn, serverConn, logger); err != nil {
			logger.Error("Error while copying message from client to server", slog.Any("details", err))
		}
		clientConn.Close()
		serverConn.Close()
	}()

	if err := CopyWebSocketMessages(serverConn, clientConn, logger); err != nil {
		logger.Error("Error while copying message from server to client", slog.Any("details", err))
		clientConn.Close()
		serverConn.Close()
	}
}

// CopyWebSocketMessages copies messages from the source WebSocket connection to the destination WebSocket connection.
// It logs the details of the messages and any errors that occur during the process.
//
// Parameters:
//   - src: The source WebSocket connection.
//   - dest: The destination WebSocket connection.
//   - logger: The logger instance.
//
// Returns:
//   - error: An error if the message copying fails.
func CopyWebSocketMessages(src, dest *websocket.Conn, logger *slog.Logger) error {
	for {
		startTime := time.Now()
		messageType, message, err := src.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error("Unexpected WebSocket closure", slog.Any("details", err))
			}
			logging.LogWebSocketMessage(logger, messageType, message, err, time.Since(startTime))
			return err
		}
		logging.LogWebSocketMessage(logger, messageType, message, nil, time.Since(startTime))

		if err := dest.WriteMessage(messageType, message); err != nil {
			logger.Error("Error writing message", slog.Any("details", err))
			logging.LogWebSocketMessage(logger, messageType, message, err, time.Since(startTime))
			return err
		}
	}
}

// IsWebSocketRequest checks if the given HTTP request is a WebSocket upgrade request.
//
// Parameters:
//   - r: The HTTP request.
//
// Returns:
//   - bool: True if the request is a WebSocket upgrade request, false otherwise.
func IsWebSocketRequest(r *http.Request) bool {
	return websocket.IsWebSocketUpgrade(r)
}
