package tracing

// Redis tracing hook, based on the OpenTelemetry hook here:
// https://github.com/go-redis/redis/blob/v8.0.0-beta.5/redisext/otel.go

import (
	"context"
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/go-redis/redis/v8"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func NewRedisHook(db int, dbName string, continueOnly bool) redis.Hook {
	return redisHook{
		db:           db,
		dbName:       dbName,
		continueOnly: continueOnly,
	}
}

type redisHook struct {
	db           int
	dbName       string
	continueOnly bool
}

var _ redis.Hook = redisHook{}

type spanContextKey struct{}

func (r redisHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if r.continueOnly {
		transaction := sentry.TransactionFromContext(ctx)
		if transaction == nil {
			return ctx, nil
		}
	}

	cmdBytes := make([]byte, 0, 32)
	cmdString := string(appendCmd(cmdBytes, cmd))

	span, ctx := StartSpan(ctx, "redis."+strings.ToLower(cmd.FullName()), r.dbName)

	AddEventDataToSpan(span, map[string]interface{}{
		"Redis Cmd": cmdString,
		"Redis DB":  r.db,
	})

	ctx = context.WithValue(span.Context(), spanContextKey{}, span)

	return ctx, nil
}

func (redisHook) AfterProcess(ctx context.Context, cmd redis.Cmder) error {
	if span, ok := ctx.Value(spanContextKey{}).(*sentry.Span); ok {
		if err := cmd.Err(); err != nil {
			AddEventDataToSpan(span, map[string]interface{}{
				"Redis Error": err,
			})
		}

		FinishSpan(span)
	}

	return nil
}

func (r redisHook) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	if r.continueOnly {
		transaction := sentry.TransactionFromContext(ctx)
		if transaction == nil {
			return ctx, nil
		}
	}

	span, ctx := StartSpan(ctx, "redis.pipeline", r.dbName)

	AddEventDataToSpan(span, map[string]interface{}{
		"Redis Pipeline Num Cmds": len(cmds),
		"Redis DB":                r.db,
	})

	ctx = context.WithValue(span.Context(), spanContextKey{}, span)

	return ctx, nil
}

func (redisHook) AfterProcessPipeline(ctx context.Context, cmds []redis.Cmder) error {
	if span, ok := ctx.Value(spanContextKey{}).(*sentry.Span); ok {
		FinishSpan(span)
	}

	return nil
}

func appendCmd(b []byte, cmd redis.Cmder) []byte {
	const lengthLimitPerArg = 64
	isSetCmd := cmd.Name() == "set"

	for i, arg := range cmd.Args() {
		if i > 0 {
			b = append(b, ' ')
		}

		start := len(b)
		b = appendArg(b, arg)
		argLength := len(b) - start

		// The third element of a set command is the payload string
		if isSetCmd && i == 2 {
			b = append(b[:start], fmt.Sprintf("[scrubbed payload: %d bytes]", argLength)...)
		} else if argLength > lengthLimitPerArg {
			b = append(b[:start+lengthLimitPerArg], "..."...)
		}
	}

	return b
}

// --------------------------------------------------------------------------------------------------------
// Code below this point was copied from the redis/internal namespace, which the OpenTelemetry hook can
// use (because it's part of the redis package) but we can't (because we're an external caller).
// --------------------------------------------------------------------------------------------------------

func appendArg(b []byte, v interface{}) []byte {
	switch v := v.(type) {
	case nil:
		return append(b, "<nil>"...)
	case string:
		return appendUTF8String(b, v)
	case []byte:
		return appendUTF8String(b, string(v))
	case int:
		return strconv.AppendInt(b, int64(v), 10)
	case int8:
		return strconv.AppendInt(b, int64(v), 10)
	case int16:
		return strconv.AppendInt(b, int64(v), 10)
	case int32:
		return strconv.AppendInt(b, int64(v), 10)
	case int64:
		return strconv.AppendInt(b, v, 10)
	case uint:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint8:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint16:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint32:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint64:
		return strconv.AppendUint(b, v, 10)
	case float32:
		return strconv.AppendFloat(b, float64(v), 'f', -1, 64)
	case float64:
		return strconv.AppendFloat(b, v, 'f', -1, 64)
	case bool:
		if v {
			return append(b, "true"...)
		}
		return append(b, "false"...)
	case time.Time:
		return v.AppendFormat(b, time.RFC3339Nano)
	default:
		return append(b, fmt.Sprint(v)...)
	}
}

func appendUTF8String(b []byte, s string) []byte {
	for _, r := range s {
		b = appendRune(b, r)
	}
	return b
}

func appendRune(b []byte, r rune) []byte {
	if r < utf8.RuneSelf {
		switch c := byte(r); c {
		case '\n':
			return append(b, "\\n"...)
		case '\r':
			return append(b, "\\r"...)
		default:
			return append(b, c)
		}
	}

	l := len(b)
	b = append(b, make([]byte, utf8.UTFMax)...)
	n := utf8.EncodeRune(b[l:l+utf8.UTFMax], r)
	b = b[:l+n]

	return b
}
