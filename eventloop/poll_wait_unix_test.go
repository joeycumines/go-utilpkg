//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestFastPollerNativeEmptyWait(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)

	if count, err := poller.PollIO(0); err != nil || count != 0 {
		t.Fatalf("PollIO(0) = (%d, %v), want (0, nil)", count, err)
	}
	started := time.Now()
	if count, err := poller.PollIO(50); err != nil || count != 0 {
		t.Fatalf("PollIO(50) = (%d, %v), want (0, nil)", count, err)
	}
	if elapsed := time.Since(started); elapsed < 49*time.Millisecond {
		t.Fatalf("PollIO(50) returned before its native timeout: %v", elapsed)
	}
}

func TestWaitPollFiniteInterruptionRetainsDeadline(t *testing.T) {
	now := time.Unix(0, 0)
	var attempts []time.Duration
	n, err := waitPoll(50, maxPollChunk, func() time.Time { return now }, func(timeout time.Duration) (int, error) {
		attempts = append(attempts, timeout)
		switch len(attempts) {
		case 1:
			now = now.Add(10 * time.Millisecond)
			return 0, unix.EINTR
		case 2:
			return 3, nil
		default:
			t.Fatalf("unexpected wait attempt %d", len(attempts))
			return 0, nil
		}
	})
	if err != nil || n != 3 {
		t.Fatalf("waitPoll = (%d, %v), want (3, nil)", n, err)
	}
	want := []time.Duration{50 * time.Millisecond, 40 * time.Millisecond}
	if len(attempts) != len(want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
	for i := range want {
		if attempts[i] != want[i] {
			t.Fatalf("attempt %d = %v, want %v", i, attempts[i], want[i])
		}
	}
}

func TestWaitPollRepeatedInterruptionExpiresOriginalDeadline(t *testing.T) {
	now := time.Unix(0, 0)
	var attempts []time.Duration
	n, err := waitPoll(50, maxPollChunk, func() time.Time { return now }, func(timeout time.Duration) (int, error) {
		attempts = append(attempts, timeout)
		if len(attempts) == 1 {
			now = now.Add(20 * time.Millisecond)
		} else {
			now = now.Add(31 * time.Millisecond)
		}
		return 0, unix.EINTR
	})
	if err != nil || n != 0 {
		t.Fatalf("waitPoll = (%d, %v), want (0, nil)", n, err)
	}
	want := []time.Duration{50 * time.Millisecond, 30 * time.Millisecond}
	if len(attempts) != len(want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
	for i := range want {
		if attempts[i] != want[i] {
			t.Fatalf("attempt %d = %v, want %v", i, attempts[i], want[i])
		}
	}
}

func TestWaitPollResamplesBeforeRetry(t *testing.T) {
	now := time.Unix(0, 0)
	nowCalls := 0
	waitCalls := 0
	n, err := waitPoll(50, maxPollChunk, func() time.Time {
		nowCalls++
		if nowCalls == 4 {
			now = now.Add(40 * time.Millisecond)
		}
		return now
	}, func(time.Duration) (int, error) {
		waitCalls++
		if waitCalls != 1 {
			t.Fatalf("unexpected retry after original deadline")
		}
		now = now.Add(10 * time.Millisecond)
		return 0, unix.EINTR
	})
	if err != nil || n != 0 {
		t.Fatalf("waitPoll = (%d, %v), want (0, nil)", n, err)
	}
	if waitCalls != 1 {
		t.Fatalf("wait calls = %d, want 1", waitCalls)
	}
}

func TestWaitPollResamplesBeforeNextChunk(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("int cannot represent the chunk boundary")
	}

	timeoutMs64 := int64(maxPollChunkMilliseconds) + 1
	timeoutMs := int(timeoutMs64)
	now := time.Unix(0, 0)
	nowCalls := 0
	waitCalls := 0
	n, err := waitPoll(timeoutMs, maxPollChunk, func() time.Time {
		nowCalls++
		if nowCalls == 4 {
			now = now.Add(time.Millisecond)
		}
		return now
	}, func(time.Duration) (int, error) {
		waitCalls++
		if waitCalls != 1 {
			t.Fatalf("unexpected next-chunk wait after original deadline")
		}
		now = now.Add(maxPollChunk)
		return 0, nil
	})
	if err != nil || n != 0 {
		t.Fatalf("waitPoll = (%d, %v), want (0, nil)", n, err)
	}
	if waitCalls != 1 {
		t.Fatalf("wait calls = %d, want 1", waitCalls)
	}
}

func TestWaitPollCappedAttemptRetainsDeadline(t *testing.T) {
	now := time.Unix(0, 0)
	var attempts []time.Duration
	n, err := waitPoll(50, 20*time.Millisecond, func() time.Time { return now }, func(timeout time.Duration) (int, error) {
		attempts = append(attempts, timeout)
		now = now.Add(timeout)
		return 0, nil
	})
	if err != nil || n != 0 {
		t.Fatalf("waitPoll = (%d, %v), want (0, nil)", n, err)
	}
	want := []time.Duration{20 * time.Millisecond, 20 * time.Millisecond, 10 * time.Millisecond}
	if len(attempts) != len(want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
	for i := range want {
		if attempts[i] != want[i] {
			t.Fatalf("attempt %d = %v, want %v", i, attempts[i], want[i])
		}
	}
}

func TestWaitPollZeroMakesOneAttempt(t *testing.T) {
	calls := 0
	n, err := waitPoll(0, maxPollChunk, time.Now, func(timeout time.Duration) (int, error) {
		calls++
		if timeout != 0 {
			t.Fatalf("timeout = %v, want 0", timeout)
		}
		return 0, unix.EINTR
	})
	if err != nil || n != 0 || calls != 1 {
		t.Fatalf("waitPoll = (%d, %v), calls %d; want (0, nil), calls 1", n, err, calls)
	}
}

func TestWaitPollInfiniteRetriesInterruption(t *testing.T) {
	calls := 0
	n, err := waitPoll(-1, maxPollChunk, time.Now, func(timeout time.Duration) (int, error) {
		calls++
		if timeout >= 0 {
			t.Fatalf("timeout = %v, want negative", timeout)
		}
		if calls < 3 {
			return 0, unix.EINTR
		}
		return 2, nil
	})
	if err != nil || n != 2 || calls != 3 {
		t.Fatalf("waitPoll = (%d, %v), calls %d; want (2, nil), calls 3", n, err, calls)
	}
}

func TestWaitPollReturnsNonInterruptionError(t *testing.T) {
	wantErr := errors.New("poll failed")
	n, err := waitPoll(50, maxPollChunk, time.Now, func(time.Duration) (int, error) {
		return 0, wantErr
	})
	if !errors.Is(err, wantErr) || n != 0 {
		t.Fatalf("waitPoll = (%d, %v), want (0, %v)", n, err, wantErr)
	}
}

func TestWaitPollInterruptionThenClosed(t *testing.T) {
	now := time.Unix(0, 0)
	calls := 0
	n, err := waitPoll(50, maxPollChunk, func() time.Time { return now }, func(time.Duration) (int, error) {
		calls++
		if calls == 1 {
			now = now.Add(time.Millisecond)
			return 0, unix.EINTR
		}
		return 0, errPollerClosed
	})
	if !errors.Is(err, errPollerClosed) || n != 0 || calls != 2 {
		t.Fatalf("waitPoll = (%d, %v), calls %d; want (0, errPollerClosed), calls 2", n, err, calls)
	}
}

func TestWaitPollOverDurationCapacityRetainsRemainder(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("int cannot represent a duration larger than time.Duration")
	}

	now := time.Now()
	timeoutMs64 := int64(maxPollMilliseconds) + 1
	timeoutMs := int(timeoutMs64)
	var attempts []time.Duration
	n, err := waitPoll(timeoutMs, maxPollChunk, func() time.Time { return now }, func(timeout time.Duration) (int, error) {
		attempts = append(attempts, timeout)
		return 0, nil
	})
	if err != nil || n != 0 {
		t.Fatalf("waitPoll = (%d, %v), want (0, nil)", n, err)
	}
	want := []time.Duration{maxPollChunk, maxPollChunk, time.Millisecond}
	if len(attempts) != len(want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
	for i := range want {
		if attempts[i] != want[i] {
			t.Fatalf("attempt %d = %v, want %v", i, attempts[i], want[i])
		}
	}
}

func TestWaitPollChunkBoundaryInterruptionRetainsRemainder(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("int cannot represent a duration larger than time.Duration")
	}

	started := time.Unix(0, 0)
	now := started
	timeoutMs64 := int64(maxPollMilliseconds) + 1
	timeoutMs := int(timeoutMs64)
	var attempts []time.Duration
	n, err := waitPoll(timeoutMs, maxPollChunk, func() time.Time { return now }, func(timeout time.Duration) (int, error) {
		attempts = append(attempts, timeout)
		if len(attempts) == 1 {
			now = started.Add(maxPollChunk)
			return 0, unix.EINTR
		}
		return 0, nil
	})
	if err != nil || n != 0 {
		t.Fatalf("waitPoll = (%d, %v), want (0, nil)", n, err)
	}
	want := []time.Duration{maxPollChunk, maxPollChunk, time.Millisecond}
	if len(attempts) != len(want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
	for i := range want {
		if attempts[i] != want[i] {
			t.Fatalf("attempt %d = %v, want %v", i, attempts[i], want[i])
		}
	}
}

func TestWaitPollOverDurationCapacityUsesElapsedTime(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("int cannot represent a duration larger than time.Duration")
	}

	started := time.Now()
	now := started
	timeoutMs64 := int64(maxPollMilliseconds) + 1
	timeoutMs := int(timeoutMs64)
	var attempts []time.Duration
	n, err := waitPoll(timeoutMs, maxPollChunk, func() time.Time { return now }, func(timeout time.Duration) (int, error) {
		attempts = append(attempts, timeout)
		if len(attempts) == 1 {
			now = started.Add(time.Millisecond)
			return 0, unix.EINTR
		}
		return 1, nil
	})
	if err != nil || n != 1 {
		t.Fatalf("waitPoll = (%d, %v), want (1, nil)", n, err)
	}
	want := []time.Duration{maxPollChunk, maxPollChunk - time.Millisecond}
	if len(attempts) != len(want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
	for i := range want {
		if attempts[i] != want[i] {
			t.Fatalf("attempt %d = %v, want %v", i, attempts[i], want[i])
		}
	}
}

func TestWaitPollChunkOvershootConsumesFollowingRemainder(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("int cannot represent a duration larger than time.Duration")
	}

	started := time.Unix(0, 0)
	now := started
	timeoutMs64 := int64(maxPollMilliseconds) + 1
	timeoutMs := int(timeoutMs64)
	var attempts []time.Duration
	n, err := waitPoll(timeoutMs, maxPollChunk, func() time.Time { return now }, func(timeout time.Duration) (int, error) {
		attempts = append(attempts, timeout)
		switch len(attempts) {
		case 1:
			now = now.Add(maxPollChunk)
			return 0, nil
		case 2:
			now = now.Add(maxPollChunk + time.Millisecond)
			return 0, unix.EINTR
		default:
			t.Fatalf("unexpected wait attempt %d after complete timeout", len(attempts))
			return 0, nil
		}
	})
	if err != nil || n != 0 {
		t.Fatalf("waitPoll = (%d, %v), want (0, nil)", n, err)
	}
	want := []time.Duration{maxPollChunk, maxPollChunk}
	if len(attempts) != len(want) {
		t.Fatalf("attempts = %v, want %v", attempts, want)
	}
	for i := range want {
		if attempts[i] != want[i] {
			t.Fatalf("attempt %d = %v, want %v", i, attempts[i], want[i])
		}
	}
}

func TestMaximumFinitePollChunks(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("int cannot represent a duration larger than time.Duration")
	}

	maxInt64 := int64(^uint64(0) >> 1)
	timeoutMs := int(maxInt64)
	wantFullChunks := uint64(maxInt64) / uint64(maxPollChunkMilliseconds)
	wantRemainderMilliseconds := uint64(maxInt64) % uint64(maxPollChunkMilliseconds)
	if wantFullChunks != 2_000_000 || wantRemainderMilliseconds != 775_807 {
		t.Fatalf("test constants changed: full chunks %d, remainder %dms", wantFullChunks, wantRemainderMilliseconds)
	}

	calls := uint64(0)
	last := time.Duration(0)
	now := time.Unix(0, 0)
	n, err := waitPoll(timeoutMs, maxPollChunk, func() time.Time { return now }, func(timeout time.Duration) (int, error) {
		calls++
		last = timeout
		return 0, nil
	})
	if err != nil || n != 0 {
		t.Fatalf("waitPoll = (%d, %v), want (0, nil)", n, err)
	}
	if want := wantFullChunks + 1; calls != want {
		t.Fatalf("wait calls = %d, want %d", calls, want)
	}
	if want := time.Duration(wantRemainderMilliseconds) * time.Millisecond; last != want {
		t.Fatalf("last attempt = %v, want %v", last, want)
	}
}
