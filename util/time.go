package util

import (
	"github.com/mikeydub/go-gallery/service/logger"
	"time"
)

// Track the time it takes to execute a function
func Track(s string, startTime time.Time) {
	endTime := time.Now()
	logger.NoCtx().Debugf("%s took %v", s, endTime.Sub(startTime))
}
