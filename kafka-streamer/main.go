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
	"github.com/mikeydub/go-gallery/kafka-streamer/schema/base"
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

var errInvalidJSON = errors.New("invalid JSON")

type streamerConfig struct {
	Topic         string
	Batcher       batcher
	DefaultOffset string
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

func newBaseOwnerConfig(deserializer *avro.GenericDeserializer, queries *mirrordb.Queries) *streamerConfig {
	parseF := func(ctx context.Context, message *kafka.Message, owner *base.Owner, params *mirrordb.ProcessBaseOwnerEntryParams) (*mirrordb.ProcessBaseOwnerEntryParams, error) {
		return parseOwnerMessage(ctx, deserializer, message, owner, params)
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
	parseF := func(ctx context.Context, message *kafka.Message, nft *base.Nft, params *mirrordb.ProcessBaseTokenEntryParams) (*mirrordb.ProcessBaseTokenEntryParams, error) {
		return parseTokenMessage(ctx, deserializer, message, nft, params)
	}

	submitF := func(ctx context.Context, entries []mirrordb.ProcessBaseTokenEntryParams) error {
		return submitTokenBatch(ctx, queries.ProcessBaseTokenEntry, entries, ccf)
	}

	return &streamerConfig{
		Topic:   "base.nft.v4",
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
	// to errors, rate limits, etc). The queries only look for contracts/collections that were created more than a
	// minute ago, since newer contracts/collections should be in the process of getting filled in already.
	go fillMissingContracts(ctx, queries, ccf)
	go fillMissingCollections(ctx, queries, ccf)

	// Creating multiple configs for each topic allows them to process separate partitions in parallel
	configs := []*streamerConfig{
		newBaseOwnerConfig(deserializer, queries),
		newBaseTokenConfig(deserializer, queries, ccf),
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

	autoOffsetReset := config.DefaultOffset
	if autoOffsetReset == "" {
		autoOffsetReset = "earliest"
	}

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  env.GetString("SIMPLEHASH_BOOTSTRAP_SERVERS"),
		"group.id":           env.GetString("SIMPLEHASH_GROUP_ID"),
		"sasl.username":      env.GetString("SIMPLEHASH_KAFKA_API_KEY"),
		"sasl.password":      env.GetString("SIMPLEHASH_KAFKA_API_SECRET"),
		"auto.offset.reset":  autoOffsetReset,
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
	if strings.TrimSpace(nftID) == "" {
		// There's nothing we can do with an empty nft_id at this point, and we can't let it hold up event processing.
		return "", pgtype.Numeric{}, nonFatalError{err: errors.New("nft_id is empty")}
	}
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

func initializeOwner(owner *base.Owner) {
	owner.Nft_id = ""
	owner.Owner_address = ""
	owner.Quantity = nil
	owner.Collection_id = nil
	owner.First_acquired_date = nil
	owner.Last_acquired_date = nil
	owner.First_acquired_transaction = nil
	owner.Last_acquired_transaction = nil
	owner.Minted_to_this_wallet = nil
	owner.Airdropped_to_this_wallet = nil
	owner.Sold_to_this_wallet = nil
}

func initializeOwnerParams(params *mirrordb.ProcessBaseOwnerEntryParams) {
	params.SimplehashKafkaKey = ""
	params.SimplehashNftID = nil
	params.KafkaOffset = nil
	params.KafkaPartition = nil
	params.KafkaTimestamp = nil
	params.ContractAddress = nil
	params.TokenID = pgtype.Numeric{}
	params.OwnerAddress = nil
	params.Quantity = pgtype.Numeric{}
	params.CollectionID = nil
	params.FirstAcquiredDate = nil
	params.LastAcquiredDate = nil
	params.FirstAcquiredTransaction = nil
	params.LastAcquiredTransaction = nil
	params.MintedToThisWallet = nil
	params.AirdroppedToThisWallet = nil
	params.SoldToThisWallet = nil
	params.ShouldUpsert = false
	params.ShouldDelete = false
}

func parseOwnerMessage(ctx context.Context, deserializer *avro.GenericDeserializer, msg *kafka.Message, owner *base.Owner, params *mirrordb.ProcessBaseOwnerEntryParams) (*mirrordb.ProcessBaseOwnerEntryParams, error) {
	key := string(msg.Key)

	initializeOwner(owner)
	err := deserializer.DeserializeInto(*msg.TopicPartition.Topic, msg.Value, owner)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize owner message with key %s: %w", key, err)
	}

	actionType, err := getActionType(msg)
	if err != nil {
		err = fmt.Errorf("failed to get action type for msg: %v", msg)
		return nil, err
	}

	contractAddress, tokenID, err := parseNftID(owner.Nft_id)
	if err != nil {
		return nil, fmt.Errorf("error parsing NftID: %w", err)
	}

	// TODO: Same as above, we'll want to normalize the address based on the chain at some point
	walletAddress := strings.ToLower(owner.Owner_address)

	quantity, err := parseNumeric(owner.Quantity)
	if err != nil {
		return nil, err
	}

	firstAcquiredDate, err := parseTimestamp(owner.First_acquired_date)
	if err != nil {
		err = fmt.Errorf("failed to parse First_acquired_date: %w", err)
		return nil, err
	}

	lastAcquiredDate, err := parseTimestamp(owner.Last_acquired_date)
	if err != nil {
		err = fmt.Errorf("failed to parse Last_acquired_date: %w", err)
		return nil, err
	}

	initializeOwnerParams(params)

	if actionType == "delete" {
		params.ShouldDelete = true
		params.SimplehashKafkaKey = key

		// pgtype.Numeric defaults to 'undefined' instead of 'null', so we actually need to set these explicitly
		// or else we'll get an error when pgx tries to encode them
		params.TokenID = pgtype.Numeric{Status: pgtype.Null}
		params.Quantity = pgtype.Numeric{Status: pgtype.Null}
	} else {
		if actionType == "insert" || actionType == "update" {
			params.ShouldUpsert = true
			params.SimplehashKafkaKey = key
			params.SimplehashNftID = &owner.Nft_id
			params.KafkaOffset = util.ToPointer(int64(msg.TopicPartition.Offset))
			params.KafkaPartition = util.ToPointer(msg.TopicPartition.Partition)
			params.KafkaTimestamp = util.ToPointer(msg.Timestamp)
			params.ContractAddress = &contractAddress
			params.TokenID = tokenID
			params.OwnerAddress = util.ToPointer(persist.Address(walletAddress))
			params.Quantity = quantity
			params.CollectionID = owner.Collection_id
			params.FirstAcquiredDate = firstAcquiredDate
			params.LastAcquiredDate = lastAcquiredDate
			params.FirstAcquiredTransaction = owner.First_acquired_transaction
			params.LastAcquiredTransaction = owner.Last_acquired_transaction
			params.MintedToThisWallet = owner.Minted_to_this_wallet
			params.AirdroppedToThisWallet = owner.Airdropped_to_this_wallet
			params.SoldToThisWallet = owner.Sold_to_this_wallet
		} else {
			return params, fmt.Errorf("invalid action type: %s", actionType)
		}
	}

	return params, nil
}

func initializeNft(nft *base.Nft) {
	nft.Nft_id = ""
	nft.Chain = nil
	nft.Contract_address = nil
	nft.Token_id = nil
	nft.Name = nil
	nft.Description = nil
	nft.Previews = nil
	nft.Image_url = nil
	nft.Image_properties = nil
	nft.Video_url = nil
	nft.Video_properties = nil
	nft.Audio_url = nil
	nft.Audio_properties = nil
	nft.Model_url = nil
	nft.Model_properties = nil
	nft.Other_url = nil
	nft.Other_properties = nil
	nft.Background_color = nil
	nft.External_url = nil
	nft.Created_date = nil
	nft.Status = nil
	nft.Token_count = nil
	nft.Owner_count = nil
	nft.Contract = nil
	nft.Collection = nil
	nft.Last_sale = nil
	nft.First_created = nil
	nft.Rarity = nil
	nft.Extra_metadata = nil
}

func initializeTokenParams(params *mirrordb.ProcessBaseTokenEntryParams) {
	params.SimplehashNftID = ""
	params.ShouldDelete = false
	params.SimplehashKafkaKey = ""
	params.ContractAddress = nil
	params.ShouldUpsert = false
	params.CollectionID = nil
	params.TokenID = pgtype.Numeric{}
	params.Name = nil
	params.Description = nil
	params.Previews = pgtype.JSONB{}
	params.ImageUrl = nil
	params.VideoUrl = nil
	params.AudioUrl = nil
	params.ModelUrl = nil
	params.OtherUrl = nil
	params.BackgroundColor = nil
	params.ExternalUrl = nil
	params.OnChainCreatedDate = nil
	params.Status = nil
	params.TokenCount = pgtype.Numeric{}
	params.OwnerCount = pgtype.Numeric{}
	params.Contract = pgtype.JSONB{}
	params.LastSale = pgtype.JSONB{}
	params.FirstCreated = pgtype.JSONB{}
	params.Rarity = pgtype.JSONB{}
	params.ExtraMetadata = nil
	params.ImageProperties = pgtype.JSONB{}
	params.VideoProperties = pgtype.JSONB{}
	params.AudioProperties = pgtype.JSONB{}
	params.ModelProperties = pgtype.JSONB{}
	params.OtherProperties = pgtype.JSONB{}
	params.KafkaOffset = nil
	params.KafkaPartition = nil
	params.KafkaTimestamp = nil
	params.ExtraMetadataJsonb = pgtype.JSONB{}
}

func parseTokenMessage(ctx context.Context, deserializer *avro.GenericDeserializer, msg *kafka.Message, nft *base.Nft, params *mirrordb.ProcessBaseTokenEntryParams) (*mirrordb.ProcessBaseTokenEntryParams, error) {
	key := string(msg.Key)

	initializeNft(nft)
	err := deserializer.DeserializeInto(*msg.TopicPartition.Topic, msg.Value, nft)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize token message with key %s: %w", key, err)
	}

	actionType, err := getActionType(msg)
	if err != nil {
		err = fmt.Errorf("failed to get action type for msg: %v", msg)
		return nil, err
	}

	// If the NftID is not set, we can try to construct it from the chain, contract_address, and token_id
	if strings.TrimSpace(nft.Nft_id) == "" && nft.Chain != nil && nft.Contract_address != nil && nft.Token_id != nil {
		nft.Nft_id = fmt.Sprintf("%s.%s.%s", *nft.Chain, *nft.Contract_address, *nft.Token_id)
	}

	contractAddress, tokenID, err := parseNftID(nft.Nft_id)
	if err != nil {
		return nil, fmt.Errorf("error parsing NftID: %w", err)
	}

	previews, err := toJSONB(nft.Previews)
	if err != nil {
		err = fmt.Errorf("failed to convert Previews to JSONB: %w", err)
		return nil, err
	}

	contract, err := toJSONB(nft.Contract)
	if err != nil {
		err = fmt.Errorf("failed to convert Contract to JSONB: %w", err)
		return nil, err
	}

	lastSale, err := toJSONB(nft.Last_sale)
	if err != nil {
		err = fmt.Errorf("failed to convert Last_sale to JSONB: %w", err)
		return nil, err
	}

	firstCreated, err := toJSONB(nft.First_created)
	if err != nil {
		err = fmt.Errorf("failed to convert First_created to JSONB: %w", err)
		return nil, err
	}

	rarity, err := toJSONB(nft.Rarity)
	if err != nil {
		err = fmt.Errorf("failed to convert Rarity to JSONB: %w", err)
		return nil, err
	}

	extraMetadataJsonb, err := cleanJSONB(nft.Extra_metadata, true)
	if err != nil {
		if errors.Is(err, errInvalidJSON) {
			// Log, but don't throw an error. It's not guaranteed that all extra_metadata strings will be valid JSON.
			logger.For(ctx).Errorf("failed to convert Extra_metadata to JSONB for token %s: %v", nft.Nft_id, err)
			extraMetadataJsonb = pgtype.JSONB{Status: pgtype.Null}
		} else {
			err = fmt.Errorf("failed to convert Extra_metadata to JSONB: %w", err)
			return nil, err
		}
	}

	imageProperties, err := toJSONB(nft.Image_properties)
	if err != nil {
		err = fmt.Errorf("failed to convert Image_properties to JSONB: %w", err)
		return nil, err
	}

	videoProperties, err := toJSONB(nft.Video_properties)
	if err != nil {
		err = fmt.Errorf("failed to convert Video_properties to JSONB: %w", err)
		return nil, err
	}

	audioProperties, err := toJSONB(nft.Audio_properties)
	if err != nil {
		err = fmt.Errorf("failed to convert Audio_properties to JSONB: %w", err)
		return nil, err
	}

	modelProperties, err := toJSONB(nft.Model_properties)
	if err != nil {
		err = fmt.Errorf("failed to convert Model_properties to JSONB: %w", err)
		return nil, err
	}

	otherProperties, err := toJSONB(nft.Other_properties)
	if err != nil {
		err = fmt.Errorf("failed to convert Other_properties to JSONB: %w", err)
		return nil, err
	}

	tokenCount, err := parseNumeric(nft.Token_count)
	if err != nil {
		err = fmt.Errorf("failed to parse Token_count: %w", err)
		return nil, err
	}

	ownerCount, err := parseNumeric(nft.Owner_count)
	if err != nil {
		err = fmt.Errorf("failed to parse Owner_count: %w", err)
		return nil, err
	}

	var collectionID *string
	if nft.Collection != nil {
		collectionID = nft.Collection.Collection_id
	}

	onChainCreatedDate, err := parseTimestamp(nft.Created_date)
	if err != nil {
		err = fmt.Errorf("failed to parse Created_date: %w", err)
		return nil, err
	}

	initializeTokenParams(params)

	if actionType == "delete" {
		params.ShouldDelete = true
		params.SimplehashKafkaKey = key

		// pgtype.Numeric and pgtype.JSONB default to 'undefined' instead of 'null', so we actually need to
		// set these explicitly or else we'll get an error when pgx tries to encode them.
		params.TokenID = pgtype.Numeric{Status: pgtype.Null}
		params.TokenCount = pgtype.Numeric{Status: pgtype.Null}
		params.OwnerCount = pgtype.Numeric{Status: pgtype.Null}
		params.Previews = pgtype.JSONB{Status: pgtype.Null}
		params.Contract = pgtype.JSONB{Status: pgtype.Null}
		params.LastSale = pgtype.JSONB{Status: pgtype.Null}
		params.FirstCreated = pgtype.JSONB{Status: pgtype.Null}
		params.Rarity = pgtype.JSONB{Status: pgtype.Null}
		params.ImageProperties = pgtype.JSONB{Status: pgtype.Null}
		params.VideoProperties = pgtype.JSONB{Status: pgtype.Null}
		params.AudioProperties = pgtype.JSONB{Status: pgtype.Null}
		params.ModelProperties = pgtype.JSONB{Status: pgtype.Null}
		params.OtherProperties = pgtype.JSONB{Status: pgtype.Null}
	} else {
		if actionType == "insert" || actionType == "update" {
			params.ShouldUpsert = true
			params.SimplehashKafkaKey = key
			params.SimplehashNftID = nft.Nft_id
			params.ContractAddress = util.ToPointer(contractAddress.String())
			params.TokenID = tokenID
			params.Name = cleanString(nft.Name)
			params.Description = cleanString(nft.Description)
			params.Previews = previews
			params.ImageUrl = cleanString(nft.Image_url)
			params.VideoUrl = cleanString(nft.Video_url)
			params.AudioUrl = cleanString(nft.Audio_url)
			params.ModelUrl = cleanString(nft.Model_url)
			params.OtherUrl = cleanString(nft.Other_url)
			params.BackgroundColor = cleanString(nft.Background_color)
			params.ExternalUrl = cleanString(nft.External_url)
			params.OnChainCreatedDate = onChainCreatedDate
			params.Status = cleanString(nft.Status)
			params.TokenCount = tokenCount
			params.OwnerCount = ownerCount
			params.Contract = contract
			params.CollectionID = collectionID
			params.LastSale = lastSale
			params.FirstCreated = firstCreated
			params.Rarity = rarity
			params.ExtraMetadata = cleanString(nft.Extra_metadata)
			params.ExtraMetadataJsonb = extraMetadataJsonb
			params.ImageProperties = imageProperties
			params.VideoProperties = videoProperties
			params.AudioProperties = audioProperties
			params.ModelProperties = modelProperties
			params.OtherProperties = otherProperties
			params.KafkaOffset = util.ToPointer(int64(msg.TopicPartition.Offset))
			params.KafkaPartition = util.ToPointer(msg.TopicPartition.Partition)
			params.KafkaTimestamp = util.ToPointer(msg.Timestamp)
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
		go ccf.FillWithNFTIDs(idsToUpdate)
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

	// No need to verify that the result is valid JSON, since we just marshaled it ourselves
	return cleanJSONB(util.ToPointer(string(jsonData)), false)
}

// cleanJSONB takes a pointer to a string data representing JSON and returns a pgtype.JSONB.
// It also cleans invalid characters and optionally ensures that the output is valid JSON.
func cleanJSONB(data *string, validate bool) (pgtype.JSONB, error) {
	if data == nil {
		return pgtype.JSONB{Status: pgtype.Null}, nil
	}

	// Strip out any literal null bytes
	jsonStr := strings.ReplaceAll(*data, "\x00", "")

	// Strip out any escaped null characters in JSON
	cleanedStr := strings.ReplaceAll(jsonStr, "\\u0000", "")

	if validate {
		// Unmarshal the data into a generic map to make sure that it's valid JSON
		var m map[string]interface{}
		err := json.Unmarshal([]byte(cleanedStr), &m)
		if err != nil {
			return pgtype.JSONB{Status: pgtype.Null}, errInvalidJSON
		}
	}

	var jsonb pgtype.JSONB
	// Convert the cleaned string back to bytes and set it to jsonb
	err := jsonb.Set([]byte(cleanedStr))
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

type contractCollectionFiller struct {
	nftIDBatcher        *batch.Batcher[string, bool]
	collectionIDBatcher *batch.Batcher[string, bool]
}

func (c *contractCollectionFiller) FillWithNFTIDs(nftIDs []string) ([]bool, []error) {
	return c.nftIDBatcher.DoAll(nftIDs)
}

func (c *contractCollectionFiller) FillWithCollectionIDs(collectionIDs []string) ([]bool, []error) {
	return c.collectionIDBatcher.DoAll(collectionIDs)
}

func newContractCollectionFiller(ctx context.Context, pgx *pgxpool.Pool) *contractCollectionFiller {
	httpClient := http.DefaultClient
	queries := mirrordb.New(pgx)

	fillByNFTIDs := func(ctx context.Context, nftIDs []string) ([]bool, []error) {
		// Fetch the NFTs from SimpleHash
		nfts, err := rest.GetSimpleHashNFTs(ctx, httpClient, nftIDs)
		if err != nil {
			return nil, []error{err}
		}

		ethContractParams := make([]mirrordb.UpdateEthereumContractParams, 0, len(nfts))
		baseContractParams := make([]mirrordb.UpdateBaseContractParams, 0, len(nfts))
		baseSepoliaContractParams := make([]mirrordb.UpdateBaseSepoliaContractParams, 0, len(nfts))
		zoraContractParams := make([]mirrordb.UpdateZoraContractParams, 0, len(nfts))
		collectionParams := make([]mirrordb.UpdateCollectionParams, 0, len(nfts))

		for _, nft := range nfts {
			nft.Normalize()

			if nft.Contract != nil && nft.Chain != nil && nft.ContractAddress != nil {
				p := contractToParams(ctx, *nft.ContractAddress, *nft.Contract)

				switch *nft.Chain {
				case "ethereum":
					ethContractParams = append(ethContractParams, p)
				case "base":
					baseContractParams = append(baseContractParams, mirrordb.UpdateBaseContractParams(p))
				case "base-sepolia":
					baseSepoliaContractParams = append(baseSepoliaContractParams, mirrordb.UpdateBaseSepoliaContractParams(p))
				case "zora":
					zoraContractParams = append(zoraContractParams, mirrordb.UpdateZoraContractParams(p))
				}
			}

			if nft.Collection != nil && nft.Collection.CollectionID != nil {
				collectionParams = append(collectionParams, collectionToParams(ctx, *nft.Collection))
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

		if len(baseSepoliaContractParams) > 0 {
			err = submitExecBatch(ctx, queries.UpdateBaseSepoliaContract, baseSepoliaContractParams)
			if err != nil {
				logger.For(ctx).Errorf("failed to update Base-Sepolia contracts: %v", err)
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

	fillByCollectionIDs := func(ctx context.Context, collectionIDs []string) ([]bool, []error) {
		// Fetch the collections from SimpleHash
		collections, err := rest.GetSimpleHashCollections(ctx, httpClient, collectionIDs)
		if err != nil {
			return nil, []error{err}
		}

		collectionParams := make([]mirrordb.UpdateCollectionParams, 0, len(collections))

		for _, collection := range collections {
			collection.Normalize()

			if collection.CollectionID != nil {
				collectionParams = append(collectionParams, collectionToParams(ctx, collection))
			}
		}

		if readOnlyMode {
			return nil, nil
		}

		// Handle any collections that SimpleHash no longer recognizes
		if len(collectionParams) != len(collectionIDs) {
			collectionsToDelete := make([]string, 0, len(collectionIDs))
			for _, id := range collectionIDs {
				found := false
				for _, c := range collections {
					if c.CollectionID != nil && *c.CollectionID == id {
						found = true
						break
					}
				}

				if !found {
					collectionsToDelete = append(collectionsToDelete, id)
				}
			}

			if len(collectionsToDelete) > 0 {
				err = submitExecBatch(ctx, queries.SetCollectionSimpleHashDeleted, collectionsToDelete)
				if err != nil {
					logger.For(ctx).Errorf("failed to delete collections: %v", err)
				}
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

	return &contractCollectionFiller{
		nftIDBatcher:        batch.NewBatcher(ctx, 25, 1*time.Second, false, false, fillByNFTIDs),
		collectionIDBatcher: batch.NewBatcher(ctx, 20, 1*time.Second, false, false, fillByCollectionIDs),
	}
}

func contractToParams(ctx context.Context, address string, contract rest.Contract) mirrordb.UpdateEthereumContractParams {
	return mirrordb.UpdateEthereumContractParams{
		Address:                address,
		Type:                   contract.Type,
		Name:                   cleanString(contract.Name),
		Symbol:                 cleanString(contract.Symbol),
		DeployedBy:             cleanString(contract.DeployedBy),
		DeployedViaContract:    cleanString(contract.DeployedViaContract),
		OwnedBy:                cleanString(contract.OwnedBy),
		HasMultipleCollections: contract.HasMultipleCollections,
	}
}

func collectionToParams(ctx context.Context, collection rest.Collection) mirrordb.UpdateCollectionParams {
	markerplacePages, err := toJSONB(&collection.MarketplacePages)
	if err != nil {
		logger.For(ctx).Errorf("failed to convert MarketplacePages to JSONB: %v", err)
		markerplacePages = pgtype.JSONB{Status: pgtype.Null}
	}

	collectionRoyalties, err := toJSONB(&collection.CollectionRoyalties)
	if err != nil {
		logger.For(ctx).Errorf("failed to convert CollectionRoyalties to JSONB: %v", err)
		collectionRoyalties = pgtype.JSONB{Status: pgtype.Null}
	}

	return mirrordb.UpdateCollectionParams{
		CollectionID:                 *collection.CollectionID,
		Name:                         cleanString(collection.Name),
		Description:                  cleanString(collection.Description),
		ImageUrl:                     cleanString(collection.ImageUrl),
		BannerImageUrl:               cleanString(collection.BannerImageUrl),
		Category:                     cleanString(collection.Category),
		IsNsfw:                       collection.IsNsfw,
		ExternalUrl:                  cleanString(collection.ExternalUrl),
		TwitterUsername:              cleanString(collection.TwitterUsername),
		DiscordUrl:                   cleanString(collection.DiscordUrl),
		InstagramUrl:                 cleanString(collection.InstagramUrl),
		MediumUsername:               cleanString(collection.MediumUsername),
		TelegramUrl:                  cleanString(collection.TelegramUrl),
		MarketplacePages:             markerplacePages,
		MetaplexMint:                 cleanString(collection.MetaplexMint),
		MetaplexCandyMachine:         cleanString(collection.MetaplexCandyMachine),
		MetaplexFirstVerifiedCreator: cleanString(collection.MetaplexFirstVerifiedCreator),
		SpamScore:                    collection.SpamScore,
		Chains:                       collection.Chains,
		TopContracts:                 collection.TopContracts,
		CollectionRoyalties:          collectionRoyalties,
	}
}

func fillMissing(ctx context.Context, queryF func(context.Context) ([]string, error), fillF func([]string) ([]bool, []error)) {
	for {
		ids, err := queryF(ctx)
		if err != nil {
			err = fmt.Errorf("failed to get IDs for missing contracts/collections: %w", err)
		} else {
			for len(ids) > 0 {
				_, errs := fillF(ids)
				if err = getFirstNonNilError(errs); err != nil {
					err = fmt.Errorf("failed to fill missing contracts/collections: %v", err)
					logger.For(ctx).Error(err)
					break
				}

				ids, err = queryF(ctx)
				if err != nil {
					err = fmt.Errorf("failed to get IDs for missing contracts/collections: %w", err)
					logger.For(ctx).Error(err)
					break
				}
			}
		}

		time.Sleep(2 * time.Minute)
	}
}

func fillMissingContracts(ctx context.Context, queries *mirrordb.Queries, ccf *contractCollectionFiller) {
	fillMissing(ctx, queries.GetNFTIDsForMissingContracts, ccf.FillWithNFTIDs)
}

func fillMissingCollections(ctx context.Context, queries *mirrordb.Queries, ccf *contractCollectionFiller) {
	fillMissing(ctx, queries.GetCollectionIDsForMissingCollections, ccf.FillWithCollectionIDs)
}

func getFirstNonNilError(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

type nonFatalError struct {
	err error
}

func (e nonFatalError) Error() string {
	return e.err.Error()
}

func (e nonFatalError) Unwrap() error {
	return e.err
}
