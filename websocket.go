package ekodb

import (
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketClient represents a WebSocket connection to ekoDB
type WebSocketClient struct {
	wsURL string
	token string
	conn  *websocket.Conn
}

// WebSocket creates a new WebSocket client
func (c *Client) WebSocket(wsURL string) (*WebSocketClient, error) {
	ws := &WebSocketClient{
		wsURL: wsURL,
		token: c.token,
	}

	if err := ws.connect(); err != nil {
		return nil, err
	}

	return ws, nil
}

// connect establishes a WebSocket connection
func (ws *WebSocketClient) connect() error {
	// Add /api/ws path if not present
	url := ws.wsURL
	if url[len(url)-7:] != "/api/ws" {
		url += "/api/ws"
	}

	// Add token as query parameter
	url += "?token=" + ws.token

	// Set up headers
	header := make(map[string][]string)
	header["Authorization"] = []string{"Bearer " + ws.token}

	// Connect
	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	ws.conn = conn
	return nil
}

// FindAll finds all records in a collection via WebSocket
func (ws *WebSocketClient) FindAll(collection string) ([]Record, error) {
	if ws.conn == nil {
		if err := ws.connect(); err != nil {
			return nil, err
		}
	}

	// Create request
	messageID := fmt.Sprintf("%d", time.Now().UnixNano())
	request := map[string]interface{}{
		"type":      "FindAll",
		"messageId": messageID,
		"payload": map[string]string{
			"collection": collection,
		},
	}

	// Send request
	if err := ws.conn.WriteJSON(request); err != nil {
		ws.conn = nil // Clear connection for reconnection
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	var response map[string]interface{}
	if err := ws.conn.ReadJSON(&response); err != nil {
		ws.conn = nil // Clear connection for reconnection
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check response type
	responseType, ok := response["type"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	if responseType == "Error" {
		message := response["message"].(string)
		return nil, fmt.Errorf("websocket error: %s", message)
	}

	// Extract data
	payload, ok := response["payload"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid payload format")
	}

	data, ok := payload["data"].([]interface{})
	if !ok {
		return []Record{}, nil
	}

	// Convert to records
	records := make([]Record, len(data))
	for i, item := range data {
		if record, ok := item.(map[string]interface{}); ok {
			records[i] = Record(record)
		}
	}

	return records, nil
}

// Close closes the WebSocket connection
func (ws *WebSocketClient) Close() error {
	if ws.conn != nil {
		return ws.conn.Close()
	}
	return nil
}
