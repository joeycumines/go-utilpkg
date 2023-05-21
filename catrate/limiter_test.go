package catrate

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	rates := map[time.Duration]int{
		time.Second: 32,
		time.Minute: 300,
	}

	limiter := NewLimiter(rates)

	if limiter == nil {
		t.Fatal("Expected limiter not to be nil")
	}

	if len(limiter.rates) != 2 {
		t.Fatal("Expected limiter to have rates length of 2")
	}
}

func TestLimiter_Ok(t *testing.T) {
	limiter := &Limiter{}

	if limiter.ok() {
		t.Fatal("Expected limiter not to be ok when no rates defined")
	}

	limiter.rates = map[time.Duration]int{time.Second: 1}

	if !limiter.ok() {
		t.Fatal("Expected limiter to be ok when rates are defined")
	}
}

func TestLimiter_Allow(t *testing.T) {
	limiter := NewLimiter(map[time.Duration]int{
		time.Second: 5,
	})

	category := "testCategory"

	next, ok := limiter.Allow(category)

	if next != (time.Time{}) {
		t.Fatal("Expected next time to be zero value")
	}

	if !ok {
		t.Fatal("Expected ok to be true")
	}
}

func TestCategoryData_LoadNext(t *testing.T) {
	atomic := new([2]int64)
	atomic[0] = 1
	data := categoryData{
		atomic: atomic,
	}

	if data.loadNext() != 1 {
		t.Fatal("Expected loadNext to be 1")
	}
}

func TestCategoryData_StoreNext(t *testing.T) {
	atomic := new([2]int64)
	data := categoryData{
		atomic: atomic,
	}
	data.storeNext(2)

	if data.atomic[0] != 2 {
		t.Fatal("Expected atomic[0] to be 2")
	}
}

func TestCategoryData_LoadRecent(t *testing.T) {
	atomic := new([2]int64)
	atomic[1] = 1
	data := categoryData{
		atomic: atomic,
	}

	if data.loadRecent() != 1 {
		t.Fatal("Expected loadRecent to be 1")
	}
}

func TestCategoryData_StoreRecent(t *testing.T) {
	atomic := new([2]int64)
	data := categoryData{
		atomic: atomic,
	}
	data.storeRecent(2)

	if data.atomic[1] != 2 {
		t.Fatal("Expected atomic[1] to be 2")
	}
}

func TestLimiter_Allow_suite1(t *testing.T) {
	old := timeNow
	defer func() { timeNow = old }()

	timeNowIn := make(chan struct{})
	timeNowOut := make(chan time.Time)
	timeNow = func() time.Time {
		timeNowIn <- struct{}{}
		return <-timeNowOut
	}

	type AllowOut struct {
		Next time.Time
		Ok   bool
	}
	callAllow := func(t *testing.T, limiter *Limiter, category any) <-chan AllowOut {
		out := make(chan AllowOut)
		go func() {
			var success bool
			defer func() {
				if !success {
					t.Error("unexpected panic")
				}
			}()
			next, ok := limiter.Allow(category)
			out <- AllowOut{next, ok}
			success = true
		}()
		return out
	}

	t.Run("allow_allowed", func(t *testing.T) {
		rates := map[time.Duration]int{time.Second: 1}
		limiter := NewLimiter(rates)
		*limiter.running = 1

		out := callAllow(t, limiter, 1)
		<-timeNowIn
		timeNowOut <- time.Unix(0, 0)

		// expected limited until 1s from now, but successfully allowd
		if v := <-out; !v.Ok || !v.Next.Equal(time.Unix(1, 0)) {
			t.Errorf("unexpected result: %+v", v)
		}

		out = callAllow(t, limiter, 1)
		<-timeNowIn
		timeNowOut <- time.Unix(0, 0)

		// expected limited until 1s from now, reservation unsuccessful
		if v := <-out; v.Ok || !v.Next.Equal(time.Unix(1, 0)) {
			t.Errorf("unexpected result: %+v", v)
		}
	})

	t.Run("complex_scenario", func(t *testing.T) {
		rates := map[time.Duration]int{time.Second: 2, time.Minute: 10}
		limiter := NewLimiter(rates)
		*limiter.running = 1

		// Allow 10 events within one minute - only the last one should start rate limiting, and even then it should
		// be discarded / trimmed immediately, since the window is only 1 minute, and the first event was at 0s.
		next := time.Unix(60, 0)
		initialIntervalSeconds := 6
		for i := 0; i < 10; i++ {
			out := callAllow(t, limiter, 1)
			<-timeNowIn
			timeNowOut <- time.Unix(int64(i*initialIntervalSeconds), 0)
			var n time.Time
			if i == 9 {
				n = next
			}
			if v := <-out; !v.Ok || !v.Next.Equal(n) {
				t.Errorf("unexpected result: %+v", v)
			}
		}

		// we should be a-ok to go ahead and allow at next, but it'll require us to wait until 1m6s
		out := callAllow(t, limiter, 1)
		<-timeNowIn
		timeNowOut <- next
		next = next.Add(time.Second * time.Duration(initialIntervalSeconds))
		if v := <-out; !v.Ok || !v.Next.Equal(next) {
			t.Errorf("unexpected result: %+v", v)
		}

		// any attempts to allow before 1m6s will fail
		out = callAllow(t, limiter, 1)
		<-timeNowIn
		timeNowOut <- next.Add(-1)
		if v := <-out; v.Ok || !v.Next.Equal(next) {
			t.Errorf("unexpected result: %+v", v)
		}
	})
}

func TestLimiter_worker(t *testing.T) {
	oldTimeNow := timeNow
	defer func() { timeNow = oldTimeNow }()

	oldTimeNewTicker := timeNewTicker
	defer func() { timeNewTicker = oldTimeNewTicker }()

	tickerC := make(chan time.Time, 1)
	timeNewTicker = func(d time.Duration) *time.Ticker {
		t := time.NewTicker(d)
		t.C = tickerC
		return t
	}

	// Mocking the time
	timeNowIn := make(chan struct{})
	timeNowOut := make(chan time.Time)
	timeNow = func() time.Time {
		timeNowIn <- struct{}{}
		return <-timeNowOut
	}

	type AllowOut struct {
		Next time.Time
		Ok   bool
	}
	callAllow := func(t *testing.T, limiter *Limiter, category any) <-chan AllowOut {
		out := make(chan AllowOut)
		go func() {
			var success bool
			defer func() {
				if !success {
					t.Error("unexpected panic")
				}
			}()
			next, ok := limiter.Allow(category)
			out <- AllowOut{next, ok}
			success = true
		}()
		return out
	}

	rates := map[time.Duration]int{time.Second: 1}
	limiter := NewLimiter(rates)
	category := 1

	// note: starts worker
	out := callAllow(t, limiter, category)
	<-timeNowIn
	timeNowOut <- time.Unix(0, 0)

	// expected limited until 1s from now, but successfully allowd
	if v := <-out; !v.Ok || !v.Next.Equal(time.Unix(1, 0)) {
		t.Errorf("unexpected result: %+v", v)
	}

	if v := atomic.LoadInt32(limiter.running); v != 1 {
		t.Fatal(v)
	}

	tickerC <- time.Unix(2, 0)
	<-timeNowIn
	timeNowOut <- time.Unix(2, 0)
	<-timeNowIn
	timeNowOut <- time.Unix(2, 0)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if v, ok := limiter.categories.Load(category); ok {
		t.Errorf("cleanup did not remove category as expected: %v", v.(*categoryData).events.Slice())
	}
}
