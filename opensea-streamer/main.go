package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/sirupsen/logrus"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/persist/postgres"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

const (
	itemTransferredEventType = "item_transferred"

	// At this false positive rate (1 out of every 1,000,000), the bloom filter uses approximately 3-4 bytes per wallet.
	falsePositiveRate        = 0.000001
	numConcurrentConnections = 10
	numConcurrentSubscribers = 2

	refIDHeartbeat = 0
	refIDSubscribe = 1
)

// TODO add more chains here
// https://docs.opensea.io/reference/supported-chains#mainnets
var enabledChains = map[string]bool{
	"base": true,
	"zora": true,
}

type phoenixEvent struct {
	Event   string `json:"event"`
	Payload struct {
		Status string `json:"status"`
	} `json:"payload"`
	Ref   int    `json:"ref"`
	Topic string `json:"topic"`
}

type openseaEvent struct {
	Event   string                      `json:"event"`
	Payload persist.OpenSeaWebhookInput `json:"payload"`
}

var bloomFilter atomic.Pointer[bloom.BloomFilter]

var mapLock = sync.Mutex{}
var seenEvents = make(map[string]bool)

func main() {
	setDefaults()
	initSentry()
	router := gin.Default()

	logger.InitWithGCPDefaults()

	pgx := postgres.NewPgxClient()
	queries := coredb.New(pgx)

	ctx := context.Background()
	taskClient := task.NewClient(ctx)

	err := generateBloomFilter(ctx, queries)
	if err != nil {
		panic(err)
	}

	// Health endpoint
	router.GET("/health", util.HealthCheckHandler())

	router.GET("/updateBloomFilter", func(c *gin.Context) {
		err := generateBloomFilter(c, queries)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.String(http.StatusOK, "OK")
	})

	go func() {
		// update the bloom filter every 15 minutes
		for {
			time.Sleep(15 * time.Minute)
			logger.For(ctx).Info("updating bloom filter...")

			err := generateBloomFilter(ctx, queries)
			if err != nil {
				err := fmt.Errorf("error updating bloom filter: %w", err)
				logger.For(ctx).Error(err)
				sentryutil.ReportError(ctx, err)
			}
		}
	}()

	cm := newConnectionManager(ctx, taskClient, numConcurrentConnections, numConcurrentSubscribers)
	go cm.start()

	err = router.Run(":3000")
	if err != nil {
		err = fmt.Errorf("error running router: %w", err)
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
		panic(err)
	}
}

func hashMessage(messageBytes []byte) string {
	hasher := sha256.New()
	hasher.Write(messageBytes)
	hashBytes := hasher.Sum(nil)
	return string(hashBytes)
}

// addSeenEvent adds the event to the seenEvents map and returns false if the event was already seen,
// true otherwise
func addSeenEvent(messageBytes []byte) bool {
	key := hashMessage(messageBytes)
	mapLock.Lock()
	defer mapLock.Unlock()
	if _, ok := seenEvents[key]; ok {
		return false
	}

	seenEvents[key] = true

	// Remove the event from the map after 30 seconds. We don't need to hold onto the keys forever;
	// we just want to deduplicate events across our websocket streams.
	go func() {
		time.Sleep(30 * time.Second)
		mapLock.Lock()
		defer mapLock.Unlock()
		delete(seenEvents, key)
	}()

	return true
}

func dispatchToTokenProcessing(ctx context.Context, taskClient *task.Client, payload persist.OpenSeaWebhookInput) {
	err := taskClient.CreateTaskForOpenseaStreamerTokenProcessing(ctx, payload)
	if err != nil {
		err = fmt.Errorf("error creating task for token processing: %w", err)
		logger.For(ctx).Error(err)
		sentryutil.ReportError(ctx, err)
	}
}

func sendHeartbeat(ctx context.Context, conn *websocket.Conn) {
	heartbeat := map[string]interface{}{
		"topic":   "phoenix",
		"event":   "heartbeat",
		"payload": map[string]interface{}{},
		"ref":     refIDHeartbeat,
	}

	err := conn.WriteJSON(heartbeat)
	if err != nil {
		logger.For(ctx).Errorf("error sending heartbeat: %s", err)
	}
}

type StateChange struct {
	ConnectionID int
	State        ConnectionState
}

type SubscriptionTimeout struct {
	ConnectionID int
	ReferenceID  uuid.UUID
}

type ConnectionState int

const (
	Connecting ConnectionState = iota
	Connected
	Subscribed
)

func (s ConnectionState) String() string {
	switch s {
	case Connecting:
		return "connecting"
	case Connected:
		return "connected"
	case Subscribed:
		return "subscribed"
	default:
		return "unknown"
	}
}

type connectionManager struct {
	ctx                 context.Context
	taskClient          *task.Client
	maxSubscribers      int
	started             bool
	timeouts            chan SubscriptionTimeout
	incoming            chan StateChange
	connections         []chan StateChange
	remoteStates        map[ConnectionState]map[int]bool
	pendingRemoteStates map[ConnectionState]map[int]uuid.UUID
}

func newConnectionManager(ctx context.Context, taskClient *task.Client, numConcurrentConnections int, numConcurrentSubscribers int) *connectionManager {
	c := &connectionManager{
		ctx:            ctx,
		taskClient:     taskClient,
		maxSubscribers: numConcurrentSubscribers,
		timeouts:       make(chan SubscriptionTimeout),
		incoming:       make(chan StateChange),
		connections:    make([]chan StateChange, numConcurrentConnections),
		remoteStates:   map[ConnectionState]map[int]bool{},
	}

	c.remoteStates = map[ConnectionState]map[int]bool{
		Connecting: {},
		Connected:  {},
		Subscribed: {},
	}

	c.pendingRemoteStates = map[ConnectionState]map[int]uuid.UUID{
		Connecting: {},
		Connected:  {},
		Subscribed: {},
	}

	return c
}

func (m *connectionManager) start() {
	if m.started {
		logger.For(m.ctx).Error("connection manager already started")
		return
	}

	go m.managerLoop()

	connections := make([]*connection, len(m.connections))

	for i := 0; i < len(m.connections); i++ {
		outgoing := make(chan StateChange)
		m.connections[i] = outgoing
		m.setRemoteState(i, Connecting)
		connections[i] = newConnection(m.incoming, outgoing, m.taskClient, i)
	}

	for i, c := range connections {
		if i != 0 {
			// Add a delay between starting each connection
			time.Sleep(17 * time.Second)
		}

		c.start()
	}
}

func (m *connectionManager) managerLoop() {
	// Disconnect a random connection every hour to keep the connection pool fresh
	randomDisconnectionTicker := time.NewTicker(1 * time.Hour)

	for {
		select {
		case stateChange := <-m.incoming:
			switch stateChange.State {
			case Connecting:
				m.onConnecting(stateChange.ConnectionID)
			case Connected:
				m.onConnected(stateChange.ConnectionID)
			case Subscribed:
				m.onSubscribed(stateChange.ConnectionID)
			}
		case timeout := <-m.timeouts:
			m.onTimeout(timeout.ConnectionID, timeout.ReferenceID)
		case <-randomDisconnectionTicker.C:
			m.disconnectRandomConnection()
		}
	}
}

func (m *connectionManager) disconnectRandomConnection() {
	connectionIDs := make([]int, 0, len(m.remoteStates[Connected]))
	for id := range m.remoteStates[Connected] {
		connectionIDs = append(connectionIDs, id)
	}

	randomID := connectionIDs[rand.Intn(len(connectionIDs))]
	logger.For(m.ctx).Infof("disconnecting random connection (id=%d)", randomID)
	m.requestStateChange(randomID, Connecting)
}

func (m *connectionManager) onConnecting(connectionID int) {
	previousNumSubscribers := len(m.remoteStates[Subscribed])
	m.setRemoteState(connectionID, Connecting)
	currentNumSubscribers := len(m.remoteStates[Subscribed])

	if previousNumSubscribers > 0 && currentNumSubscribers == 0 {
		logger.For(m.ctx).Warnf("no active subscribers! events will be missed until a subscription is re-established.")
	}

	m.updateSubscribers()
}

func (m *connectionManager) onConnected(connectionID int) {
	wasConnecting := m.hasRemoteState(connectionID, Connecting)
	m.setRemoteState(connectionID, Connected)

	if !wasConnecting {
		logger.For(m.ctx).Warnf("unexpected state transition to \"connected\" for connection %d", connectionID)
		m.requestStateChange(connectionID, Connecting)
		return
	}

	m.updateSubscribers()
}

func (m *connectionManager) onSubscribed(connectionID int) {
	wasConnected := m.hasRemoteState(connectionID, Connected)
	m.setRemoteState(connectionID, Subscribed)

	if !wasConnected {
		logger.For(m.ctx).Warnf("unexpected state transition to \"subscribed\" for connection %d", connectionID)
		m.requestStateChange(connectionID, Connecting)
		return
	}

	m.updateSubscribers()
}

func (m *connectionManager) onTimeout(connectionID int, refID uuid.UUID) {
	// If the subscription is still pending when the timeout message is received,
	// subscribing took too long and we should force a reconnection
	if pendingRefID, ok := m.pendingRemoteStates[Subscribed][connectionID]; ok && pendingRefID == refID {
		m.requestStateChange(connectionID, Connecting)
	}
}

func (m *connectionManager) hasRemoteState(connectionID int, state ConnectionState) bool {
	_, ok := m.remoteStates[state][connectionID]
	return ok
}

func (m *connectionManager) setRemoteState(connectionID int, state ConnectionState) {
	// Remove the connection from its existing state map
	for _, stateMap := range m.remoteStates {
		delete(stateMap, connectionID)
	}

	// Clear out pending states, too, since we've gotten an actual state
	for _, stateMap := range m.pendingRemoteStates {
		delete(stateMap, connectionID)
	}

	// Add the connection to its new state map
	m.remoteStates[state][connectionID] = true
}

func (m *connectionManager) requestStateChange(connectionID int, state ConnectionState) uuid.UUID {
	logger.For(m.ctx).Infof("requesting state change for connection %d to %s", connectionID, state)

	m.connections[connectionID] <- StateChange{
		ConnectionID: connectionID,
		State:        state,
	}

	randomID := uuid.New()
	m.pendingRemoteStates[state][connectionID] = randomID

	return randomID
}

func (m *connectionManager) updateSubscribers() {
	numRequiredSubscribers := m.maxSubscribers - (len(m.remoteStates[Subscribed]) + len(m.pendingRemoteStates[Subscribed]))
	logger.For(m.ctx).Infof("required subscribers: %d, remoteStates[Subscribed]: %v, pendingRemoteStates[Subscribed]: %v", numRequiredSubscribers, m.remoteStates[Subscribed], m.pendingRemoteStates[Subscribed])

	// If we have enough subscribers (including pending subscribers), we don't need to do anything
	if numRequiredSubscribers <= 0 {
		return
	}

	// If we don't have enough subscribers, we need to promote some connections from connected to subscribing
	for connectionID := range m.remoteStates[Connected] {
		if numRequiredSubscribers == 0 {
			break
		}

		// Skip connections that are already attempting to subscribe
		if _, ok := m.pendingRemoteStates[Subscribed][connectionID]; ok {
			continue
		}

		refID := m.requestStateChange(connectionID, Subscribed)
		numRequiredSubscribers--

		go func(cID int, rID uuid.UUID) {
			time.Sleep(5 * time.Second)
			m.timeouts <- SubscriptionTimeout{
				ConnectionID: cID,
				ReferenceID:  rID,
			}
		}(connectionID, refID)
	}
}

type connection struct {
	ctx                 context.Context
	state               ConnectionState
	toManager           chan StateChange
	fromManager         chan StateChange
	fromListener        chan StateChange
	conn                *websocket.Conn
	taskClient          *task.Client
	madeFirstConnection bool
	connectionID        int
	lastEventReceived   atomic.Pointer[time.Time]
}

func newConnection(toManager chan StateChange, fromManager chan StateChange, taskClient *task.Client, connectionID int) *connection {
	c := &connection{
		ctx:          context.Background(),
		toManager:    toManager,
		fromManager:  fromManager,
		fromListener: make(chan StateChange),
		taskClient:   taskClient,
		connectionID: connectionID,
	}

	c.lastEventReceived.Store(util.ToPointer(time.Now()))

	c.ctx = logger.NewContextWithFields(c.ctx, logrus.Fields{
		"connectionID": connectionID,
	})

	return c
}

func (c *connection) start() {
	go c.connectionLoop()
	c.startNewListener()
}

func openWebsocket(ctx context.Context, reconnecting bool) *websocket.Conn {
	apiKey := env.GetString("OPENSEA_API_KEY")
	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	for {
		if reconnecting {
			// Add a random delay to reconnections in an attempt to keep all connections from ending up
			// on the same backend server. Hopefully we can distribute them among backend servers so
			// we don't lose all of our connections when one backend node goes down.
			time.Sleep(time.Duration(random.Intn(60)) * time.Second)
		}

		dialer := websocket.DefaultDialer

		// Set a timeout to ensure that connection attempts never hang indefinitely
		dialCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)

		conn, _, err := dialer.DialContext(dialCtx, "wss://stream.openseabeta.com/socket/websocket?token="+apiKey, nil)
		if err == nil {
			cancel()
			return conn
		}

		cancel()
		logger.For(ctx).Errorf("error connecting to OpenSea: %s", err)
		reconnecting = true
	}
}

func (c *connection) subscribe() error {
	logger.For(c.ctx).Infof("subscribing to events...")

	// Subscribe to events
	subscribeMessage := map[string]interface{}{
		"topic":   "collection:*",
		"event":   "phx_join",
		"payload": map[string]interface{}{},
		"ref":     refIDSubscribe,
	}

	if err := c.conn.WriteJSON(subscribeMessage); err != nil {
		logger.For(c.ctx).Errorf("error subscribing to events: %s", err)
		return err
	}

	return nil
}

func (c *connection) connectionLoop() {
	// Per OpenSea docs, we need to send a heartbeat every 30 seconds
	heartbeat := time.NewTicker(30 * time.Second)
	lastEventCheck := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-heartbeat.C:
			if c.state != Connecting {
				sendHeartbeat(c.ctx, c.conn)
			}
		case <-lastEventCheck.C:
			c.checkLastEventTime()
		case stateChange := <-c.fromManager:
			switch stateChange.State {
			case Connecting:
				c.onManagerRequestedConnecting()
			case Subscribed:
				c.onManagerRequestedSubscribed()
			default:
				logger.For(c.ctx).Errorf("received unexpected state change: %d", stateChange.State)
			}
		case stateChange := <-c.fromListener:
			switch stateChange.State {
			case Connecting:
				c.onListenerRequestedConnecting()
			case Subscribed:
				c.onListenerSubscribed()
			default:
				logger.For(c.ctx).Errorf("received unexpected state change: %d", stateChange.State)
			}
		}
	}
}

func (c *connection) startNewListener() {
	if c.conn != nil {
		logger.For(c.ctx).Info("closing existing connection")
		c.conn.Close()
	}

	logger.For(c.ctx).Info("connecting to OpenSea...")
	c.setState(Connecting)

	c.fromListener = make(chan StateChange)

	c.conn = openWebsocket(c.ctx, c.madeFirstConnection)
	c.madeFirstConnection = true

	logger.For(c.ctx).Info("connected to OpenSea")
	c.setState(Connected)

	// Update the lastEventReceived time so it's not uninitialized or stale from a previous listener
	c.lastEventReceived.Store(util.ToPointer(time.Now()))

	go listenerLoop(c.ctx, c.connectionID, c.conn, c.taskClient, c.fromListener, &c.lastEventReceived)
}

func (c *connection) setState(state ConnectionState) {
	logger.For(c.ctx).Infof("setting state to %s", state)
	c.state = state
	c.toManager <- StateChange{
		ConnectionID: c.connectionID,
		State:        c.state,
	}
}

func (c *connection) onManagerRequestedConnecting() {
	if c.state == Connecting {
		return
	}

	c.startNewListener()
}

func (c *connection) onManagerRequestedSubscribed() {
	if c.state != Connected {
		return
	}

	err := c.subscribe()
	if err != nil {
		c.startNewListener()
	}
}

func (c *connection) onListenerRequestedConnecting() {
	c.startNewListener()
}

func (c *connection) onListenerSubscribed() {
	logger.For(c.ctx).Infof("subscribed to events")
	c.setState(Subscribed)
}

func (c *connection) checkLastEventTime() {
	lastEventTime := c.lastEventReceived.Load()
	timeSince := time.Since(*lastEventTime)

	if c.state == Subscribed && timeSince > (10*time.Second) {
		// Subscribers should receive many events per second (though we filter out most of them). If we haven't seen
		// any events at all in 10 seconds, something is probably wrong, and we should reconnect.
		logger.For(c.ctx).Warnf("subscriber hasn't received any events in %v, reconnecting...", timeSince)
		c.startNewListener()
	} else if c.state == Connected && timeSince > (2*time.Minute) {
		// Non-subscribed connections should at least receive a heartbeat response every 30 seconds. If we go 2 minutes
		// without seeing any events, something is probably wrong, and we should reconnect.
		logger.For(c.ctx).Warnf("connection hasn't received any events in %v, reconnecting...", timeSince)
		c.startNewListener()
	}
}

func listenerLoop(ctx context.Context, connectionID int, conn *websocket.Conn, taskClient *task.Client, toConnection chan StateChange, lastEventReceived *atomic.Pointer[time.Time]) {
	defer conn.Close()
	defer close(toConnection)

	// Listen for messages
	for {
		t, message, err := conn.ReadMessage()
		if err != nil {
			logger.For(ctx).Errorf("error while reading messages: %s", err)
			toConnection <- StateChange{
				ConnectionID: connectionID,
				State:        Connecting,
			}
			break
		}

		if t != 1 {
			logger.For(ctx).Warnf("received message with type %d and payload: %s", t, message)
		}

		// Keep track of when the last event was received
		lastEventReceived.Store(util.ToPointer(time.Now()))

		pe := phoenixEvent{}
		err = json.Unmarshal(message, &pe)
		if err != nil {
			logger.For(ctx).Errorf("error unmarshaling phoenix event: %s. Message is: %s", err, string(message))
			continue
		}

		if pe.Event == "phx_reply" {
			if pe.Payload.Status == "ok" && pe.Ref == refIDSubscribe {
				toConnection <- StateChange{
					ConnectionID: connectionID,
					State:        Subscribed,
				}
				continue
			}
		}

		var oe openseaEvent
		err = json.Unmarshal(message, &oe)
		if err != nil {
			// No need to log these errors; they're expected and spammy. But if we see any other errors, we want to know about them!
			errStr := err.Error()
			if !strings.HasPrefix(errStr, "invalid opensea chain") &&
				!strings.HasPrefix(errStr, "json: cannot unmarshal object into Go struct field .payload.payload.chain of type string") &&
				!strings.HasPrefix(errStr, "json: cannot unmarshal number") {
				logger.For(ctx).Errorf("unmarshaling error: %s", err)
			}
			continue
		}

		win := oe.Payload

		if win.EventType != itemTransferredEventType {
			continue
		}

		if !enabledChains[win.Payload.Chain] {
			continue
		}

		// check if the wallet is in the bloom filter
		chainAddress, err := persist.NewL1ChainAddress(persist.Address(win.Payload.ToAccount.Address.String()), win.Payload.Item.NFTID.Chain).MarshalJSON()
		if err != nil {
			err = fmt.Errorf("error marshaling chain address: %w", err)
			logger.For(ctx).Error(err)
			continue
		}

		if !bloomFilter.Load().Test(chainAddress) {
			continue
		}

		reportCtx := logger.NewContextWithFields(ctx, logrus.Fields{
			"eventSizeBytes": len(message),
			"eventType":      win.EventType,
			"contract":       win.Payload.Item.NFTID.ContractAddress.String(),
			"tokenID":        win.Payload.Item.NFTID.TokenID.String(),
			"walletAddress":  win.Payload.ToAccount.Address.String(),
			"chain":          win.Payload.Item.NFTID.Chain,
		})

		logger.For(reportCtx).Infof("received user item transfer event for token (contract=%s, tokenID=%s) transferred to wallet %s on chain %d",
			win.Payload.Item.NFTID.ContractAddress.String(), win.Payload.Item.NFTID.TokenID.String(), win.Payload.ToAccount.Address.String(), win.Payload.Item.NFTID.Chain)

		if !addSeenEvent(message) {
			continue
		}

		// send to token processing service
		go dispatchToTokenProcessing(ctx, taskClient, win)
	}
}

func generateBloomFilter(ctx context.Context, q *coredb.Queries) error {
	wallets, err := q.GetActiveWallets(ctx)
	if err != nil {
		return err
	}

	logger.For(ctx).Infof("resetting bloom filter with %d wallets", len(wallets))

	bfp := bloom.NewWithEstimates(uint(len(wallets)), falsePositiveRate)
	for _, w := range wallets {
		chainAddress, err := persist.NewL1ChainAddress(w.Address, w.Chain).MarshalJSON()
		if err != nil {
			return err
		}
		bfp.Add(chainAddress)
	}

	buf := &bytes.Buffer{}
	_, err = bfp.WriteTo(buf)
	if err != nil {
		return err
	}

	bloomFilter.Store(bfp)

	return nil
}

func setDefaults() {
	viper.SetDefault("ENV", "local")
	viper.SetDefault("POSTGRES_HOST", "0.0.0.0")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "gallery_backend")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "postgres")
	viper.SetDefault("REDIS_URL", "localhost:6379")
	viper.SetDefault("OPENSEA_API_KEY", "")
	viper.SetDefault("WEBHOOK_TOKEN", "")
	viper.SetDefault("TOKEN_PROCESSING_URL", "http://localhost:6500")
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
		envFile := util.ResolveEnvFile("opensea-streamer", fi)
		util.LoadEncryptedEnvFile(envFile)
	}

	if env.GetString("ENV") != "local" {
		util.VarNotSetTo("SENTRY_DSN", "")
	}
}

func initSentry() {
	if env.GetString("ENV") == "local" {
		logger.For(nil).Info("skipping sentry init")
		return
	}

	logger.For(nil).Info("initializing sentry...")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              env.GetString("SENTRY_DSN"),
		Environment:      env.GetString("ENV"),
		TracesSampleRate: env.GetFloat64("SENTRY_TRACES_SAMPLE_RATE"),
		Release:          env.GetString("GAE_VERSION"),
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			event = auth.ScrubEventCookies(event, hint)
			return event
		},
	})

	if err != nil {
		logger.For(nil).Fatalf("failed to start sentry: %s", err)
	}
}
