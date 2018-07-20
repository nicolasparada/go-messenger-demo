package main

import "sync"

// MessageBus to dispatch data.
type MessageBus struct {
	μ       sync.Mutex
	Clients map[MessageClient]struct{}
}

// MessageClient to register in the MessageBus and receive new messages.
type MessageClient struct {
	Ch     chan Message
	UserID string
}

func (bus *MessageBus) register(ch chan Message, userID string) func() {
	client := MessageClient{ch, userID}

	bus.μ.Lock()
	defer bus.μ.Unlock()
	bus.Clients[client] = struct{}{}

	return func() {
		bus.μ.Lock()
		defer bus.μ.Unlock()
		delete(bus.Clients, client)
	}
}

func (bus *MessageBus) send(message Message) {
	bus.μ.Lock()
	defer bus.μ.Unlock()
	for client := range bus.Clients {
		if client.UserID == message.ReceiverID {
			client.Ch <- message
		}
	}
}
