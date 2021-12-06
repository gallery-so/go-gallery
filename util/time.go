package util

import (
	"time"

	"github.com/sirupsen/logrus"
)

// Track the time it takes to execute a function
func Track(s string, startTime time.Time) {
	endTime := time.Now()
	logrus.Debugf("%s took %v", s, endTime.Sub(startTime))
}
