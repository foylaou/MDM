package service

import (
	"encoding/json"
	"log"
	"time"

	"github.com/anthropics/mdm-server/internal/port"
)

type SocketIORelay struct {
	url     string
	apiKey  string
	webhook *WebhookHandler
	broker  port.EventBroker
}

func NewSocketIORelay(url, apiKey string, webhook *WebhookHandler) *SocketIORelay {
	return &SocketIORelay{url: url, apiKey: apiKey, webhook: webhook}
}

func (r *SocketIORelay) Start() {
	go r.connectLoop()
}

func (r *SocketIORelay) connectLoop() {
	for {
		log.Printf("[socketio-relay] connecting to %s ...", r.url)
		if err := r.connect(); err != nil {
			log.Printf("[socketio-relay] error: %v", err)
		}
		log.Println("[socketio-relay] will reconnect in 3s")
		time.Sleep(3 * time.Second)
	}
}

func (r *SocketIORelay) connect() error {
	client := NewSocketIOClient(r.url)

	disconnected := make(chan struct{})

	client.On("connect", func(_ json.RawMessage) {
		log.Println("[socketio-relay] connected, sending auth")
		client.Emit("auth", map[string]string{"api_key": r.apiKey})
	})

	client.On("auth_result", func(data json.RawMessage) {
		var result struct {
			Status string `json:"status"`
		}
		json.Unmarshal(data, &result)
		if result.Status == "ok" {
			log.Println("[socketio-relay] auth success, listening for events")
		} else {
			log.Printf("[socketio-relay] auth failed: %s", string(data))
		}
	})

	client.On("mdm_event", func(data json.RawMessage) {
		log.Printf("[socketio-relay] received mdm_event (%d bytes)", len(data))
		r.processEvent(data)
	})

	client.On("disconnect", func(_ json.RawMessage) {
		log.Println("[socketio-relay] disconnected")
		close(disconnected)
	})

	if err := client.Connect(); err != nil {
		return err
	}

	// Block until disconnect — then return to let connectLoop retry
	<-disconnected
	client.Close()
	return nil
}

func (r *SocketIORelay) processEvent(raw json.RawMessage) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		log.Printf("[socketio-relay] invalid event json: %v", err)
		return
	}
	r.webhook.ProcessEvent(data)
}
