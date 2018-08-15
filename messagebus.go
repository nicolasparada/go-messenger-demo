package main

import "sync"

// MessageBus to dispatch data.
type MessageBus struct {
	Clients sync.Map
}

// MessageClient to register in the MessageBus and receive new messages.
type MessageClient struct {
	Ch     chan Message
	UserID string
}

func (bus *MessageBus) register(ch chan Message, userID string) func() {
	client := &MessageClient{Ch: ch, UserID: userID}
	bus.Clients.Store(client, nil)
	return func() {
		bus.Clients.Delete(client)
	}
}

func (bus *MessageBus) send(message Message) {
	bus.Clients.Range(func(key, _ interface{}) bool {
		client, ok := key.(*MessageClient)
		if !ok {
			return false
		}
		if client.UserID == message.ReceiverID {
			client.Ch <- message
		}
		return true
	})
}
