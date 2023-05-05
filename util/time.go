package util

import (
	"time"

	"github.com/mikeydub/go-gallery/service/logger"
)

// Track the time it takes to execute a function
func Track(s string, startTime time.Time) {
	endTime := time.Now()
	logger.For(nil).Debugf("%s took %v", s, endTime.Sub(startTime))
}
