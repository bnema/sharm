package service

import (
	"sync"
)

type EventBus struct {
	subscribers map[string][]chan Event
	mu          sync.RWMutex
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]chan Event),
	}
}

func (eb *EventBus) Subscribe(mediaID string) chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(chan Event, 16)
	eb.subscribers[mediaID] = append(eb.subscribers[mediaID], ch)
	return ch
}

func (eb *EventBus) Unsubscribe(mediaID string, ch chan Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	subs := eb.subscribers[mediaID]
	for i, sub := range subs {
		if sub == ch {
			eb.subscribers[mediaID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}

	if len(eb.subscribers[mediaID]) == 0 {
		delete(eb.subscribers, mediaID)
	}
}

func (eb *EventBus) Publish(mediaID string, event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for _, ch := range eb.subscribers[mediaID] {
		select {
		case ch <- event:
		default:
			// Drop event if subscriber is slow
		}
	}
}
