package tracing

import (
	"context"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"github.com/getsentry/sentry-go"
	"google.golang.org/grpc"
)

type TracingInterceptor struct {
	continueOnly bool
}

func (t TracingInterceptor) UnaryInterceptor(ctx context.Context, method string, req interface{}, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if t.continueOnly {
		transaction := sentry.TransactionFromContext(ctx)
		if transaction == nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
	}

	span, ctx := StartSpan(ctx, "grpc.call", method, sentry.TransactionName(method))
	defer FinishSpan(span)

	err := invoker(ctx, method, req, reply, cc, opts...)

	if taskReq, ok := req.(*taskspb.CreateTaskRequest); ok {
		AddEventDataToSpan(span, map[string]interface{}{
			"Queue":     taskReq.Parent,
			"Task Name": taskReq.Task.Name,
		})

		if appReq, ok := taskReq.Task.MessageType.(*taskspb.Task_AppEngineHttpRequest); ok {
			if _, ok := appReq.AppEngineHttpRequest.Headers["Authorization"]; ok {
				appReq.AppEngineHttpRequest.Headers["Authorization"] = "[filtered]"
			}

			AddEventDataToSpan(span, map[string]interface{}{
				"HTTP Method":  appReq.AppEngineHttpRequest.HttpMethod,
				"Relative URI": appReq.AppEngineHttpRequest.RelativeUri,
				"Headers":      appReq.AppEngineHttpRequest.Headers,
				"Body":         string(appReq.AppEngineHttpRequest.GetBody()),
			})
		}
	}

	return err
}

func NewTracingInterceptor(continueOnly bool) TracingInterceptor {
	return TracingInterceptor{continueOnly: continueOnly}
}
