package gcp

import (
	"context"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
)

// PubSub is a GCP PubSub client
type PubSub struct {
	pubsub *pubsub.Client
}

// NewGCPPubSub creates a new GCPPubSub instance.
func NewGCPPubSub(pCtx context.Context, projectID string, opts ...option.ClientOption) (*PubSub, error) {
	pubsub, err := pubsub.NewClient(pCtx, projectID, opts...)
	if err != nil {
		return nil, err
	}
	return &PubSub{pubsub: pubsub}, nil
}

// Publish publishes a message to a topic
func (g *PubSub) Publish(pCtx context.Context, topicName string, message []byte, block bool) error {
	topic := g.pubsub.Topic(topicName)
	res := topic.Publish(context.Background(), &pubsub.Message{
		Data: message,
	})
	if block {
		_, err := res.Get(pCtx)
		return err
	}
	return nil
}

// Subscribe subscribes to a topic
func (g *PubSub) Subscribe(pCtx context.Context, topicName string, handler func(context.Context, []byte) error) error {

	sub, err := g.pubsub.CreateSubscription(pCtx, topicName, pubsub.SubscriptionConfig{
		Topic:       g.pubsub.Topic(topicName),
		AckDeadline: time.Second * 10,
	})
	if err != nil {
		return err
	}
	err = sub.Receive(context.Background(), func(ctx context.Context, msg *pubsub.Message) {
		err := handler(ctx, msg.Data)
		if err != nil {
			logrus.WithError(err).Error("error handling sub message")
			msg.Nack()
			return
		}
		msg.Ack()
	})
	return err
}

// CreateTopic creates a new topic
func (g *PubSub) CreateTopic(pCtx context.Context, topic string) error {
	_, err := g.pubsub.CreateTopic(pCtx, topic)
	return err
}
