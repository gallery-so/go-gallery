package gcp

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

// PubSub is a GCP PubSub client
type PubSub struct {
	pubsub *pubsub.Client
}

// NewPubSub creates a new GCPPubSub instance.
func NewPubSub(pCtx context.Context, opts ...option.ClientOption) (*PubSub, error) {

	if viper.GetString("ENV") != "local" {
		pubsub, err := pubsub.NewClient(pCtx, viper.GetString("GOOGLE_CLOUD_PROJECT"), opts...)
		if err != nil {
			return nil, err
		}
		return &PubSub{pubsub: pubsub}, nil
	}
	srv := pstest.NewServer()
	// Connect to the server without using TLS.
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	opts = append(opts, option.WithGRPCConn(conn))
	// Use the connection when creating a pubsub client.
	pubsub, err := pubsub.NewClient(pCtx, viper.GetString("GOOGLE_PROJECT_ID"), opts...)
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
			logger.For(ctx).WithError(err).Error("error handling sub message")
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
