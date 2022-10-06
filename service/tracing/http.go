package tracing

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/getsentry/sentry-go"
)

type tracingTransport struct {
	http.RoundTripper

	continueOnly bool
	opts         []sentry.SpanOption
}

// NewTracingTransport creates an http transport that will trace requests via Sentry. If continueOnly is true,
// traces will only be generated if they'd contribute to an existing parent trace (e.g. if a trace is not in progress,
// no new trace would be started). It errorsOnly is true, only requests that returned an error status code (400 and above) are reported.
func NewTracingTransport(roundTripper http.RoundTripper, continueOnly bool, spanOptions ...sentry.SpanOption) *tracingTransport {
	// If roundTripper is already a tracer, grab its underlying RoundTripper instead
	if existingTracer, ok := roundTripper.(*tracingTransport); ok {
		return &tracingTransport{
			RoundTripper: existingTracer.RoundTripper,
			continueOnly: continueOnly,
			opts:         spanOptions,
		}
	}

	return &tracingTransport{
		RoundTripper: roundTripper,
		continueOnly: continueOnly,
		opts:         spanOptions,
	}
}

func (t *tracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.continueOnly {
		transaction := sentry.TransactionFromContext(req.Context())
		if transaction == nil {
			return t.RoundTripper.RoundTrip(req)
		}
	}

	span, _ := StartSpan(req.Context(), "http."+strings.ToLower(req.Method), fmt.Sprintf("HTTP %s %s", req.Method, req.URL.String()), t.opts...)

	// Send sentry-trace header in case the receiving service can continue our trace
	req.Header.Add("sentry-trace", span.TraceID.String())

	response, err := t.RoundTripper.RoundTrip(req)
	defer FinishSpan(span)

	AddEventDataToSpan(span, map[string]interface{}{
		"HTTP Status Code": response.StatusCode,
	})

	return response, err
}
