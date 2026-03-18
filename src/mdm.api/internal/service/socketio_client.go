package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"


	"github.com/gorilla/websocket"
)

// SocketIOClient implements a minimal SocketIO v4 client using Engine.IO v4 protocol.
type SocketIOClient struct {
	baseURL    string
	conn       *websocket.Conn
	mu         sync.Mutex
	handlers   map[string]func(json.RawMessage)
	sid        string
	pingInterval time.Duration
	done       chan struct{}
}

func NewSocketIOClient(rawURL string) *SocketIOClient {
	if !strings.HasPrefix(rawURL, "http") {
		rawURL = "https://" + rawURL
	}
	return &SocketIOClient{
		baseURL:  rawURL,
		handlers: make(map[string]func(json.RawMessage)),
		done:     make(chan struct{}),
	}
}

func (c *SocketIOClient) On(event string, handler func(json.RawMessage)) {
	c.handlers[event] = handler
}

func (c *SocketIOClient) Emit(event string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	// SocketIO event frame: 42["event_name", data]
	msg := fmt.Sprintf(`42["%s",%s]`, event, string(payload))
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.conn.WriteMessage(websocket.TextMessage, []byte(msg))
}

// Connect performs Engine.IO handshake and upgrades to WebSocket.
func (c *SocketIOClient) Connect() error {
	// Step 1: Polling handshake to get session ID
	sid, pingInterval, err := c.handshake()
	if err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	c.sid = sid
	c.pingInterval = pingInterval
	log.Printf("[socketio] handshake ok, sid=%s ping=%s", sid, pingInterval)

	// Step 2: Upgrade to WebSocket
	wsURL := c.buildWSURL(sid)
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}
	c.conn = conn

	// Step 3: Send probe
	conn.WriteMessage(websocket.TextMessage, []byte("2probe"))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return fmt.Errorf("probe response: %w", err)
	}
	if string(msg) != "3probe" {
		conn.Close()
		return fmt.Errorf("unexpected probe response: %s", msg)
	}

	// Step 4: Send upgrade
	conn.WriteMessage(websocket.TextMessage, []byte("5"))

	// Read post-upgrade message (usually "6" NOOP)
	_, msg, err = conn.ReadMessage()
	if err != nil {
		conn.Close()
		return fmt.Errorf("read post-upgrade: %w", err)
	}
	log.Printf("[socketio] post-upgrade: %s", string(msg))

	// Send SocketIO CONNECT packet for default namespace
	conn.WriteMessage(websocket.TextMessage, []byte("40"))

	// Read SocketIO CONNECT response (40{...} or 40)
	_, msg, err = conn.ReadMessage()
	if err != nil {
		conn.Close()
		return fmt.Errorf("read connect response: %w", err)
	}
	log.Printf("[socketio] connect response: %s", string(msg))

	if strings.HasPrefix(string(msg), "40") {
		if h, ok := c.handlers["connect"]; ok {
			h(nil)
		}
	}

	log.Printf("[socketio] websocket connected")

	// Start ping loop
	go c.pingLoop()

	// Start read loop
	go c.readLoop()

	return nil
}

func (c *SocketIOClient) Close() {
	c.mu.Lock()
	select {
	case <-c.done:
		// already closed
	default:
		close(c.done)
	}
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()
}

func (c *SocketIOClient) handshake() (sid string, pingInterval time.Duration, err error) {
	u, _ := url.Parse(c.baseURL)
	u.Path = "/socket.io/"
	q := u.Query()
	q.Set("EIO", "4")
	q.Set("transport", "polling")
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	// Response format: "0{...json...}" (Engine.IO OPEN packet)
	bodyStr := string(body)
	// Find the JSON object after the "0"
	jsonStart := strings.Index(bodyStr, "{")
	if jsonStart < 0 {
		return "", 0, fmt.Errorf("no JSON in handshake: %s", bodyStr)
	}
	jsonStr := bodyStr[jsonStart:]

	var hsData struct {
		SID          string   `json:"sid"`
		Upgrades     []string `json:"upgrades"`
		PingInterval int      `json:"pingInterval"`
		PingTimeout  int      `json:"pingTimeout"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &hsData); err != nil {
		return "", 0, fmt.Errorf("parse handshake: %w (body: %s)", err, jsonStr)
	}

	return hsData.SID, time.Duration(hsData.PingInterval) * time.Millisecond, nil
}

func (c *SocketIOClient) buildWSURL(sid string) string {
	u, _ := url.Parse(c.baseURL)
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/socket.io/"
	q := u.Query()
	q.Set("EIO", "4")
	q.Set("transport", "websocket")
	q.Set("sid", sid)
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *SocketIOClient) pingLoop() {
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.mu.Lock()
			if c.conn != nil {
				c.conn.WriteMessage(websocket.TextMessage, []byte("2"))
			}
			c.mu.Unlock()
		}
	}
}

func (c *SocketIOClient) readLoop() {
	defer func() {
		if h, ok := c.handlers["disconnect"]; ok {
			h(nil)
		}
	}()

	for {
		select {
		case <-c.done:
			return
		default:
		}

		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("[socketio] read error: %v", err)
			return
		}

		c.handleMessage(string(msg))
	}
}

func (c *SocketIOClient) handleMessage(msg string) {
	if len(msg) == 0 {
		return
	}

	switch {
	case msg == "3":
		// Pong — ignore
	case msg == "40":
		// SocketIO CONNECT — trigger connect handler
		if h, ok := c.handlers["connect"]; ok {
			h(nil)
		}
	case strings.HasPrefix(msg, "42"):
		// SocketIO EVENT: 42["event_name", data]
		payload := msg[2:]
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(payload), &arr); err != nil || len(arr) < 2 {
			return
		}
		var eventName string
		if err := json.Unmarshal(arr[0], &eventName); err != nil {
			return
		}
		if h, ok := c.handlers[eventName]; ok {
			h(arr[1])
		}
	case msg == "41":
		// SocketIO DISCONNECT
		if h, ok := c.handlers["disconnect"]; ok {
			h(nil)
		}
	}
}
