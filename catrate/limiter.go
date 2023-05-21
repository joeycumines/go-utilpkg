package catrate

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const (
	nextZeroValue = math.MinInt64
)

type (
	Limiter struct {
		running    *int32
		rates      map[time.Duration]int
		categories sync.Map
		// calculated from rates, for cleanup
		retention time.Duration
		mu        sync.RWMutex
	}

	categoryData struct {
		// at [0] is the next allowed event, or nextZeroValue if none
		// at [1] is the value of events[len(events)-1], or the value that _was_ that
		atomic *[2]int64
		events *ringBuffer[int64]
		mu     sync.Mutex
	}

	cleanupCategory struct {
		Category any
		Data     *categoryData
	}
)

// for testing purposes
var (
	timeNow       = time.Now
	timeNewTicker = time.NewTicker
)

var categoryDataPool = sync.Pool{New: func() any {
	return &categoryData{
		// note: the value of atomic is initialized within allow
		atomic: new([2]int64),
		events: newRingBuffer[int64](8),
	}
}}

func NewLimiter(rates map[time.Duration]int) *Limiter {
	// TODO document requirements for rates
	retention, ok := parseRates(rates)
	if !ok {
		panic(fmt.Errorf(`catrate: invalid rates: %v`, rates))
	}

	return &Limiter{
		running:   new(int32),
		rates:     rates,
		retention: retention,
	}
}

func (x *Limiter) ok() bool {
	return x != nil && len(x.rates) != 0
}

// Allow is a non-blocking call that attempts to register an event for the
// given category. True indicates that an event was registered. In all cases,
// the returned time is the next time that an event can be registered for the
// given category. If at least one more event may be registered prior to a rate
// limit being applied (at the current system time), the time will be the zero
// value.
func (x *Limiter) Allow(category any) (time.Time, bool) {
	if !x.ok() {
		// no rate limits applied
		return time.Time{}, true
	}

	// to avoid racing with cleanup
	x.mu.RLock()
	defer x.mu.RUnlock()

	now := timeNow()
	nowUnixNano := now.UnixNano()

	// start the worker if not running
	// WARNING: avoiding race on STOP of the worker is handled via Limiter.mu
	if atomic.CompareAndSwapInt32(x.running, 0, 1) {
		go x.worker()
	}

	// load or store data for category...
	var (
		data   *categoryData
		loaded bool
	)
	{
		poolValue := categoryDataPool.Get().(*categoryData)
		*poolValue.atomic = [2]int64{nextZeroValue, nowUnixNano}
		poolValue.mu.Lock()

		var value any
		value, loaded = x.categories.LoadOrStore(category, poolValue)
		if loaded {
			poolValue.mu.Unlock()
			categoryDataPool.Put(poolValue)
			data = value.(*categoryData)
		} else {
			defer poolValue.mu.Unlock()
			data = poolValue
		}
	}

	// fast path, checking if we're limited
	if next := data.loadNext(); next != nextZeroValue && nowUnixNano < next {
		return time.Unix(0, next), false
	}

	if loaded {
		data.mu.Lock()
		defer data.mu.Unlock()

		// slower path, checking if we're limited
		if data.atomic[0] != nextZeroValue && nowUnixNano < data.atomic[0] {
			return time.Unix(0, data.atomic[0]), false
		}

		// note: on the !loaded code path, this has already been done
		if data.atomic[1] < nowUnixNano {
			data.storeRecent(nowUnixNano)
		}
	}

	// insert sort into data.events
	data.events.Insert(data.events.Search(nowUnixNano), nowUnixNano)

	// remove expired events, calculating the next allowed event, if rate limited
	remaining := filterEvents(now, x.rates, data.events)
	if remaining <= 0 {
		// reservation success, and at least one more event is allowed (prior to rate limiting)
		data.storeNext(nextZeroValue)
		return time.Time{}, true
	}

	// reservation success, but rate limit is now in effect
	next := now.Add(remaining)
	data.storeNext(next.UnixNano())

	return next, true
}

// worker handles cleanup, it polls, with some optimization around avoiding
// locking Limiter.mu when there's nothing to do
func (x *Limiter) worker() {
	var toDelete []cleanupCategory

	ticker := timeNewTicker(time.Duration(math.Max(
		float64(x.retention)*0.5,
		float64(time.Second),
	)))
	defer ticker.Stop()

	for {
		<-ticker.C

		// identify categories we (might) need to delete
		chanceOfStop := true
		x.categories.Range(func(key, value any) bool {
			if data := value.(*categoryData); data.loadRecent() < x.cleanupThreshold() {
				toDelete = append(toDelete, cleanupCategory{key, data})
			} else {
				chanceOfStop = false
			}
			return true
		})

		if len(toDelete) != 0 {
			mustStop := x.cleanup(toDelete, chanceOfStop)
			if mustStop {
				return
			}
			toDelete = toDelete[:0]
		}
	}
}

func (x *Limiter) cleanupThreshold() int64 {
	return timeNow().Add(-x.retention).UnixNano()
}

func (x *Limiter) cleanup(toDelete []cleanupCategory, chanceOfStop bool) (mustStop bool) {
	// avoid racing with Allow (loading from Limiter.categories vs deleting)
	x.mu.Lock()
	defer x.mu.Unlock()

	threshold := x.cleanupThreshold()

	// with the write side of Limiter.mu held, we don't need to lock categoryData.mu
	// additionally, we're the only goroutine that can delete from Limiter.categories
	// as such we don't need to load the data from the sync.Map again
	for i, v := range toDelete {
		if v.Data.atomic[1] < threshold {
			x.categories.Delete(v.Category)
			// https://golang.org/issue/23199
			const maxEventsCap = 1 << 10
			if v.Data.events.Cap() <= maxEventsCap {
				v.Data.events.RemoveBefore(v.Data.events.Len())
				categoryDataPool.Put(v.Data)
			}
		} else {
			chanceOfStop = false
		}
		toDelete[i] = cleanupCategory{}
	}

	if chanceOfStop {
		// while we have Limiter.mu, check if we can stop the worker
		x.categories.Range(func(_, _ any) bool {
			chanceOfStop = false
			return false
		})
		if chanceOfStop {
			// while we hold the mutex, again, so we can avoid not having a worker in cases when we should
			*x.running = 0
			return true
		}
	}

	return false
}

func (x *categoryData) loadNext() int64 {
	return atomic.LoadInt64(&x.atomic[0])
}

func (x *categoryData) storeNext(v int64) {
	atomic.StoreInt64(&x.atomic[0], v)
}

func (x *categoryData) loadRecent() int64 {
	return atomic.LoadInt64(&x.atomic[1])
}

func (x *categoryData) storeRecent(v int64) {
	atomic.StoreInt64(&x.atomic[1], v)
}
