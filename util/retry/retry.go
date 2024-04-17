package retry

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"
	"github.com/shurcooL/graphql"
)

var ErrOutOfRetries = fmt.Errorf("tried too many times")
var DefaultRetry = Retry{MaxWait: 64, MaxRetries: 8}

// Retry is a configuration for retrying requests
type Retry struct {
	MinWait    int // Min amount of time to sleep per iteration in seconds
	MaxWait    int // Max amount of time to sleep per iteration in seconds
	MaxRetries int // Number of times to retry
}

type Limiter interface {
	ForKey(context.Context, string) (bool, time.Duration, error)
}

// Retryer protects against rate limits by requiring requests to be sent through it.
// Retryer checks its underlying rate limiter before sending the request, and if the rate limit is exceeded,
// it will wait before retrying the request. If the request does get rate limited by the external service even after
// checking the rate limiter, then Retryer will automatically re-enqueue the request with exponential backoff.
// Retryer will handle requests in the same order they are received. For retries, the request will be re-enqueued
// and won't be retried until it is popped off the queue again.
type Retryer struct {
	l       Limiter
	c       *http.Client
	q       chan pending
	closing chan struct{}
	done    chan struct{}
}

type pending struct {
	req        *http.Request
	done       chan error
	retryCount int
	r          Retry
	queuedAt   time.Time
}

func New(l Limiter, c *http.Client) (*Retryer, func()) {
	r := &Retryer{
		l:       l,
		c:       c,
		q:       make(chan pending),
		closing: make(chan struct{}),
		done:    make(chan struct{}),
	}
	go r.enqueue()
	return r, func() {
		r.closing <- struct{}{}
		<-r.done
	}
}

func (r *Retryer) Do(req *http.Request) (*http.Response, error) {
	return r.DoRetry(req, DefaultRetry)
}

func (r *Retryer) DoRetry(req *http.Request, c Retry) (*http.Response, error) {
	p := pending{req, make(chan error), 0, c, time.Now()}
	err := r.wait(p)
	if err != nil {
		return nil, err
	}
	return r.do(p)
}

func (r *Retryer) wait(p pending) error {
	r.q <- p
	return <-p.done
}

func (r *Retryer) do(p pending) (*http.Response, error) {
	resp, err := r.c.Do(p.req)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		logger.For(p.req.Context()).Infof("request to %s was successful, took %s in total", p.req.Host, time.Since(p.queuedAt))
		return resp, nil
	}
	if p.retryCount >= p.r.MaxRetries {
		logger.For(p.req.Context()).Errorf("ran out of retries, waited for %s to handle request to %s", time.Since(p.queuedAt), p.req.Host)
		return resp, ErrOutOfRetries
	}

	// wait for a bit before retrying
	p.retryCount++
	wait := WaitTime(p.r.MinWait, p.r.MaxWait, p.retryCount)
	logger.For(p.req.Context()).Infof("rate limited by %s (attempt=%d/%d); waiting for %s (so far waited for %s)", p.req.Host, p.retryCount, p.r.MaxRetries, wait, time.Since(p.queuedAt))
	<-time.After(wait)

	// re-enqueue the request
	p.done = make(chan error)
	r.wait(p)
	return r.do(p)
}

func (r *Retryer) enqueue() {
	for {
		select {
		case pending := <-r.q:
			ctx := pending.req.Context()
			var waited time.Duration
		msgLoop:
			for {
				select {
				case <-ctx.Done():
					pending.done <- ctx.Err()
					close(pending.done)
					break msgLoop
				default:
					_, wait, err := r.l.ForKey(ctx, pending.req.Host)
					if err != nil {
						logger.For(ctx).Errorf("error checking rate limiter for %s: %s", pending.req.Host, err)
						pending.done <- err
						close(pending.done)
						break msgLoop
					}

					if wait <= 0 {
						close(pending.done)
						break msgLoop
					}

					logger.For(ctx).Infof("out of tokens for %s, waiting for %s to refill (so far waited for %s)", pending.req.Host, wait, waited)
					waited += wait
					<-time.After(wait)
				}
			}
		case <-r.closing:
			logger.For(context.Background()).Infof("stopping retryer, flushing buffered requests")
			close(r.q)
			for pending := range r.q {
				pending.done <- fmt.Errorf("retryer is closing; cancelling request to %s", pending.req.Host)
				close(pending.done)
			}
			close(r.done)
			return
		}
	}
}

func RetryRequest(c *http.Client, req *http.Request) (*http.Response, error) {
	return RetryRequestWithRetry(c, req, DefaultRetry)
}

func RetryRequestWithRetry(c *http.Client, req *http.Request, r Retry) (*http.Response, error) {
	var resp *http.Response
	var err error
	for i := 0; i < r.MaxRetries; i++ {
		resp, err = c.Do(req)
		if err != nil {
			return resp, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, err
		}
		wait := WaitTime(r.MinWait, r.MaxWait, i)
		logger.For(req.Context()).Infof("rate limited by %s (attempt=%d/%d); waiting for %s", req.Host, i+1, r.MaxRetries, wait)
		<-time.After(wait)
	}
	return nil, ErrOutOfRetries
}

func RetryQuery(ctx context.Context, c *graphql.Client, query any, vars map[string]any) error {
	return RetryQueryConfig(ctx, c, query, vars, DefaultRetry)
}

func RetryQueryConfig(ctx context.Context, c *graphql.Client, query any, vars map[string]any, r Retry) error {
	f := func(ctx context.Context) error { return c.Query(ctx, query, vars) }
	shouldRetry := func(err error) bool { return strings.Contains(err.Error(), "429") }
	return RetryFunc(ctx, f, shouldRetry, r)
}

func RetryFunc(ctx context.Context, f func(ctx context.Context) error, shouldRetry func(error) bool, c Retry) error {
	var err error
	for i := 0; i < c.MaxRetries; i++ {
		err = f(ctx)
		if err == nil {
			return nil
		}

		if !shouldRetry(err) {
			return err
		}

		if i != c.MaxRetries-1 {
			<-time.After(WaitTime(c.MinWait, c.MinWait, i))
		}
	}
	return ErrOutOfRetries
}

func HTTPErrNotFound(err error) bool {
	if err != nil {
		if it, ok := err.(util.ErrHTTP); ok && it.Status == http.StatusNotFound {
			return true
		}
	}
	return false
}

func WaitTime(minWait, maxWait int, tryIteration int) time.Duration {
	if tryIteration <= 0 {
		return 0
	}

	r := 1
	for i := 0; i < tryIteration; i++ {
		r *= 2
	}

	wait := maxWait

	// 1 second plus 10 percent of maxWait
	rand.Seed(time.Now().UnixNano())
	jitter := rand.Intn(1 + int(float64(maxWait)*0.1))

	if iterWait := minWait + r + jitter; iterWait < maxWait {
		wait = iterWait
	}

	return time.Duration(wait) * time.Second
}
