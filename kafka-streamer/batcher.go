package main

import (
	"context"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"sync"
	"time"
)

type workerMessage[T any] struct {
	Message *kafka.Message
	Index   int
}

type messageBatcher[T any] struct {
	maxSize         int
	timeoutDuration time.Duration
	nextTimeout     time.Time
	entries         []T
	errors          []error
	errorLock       sync.Mutex
	workerPool      chan workerMessage[T]
	wg              sync.WaitGroup
	closeCh         chan struct{}
	closed          bool
	parseF          func(context.Context, *kafka.Message) (T, error)
	submitF         func(context.Context, []T) error
}

func newMessageBatcher[T any](maxSize int, timeout time.Duration, workerCount int, parseF func(context.Context, *kafka.Message) (T, error), submitF func(context.Context, []T) error) *messageBatcher[T] {
	mb := &messageBatcher[T]{
		maxSize:         maxSize,
		timeoutDuration: timeout,
		entries:         make([]T, 0, maxSize),
		workerPool:      make(chan workerMessage[T], workerCount), // Buffered according to number of workers
		closeCh:         make(chan struct{}),
		parseF:          parseF,
		submitF:         submitF,
	}

	for i := 0; i < workerCount; i++ {
		go mb.worker()
	}

	return mb
}

func (mb *messageBatcher[T]) Add(ctx context.Context, msg *kafka.Message) error {
	if mb.closed {
		return fmt.Errorf("cannot add message: batcher is stopped")
	}

	mb.wg.Add(1)
	index := len(mb.entries)
	if index == 0 {
		// The first message added starts the timeout clock
		mb.nextTimeout = time.Now().Add(mb.timeoutDuration)
	}
	mb.entries = append(mb.entries, *new(T))
	mb.workerPool <- workerMessage[T]{Message: msg, Index: index}
	return nil
}

func (mb *messageBatcher[T]) worker() {
	for wm := range mb.workerPool {
		result, err := mb.parseF(context.Background(), wm.Message)
		if err != nil {
			mb.errorLock.Lock()
			mb.errors = append(mb.errors, err)
			mb.errorLock.Unlock()
		} else {
			mb.entries[wm.Index] = result
		}
		mb.wg.Done()
	}
}

func (mb *messageBatcher[T]) IsReady() bool {
	if len(mb.entries) == 0 {
		return false
	}

	return len(mb.entries) >= mb.maxSize || time.Now().After(mb.nextTimeout)
}

func (mb *messageBatcher[T]) Submit(ctx context.Context, c *kafka.Consumer) error {
	if len(mb.entries) == 0 {
		return nil
	}

	mb.wg.Wait()

	if len(mb.errors) > 0 {
		return fmt.Errorf("errors occurred during batch processing: %v", mb.errors)
	}

	if readOnlyMode {
		mb.Reset()
		return nil
	}

	err := mb.submitF(ctx, mb.entries)
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

func (mb *messageBatcher[T]) Reset() {
	// Wait for any outstanding workers to finish processing
	if len(mb.entries) > 0 {
		mb.wg.Wait()
	}

	mb.nextTimeout = time.Time{}
	mb.entries = make([]T, 0, mb.maxSize)
	mb.errors = nil
}

func (mb *messageBatcher[T]) Stop() {
	if mb.closed {
		return
	}
	mb.closed = true

	mb.wg.Wait()         // Wait for all workers to finish processing
	close(mb.closeCh)    // Close the shutdown signal channel
	close(mb.workerPool) // Safely close the worker pool channel
}
