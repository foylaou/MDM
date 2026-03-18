package service

import (
	"context"
	"sync"

	"github.com/anthropics/mdm-server/internal/domain"
)

type EventBroker struct {
	mu   sync.RWMutex
	subs map[chan *domain.MDMEvent]struct{}
}

func NewEventBroker() *EventBroker {
	return &EventBroker{subs: make(map[chan *domain.MDMEvent]struct{})}
}

func (b *EventBroker) Publish(event *domain.MDMEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- event:
		default:
			// drop if subscriber is slow
		}
	}
}

func (b *EventBroker) Subscribe(ctx context.Context) <-chan *domain.MDMEvent {
	ch := make(chan *domain.MDMEvent, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
		close(ch)
	}()
	return ch
}
