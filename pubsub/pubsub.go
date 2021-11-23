package pubsub

import "context"

// PubSub is a wrapper around the Pub/Sub client
type PubSub interface {
	// Publish publishes a message to a topic and optionally blocks for that message to be acknowledged by the server
	Publish(ctx context.Context, topic string, message []byte, block bool) error
	// Subscribe subscribes to a topic and processes messages using the given handler func
	Subscribe(ctx context.Context, topic string, handler func(ctx context.Context, message []byte) error) error
}
