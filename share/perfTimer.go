package share

import (
	"sort"
	"time"
)

var timeThresholds = []time.Duration{
	time.Millisecond,
	time.Second,
	time.Minute,
	time.Hour,
	time.Hour * 24,
}

type PerfTimer time.Time

func NewPerfTimer() PerfTimer {
	return PerfTimer(time.Now())
}

func (t PerfTimer) Elapsed() time.Duration {
	result := time.Since(time.Time(t))
	pos := sort.Search(len(timeThresholds), func(i int) bool {
		return timeThresholds[i] > result
	})
	return result.Round(timeThresholds[max(pos-2, 0)])
}
