package event

import (
	"log/slog"
	"sync"
)

const defaultChannelSize = 64

// Bus is an event bus.
type Bus struct {
	consumers map[Name][]chan Event
	mu        sync.Mutex
	logger    *slog.Logger
}

// NewBus returns a new event bus.
func NewBus(logger *slog.Logger) *Bus {
	return &Bus{
		consumers: make(map[Name][]chan Event),
		logger:    logger,
	}
}

// Register registers a consumer for a given event.
func (b *Bus) Register(name Name) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, defaultChannelSize)
	b.consumers[name] = append(b.consumers[name], ch)
	return ch
}

// Send sends an event to all registered consumers.
func (b *Bus) Send(evt Event) {
	// The mutex is needed to ensure the backing array of b.consumers cannot be
	// modified under our feet. There is probably a more efficient way to do this
	// but this should be ok.
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.consumers[evt.name()] {
		select {
		case ch <- evt:
		default:
			b.logger.Warn("Event dropped", "name", evt.name())
		}
	}
}
