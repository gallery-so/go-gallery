package tracing

import (
	"context"
	"github.com/getsentry/sentry-go"
)

// dataloaderSpanContextKey is used to store span values in contexts
type dataloaderSpanContextKey struct{}

func DataloaderPreFetchHook(ctx context.Context, loaderName string) context.Context {
	span, traceCtx := StartSpan(ctx, "dataloader.fetch", loaderName)
	spanCtx := context.WithValue(traceCtx, dataloaderSpanContextKey{}, span)
	return spanCtx
}

func DataloaderPostFetchHook(ctx context.Context, loaderName string) {
	if span, ok := ctx.Value(dataloaderSpanContextKey{}).(*sentry.Span); ok {
		FinishSpan(span)
	}
}
