package main

import (
	"context"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"
	"sync"
	"time"
)

type workerMessage[TParam any, TAvroMessage any] struct {
	Message     *kafka.Message
	AvroTarget  *TAvroMessage
	OutputParam *TParam
	Index       int
}

type errorAtIndex struct {
	Error error
	Index int
}

// messageBatcher processes and submits batches of messages. Its public methods are not safe for concurrent use and
// should only be called from a single thread, but the work added to it will be distributed among a pool of parallel
// workers.
type messageBatcher[TParam any, TAvroMessage any] struct {
	maxSize         int
	timeoutDuration time.Duration
	nextTimeout     time.Time
	entries         []*TParam
	errors          []errorAtIndex
	errorLock       sync.Mutex
	workerPool      chan workerMessage[TParam, TAvroMessage]
	wg              sync.WaitGroup
	closeCh         chan struct{}
	closed          bool
	entryPool       *ObjectPool[TParam]
	messagePool     *ObjectPool[TAvroMessage]
	usedEntries     []*TParam
	usedMessages    []*TAvroMessage
	parseF          func(context.Context, *kafka.Message, *TAvroMessage, *TParam) (*TParam, error)
	submitF         func(context.Context, []TParam) error
}

func newMessageBatcher[TParam any, TAvroMessage any](maxSize int, timeout time.Duration, workerCount int, parseF func(context.Context, *kafka.Message, *TAvroMessage, *TParam) (*TParam, error), submitF func(context.Context, []TParam) error) *messageBatcher[TParam, TAvroMessage] {
	mb := &messageBatcher[TParam, TAvroMessage]{
		maxSize:         maxSize,
		timeoutDuration: timeout,
		entries:         make([]*TParam, 0, maxSize),
		workerPool:      make(chan workerMessage[TParam, TAvroMessage], workerCount), // Buffered according to number of workers
		closeCh:         make(chan struct{}),
		entryPool:       NewObjectPool[TParam](maxSize),
		messagePool:     NewObjectPool[TAvroMessage](maxSize),
		usedEntries:     make([]*TParam, 0, maxSize),
		usedMessages:    make([]*TAvroMessage, 0, maxSize),
		parseF:          parseF,
		submitF:         submitF,
	}

	for i := 0; i < workerCount; i++ {
		go mb.worker()
	}

	return mb
}

func (mb *messageBatcher[TParam, TAvroMessage]) Add(ctx context.Context, msg *kafka.Message) error {
	if mb.closed {
		return fmt.Errorf("cannot add message: batcher is stopped")
	}

	mb.wg.Add(1)
	index := len(mb.entries)
	target := mb.messagePool.Get()
	mb.usedMessages = append(mb.usedMessages, target)
	output := mb.entryPool.Get()
	mb.usedEntries = append(mb.usedEntries, output)
	if index == 0 {
		// The first message added starts the timeout clock
		mb.nextTimeout = time.Now().Add(mb.timeoutDuration)
	}
	mb.entries = append(mb.entries, nil)
	mb.workerPool <- workerMessage[TParam, TAvroMessage]{Message: msg, AvroTarget: target, OutputParam: output, Index: index}
	return nil
}

func (mb *messageBatcher[TParam, TAvroMessage]) worker() {
	for wm := range mb.workerPool {
		result, err := mb.parseF(context.Background(), wm.Message, wm.AvroTarget, wm.OutputParam)
		if err != nil {
			mb.errorLock.Lock()
			mb.errors = append(mb.errors, errorAtIndex{Error: err, Index: wm.Index})
			mb.errorLock.Unlock()
		} else {
			mb.entries[wm.Index] = result
		}
		mb.wg.Done()
	}
}

func (mb *messageBatcher[TParam, TAvroMessage]) IsReady() bool {
	if len(mb.entries) == 0 {
		return false
	}

	return len(mb.entries) >= mb.maxSize || time.Now().After(mb.nextTimeout)
}

func (mb *messageBatcher[TParam, TAvroMessage]) Submit(ctx context.Context, c *kafka.Consumer) error {
	if len(mb.entries) == 0 {
		return nil
	}

	mb.wg.Wait()

	if len(mb.errors) > 0 {
		for _, e := range mb.errors {
			if !util.ErrorIs[nonFatalError](e.Error) {
				return fmt.Errorf("errors occurred during batch processing: %v", mb.errors)
			}
		}

		logger.For(ctx).Warnf("non-fatal errors occurred during batch processing: %v", mb.errors)
		errorIndices := make(map[int]bool)
		for _, e := range mb.errors {
			errorIndices[e.Index] = true
		}

		newEntries := mb.entries[:0]
		for i, entry := range mb.entries {
			if _, exists := errorIndices[i]; !exists {
				newEntries = append(newEntries, entry)
			}
		}

		mb.entries = newEntries
	}

	if readOnlyMode {
		mb.Reset()
		return nil
	}

	// Finally dereference the entries, since sqlc still expects non-pointers
	toSubmit := make([]TParam, len(mb.entries))
	for i, entry := range mb.entries {
		toSubmit[i] = *entry
	}

	err := mb.submitF(ctx, toSubmit)
	if err != nil {
		return fmt.Errorf("failed to submit batch: %w", err)
	}

	_, err = c.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit offsets: %w", err)
	}

	mb.Reset()
	return nil
}

func (mb *messageBatcher[TParam, TAvroMessage]) Reset() {
	// Wait for any outstanding workers to finish processing
	if len(mb.entries) > 0 {
		mb.wg.Wait()
	}

	mb.nextTimeout = time.Time{}
	mb.entries = mb.entries[:0]
	mb.messagePool.PutMany(mb.usedMessages)
	mb.usedMessages = mb.usedMessages[:0]
	mb.entryPool.PutMany(mb.usedEntries)
	mb.usedEntries = mb.usedEntries[:0]
	mb.errors = nil
}

func (mb *messageBatcher[TParam, TAvroMessage]) Stop() {
	if mb.closed {
		return
	}
	mb.closed = true

	mb.wg.Wait()         // Wait for all workers to finish processing
	close(mb.closeCh)    // Close the shutdown signal channel
	close(mb.workerPool) // Safely close the worker pool channel
}
