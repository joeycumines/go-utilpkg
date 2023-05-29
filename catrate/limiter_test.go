package catrate

import (
	"math/rand"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
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
	*limiter.running = 1

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

func TestLimiter_worker_cleanupRace(t *testing.T) {
	{
		oldTimeNow := timeNow
		defer func() { timeNow = oldTimeNow }()
		oldTimeNewTicker := timeNewTicker
		defer func() { timeNewTicker = oldTimeNewTicker }()
	}

	nowIn := make(chan struct{})
	nowOut := make(chan time.Time)
	timeNow = func() time.Time {
		nowIn <- struct{}{}
		return <-nowOut
	}

	tickerIn := make(chan time.Duration)
	tickerOut := make(chan *time.Ticker)
	timeNewTicker = func(d time.Duration) *time.Ticker {
		tickerIn <- d
		return <-tickerOut
	}

	now := new(time.Time)
	*now = time.Unix(0, 12356677542152131)
	getNow := func() time.Time {
		return *(*time.Time)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&now))))
	}
	setNow := func(v time.Time) {
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&now)), unsafe.Pointer(&v))
	}
	var incNow func(d time.Duration)
	{
		r := rand.New(rand.NewSource(12355123))
		incNow = func(d time.Duration) {
			if d <= 0 {
				t.Error(d)
				panic("invalid duration")
			}
			d = time.Duration(r.Int63n(int64(d)) + 1)
			o := (*time.Time)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&now))))
			v := o.Add(d)
			if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&now)), unsafe.Pointer(o), unsafe.Pointer(&v)) {
				t.Error(`race on incrementing "now"`)
			}
		}
	}

	limiter := &Limiter{
		running:   new(int32),
		retention: time.Hour * 24,
	}
	*limiter.running = 1

	cat1 := categoryDataPool.Get().(*categoryData)
	cat1Last := getNow()
	*cat1.atomic = [2]int64{nextZeroValue, cat1Last.UnixNano()}
	limiter.categories.Store(1, cat1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		var success bool
		defer func() {
			if !success {
				t.Errorf("unexpected panic: %v", recover())
			}
		}()
		limiter.worker()
		success = true
	}()

	if v := <-tickerIn; v != time.Hour*12 {
		t.Fatal(v)
	}
	tickerCh := make(chan time.Time)
	ticker := time.NewTicker(1)
	ticker.C = tickerCh
	tickerOut <- ticker

	tickerCh <- time.Time{}

	// checking cat1
	<-nowIn
	incNow(time.Second)
	nowOut <- getNow()

	// not time to delete yet

	tickerCh <- time.Time{}

	// checking cat1 again
	<-nowIn
	incNow(time.Second)
	nowOut <- getNow()

	whileNotSentToTicker := func() {
		sentToTicker := make(chan struct{})
		go func() {
			tickerCh <- time.Time{}
			close(sentToTicker)
		}()
		for {
			select {
			case <-sentToTicker:
				return
			case <-nowIn:
				nowOut <- getNow()
				select {
				case <-sentToTicker:
					return
				default:
				}
			}
		}
	}

	const addCount = 10000

	// adding other tickers work fine and can happen in parallel
	{
		addDone := make(chan struct{})
		go func() {
			for i := 0; i < addCount; i++ {
				cat := categoryDataPool.Get().(*categoryData)
				incNow(time.Second)
				last := getNow()
				*cat.atomic = [2]int64{nextZeroValue, last.UnixNano()}
				limiter.categories.Store(-(i + 1), cat)
				time.Sleep(100 * time.Nanosecond)
			}
			incNow(time.Second)
			close(addDone)
		}()
		{
			ticker := time.NewTicker(1)
			defer ticker.Stop()
			tickerCh <- time.Time{}
		CheckLoop:
			for {
				select {
				case <-addDone:
					// twice because the worker range needs to after addDone in order for the final range to be correct
					whileNotSentToTicker()
					whileNotSentToTicker()
					break CheckLoop
				case <-ticker.C:
					whileNotSentToTicker()
				}
			}
		}
		{
			tickerDone := make(chan struct{})
			go func() {
				tickerCh <- time.Time{}
				close(tickerDone)
			}()
			var size int
			limiter.categories.Range(func(_, _ interface{}) bool {
				size++
				<-nowIn
				incNow(time.Second)
				nowOut <- getNow()
				return true
			})
			select {
			case <-tickerCh:
			case <-tickerDone:
			}
			if size != addCount+1 {
				t.Fatal(size)
			}
		}
	}

	// bounds check: cat1 won't be deleted until after last+retention
	{
		cat1Deadline := cat1Last.Add(limiter.retention)
		if now := getNow(); !now.Before(cat1Deadline) {
			t.Fatal(now, cat1Deadline)
		}
		setNow(cat1Deadline)
		whileNotSentToTicker()
		whileNotSentToTicker()
		if _, ok := limiter.categories.Load(1); !ok {
			t.Fatal("cat1 deleted too early")
		}
	}

	// bounds check: cat1 deletion attempted after last+retention
	{
		limiter.mu.Lock()
		// still works fine
		whileNotSentToTicker()
		setNow(getNow().Add(1))
		tickerDone := make(chan struct{})
		go func() {
			tickerCh <- time.Time{}
			close(tickerDone)
		}()
		for {
			select {
			case <-nowIn:
				nowOut <- getNow()
				continue
			default:
			}
			time.Sleep(time.Millisecond * 50)
			select {
			case <-nowIn:
				nowOut <- getNow()
				continue
			default:
			}
			break
		}
		time.Sleep(time.Millisecond * 70)
		// shouldn't be deleted yet - mutex
		if v, ok := limiter.categories.Load(1); !ok || v != cat1 {
			t.Fatal("cat1 deleted too early")
		}
		if cat1.atomic[1] != cat1Last.UnixNano() {
			t.Fatal()
		}
		cat1.atomic[1]++
		limiter.mu.Unlock()
		for {
			select {
			case <-nowIn:
				nowOut <- getNow()
				continue
			case <-tickerCh:
			case <-tickerDone:
			}
			break
		}
		whileNotSentToTicker()
		whileNotSentToTicker()
		// shouldn't be deleted yet
		if v, ok := limiter.categories.Load(1); !ok || v != cat1 {
			t.Fatal("cat1 deleted too early")
		}
	}

	// bounds check: cat1 deletion success
	{
		setNow(getNow().Add(1))
		whileNotSentToTicker()
		whileNotSentToTicker()
		whileNotSentToTicker()
		if _, ok := limiter.categories.Load(1); ok {
			t.Fatal("cat1 should have been deleted")
		}
	}

	// no others should be deleted
	{
		var size int
		limiter.categories.Range(func(_, _ interface{}) bool {
			size++
			return true
		})
		if size != addCount {
			t.Fatal(size)
		}
	}

	// increase time ahead, make everything cleaned up, wait for worker to finish
	setNow(getNow().Add(limiter.retention + 1))
	for i := 0; i < 3; i++ {
		sentToTicker := make(chan struct{})
		go func() {
			select {
			case <-done:
			case tickerCh <- time.Time{}:
			}
			close(sentToTicker)
		}()
		for {
			select {
			case <-done:
			case <-sentToTicker:
			case <-nowIn:
				nowOut <- getNow()
				select {
				case <-sentToTicker:
				default:
					continue
				}
			}
			break
		}
	}

	// the worker should be complete
	<-done

	// all categories should be deleted
	{
		var size int
		limiter.categories.Range(func(_, _ interface{}) bool {
			size++
			return true
		})
		if size != 0 {
			t.Fatal(size)
		}
	}

	// and running should be 0
	if *limiter.running != 0 {
		t.Fatal(limiter.running)
	}
}
