package event

import (
	"log/slog"
	"slices"
	"sync"
)

const defaultChannelSize = 64

// Bus is an event bus.
type Bus struct {
	consumers []chan Event
	mu        sync.Mutex
	logger    *slog.Logger
}

// NewBus returns a new event bus.
func NewBus(logger *slog.Logger) *Bus {
	return &Bus{
		logger: logger,
	}
}

// Register registers a consumer for all events.
func (b *Bus) Register() <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, defaultChannelSize)
	b.consumers = append(b.consumers, ch)
	return ch
}

// Deregister deregisters a consumer for all events.
func (b *Bus) Deregister(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.consumers = slices.DeleteFunc(b.consumers, func(other chan Event) bool {
		if ch == other {
			close(other)
			return true
		}

		return false
	})
}

// Send sends an event to all registered consumers.
func (b *Bus) Send(evt Event) {
	// The mutex is needed to ensure the backing array of b.consumers cannot be
	// modified under our feet. There is probably a more efficient way to do this
	// but this should be ok.
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.consumers {
		select {
		case ch <- evt:
		default:
			b.logger.Warn("Event dropped", "name", evt.EventName())
		}
	}
}
