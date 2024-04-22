package main

import (
	"context"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	registry "github.com/confluentinc/confluent-kafka-go/schemaregistry"
	"github.com/confluentinc/confluent-kafka-go/schemaregistry/serde"
	"github.com/confluentinc/confluent-kafka-go/schemaregistry/serde/avro"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/db/gen/mirrordb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/kafka-streamer/schema/ethereum"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"strings"
	"time"
)

type streamerConfig struct {
	Topic           string
	ProcessMessageF func(ctx context.Context, msg *kafka.Message) error
}

func main() {
	setDefaults()
	logger.InitWithGCPDefaults()

	ctx := context.Background()

	// Health endpoint
	router := gin.Default()
	router.GET("/health", util.HealthCheckHandler())

	go streamTopics(ctx)

	err := router.Run(":3000")
	if err != nil {
		err = fmt.Errorf("error running router: %w", err)
		panic(err)
	}
}

func streamTopics(ctx context.Context) {
	pgx := postgres.NewPgxClient()
	defer pgx.Close()

	queries := mirrordb.New(pgx)
	deserializer, err := newDeserializerFromRegistry()
	if err != nil {
		panic(fmt.Errorf("failed to create Avro deserializer: %w", err))
	}

	config := &streamerConfig{
		Topic: "ethereum.owner.v4",
		ProcessMessageF: func(ctx context.Context, msg *kafka.Message) error {
			return processOwnerMessage(ctx, queries, deserializer, msg)
		},
	}

	errChannel := make(chan error)
	go func() {
		err := runStreamer(ctx, pgx, config)
		if err != nil {
			errChannel <- err
		}
	}()

	for {
		select {
		case err := <-errChannel:
			panic(err)
		}
	}
}

func newDeserializerFromRegistry() (*avro.GenericDeserializer, error) {
	registryURL := env.GetString("SIMPLEHASH_REGISTRY_URL")
	registryAPIKey := env.GetString("SIMPLEHASH_REGISTRY_API_KEY")
	registrySecretKey := env.GetString("SIMPLEHASH_REGISTRY_SECRET_KEY")

	registryClient, err := registry.NewClient(registry.NewConfigWithAuthentication(registryURL, registryAPIKey, registrySecretKey))

	deserializer, err := avro.NewGenericDeserializer(registryClient, serde.ValueSerde, avro.NewDeserializerConfig())
	if err != nil {
		return nil, err
	}

	return deserializer, nil
}

func runStreamer(ctx context.Context, pgx *pgxpool.Pool, config *streamerConfig) error {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"topic": config.Topic})

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  env.GetString("SIMPLEHASH_BOOTSTRAP_SERVERS"),
		"group.id":           env.GetString("SIMPLEHASH_GROUP_ID"),
		"sasl.username":      env.GetString("SIMPLEHASH_API_KEY"),
		"sasl.password":      env.GetString("SIMPLEHASH_API_SECRET"),
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": true,
		"security.protocol":  "SASL_SSL",
		"sasl.mechanisms":    "PLAIN",
	})

	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}

	defer c.Close()

	err = c.SubscribeTopics([]string{config.Topic}, nil)
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", config.Topic, err)
	}

	var messagesPerHour int64 = 0
	var nextHourReportTime = time.Now().Add(time.Hour)

	for {
		msg, err := c.ReadMessage(100)
		if err != nil {
			if kafkaErr, ok := util.ErrorAs[kafka.Error](err); ok {
				if kafkaErr.Code() == kafka.ErrTimedOut {
					// No need to log polling timeouts
					continue
				}
			}

			return fmt.Errorf("error reading message: %w", err)
		}

		err = config.ProcessMessageF(ctx, msg)
		if err != nil {
			return fmt.Errorf("failed to process message: %w", err)
		}

		messagesPerHour++
		if time.Now().After(nextHourReportTime) {
			logger.For(ctx).Infof("Processed %d messages in the last hour", messagesPerHour)
			messagesPerHour = 0
			nextHourReportTime = time.Now().Add(time.Hour)
		}
	}
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("SENTRY_DSN", "")
	viper.SetDefault("GAE_VERSION", "")
	viper.SetDefault("SENTRY_TRACES_SAMPLE_RATE", 0.2)

	viper.AutomaticEnv()

	if env.GetString("ENV") != "local" {
		logger.For(nil).Info("running in non-local environment, skipping environment configuration")
	} else {
		fi := "local"
		if len(os.Args) > 1 {
			fi = os.Args[1]
		}
		envFile := util.ResolveEnvFile("kafka-streamer", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString("ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
	}
}

func parseNumeric(s *string) (*pgtype.Numeric, error) {
	if s == nil {
		return nil, nil
	}

	n := &pgtype.Numeric{}
	err := n.Set(*s)
	if err != nil {
		err = fmt.Errorf("failed to parse numeric '%s': %w", *s, err)
		return nil, err
	}

	return n, nil
}

func parseNftID(nftID string) (contractAddress persist.Address, tokenID pgtype.Numeric, err error) {
	// NftID is in the format: chain.contract_address.token_id
	parts := strings.Split(nftID, ".")
	if len(parts) != 3 {
		return "", pgtype.Numeric{}, fmt.Errorf("invalid nft_id: %s", nftID)
	}

	id, err := parseNumeric(&parts[2])
	if err != nil {
		return "", pgtype.Numeric{}, fmt.Errorf("failed to parse tokenID: %w", err)
	}

	// TODO: Use the chain to map to one of our chains and normalize the address accordingly.
	// For now, just assume Ethereum and convert to lowercase.
	return persist.Address(strings.ToLower(parts[1])), *id, nil
}

func parseTimestamp(s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}

	ts, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp %s: %w", *s, err)
	}

	return &ts, nil
}

func processOwnerMessage(ctx context.Context, queries *mirrordb.Queries, deserializer *avro.GenericDeserializer, msg *kafka.Message) error {
	key := string(msg.Key)

	owner := ethereum.Owner{}
	err := deserializer.DeserializeInto(*msg.TopicPartition.Topic, msg.Value, &owner)
	if err != nil {
		return fmt.Errorf("failed to deserialize owner message with key %s: %w", key, err)
	}

	actionType, err := getActionType(msg)
	if err != nil {
		err = fmt.Errorf("failed to get action type for msg: %v", msg)
		return err
	}

	contractAddress, tokenID, err := parseNftID(owner.Nft_id)
	if err != nil {
		return fmt.Errorf("error parsing NftID: %w", err)
	}

	// TODO: Same as above, we'll want to normalize the address based on the chain at some point
	walletAddress := strings.ToLower(owner.Owner_address)

	quantity, err := parseNumeric(owner.Quantity)
	if err != nil {
		return err
	}

	firstAcquiredDate, err := parseTimestamp(owner.First_acquired_date)
	if err != nil {
		err = fmt.Errorf("failed to parse First_acquired_date: %w", err)
		return err
	}

	lastAcquiredDate, err := parseTimestamp(owner.Last_acquired_date)
	if err != nil {
		err = fmt.Errorf("failed to parse Last_acquired_date: %w", err)
		return err
	}

	if actionType == "delete" {
		err := queries.DeleteEthereumOwnerEntry(ctx, key)
		if err != nil {
			return fmt.Errorf("failed to delete ethereum owner entry with key %s: %w", key, err)
		}
	} else {
		if actionType == "insert" || actionType == "update" {
			err := queries.UpsertEthereumOwnerEntry(ctx, mirrordb.UpsertEthereumOwnerEntryParams{
				SimplehashKafkaKey:       key,
				SimplehashNftID:          &owner.Nft_id,
				KafkaOffset:              util.ToPointer(int64(msg.TopicPartition.Offset)),
				KafkaPartition:           util.ToPointer(msg.TopicPartition.Partition),
				KafkaTimestamp:           util.ToPointer(msg.Timestamp),
				ContractAddress:          &contractAddress,
				TokenID:                  tokenID,
				OwnerAddress:             util.ToPointer(persist.Address(walletAddress)),
				Quantity:                 *quantity,
				CollectionID:             owner.Collection_id,
				FirstAcquiredDate:        firstAcquiredDate,
				LastAcquiredDate:         lastAcquiredDate,
				FirstAcquiredTransaction: owner.First_acquired_transaction,
				LastAcquiredTransaction:  owner.Last_acquired_transaction,
				MintedToThisWallet:       owner.Minted_to_this_wallet,
				AirdroppedToThisWallet:   owner.Airdropped_to_this_wallet,
				SoldToThisWallet:         owner.Sold_to_this_wallet,
			})
			if err != nil {
				return fmt.Errorf("failed to upsert ethereum owner entry with key %s: %w", key, err)
			}
		} else {
			return fmt.Errorf("invalid action type: %s", actionType)
		}
	}

	return nil
}

func getActionType(msg *kafka.Message) (string, error) {
	for _, h := range msg.Headers {
		if h.Key == "modType" {
			return strings.ToLower(string(h.Value)), nil
		}
	}

	return "", fmt.Errorf("no action type found in message headers")
}
