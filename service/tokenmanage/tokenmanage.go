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

// ErrContractFlaking indicates that runs of this contract are frequently failing
type ErrContractFlaking struct {
	Chain    persist.Chain
	Contract persist.Address
	Err      error
	Duration time.Duration
}

func (e ErrContractFlaking) Unwrap() error { return e.Err }
func (e ErrContractFlaking) Error() string {
	return fmt.Sprintf("runs of chain=%d; contract=%s are paused for %s because of too many failed runs; last error: %s", e.Chain, e.Contract, e.Duration, e.Err)
}

type TickToken func(db.TokenDefinition) (time.Duration, error)

type Manager struct {
	ProcessRegistry *Registry
	taskClient      *task.Client
	throttle        *throttle.Locker
	errorCounter    *limiters.KeyRateLimiter
	tickToken       TickToken
	metricReporter  metric.MetricReporter
	maxRetries      func(db.TokenDefinition) int
}

func New(ctx context.Context, taskClient *task.Client, cache *redis.Cache, tickToken TickToken) *Manager {
	return &Manager{
		ProcessRegistry: &Registry{cache},
		taskClient:      taskClient,
		throttle:        throttle.NewThrottleLocker(cache, 30*time.Minute),
		metricReporter:  metric.NewLogMetricReporter(),
		errorCounter:    limiters.NewKeyRateLimiter(ctx, cache, "errorCount", 100, 3*time.Hour),
		tickToken:       tickToken,
	}
}

func NewWithRetries(ctx context.Context, taskClient *task.Client, cache *redis.Cache, maxRetries func(db.TokenDefinition) int, tickToken TickToken) *Manager {
	m := New(ctx, taskClient, cache, tickToken)
	m.maxRetries = maxRetries
	return m
}

// Processing returns true if the token is processing or enqueued.
func (m Manager) Processing(ctx context.Context, tDefID persist.DBID) bool {
	p, _ := m.ProcessRegistry.processing(ctx, tDefID)
	return p
}

// SubmitBatch enqueues tokens for processing.
func (m Manager) SubmitBatch(ctx context.Context, tDefIDs []persist.DBID) error {
	if len(tDefIDs) == 0 {
		return nil
	}
	m.ProcessRegistry.setManyEnqueue(ctx, tDefIDs)
	message := task.TokenProcessingBatchMessage{BatchID: persist.GenerateID(), TokenDefinitionIDs: tDefIDs}
	logger.For(ctx).WithField("batchID", message.BatchID).Infof("enqueued batch: %s (size=%d)", message.BatchID, len(tDefIDs))
	return m.taskClient.CreateTaskForTokenProcessing(ctx, message)
}

// IsPaused returns true if runs of this token are paused.
func (m Manager) Paused(ctx context.Context, td db.TokenDefinition) bool {
	p, _ := m.ProcessRegistry.pausedContract(ctx, td.Chain, td.ContractAddress)
	return p
}

// StartProcessing marks a token as processing. It returns a callback that must be called when work on the token is finished in order to mark
// it as finished. If withRetry is true, the callback will attempt to reenqueue the token if an error is passed. attemps is ignored when MaxRetries
// is set to the default value of 0.
func (m Manager) StartProcessing(ctx context.Context, td db.TokenDefinition, attempts int, cause persist.ProcessingCause) (func(db.TokenMedia, error) error, error) {
	if m.Paused(ctx, td) {
		recordPipelinePaused(ctx, m.metricReporter, td.Chain, td.ContractAddress, cause)
		return nil, ErrContractPaused{Chain: td.Chain, Contract: td.ContractAddress}
	}

	err := m.throttle.Lock(ctx, "lock:"+td.ID.String())
	if err != nil {
		return nil, err
	}

	stop := make(chan bool)
	done := make(chan bool)
	tick := time.NewTicker(10 * time.Second)

	go func() {
		defer tick.Stop()
		m.ProcessRegistry.keepAlive(ctx, td.ID)
		for {
			select {
			case <-tick.C:
				m.ProcessRegistry.keepAlive(ctx, td.ID)
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
		m.tickToken(td) // mark that the token ran so if an error occured tryRetry delays the next run appropriately
		m.recordError(ctx, td, err)
		m.tryRetry(ctx, td, err, attempts)
		m.throttle.Unlock(ctx, "lock:"+td.ID.String())
		recordMetrics(ctx, m.metricReporter, td.Chain, tm.Media.MediaType, err, time.Since(start), cause)
		return nil
	}

	return callback, err
}

func (m Manager) recordError(ctx context.Context, td db.TokenDefinition, originalErr error) {
	// Don't penalize non-token related errors e.g. errors related to the pipeline
	if originalErr == nil || !util.ErrorIs[ErrBadToken](originalErr) {
		return
	}

	if paused, _ := m.ProcessRegistry.pausedContract(ctx, td.Chain, td.ContractAddress); paused {
		return
	}

	canRetry, _, err := m.errorCounter.ForKey(ctx, fmt.Sprintf("%d:%s", td.Chain, td.ContractAddress))
	if err != nil {
		logger.For(ctx).Errorf("failed to track error: %s", err)
		sentryutil.ReportError(ctx, err)
	}

	if canRetry {
		return
	}

	nowFlaky, err := m.ProcessRegistry.pauseContract(ctx, td.Chain, td.ContractAddress, time.Hour*3)
	if err != nil {
		logger.For(ctx).Errorf("failed to pause contract:%s", err)
		sentryutil.ReportError(ctx, err)
		return
	}

	if nowFlaky {
		err := ErrContractFlaking{Chain: td.Chain, Contract: td.ContractAddress, Err: originalErr, Duration: time.Hour * 3}
		logger.For(ctx).Warnf(err.Error())
		sentryutil.ReportError(ctx, err)
	}
}

func (m Manager) tryRetry(ctx context.Context, td db.TokenDefinition, err error, attempts int) error {
	// Only retry intermittent errors related to the token e.g. missing metadata
	if !util.ErrorIs[ErrBadToken](err) {
		m.ProcessRegistry.finish(ctx, td.ID)
		return nil
	}

	if m.Paused(ctx, td) {
		m.ProcessRegistry.finish(ctx, td.ID)
		return nil
	}

	if err == nil || m.maxRetries == nil || attempts >= m.maxRetries(td) {
		m.ProcessRegistry.finish(ctx, td.ID)
		return nil
	}

	delay, err := m.tickToken(td)
	if err != nil {
		logger.For(ctx).Errorf("failed to get retry delay, not retrying: %s", err)
		m.ProcessRegistry.finish(ctx, td.ID)
		return err
	}

	m.ProcessRegistry.SetEnqueue(ctx, td.ID)
	message := task.TokenProcessingTokenMessage{TokenDefinitionID: td.ID, Attempts: attempts + 1}
	return m.taskClient.CreateTaskForTokenTokenProcessing(ctx, message, delay)
}

// Registry handles the storing of object state managed by Manager
type Registry struct{ Cache *redis.Cache }

func (r Registry) SetEnqueue(ctx context.Context, tDefID persist.DBID) error {
	return r.setManyEnqueue(ctx, []persist.DBID{tDefID})
}

func (r Registry) processing(ctx context.Context, tDefID persist.DBID) (bool, error) {
	_, err := r.Cache.Get(ctx, processingKey(tDefID))
	return err == nil, err
}

func (r Registry) finish(ctx context.Context, tDefID persist.DBID) error {
	return r.Cache.Delete(ctx, processingKey(tDefID))
}

func (r Registry) setManyEnqueue(ctx context.Context, tDefIDs []persist.DBID) error {
	keyValues := make(map[string]any, len(tDefIDs))
	for _, id := range tDefIDs {
		keyValues[processingKey(id)] = []byte("enqueued")
	}
	return r.Cache.MSetWithTTL(ctx, keyValues, time.Hour)
}

func (r Registry) pausedContract(ctx context.Context, chain persist.Chain, address persist.Address) (bool, error) {
	_, err := r.Cache.Get(ctx, pauseContractKey(chain, address))
	return err == nil, err
}

func (r Registry) pauseContract(ctx context.Context, chain persist.Chain, address persist.Address, ttl time.Duration) (bool, error) {
	b := make([]byte, 64)
	binary.BigEndian.PutUint64(b, uint64(time.Now().UnixMilli()))
	return r.Cache.SetNX(ctx, pauseContractKey(chain, address), b, ttl)
}

func (r Registry) keepAlive(ctx context.Context, tDefID persist.DBID) error {
	return r.Cache.Set(ctx, processingKey(tDefID), []byte("processing"), time.Minute)
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

func recordMetrics(ctx context.Context, mr metric.MetricReporter, chain persist.Chain, mediaType persist.MediaType, err error, d time.Duration, cause persist.ProcessingCause) {
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
