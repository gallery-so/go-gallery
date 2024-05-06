package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	registry "github.com/confluentinc/confluent-kafka-go/schemaregistry"
	"github.com/confluentinc/confluent-kafka-go/schemaregistry/serde"
	"github.com/confluentinc/confluent-kafka-go/schemaregistry/serde/avro"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/mikeydub/go-gallery/db/gen/mirrordb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/kafka-streamer/rest"
	"github.com/mikeydub/go-gallery/kafka-streamer/schema/ethereum"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/util/batch"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net/http"
	"os"
	"strings"
	"time"
)

// Enable for debugging; will not commit offsets or write to the database
const readOnlyMode = false

type streamerConfig struct {
	Topic   string
	Batcher batcher
}

func main() {
	setDefaults()
	logger.InitWithGCPDefaults()

	ctx := context.Background()

	// Health endpoint
	router := gin.Default()
	router.GET("/health", util.HealthCheckHandler())

	pgx := postgres.NewPgxClient()
	defer pgx.Close()

	ccf := newContractCollectionFiller(ctx, pgx)

	go runStreamer(ctx, pgx, ccf)

	err := router.Run(":3000")
	if err != nil {
		err = fmt.Errorf("error running router: %w", err)
		panic(err)
	}
}

func newEthereumOwnerConfig(deserializer *avro.GenericDeserializer, queries *mirrordb.Queries) *streamerConfig {
	parseF := func(ctx context.Context, message *kafka.Message) (mirrordb.ProcessEthereumOwnerEntryParams, error) {
		return parseOwnerMessage(ctx, deserializer, message)
	}

	submitF := func(ctx context.Context, entries []mirrordb.ProcessEthereumOwnerEntryParams) error {
		return submitOwnerBatch(ctx, queries.ProcessEthereumOwnerEntry, entries)
	}

	return &streamerConfig{
		Topic:   "ethereum.owner.v4",
		Batcher: newMessageBatcher(250, time.Second, 10, parseF, submitF),
	}
}

func newEthereumTokenConfig(deserializer *avro.GenericDeserializer, queries *mirrordb.Queries, ccf *contractCollectionFiller) *streamerConfig {
	parseF := func(ctx context.Context, message *kafka.Message) (mirrordb.ProcessEthereumTokenEntryParams, error) {
		return parseTokenMessage(ctx, deserializer, message)
	}

	submitF := func(ctx context.Context, entries []mirrordb.ProcessEthereumTokenEntryParams) error {
		return submitTokenBatch(ctx, queries.ProcessEthereumTokenEntry, entries, ccf)
	}

	return &streamerConfig{
		Topic:   "ethereum.nft.v4",
		Batcher: newMessageBatcher(250, time.Second, 10, parseF, submitF),
	}
}

func newBaseOwnerConfig(deserializer *avro.GenericDeserializer, queries *mirrordb.Queries) *streamerConfig {
	parseF := func(ctx context.Context, message *kafka.Message) (mirrordb.ProcessBaseOwnerEntryParams, error) {
		ethereumEntry, err := parseOwnerMessage(ctx, deserializer, message)
		if err != nil {
			return mirrordb.ProcessBaseOwnerEntryParams{}, err
		}

		// All EVM chains (Ethereum, Base, Zora) have the same database and query structure, so we can cast between their parameters
		return mirrordb.ProcessBaseOwnerEntryParams(ethereumEntry), nil
	}

	submitF := func(ctx context.Context, entries []mirrordb.ProcessBaseOwnerEntryParams) error {
		return submitOwnerBatch(ctx, queries.ProcessBaseOwnerEntry, entries)
	}

	return &streamerConfig{
		Topic:   "base.owner.v4",
		Batcher: newMessageBatcher(250, time.Second, 10, parseF, submitF),
	}
}

func newBaseTokenConfig(deserializer *avro.GenericDeserializer, queries *mirrordb.Queries, ccf *contractCollectionFiller) *streamerConfig {
	parseF := func(ctx context.Context, message *kafka.Message) (mirrordb.ProcessBaseTokenEntryParams, error) {
		ethereumEntry, err := parseTokenMessage(ctx, deserializer, message)
		if err != nil {
			return mirrordb.ProcessBaseTokenEntryParams{}, err
		}

		// All EVM chains (Ethereum, Base, Zora) have the same database and query structure, so we can cast between their parameters
		return mirrordb.ProcessBaseTokenEntryParams(ethereumEntry), nil
	}

	submitF := func(ctx context.Context, entries []mirrordb.ProcessBaseTokenEntryParams) error {
		return submitTokenBatch(ctx, queries.ProcessBaseTokenEntry, entries, ccf)
	}

	return &streamerConfig{
		Topic:   "base.nft.v4",
		Batcher: newMessageBatcher(250, time.Second, 10, parseF, submitF),
	}
}

func newZoraOwnerConfig(deserializer *avro.GenericDeserializer, queries *mirrordb.Queries) *streamerConfig {
	parseF := func(ctx context.Context, message *kafka.Message) (mirrordb.ProcessZoraOwnerEntryParams, error) {
		ethereumEntry, err := parseOwnerMessage(ctx, deserializer, message)
		if err != nil {
			return mirrordb.ProcessZoraOwnerEntryParams{}, err
		}

		// All EVM chains (Ethereum, Base, Zora) have the same database and query structure, so we can cast between their parameters
		return mirrordb.ProcessZoraOwnerEntryParams(ethereumEntry), nil
	}

	submitF := func(ctx context.Context, entries []mirrordb.ProcessZoraOwnerEntryParams) error {
		return submitOwnerBatch(ctx, queries.ProcessZoraOwnerEntry, entries)
	}

	return &streamerConfig{
		Topic:   "zora.owner.v4",
		Batcher: newMessageBatcher(250, time.Second, 10, parseF, submitF),
	}
}

func newZoraTokenConfig(deserializer *avro.GenericDeserializer, queries *mirrordb.Queries, ccf *contractCollectionFiller) *streamerConfig {
	parseF := func(ctx context.Context, message *kafka.Message) (mirrordb.ProcessZoraTokenEntryParams, error) {
		ethereumEntry, err := parseTokenMessage(ctx, deserializer, message)
		if err != nil {
			return mirrordb.ProcessZoraTokenEntryParams{}, err
		}

		// All EVM chains (Ethereum, Base, Zora) have the same database and query structure, so we can cast between their parameters
		return mirrordb.ProcessZoraTokenEntryParams(ethereumEntry), nil
	}

	submitF := func(ctx context.Context, entries []mirrordb.ProcessZoraTokenEntryParams) error {
		return submitTokenBatch(ctx, queries.ProcessZoraTokenEntry, entries, ccf)
	}

	return &streamerConfig{
		Topic:   "zora.nft.v4",
		Batcher: newMessageBatcher(250, time.Second, 10, parseF, submitF),
	}
}

func runStreamer(ctx context.Context, pgx *pgxpool.Pool, ccf *contractCollectionFiller) {
	deserializer, err := newDeserializerFromRegistry()
	if err != nil {
		panic(fmt.Errorf("failed to create Avro deserializer: %w", err))
	}

	queries := mirrordb.New(pgx)

	// Every few minutes, check the database for contracts or collections that didn't get filled in somehow (due
	// to errors, rate limits, etc). The query only looks for contracts/collections that were created more than a
	// minute ago, since newer contracts/collections should be in the process of getting filled in already.
	go fillMissingContractsAndCollections(ctx, queries, ccf)

	// Creating multiple configs for each topic allows them to process separate partitions in parallel
	configs := []*streamerConfig{
		newEthereumOwnerConfig(deserializer, queries),
		newEthereumTokenConfig(deserializer, queries, ccf),
		newBaseOwnerConfig(deserializer, queries),
		newBaseTokenConfig(deserializer, queries, ccf),
		newZoraOwnerConfig(deserializer, queries),
		newZoraTokenConfig(deserializer, queries, ccf),
	}

	// If any topic errors more than 10 times in 10 minutes, panic and restart the whole service
	maxRetries := 10
	retryResetInterval := 10 * time.Minute

	for _, config := range configs {
		go func(config *streamerConfig) {
			retriesRemaining := maxRetries
			retriesResetAt := time.Now().Add(retryResetInterval)
			for {
				err := streamTopic(ctx, config)
				if err != nil {
					if time.Now().After(retriesResetAt) {
						retriesRemaining = maxRetries
						retriesResetAt = time.Now().Add(retryResetInterval)
					}

					if retriesRemaining == 0 {
						panic(fmt.Errorf("streaming topic '%s' failed with error: %w", config.Topic, err))
					}

					retriesRemaining--
					config.Batcher.Reset()
					logger.For(ctx).Warnf("streaming topic '%s' failed with error: %v, restarting...", config.Topic, err)
				}
			}
		}(config)
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

type messageStats struct {
	interval       time.Duration
	nextReportTime time.Time
	inserts        int64
	updates        int64
	deletes        int64
}

func newMessageStats(reportingInterval time.Duration) *messageStats {
	return &messageStats{
		interval:       reportingInterval,
		nextReportTime: time.Now().Add(reportingInterval),
	}
}

func (m *messageStats) Update(ctx context.Context, msg *kafka.Message) {
	actionType, err := getActionType(msg)
	if err != nil {
		logger.For(ctx).Errorf("failed to get action type for message: %v", msg)
		return
	}

	switch actionType {
	case "insert":
		m.inserts++
	case "update":
		m.updates++
	case "delete":
		m.deletes++
	default:
		logger.For(ctx).Errorf("invalid action type %s for message %v", actionType, msg)
	}

	if time.Now().After(m.nextReportTime) {
		logger.For(ctx).Infof("processed %d messages in the last %s (%d inserts, %d updates, and %d deletes)", m.inserts+m.updates+m.deletes, m.interval, m.inserts, m.updates, m.deletes)
		m.inserts = 0
		m.updates = 0
		m.deletes = 0
		m.nextReportTime = time.Now().Add(m.interval)
	}
}

type batcher interface {
	Reset()
	Stop()
	Add(context.Context, *kafka.Message) error
	IsReady() bool
	Submit(context.Context, *kafka.Consumer) error
}

func streamTopic(ctx context.Context, config *streamerConfig) error {
	ctx = logger.NewContextWithFields(ctx, logrus.Fields{"topic": config.Topic})

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  env.GetString("SIMPLEHASH_BOOTSTRAP_SERVERS"),
		"group.id":           env.GetString("SIMPLEHASH_GROUP_ID"),
		"sasl.username":      env.GetString("SIMPLEHASH_KAFKA_API_KEY"),
		"sasl.password":      env.GetString("SIMPLEHASH_KAFKA_API_SECRET"),
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
		"security.protocol":  "SASL_SSL",
		"sasl.mechanisms":    "PLAIN",
	})

	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}

	defer c.Close()

	batch := config.Batcher
	var rebalanceErr error
	rebalanceErrPtr := &rebalanceErr

	rebalanceCb := func(c *kafka.Consumer, event kafka.Event) error {
		switch e := event.(type) {
		case kafka.AssignedPartitions:
			err := c.Assign(e.Partitions)
			if err != nil {
				err = fmt.Errorf("failed to assign partitions: %w", err)
				*rebalanceErrPtr = err
				return err
			}
		case kafka.RevokedPartitions:
			err := batch.Submit(ctx, c)
			if err != nil {
				*rebalanceErrPtr = err
				return err
			}
			err = c.Unassign()
			if err != nil {
				err = fmt.Errorf("failed to unassign partitions: %w", err)
				*rebalanceErrPtr = err
				return err
			}
		}
		return nil
	}

	err = c.SubscribeTopics([]string{config.Topic}, rebalanceCb)
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", config.Topic, err)
	}

	stats := newMessageStats(time.Hour)
	readTimeout := 100 * time.Millisecond

	for {
		msg, err := c.ReadMessage(readTimeout)

		// rebalanceCb is triggered by ReadMessage, so we want to check immediately after ReadMessage to see if
		// the rebalance failed and we need to restart
		if rebalanceErr != nil {
			return fmt.Errorf("rebalance failed: %w", rebalanceErr)
		}

		if err != nil {
			if kafkaErr, ok := util.ErrorAs[kafka.Error](err); ok {
				if kafkaErr.Code() == kafka.ErrTimedOut {
					if batch.IsReady() {
						err := batch.Submit(ctx, c)
						if err != nil {
							return fmt.Errorf("failed to submit batch: %w", err)
						}
					}
					continue
				}
			}

			return fmt.Errorf("error reading message: %w", err)
		}

		err = batch.Add(ctx, msg)
		if err != nil {
			return fmt.Errorf("failed to add message to batch: %w", err)
		}

		if batch.IsReady() {
			err := batch.Submit(ctx, c)
			if err != nil {
				return fmt.Errorf("failed to submit batch: %w", err)
			}
		}

		stats.Update(ctx, msg)
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

func parseNumeric(s *string) (pgtype.Numeric, error) {
	if s == nil {
		return pgtype.Numeric{Status: pgtype.Null}, nil
	}

	n := pgtype.Numeric{}
	err := n.Set(*s)
	if err != nil {
		err = fmt.Errorf("failed to parse numeric '%s': %w", *s, err)
		return n, err
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
	return persist.Address(strings.ToLower(parts[1])), id, nil
}

func parseTimestamp(s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}

	// Try parsing with the RFC3339 format first
	ts, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		// If RFC3339 fails, try ISO8601 without timezone, since that's what some messages appear to use
		const layout = "2006-01-02T15:04:05" //
		ts, err = time.Parse(layout, *s)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp %s: %w", *s, err)
		}
	}

	return &ts, nil
}

func parseOwnerMessage(ctx context.Context, deserializer *avro.GenericDeserializer, msg *kafka.Message) (mirrordb.ProcessEthereumOwnerEntryParams, error) {
	key := string(msg.Key)

	owner := ethereum.Owner{}
	err := deserializer.DeserializeInto(*msg.TopicPartition.Topic, msg.Value, &owner)
	if err != nil {
		return mirrordb.ProcessEthereumOwnerEntryParams{}, fmt.Errorf("failed to deserialize owner message with key %s: %w", key, err)
	}

	actionType, err := getActionType(msg)
	if err != nil {
		err = fmt.Errorf("failed to get action type for msg: %v", msg)
		return mirrordb.ProcessEthereumOwnerEntryParams{}, err
	}

	contractAddress, tokenID, err := parseNftID(owner.Nft_id)
	if err != nil {
		return mirrordb.ProcessEthereumOwnerEntryParams{}, fmt.Errorf("error parsing NftID: %w", err)
	}

	// TODO: Same as above, we'll want to normalize the address based on the chain at some point
	walletAddress := strings.ToLower(owner.Owner_address)

	quantity, err := parseNumeric(owner.Quantity)
	if err != nil {
		return mirrordb.ProcessEthereumOwnerEntryParams{}, err
	}

	firstAcquiredDate, err := parseTimestamp(owner.First_acquired_date)
	if err != nil {
		err = fmt.Errorf("failed to parse First_acquired_date: %w", err)
		return mirrordb.ProcessEthereumOwnerEntryParams{}, err
	}

	lastAcquiredDate, err := parseTimestamp(owner.Last_acquired_date)
	if err != nil {
		err = fmt.Errorf("failed to parse Last_acquired_date: %w", err)
		return mirrordb.ProcessEthereumOwnerEntryParams{}, err
	}

	var params mirrordb.ProcessEthereumOwnerEntryParams

	if actionType == "delete" {
		params = mirrordb.ProcessEthereumOwnerEntryParams{
			ShouldDelete:       true,
			SimplehashKafkaKey: key,

			// pgtype.Numeric defaults to 'undefined' instead of 'null', so we actually need to set these explicitly
			// or else we'll get an error when pgx tries to encode them
			TokenID:  pgtype.Numeric{Status: pgtype.Null},
			Quantity: pgtype.Numeric{Status: pgtype.Null},
		}
	} else {
		if actionType == "insert" || actionType == "update" {
			params = mirrordb.ProcessEthereumOwnerEntryParams{
				ShouldUpsert:             true,
				SimplehashKafkaKey:       key,
				SimplehashNftID:          &owner.Nft_id,
				KafkaOffset:              util.ToPointer(int64(msg.TopicPartition.Offset)),
				KafkaPartition:           util.ToPointer(msg.TopicPartition.Partition),
				KafkaTimestamp:           util.ToPointer(msg.Timestamp),
				ContractAddress:          &contractAddress,
				TokenID:                  tokenID,
				OwnerAddress:             util.ToPointer(persist.Address(walletAddress)),
				Quantity:                 quantity,
				CollectionID:             owner.Collection_id,
				FirstAcquiredDate:        firstAcquiredDate,
				LastAcquiredDate:         lastAcquiredDate,
				FirstAcquiredTransaction: owner.First_acquired_transaction,
				LastAcquiredTransaction:  owner.Last_acquired_transaction,
				MintedToThisWallet:       owner.Minted_to_this_wallet,
				AirdroppedToThisWallet:   owner.Airdropped_to_this_wallet,
				SoldToThisWallet:         owner.Sold_to_this_wallet,
			}
		} else {
			return params, fmt.Errorf("invalid action type: %s", actionType)
		}
	}

	return params, nil
}

func parseTokenMessage(ctx context.Context, deserializer *avro.GenericDeserializer, msg *kafka.Message) (mirrordb.ProcessEthereumTokenEntryParams, error) {
	key := string(msg.Key)

	nft := ethereum.Nft{}
	err := deserializer.DeserializeInto(*msg.TopicPartition.Topic, msg.Value, &nft)
	if err != nil {
		return mirrordb.ProcessEthereumTokenEntryParams{}, fmt.Errorf("failed to deserialize token message with key %s: %w", key, err)
	}

	actionType, err := getActionType(msg)
	if err != nil {
		err = fmt.Errorf("failed to get action type for msg: %v", msg)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	contractAddress, tokenID, err := parseNftID(nft.Nft_id)
	if err != nil {
		return mirrordb.ProcessEthereumTokenEntryParams{}, fmt.Errorf("error parsing NftID: %w", err)
	}

	previews, err := toJSONB(nft.Previews)
	if err != nil {
		err = fmt.Errorf("failed to convert Previews to JSONB: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	contract, err := toJSONB(nft.Contract)
	if err != nil {
		err = fmt.Errorf("failed to convert Contract to JSONB: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	lastSale, err := toJSONB(nft.Last_sale)
	if err != nil {
		err = fmt.Errorf("failed to convert Last_sale to JSONB: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	firstCreated, err := toJSONB(nft.First_created)
	if err != nil {
		err = fmt.Errorf("failed to convert First_created to JSONB: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	rarity, err := toJSONB(nft.Rarity)
	if err != nil {
		err = fmt.Errorf("failed to convert Rarity to JSONB: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	imageProperties, err := toJSONB(nft.Image_properties)
	if err != nil {
		err = fmt.Errorf("failed to convert Image_properties to JSONB: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	videoProperties, err := toJSONB(nft.Video_properties)
	if err != nil {
		err = fmt.Errorf("failed to convert Video_properties to JSONB: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	audioProperties, err := toJSONB(nft.Audio_properties)
	if err != nil {
		err = fmt.Errorf("failed to convert Audio_properties to JSONB: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	modelProperties, err := toJSONB(nft.Model_properties)
	if err != nil {
		err = fmt.Errorf("failed to convert Model_properties to JSONB: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	otherProperties, err := toJSONB(nft.Other_properties)
	if err != nil {
		err = fmt.Errorf("failed to convert Other_properties to JSONB: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	tokenCount, err := parseNumeric(nft.Token_count)
	if err != nil {
		err = fmt.Errorf("failed to parse Token_count: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	ownerCount, err := parseNumeric(nft.Owner_count)
	if err != nil {
		err = fmt.Errorf("failed to parse Owner_count: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	var collectionID *string
	if nft.Collection != nil {
		collectionID = nft.Collection.Collection_id
	}

	onChainCreatedDate, err := parseTimestamp(nft.Created_date)
	if err != nil {
		err = fmt.Errorf("failed to parse Created_date: %w", err)
		return mirrordb.ProcessEthereumTokenEntryParams{}, err
	}

	var params mirrordb.ProcessEthereumTokenEntryParams

	if actionType == "delete" {
		params = mirrordb.ProcessEthereumTokenEntryParams{
			ShouldDelete:       true,
			SimplehashKafkaKey: key,

			// pgtype.Numeric and pgtype.JSONB default to 'undefined' instead of 'null', so we actually need to
			// set these explicitly or else we'll get an error when pgx tries to encode them.
			TokenID:         pgtype.Numeric{Status: pgtype.Null},
			TokenCount:      pgtype.Numeric{Status: pgtype.Null},
			OwnerCount:      pgtype.Numeric{Status: pgtype.Null},
			Previews:        pgtype.JSONB{Status: pgtype.Null},
			Contract:        pgtype.JSONB{Status: pgtype.Null},
			LastSale:        pgtype.JSONB{Status: pgtype.Null},
			FirstCreated:    pgtype.JSONB{Status: pgtype.Null},
			Rarity:          pgtype.JSONB{Status: pgtype.Null},
			ImageProperties: pgtype.JSONB{Status: pgtype.Null},
			VideoProperties: pgtype.JSONB{Status: pgtype.Null},
			AudioProperties: pgtype.JSONB{Status: pgtype.Null},
			ModelProperties: pgtype.JSONB{Status: pgtype.Null},
			OtherProperties: pgtype.JSONB{Status: pgtype.Null},
		}
	} else {
		if actionType == "insert" || actionType == "update" {
			params = mirrordb.ProcessEthereumTokenEntryParams{
				ShouldUpsert:       true,
				SimplehashKafkaKey: key,
				SimplehashNftID:    nft.Nft_id,
				ContractAddress:    util.ToPointer(contractAddress.String()),
				TokenID:            tokenID,
				Name:               cleanString(nft.Name),
				Description:        cleanString(nft.Description),
				Previews:           previews,
				ImageUrl:           cleanString(nft.Image_url),
				VideoUrl:           cleanString(nft.Video_url),
				AudioUrl:           cleanString(nft.Audio_url),
				ModelUrl:           cleanString(nft.Model_url),
				OtherUrl:           cleanString(nft.Other_url),
				BackgroundColor:    cleanString(nft.Background_color),
				ExternalUrl:        cleanString(nft.External_url),
				OnChainCreatedDate: onChainCreatedDate,
				Status:             cleanString(nft.Status),
				TokenCount:         tokenCount,
				OwnerCount:         ownerCount,
				Contract:           contract,
				CollectionID:       collectionID,
				LastSale:           lastSale,
				FirstCreated:       firstCreated,
				Rarity:             rarity,
				ExtraMetadata:      cleanString(nft.Extra_metadata),
				ImageProperties:    imageProperties,
				VideoProperties:    videoProperties,
				AudioProperties:    audioProperties,
				ModelProperties:    modelProperties,
				OtherProperties:    otherProperties,
				KafkaOffset:        util.ToPointer(int64(msg.TopicPartition.Offset)),
				KafkaPartition:     util.ToPointer(msg.TopicPartition.Partition),
				KafkaTimestamp:     util.ToPointer(msg.Timestamp),
			}
		} else {
			return params, fmt.Errorf("invalid action type: %s", actionType)
		}
	}

	return params, nil
}

type execBatchHandler interface {
	Exec(func(i int, e error))
	Close() error
}

type queryBatchHandler interface {
	QueryRow(f func(int, string, error))
	Close() error
}

func submitExecBatch[TBatch execBatchHandler, TEntries any](ctx context.Context, queryF func(context.Context, []TEntries) TBatch, params []TEntries) error {
	if readOnlyMode {
		return nil
	}

	b := queryF(ctx, params)
	defer b.Close()

	var err error
	b.Exec(func(i int, e error) {
		if e != nil {
			err = fmt.Errorf("failed to execute batch operation: %w. parameters: %v", e, params[i])
		}
	})

	return err
}

func submitOwnerBatch[TBatch execBatchHandler, TEntries any](ctx context.Context, queryF func(context.Context, []TEntries) TBatch, entries []TEntries) error {
	return submitExecBatch(ctx, queryF, entries)
}

func submitTokenBatch[TBatch queryBatchHandler, TEntries any](ctx context.Context, queryF func(context.Context, []TEntries) TBatch, entries []TEntries, ccf *contractCollectionFiller) error {
	if readOnlyMode {
		return nil
	}

	b := queryF(ctx, entries)
	defer b.Close()

	var idsToUpdate []string
	var err error

	b.QueryRow(func(i int, s string, e error) {
		if e != nil {
			// It's okay if there isn't a result row; it just means no collection or contract rows were inserted
			if !errors.Is(e, pgx.ErrNoRows) {
				err = fmt.Errorf("failed to process entry: %w. entry data: %v", e, entries[i])
			}
			return
		}

		idsToUpdate = append(idsToUpdate, s)
	})

	if err != nil {
		return err
	}

	if len(idsToUpdate) > 0 {
		// Do this in a goroutine since we don't care about errors (they'll be retried by the "fill missing" loop)
		// and we don't want to block Kafka event processing
		go ccf.DoAll(idsToUpdate)
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

// toJSONB takes a pointer to any JSON-serializable type and converts it into a pgtype.JSONB.
// If the input is nil, it returns a JSONB with a status of Null to represent a NULL value in the database.
func toJSONB[T any](data *T) (pgtype.JSONB, error) {
	if data == nil {
		return pgtype.JSONB{Status: pgtype.Null}, nil
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return pgtype.JSONB{}, err
	}

	// Convert jsonData to a string
	jsonStr := string(jsonData)

	// Strip out any literal null bytes
	jsonStr = strings.ReplaceAll(jsonStr, "\x00", "")

	// Strip out any escaped null characters in JSON
	cleanedStr := strings.ReplaceAll(jsonStr, "\\u0000", "")

	var jsonb pgtype.JSONB
	// Convert the cleaned string back to bytes and set it to jsonb
	err = jsonb.Set([]byte(cleanedStr))
	if err != nil {
		return pgtype.JSONB{}, err
	}

	jsonb.Status = pgtype.Present
	return jsonb, nil
}

func cleanString(s *string) *string {
	if s == nil {
		return nil
	}

	// Remove null characters
	cleanedStr := strings.ReplaceAll(*s, "\x00", "")

	// Remove invalid UTF-8 sequences
	cleanedStr = strings.ToValidUTF8(cleanedStr, "")

	return &cleanedStr
}

type contractCollectionFiller batch.Batcher[string, bool]

func (c *contractCollectionFiller) DoAll(nftIDs []string) ([]bool, []error) {
	return (*batch.Batcher[string, bool])(c).DoAll(nftIDs)
}

func newContractCollectionFiller(ctx context.Context, pgx *pgxpool.Pool) *contractCollectionFiller {
	httpClient := http.DefaultClient
	queries := mirrordb.New(pgx)

	fetchAndFill := func(ctx context.Context, nftIDs []string) ([]bool, []error) {
		// Fetch the NFTs from SimpleHash
		nfts, err := rest.GetSimpleHashNFTs(ctx, httpClient, nftIDs)
		if err != nil {
			return nil, []error{err}
		}

		ethContractParams := make([]mirrordb.UpdateEthereumContractParams, 0, len(nfts))
		baseContractParams := make([]mirrordb.UpdateBaseContractParams, 0, len(nfts))
		zoraContractParams := make([]mirrordb.UpdateZoraContractParams, 0, len(nfts))
		collectionParams := make([]mirrordb.UpdateCollectionParams, 0, len(nfts))

		for _, nft := range nfts {
			nft.Normalize()

			if nft.Contract != nil && nft.Chain != nil && nft.ContractAddress != nil {
				p := mirrordb.UpdateEthereumContractParams{
					Address:                *nft.ContractAddress,
					Type:                   nft.Contract.Type,
					Name:                   cleanString(nft.Contract.Name),
					Symbol:                 cleanString(nft.Contract.Symbol),
					DeployedBy:             cleanString(nft.Contract.DeployedBy),
					DeployedViaContract:    cleanString(nft.Contract.DeployedViaContract),
					OwnedBy:                cleanString(nft.Contract.OwnedBy),
					HasMultipleCollections: nft.Contract.HasMultipleCollections,
				}

				switch *nft.Chain {
				case "ethereum":
					ethContractParams = append(ethContractParams, p)
				case "base":
					baseContractParams = append(baseContractParams, mirrordb.UpdateBaseContractParams(p))
				case "zora":
					zoraContractParams = append(zoraContractParams, mirrordb.UpdateZoraContractParams(p))
				}
			}

			if nft.Collection != nil && nft.Collection.CollectionID != nil {
				markerplacePages, err := toJSONB(&nft.Collection.MarketplacePages)
				if err != nil {
					logger.For(ctx).Errorf("failed to convert MarketplacePages to JSONB: %v", err)
					markerplacePages = pgtype.JSONB{Status: pgtype.Null}
				}

				collectionRoyalties, err := toJSONB(&nft.Collection.CollectionRoyalties)
				if err != nil {
					logger.For(ctx).Errorf("failed to convert CollectionRoyalties to JSONB: %v", err)
					collectionRoyalties = pgtype.JSONB{Status: pgtype.Null}
				}

				collectionParams = append(collectionParams, mirrordb.UpdateCollectionParams{
					CollectionID:                 *nft.Collection.CollectionID,
					Name:                         cleanString(nft.Collection.Name),
					Description:                  cleanString(nft.Collection.Description),
					ImageUrl:                     cleanString(nft.Collection.ImageUrl),
					BannerImageUrl:               cleanString(nft.Collection.BannerImageUrl),
					Category:                     cleanString(nft.Collection.Category),
					IsNsfw:                       nft.Collection.IsNsfw,
					ExternalUrl:                  cleanString(nft.Collection.ExternalUrl),
					TwitterUsername:              cleanString(nft.Collection.TwitterUsername),
					DiscordUrl:                   cleanString(nft.Collection.DiscordUrl),
					InstagramUrl:                 cleanString(nft.Collection.InstagramUrl),
					MediumUsername:               cleanString(nft.Collection.MediumUsername),
					TelegramUrl:                  cleanString(nft.Collection.TelegramUrl),
					MarketplacePages:             markerplacePages,
					MetaplexMint:                 cleanString(nft.Collection.MetaplexMint),
					MetaplexCandyMachine:         cleanString(nft.Collection.MetaplexCandyMachine),
					MetaplexFirstVerifiedCreator: cleanString(nft.Collection.MetaplexFirstVerifiedCreator),
					SpamScore:                    nft.Collection.SpamScore,
					Chains:                       nft.Collection.Chains,
					TopContracts:                 nft.Collection.TopContracts,
					CollectionRoyalties:          collectionRoyalties,
				})
			}
		}

		if readOnlyMode {
			return nil, nil
		}

		if len(ethContractParams) > 0 {
			err = submitExecBatch(ctx, queries.UpdateEthereumContract, ethContractParams)
			if err != nil {
				logger.For(ctx).Errorf("failed to update Ethereum contracts: %v", err)
			}
		}

		if len(baseContractParams) > 0 {
			err = submitExecBatch(ctx, queries.UpdateBaseContract, baseContractParams)
			if err != nil {
				logger.For(ctx).Errorf("failed to update Base contracts: %v", err)
			}
		}

		if len(zoraContractParams) > 0 {
			err = submitExecBatch(ctx, queries.UpdateZoraContract, zoraContractParams)
			if err != nil {
				logger.For(ctx).Errorf("failed to update Zora contracts: %v", err)
			}
		}

		if len(collectionParams) > 0 {
			err = submitExecBatch(ctx, queries.UpdateCollection, collectionParams)
			if err != nil {
				logger.For(ctx).Errorf("failed to update collections: %v", err)
			}
		}

		return nil, nil
	}

	return (*contractCollectionFiller)(batch.NewBatcher(ctx, 50, 1*time.Second, false, false, fetchAndFill))
}

func fillMissingContractsAndCollections(ctx context.Context, queries *mirrordb.Queries, ccf *contractCollectionFiller) {
	for {
		nftIDs, err := queries.GetNFTIDsForMissingContractsAndCollections(ctx)
		if err != nil {
			err = fmt.Errorf("failed to get NFT IDs for missing contracts and collections: %w", err)
		} else {
			for len(nftIDs) > 0 {
				_, errs := ccf.DoAll(nftIDs)
				if err = getFirstNonNilError(errs); err != nil {
					err = fmt.Errorf("failed to fill missing contracts and collections: %v", err)
					logger.For(ctx).Error(err)
					break
				}

				nftIDs, err = queries.GetNFTIDsForMissingContractsAndCollections(ctx)
				if err != nil {
					err = fmt.Errorf("failed to get NFT IDs for missing contracts and collections: %w", err)
					logger.For(ctx).Error(err)
					break
				}
			}
		}
		time.Sleep(2 * time.Minute)
	}
}

func getFirstNonNilError(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
