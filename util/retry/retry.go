package retry

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/shurcooL/graphql"
)

var (
	DefaultRetry    = Retry{Base: 4, Cap: 64, Tries: 12}
	ErrOutOfRetries = errors.New("tried too many times")
)

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
	return nil, ErrOutOfRetries
}

func RetryQuery(ctx context.Context, c *graphql.Client, query any, vars map[string]any) error {
	return RetryQueryWithRetry(ctx, c, query, vars, DefaultRetry)
}

func RetryQueryWithRetry(ctx context.Context, c *graphql.Client, query any, vars map[string]any, r Retry) error {
	var err error
	for i := 0; i < r.Tries; i++ {
		err = c.Query(ctx, query, vars)
		if err == nil {
			return nil
		}

		if !strings.Contains(err.Error(), "429") {
			return err
		}

		r.Sleep(i)
	}
	return ErrOutOfRetries
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
	return ErrOutOfRetries
}
