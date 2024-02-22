package tokenmanage

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/limiters"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/metric"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/redis"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/service/throttle"
	"github.com/mikeydub/go-gallery/util"
)

// ErrBadToken is an error indicating that there is an issue with the token itself
type ErrBadToken struct{ Err error }

func (e ErrBadToken) Unwrap() error { return e.Err }
func (e ErrBadToken) Error() string { return fmt.Sprintf("issue with token: %s", e.Err) }

// ErrContractPaused indicates that runs for this contract have been paused
type ErrContractPaused struct {
	Chain    persist.Chain
	Contract persist.Address
}

func (e ErrContractPaused) Error() string {
	return fmt.Sprintf("processing for chain=%d; contract=%s is paused", e.Chain, e.Contract)
}

// NumRetryF is a function that returns the number of times a token can be re-enqueued before it is not retried again
type NumRetryF func() int

type Manager struct {
	cache           *redis.Cache
	processRegistry *registry
	taskClient      *task.Client
	throttle        *throttle.Locker
	taskDelayer     *limiters.KeyRateLimiter
	errorCounter    *limiters.KeyRateLimiter
	numRetryF       NumRetryF
	metricReporter  metric.MetricReporter
}

func New(ctx context.Context, taskClient *task.Client, cache *redis.Cache) *Manager {
	return &Manager{
		cache:           cache,
		processRegistry: &registry{cache},
		taskClient:      taskClient,
		throttle:        throttle.NewThrottleLocker(cache, 30*time.Minute),
		metricReporter:  metric.NewLogMetricReporter(),
		errorCounter:    limiters.NewKeyRateLimiter(ctx, cache, "errorCount", 100, 3*time.Hour),
	}
}

func NewWithRetries(ctx context.Context, taskClient *task.Client, cache *redis.Cache, numRetryF NumRetryF) *Manager {
	m := New(ctx, taskClient, cache)
	m.numRetryF = numRetryF
	m.taskDelayer = limiters.NewKeyRateLimiter(ctx, m.cache, "retry", 2, 1*time.Minute)
	return m
}

// Processing returns true if the token is processing or enqueued.
func (m Manager) Processing(ctx context.Context, tDefID persist.DBID) bool {
	p, _ := m.processRegistry.processing(ctx, tDefID)
	return p
}

// SubmitBatch enqueues tokens for processing.
func (m Manager) SubmitBatch(ctx context.Context, tDefIDs []persist.DBID) error {
	if len(tDefIDs) == 0 {
		return nil
	}
	m.processRegistry.setManyEnqueue(ctx, tDefIDs)
	message := task.TokenProcessingBatchMessage{BatchID: persist.GenerateID(), TokenDefinitionIDs: tDefIDs}
	logger.For(ctx).WithField("batchID", message.BatchID).Infof("enqueued batch: %s (size=%d)", message.BatchID, len(tDefIDs))
	return m.taskClient.CreateTaskForTokenProcessing(ctx, message)
}

// IsPaused returns true if runs of this token are paused.
func (m Manager) Paused(ctx context.Context, tID persist.TokenIdentifiers) bool {
	p, _ := m.processRegistry.pausedContract(ctx, tID.Chain, tID.ContractAddress)
	return p
}

// StartProcessing marks a token as processing. It returns a callback that must be called when work on the token is finished in order to mark
// it as finished. If withRetry is true, the callback will attempt to reenqueue the token if an error is passed. attemps is ignored when MaxRetries
// is set to the default value of 0.
func (m Manager) StartProcessing(ctx context.Context, tDefID persist.DBID, tID persist.TokenIdentifiers, attempts int, cause persist.ProcessingCause) (func(db.TokenMedia, error) error, error) {
	if m.Paused(ctx, tID) {
		recordPipelinePaused(ctx, m.metricReporter, tID.Chain, tID.ContractAddress, cause)
		return nil, ErrContractPaused{Chain: tID.Chain, Contract: tID.ContractAddress}
	}

	err := m.throttle.Lock(ctx, "lock:"+tDefID.String())
	if err != nil {
		return nil, err
	}

	stop := make(chan bool)
	done := make(chan bool)
	tick := time.NewTicker(10 * time.Second)

	go func() {
		defer tick.Stop()
		m.processRegistry.keepAlive(ctx, tDefID)
		for {
			select {
			case <-tick.C:
				m.processRegistry.keepAlive(ctx, tDefID)
			case <-stop:
				close(done)
				return
			}
		}
	}()

	start := time.Now()

	callback := func(tm db.TokenMedia, err error) error {
		close(stop)
		<-done
		m.recordError(ctx, tID, err)
		m.tryRetry(ctx, tDefID, tID, err, attempts)
		m.throttle.Unlock(ctx, "lock:"+tDefID.String())
		recordPipelineResults(ctx, m.metricReporter, tID.Chain, tm.Media.MediaType, err, time.Since(start), cause)
		return nil
	}

	return callback, err
}

func (m Manager) recordError(ctx context.Context, tID persist.TokenIdentifiers, err error) {
	if err == nil || !util.ErrorIs[ErrBadToken](err) {
		return
	}

	if paused, _ := m.processRegistry.pausedContract(ctx, tID.Chain, tID.ContractAddress); paused {
		return
	}

	canRetry, _, err := m.errorCounter.ForKey(ctx, fmt.Sprintf("%d:%s", tID.Chain, tID.ContractAddress))
	if err != nil {
		logger.For(ctx).Errorf("failed to track error: %s", err)
		sentryutil.ReportError(ctx, err)
	}

	if canRetry {
		return
	}

	nowFlaky, err := m.processRegistry.pauseContract(ctx, tID.Chain, tID.ContractAddress, time.Hour*3)
	if err != nil {
		logger.For(ctx).Errorf("failed to pause contract:%s", err)
		sentryutil.ReportError(ctx, err)
		return
	}

	if nowFlaky {
		err := fmt.Errorf("processing of chain=%d; contract=%s is paused for %s because of too many errors", tID.Chain, tID.ContractAddress, time.Hour*3)
		logger.For(ctx).Warnf(err.Error())
		sentryutil.ReportError(ctx, err)
	}
}

func (m Manager) tryRetry(ctx context.Context, tDefID persist.DBID, tID persist.TokenIdentifiers, err error, attempts int) error {
	if m.Paused(ctx, tID) {
		m.processRegistry.finish(ctx, tDefID)
		return nil
	}

	if err == nil || m.numRetryF == nil || attempts >= m.numRetryF() {
		m.processRegistry.finish(ctx, tDefID)
		return nil
	}

	_, delay, err := m.taskDelayer.ForKey(ctx, tDefID.String())
	if err != nil {
		m.processRegistry.finish(ctx, tDefID)
		return err
	}

	m.processRegistry.setEnqueue(ctx, tDefID)
	message := task.TokenProcessingTokenMessage{TokenDefinitionID: tDefID, Attempts: attempts + 1}
	return m.taskClient.CreateTaskForTokenTokenProcessing(ctx, message, delay)
}

// registry handles the storing of object state managed by Manager
type registry struct{ c *redis.Cache }

func (r registry) processing(ctx context.Context, tDefID persist.DBID) (bool, error) {
	_, err := r.c.Get(ctx, processingKey(tDefID))
	return err == nil, err
}

func (r registry) finish(ctx context.Context, tDefID persist.DBID) error {
	return r.c.Delete(ctx, processingKey(tDefID))
}

func (r registry) setEnqueue(ctx context.Context, tDefID persist.DBID) error {
	return r.setManyEnqueue(ctx, []persist.DBID{tDefID})
}

func (r registry) setManyEnqueue(ctx context.Context, tDefIDs []persist.DBID) error {
	keyValues := make(map[string]any, len(tDefIDs))
	for _, id := range tDefIDs {
		keyValues[processingKey(id)] = []byte("enqueued")
	}
	return r.c.MSetWithTTL(ctx, keyValues, time.Hour)
}

func (r registry) pausedContract(ctx context.Context, chain persist.Chain, address persist.Address) (bool, error) {
	_, err := r.c.Get(ctx, pauseContractKey(chain, address))
	return err == nil, err
}

func (r registry) pauseContract(ctx context.Context, chain persist.Chain, address persist.Address, ttl time.Duration) (bool, error) {
	b := make([]byte, 64)
	binary.BigEndian.PutUint64(b, uint64(time.Now().UnixMilli()))
	return r.c.SetNX(ctx, pauseContractKey(chain, address), b, ttl)
}

func (r registry) keepAlive(ctx context.Context, tDefID persist.DBID) error {
	return r.c.Set(ctx, processingKey(tDefID), []byte("processing"), time.Minute)
}

func processingKey(id persist.DBID) string { return "inflight:" + id.String() }
func pauseContractKey(c persist.Chain, a persist.Address) string {
	return fmt.Sprintf("paused:%d:%s", c, a)
}

const (
	// Metrics emitted by the pipeline
	metricPipelineCompleted      = "pipeline_completed"
	metricPipelineDuration       = "pipeline_duration"
	metricPipelineErrored        = "pipeline_errored"
	metricPipelineTimedOut       = "pipeline_timedout"
	metricPipelineContractPaused = "pipeline_contract_paused"
)

func pipelineDurationMetric(d time.Duration) metric.Measure {
	return metric.Measure{Name: metricPipelineDuration, Value: d.Seconds()}
}

func pipelineTimedOutMetric() metric.Measure {
	return metric.Measure{Name: metricPipelineTimedOut}
}

func pipelineCompletedMetric() metric.Measure {
	return metric.Measure{Name: metricPipelineCompleted}
}

func pipelineErroredMetric() metric.Measure {
	return metric.Measure{Name: metricPipelineErrored}
}

func pipelinePausedMetric() metric.Measure {
	return metric.Measure{Name: metricPipelineContractPaused}
}

func recordPipelinePaused(ctx context.Context, mr metric.MetricReporter, chain persist.Chain, contract persist.Address, cause persist.ProcessingCause) {
	baseOpts := append([]any{}, metric.LogOptions.WithTags(map[string]string{
		"chain":    fmt.Sprintf("%d", chain),
		"cause":    cause.String(),
		"contract": contract.String(),
	}))
	mr.Record(ctx, pipelinePausedMetric(), append(baseOpts,
		metric.LogOptions.WithLevel(logrus.WarnLevel),
		metric.LogOptions.WithLogMessage(fmt.Sprintf("processing for chain=%d; contract=%s is paused", chain, contract)),
	)...)
}

func recordPipelineResults(ctx context.Context, mr metric.MetricReporter, chain persist.Chain, mediaType persist.MediaType, err error, d time.Duration, cause persist.ProcessingCause) {
	baseOpts := append([]any{}, metric.LogOptions.WithTags(map[string]string{
		"chain":      fmt.Sprintf("%d", chain),
		"mediaType":  mediaType.String(),
		"cause":      cause.String(),
		"isBadToken": fmt.Sprintf("%t", util.ErrorIs[ErrBadToken](err)),
	}))

	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		mr.Record(ctx, pipelineTimedOutMetric(), append(baseOpts,
			metric.LogOptions.WithLogMessage("pipeline timed out"),
		)...)
		return
	}

	mr.Record(ctx, pipelineDurationMetric(d), append(baseOpts,
		metric.LogOptions.WithLogMessage(fmt.Sprintf("pipeline finished (took: %s)", d)),
	)...)

	if err != nil {
		mr.Record(ctx, pipelineErroredMetric(), append(baseOpts,
			metric.LogOptions.WithLevel(logrus.ErrorLevel),
			metric.LogOptions.WithLogMessage("pipeline completed with error: "+err.Error()),
		)...)
		return
	}

	mr.Record(ctx, pipelineCompletedMetric(), append(baseOpts,
		metric.LogOptions.WithLogMessage("pipeline completed successfully"),
	)...)
}
