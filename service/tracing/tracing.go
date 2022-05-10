package tracing

import (
	"context"
	"github.com/getsentry/sentry-go"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/sirupsen/logrus"
)

func StartSpan(ctx context.Context, operation string, description string, options ...sentry.SpanOption) (*sentry.Span, context.Context) {
	// TODO: Consider checking for a transaction, seeing if this trace should be sampled, and returning a nil span if not.
	// Trace functionality in other methods can check for nil to see if trace data should be added.
	span := sentry.StartSpan(ctx, operation, options...)
	ctx = logger.NewContextWithFields(span.Context(), logrus.Fields{
		"spanId":       span.SpanID,
		"parentSpanId": span.ParentSpanID,
	})

	span.Description = description

	return span, ctx
}

func FinishSpan(span *sentry.Span) {
	if span == nil {
		return
	}

	span.Finish()
}

func AddEventDataToSpan(span *sentry.Span, eventData map[string]interface{}) {
	if span == nil {
		return
	}

	if span.Data == nil {
		span.Data = make(map[string]interface{})
	}

	for k, v := range eventData {
		span.Data[k] = v
	}
}
