//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"time"

	"golang.org/x/sys/unix"
)

const maxPollMilliseconds = int64((1<<63 - 1) / int64(time.Millisecond))

const maxPollChunkMilliseconds = maxPollMilliseconds / 2

const maxPollChunk = time.Duration(maxPollChunkMilliseconds) * time.Millisecond

// waitPoll preserves a finite poll deadline across interrupted syscalls. The
// maximum attempt duration supports platforms whose native timeout field is
// narrower than time.Duration; an empty capped attempt continues against the
// same deadline instead of completing the caller's longer wait early.
func waitPoll(
	timeoutMs int,
	maxAttempt time.Duration,
	now func() time.Time,
	wait func(time.Duration) (int, error),
) (int, error) {
	if timeoutMs == 0 {
		n, err := wait(0)
		if err == unix.EINTR {
			return 0, nil
		}
		return n, err
	}
	if timeoutMs < 0 {
		for {
			n, err := wait(-1)
			if err != unix.EINTR {
				return n, err
			}
		}
	}

	remainingMilliseconds := uint64(timeoutMs)
	overrun := time.Duration(0)
	sampledAt := now()
	for remainingMilliseconds != 0 {
		chunkMilliseconds := min(remainingMilliseconds, uint64(maxPollChunkMilliseconds))
		chunkTotal := time.Duration(chunkMilliseconds) * time.Millisecond
		if overrun >= chunkTotal {
			overrun -= chunkTotal
			remainingMilliseconds -= chunkMilliseconds
			continue
		}
		chunkTotal -= overrun
		overrun = 0
		chunkStarted := sampledAt
	chunkLoop:
		for {
			beforeAttempt := now()
			elapsed := max(beforeAttempt.Sub(chunkStarted), 0)
			if elapsed >= chunkTotal {
				overrun = elapsed - chunkTotal
				sampledAt = beforeAttempt
				remainingMilliseconds -= chunkMilliseconds
				break chunkLoop
			}
			chunkRemaining := chunkTotal - elapsed
			attempt := min(chunkRemaining, maxAttempt)
			n, err := wait(attempt)
			switch {
			case err == nil && n != 0:
				return n, nil
			case err != nil && err != unix.EINTR:
				return 0, err
			}

			afterAttempt := now()
			sampledAt = afterAttempt
			elapsed = max(afterAttempt.Sub(chunkStarted), 0)
			if err == nil && attempt == chunkRemaining {
				if elapsed > chunkTotal {
					overrun = elapsed - chunkTotal
				}
				remainingMilliseconds -= chunkMilliseconds
				break chunkLoop
			}
			if elapsed >= chunkTotal {
				overrun = elapsed - chunkTotal
				remainingMilliseconds -= chunkMilliseconds
				break chunkLoop
			}
		}
	}
	return 0, nil
}
