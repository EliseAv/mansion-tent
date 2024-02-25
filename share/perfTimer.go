package share

import (
	"sort"
	"time"
)

var magnitudes = []struct {
	threshold time.Duration
	round     time.Duration
}{
	{0, time.Millisecond},
	{time.Second, 10 * time.Millisecond},
	{time.Minute, 100 * time.Millisecond},
	{10 * time.Minute, time.Second},
}

type PerfTimer time.Time
type Elapsed time.Duration

func NewPerfTimer() PerfTimer {
	return PerfTimer(time.Now())
}

func (t PerfTimer) Elapsed() Elapsed {
	return Elapsed(time.Since(time.Time(t)))
}

func (t Elapsed) String() string {
	value := time.Duration(t)
	pos := sort.Search(len(magnitudes), func(i int) bool {
		return magnitudes[i].threshold > value
	})
	i := max(pos-1, 0)
	return value.Round(magnitudes[i].round).String()
}
