package port

// Event represents a real-time notification about media processing.
type Event struct {
	Type    string // "status", "progress"
	Status  string
	Message string
}

// EventPublisher publishes events to subscribers.
type EventPublisher interface {
	Publish(mediaID string, event Event)
}

// EventSubscriber subscribes to and unsubscribes from media events.
type EventSubscriber interface {
	Subscribe(mediaID string) chan Event
	Unsubscribe(mediaID string, ch chan Event)
}
