package event

import (
	"log/slog"
	"sync"

	"git.netflux.io/rob/octoplex/internal/shortid"
)

const defaultChannelSize = 64

// ClientID is a unique identifier for a client.
type ClientID string

// Bus is an event bus.
type Bus struct {
	consumers map[ClientID]chan Event
	mu        sync.Mutex
	logger    *slog.Logger
}

// NewBus returns a new event bus.
func NewBus(logger *slog.Logger) *Bus {
	return &Bus{
		consumers: make(map[ClientID]chan Event),
		logger:    logger,
	}
}

// Register registers a consumer for all events.
func (b *Bus) Register() (ClientID, <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	clientID := NewClientID()
	ch := make(chan Event, defaultChannelSize)
	b.consumers[clientID] = ch
	return clientID, ch
}

// Deregister deregisters a consumer for all events.
func (b *Bus) Deregister(clientID ClientID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.consumers[clientID]; ok {
		close(ch)
		delete(b.consumers, clientID)
	}
}

// Send sends an event to all registered consumers.
func (b *Bus) Send(evt Event) {
	// The mutex is needed to ensure that b.consumers cannot be modified under
	// our feet. There is probably a more efficient way to do this but this
	// should be ok.
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

// SendTo sends an event to a specific consumer identified by clientID.
func (b *Bus) SendTo(clientID ClientID, evt Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch, ok := b.consumers[clientID]
	if !ok {
		b.logger.Warn("Attempted to send event to unknown client", "client_id", clientID, "name", evt.EventName())
		return
	}

	select {
	case ch <- evt:
	default:
		b.logger.Warn("Event dropped for client", "client_id", clientID, "name", evt.EventName())
	}
}

// NewClientID generates a new unique client ID.
func NewClientID() ClientID {
	return ClientID(shortid.New().String())
}
