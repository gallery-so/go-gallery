package retry

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/shurcooL/graphql"
)

var (
	DefaultRetry = Retry{Base: 4, Cap: 64, Tries: 12}
)

type ErrOutOfRetries struct {
	Err   error
	Retry Retry
}

func (e ErrOutOfRetries) Error() string {
	return fmt.Sprintf("retried %d times: last error: %s", e.Retry.Tries, e.Err.Error())
}

type Retry struct {
	Base  int // Min amount of time to sleep per iteration
	Cap   int // Max amount of time to sleep per iteration
	Tries int // Number of times to retry
}

func (r Retry) Sleep(i int) {
	// powerInt returns the base-x exponential of y.
	powerInt := func(x, y int) int {
		ret := 1
		for i := 0; i < y; i++ {
			ret *= x
		}
		return ret
	}

	// minInt returns the minimum of two ints.
	minInt := func(x, y int) int {
		if x < y {
			return x
		}
		return y
	}

	sleepFor := rand.Intn(minInt(r.Cap, r.Base*powerInt(2, i)))
	time.Sleep(time.Duration(sleepFor) * time.Second)
}

func RetryRequest(c *http.Client, req *http.Request) (*http.Response, error) {
	return RetryRequestWithRetry(c, req, DefaultRetry)
}

func RetryRequestWithRetry(c *http.Client, req *http.Request, r Retry) (*http.Response, error) {
	var resp *http.Response
	var err error
	for i := 0; i < r.Tries; i++ {
		resp, err = c.Do(req)
		if err != nil {
			return resp, err
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, err
		}

		r.Sleep(i)
	}
	return nil, ErrOutOfRetries{err, r}
}

func RetryQuery(ctx context.Context, c *graphql.Client, query any, vars map[string]any) error {
	return RetryQueryWithRetry(ctx, c, query, vars, DefaultRetry)
}

func RetryQueryWithRetry(ctx context.Context, c *graphql.Client, query any, vars map[string]any, r Retry) error {
	f := func(ctx context.Context) error { return c.Query(ctx, query, vars) }
	shouldRetry := func(err error) bool { return strings.Contains(err.Error(), "429") }
	return RetryFunc(ctx, f, shouldRetry, r)
}

func RetryFunc(ctx context.Context, f func(ctx context.Context) error, shouldRetry func(error) bool, r Retry) error {
	var err error
	for i := 0; i < r.Tries; i++ {
		err = f(ctx)
		if err == nil {
			return nil
		}

		if !shouldRetry(err) {
			return err
		}

		r.Sleep(i)
	}
	return ErrOutOfRetries{err, r}
}
