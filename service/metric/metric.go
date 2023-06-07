package metric

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type Measure struct {
	Name  string
	Value float64
}

type MetricReporter struct {
	Record func(ctx context.Context, m Measure, opts ...any)
}

var LogOptions = LogOptionBulder{}

func NewLogMetricReporter() MetricReporter {
	return MetricReporter{Record: LogMetricReporter{}.Record}
}

type LogMetricReporter struct{}

type LogArgs struct {
	Tags   map[string]string
	LogMsg string
	Level  *logrus.Level
}

type LogOptionBulder struct{}

func (LogOptionBulder) WithLogMessage(msg string) func(*LogArgs) {
	return func(a *LogArgs) {
		a.LogMsg = msg
	}
}

func (LogOptionBulder) WithTags(tags map[string]string) func(*LogArgs) {
	return func(a *LogArgs) {
		a.Tags = tags
	}
}

func (LogOptionBulder) WithLevel(l logrus.Level) func(*LogArgs) {
	return func(a *LogArgs) {
		a.Level = &l
	}
}

func (l LogMetricReporter) Record(ctx context.Context, metric Measure, opts ...any) {
	args := LogArgs{}
	for _, opt := range opts {
		opt.(func(*LogArgs))(&args)
	}

	metricPayload := logrus.Fields{"metric": logrus.Fields{
		"metricName":  metric.Name,
		"metricValue": metric.Value,
		"metricTags":  args.Tags,
	}}

	logLine := fmt.Sprintf("reporting metric %s(val=%0.2f)", metric.Name, metric.Value)

	if args.LogMsg != "" {
		logLine += ": " + args.LogMsg
	}

	if args.Level == nil {
		args.Level = util.ToPointer(logrus.InfoLevel)
	}

	logger.For(ctx).WithFields(metricPayload).Log(*args.Level, logLine)
}
