package gcp

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// PubSub is a GCP PubSub client
type PubSub struct {
	pubsub *pubsub.Client
}

// NewPubSub creates a new GCPPubSub instance.
func NewPubSub(pCtx context.Context, opts ...option.ClientOption) (*PubSub, error) {
	return &PubSub{pubsub: NewClient(pCtx)}, nil
}

func NewClient(ctx context.Context) *pubsub.Client {
	options := []option.ClientOption{}
	projectID := env.GetString("GOOGLE_CLOUD_PROJECT")

	if env.GetString("ENV") == "local" {
		if host := env.GetString("PUBSUB_EMULATOR_HOST"); host != "" {
			projectID = "gallery-local"
			options = append(
				options,
				option.WithEndpoint(host),
				option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
				option.WithoutAuthentication(),
			)
		} else {
			fi, err := util.LoadEncryptedServiceKeyOrError("./secrets/dev/service-key-dev.json")
			if err != nil {
				logger.For(ctx).WithError(err).Error("failed to find service key, running without pubsub client")
				return nil
			}
			options = append(options, option.WithCredentialsJSON(fi))
		}
	}

	pub, err := pubsub.NewClient(ctx, projectID, options...)
	if err != nil {
		panic(err)
	}

	return pub
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
		Topic:            g.pubsub.Topic(topicName),
		AckDeadline:      time.Second * 10,
		ExpirationPolicy: time.Hour * 24 * 3,
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
