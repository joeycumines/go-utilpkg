package prompt

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/joeycumines/go-prompt/debug"
)

// Sync protocol constants for deterministic terminal I/O synchronization.
//
// This implements a request-response mechanism using APC (Application Program Command)
// escape sequences. APC sequences are explicitly ignored by standard terminal emulators
// (xterm, etc.) per ECMA-48/VT220+ specifications, making them safe for custom protocols.
//
// Protocol:
//   - Request:  ESC _ go-prompt:sync:<id> ESC \
//   - Response: ESC _ go-prompt:sync-ack:<id> ESC \
//
// Where:
//   - ESC _ (0x1b 0x5f) is the APC introducer
//   - ESC \ (0x1b 0x5c) is ST (String Terminator)
//   - <id> is a caller-provided identifier (should be unique per request)
//
// Usage:
// The test harness sends a sync request as PTY input.
// go-prompt detects it (does NOT process as user input), and queues a response.
// The response is written after the current render cycle completes.
// The test harness waits for the response in output to confirm synchronization.

const (
	// SyncPrefix is the protocol prefix for sync requests.
	// Format: ESC _ go-prompt:sync:
	SyncPrefix = "\x1b_go-prompt:sync:"

	// SyncAckPrefix is the protocol prefix for sync responses.
	// Format: ESC _ go-prompt:sync-ack:
	SyncAckPrefix = "\x1b_go-prompt:sync-ack:"

	// StringTerminator is ST (ESC \) per ECMA-48.
	StringTerminator = "\x1b\\"

	// maxSyncBufferSize limits the internal parser buffer for APC sequences.
	// Prevents unbounded growth (DoS / OOM) if an input source sends a very
	// long, unterminated APC stream. 4KB is more than enough for ids used by
	// test harnesses.
	maxSyncBufferSize = 4096
)

var (
	syncPrefixBytes = []byte(SyncPrefix)
	stringTermBytes = []byte(StringTerminator)
)

// syncState manages the sync protocol state for a Prompt instance.
type syncState struct {
	mu       sync.Mutex
	enabled  bool
	pending  []string // pending sync IDs to acknowledge
	renderer *Renderer
	// buf accumulates bytes from fragmented reads so we can parse APC sequences
	// that span multiple Read calls.
	buf []byte
}

// newSyncState creates a new syncState.
func newSyncState(r *Renderer) *syncState {
	return &syncState{
		renderer: r,
	}
}

// SetEnabled enables or disables the sync protocol.
// When disabled, sync requests are passed through as regular input.
func (s *syncState) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
}

// Enabled returns whether the sync protocol is enabled.
func (s *syncState) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

// QueueAck queues a sync acknowledgment for the given ID.
// The acknowledgment will be written on the next flush.
func (s *syncState) QueueAck(ID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled {
		s.pending = append(s.pending, ID)
	}
}

// FlushAcks writes all pending sync acknowledgments to the renderer's writer.
// This should be called after the renderer has flushed its output buffer.
func (s *syncState) FlushAcks() {
	s.mu.Lock()
	pending := s.pending
	s.pending = nil
	s.mu.Unlock()

	if len(pending) == 0 || s.renderer == nil {
		return
	}

	// Write each acknowledgment directly to the output
	for _, ID := range pending {
		ack := BuildSyncAck(ID)
		s.renderer.out.WriteRaw([]byte(ack))
	}

	// Ensure we flush so ACK bytes are forced out to the underlying FD.
	// The renderer's main loop may have flushed earlier; however in the
	// observed deadlock the renderer flushed before pending acks were
	// written to the buffer. We must explicitly flush here to guarantee
	// the test harness (or any external consumer) sees the ACK bytes.
	// This keeps writes deterministic and avoids ACKs being lost in
	// writer buffers.
	if err := s.renderer.out.Flush(); err != nil {
		debug.Log(fmt.Sprintf("failed to flush sync acks: %v", err))
	}
}

// ProcessInputBytes feeds arbitrary input bytes into the internal parser
// state, extracting complete sync request IDs and returning the remaining
// input that should be treated as regular user input. This method is
// safe to call from the reader goroutine and will buffer partial APC
// sequences so they are not leaked as regular input.
func (s *syncState) ProcessInputBytes(input []byte) (remaining []byte, ids []string) {
	s.mu.Lock()
	// append incoming data to our buffer. We intentionally do NOT truncate
	// here because we must process the newly-arrived data first. Truncating
	// before parsing would discard the oldest bytes of the current input
	// chunk and could result in data loss for a large paste. We will enforce
	// the max cap only when saving remaining partial sequences for the next
	// cycle.
	s.buf = append(s.buf, input...)
	buf := s.buf
	s.mu.Unlock()

	var result []byte
	i := 0
	for i < len(buf) {
		// Look for the sync prefix starting at i
		rel := bytes.Index(buf[i:], syncPrefixBytes)
		if rel == -1 {
			// No prefix found. We may need to keep a trailing partial prefix
			// in case it starts at the end of the buffer. Detect the largest
			// suffix of buf that is a prefix of syncPrefixBytes and keep it.
			// Everything before that is regular input.
			keep := 0
			maxKeep := len(syncPrefixBytes) - 1
			if maxKeep < 0 {
				maxKeep = 0
			}
			// Find longest k such that buf[len(buf)-k:] == syncPrefix[:k]
			for k := 1; k <= maxKeep && k <= len(buf); k++ {
				if bytes.Equal(buf[len(buf)-k:], syncPrefixBytes[:k]) {
					keep = k
				}
			}

			if keep > 0 {
				// everything except the final 'keep' bytes is regular input.
				// Only append bytes starting from the current cursor 'i' to
				// avoid re-emitting already-processed bytes (duplication).
				if i < len(buf)-keep {
					result = append(result, buf[i:len(buf)-keep]...)
				}
				// save trailing partial prefix and ensure it fits cap for the
				// next cycle (protect against oversized partials).
				s.mu.Lock()
				s.buf = append([]byte(nil), buf[len(buf)-keep:]...)
				if len(s.buf) > maxSyncBufferSize {
					s.buf = s.buf[len(s.buf)-maxSyncBufferSize:]
				}
				s.mu.Unlock()
			} else {
				// nothing to keep; all of buf is regular input
				result = append(result, buf[i:]...)
				s.mu.Lock()
				s.buf = s.buf[:0]
				s.mu.Unlock()
			}
			return result, ids
		}

		idx := i + rel
		// append content before the sync request
		if idx > i {
			result = append(result, buf[i:idx]...)
		}

		prefixEnd := idx + len(syncPrefixBytes)
		if prefixEnd > len(buf) {
			// Shouldn't happen because Index found the full prefix, but be safe
			s.mu.Lock()
			s.buf = append([]byte(nil), buf[idx:]...)
			s.mu.Unlock()
			return result, ids
		}

		// Look for terminator after prefix
		rest := buf[prefixEnd:]
		termIdx := bytes.Index(rest, stringTermBytes)
		if termIdx == -1 {
			// incomplete APC - keep from idx to end and enforce cap
			s.mu.Lock()
			// keep the suffix starting at idx for the next cycle
			s.buf = append([]byte(nil), buf[idx:]...)
			if len(s.buf) > maxSyncBufferSize {
				// When we need to truncate the buffer to enforce the cap,
				// flush the oldest bytes as regular input so we don't lose
				// user-provided data that merely looked like a sync prefix.
				overflow := len(s.buf) - maxSyncBufferSize
				// Append the discarded bytes to result so they'll be treated
				// as normal user input by the caller.
				result = append(result, s.buf[:overflow]...)
				s.buf = s.buf[overflow:]
			}
			s.mu.Unlock()
			return result, ids
		}

		// Check if the total sequence length exceeds our maximum allowed size.
		// totalLen = prefix + ID + terminator
		totalLen := len(syncPrefixBytes) + termIdx + len(stringTermBytes)
		if totalLen > maxSyncBufferSize {
			// The sequence is too large. We treat this as invalid (not a sync request)
			// to prevent unbounded allocations for the ID string.
			// We consume just the first byte of the prefix as text, effectively
			// breaking the prefix pattern, and resume scanning from the next byte.
			// This "fails open" by printing the garbage to the screen rather than
			// swallowing or deadlocking on huge inputs.
			result = append(result, buf[idx])
			i = idx + 1
			continue
		}

		id := string(rest[:termIdx])
		// Accept empty IDs as valid -- the protocol allows empty IDs and
		// test harnesses may use them. Preserve order; append even when
		// id == "".
		ids = append(ids, id)

		// move i past the terminator
		i = prefixEnd + termIdx + len(stringTermBytes)
	}

	// we've consumed the whole buffer and have no partial sequences
	s.mu.Lock()
	s.buf = s.buf[:0]
	s.mu.Unlock()

	return result, ids
}

// BuildSyncRequest builds a sync request string for the given ID.
// The ID should be unique for each request to allow proper matching.
func BuildSyncRequest(id string) string {
	return SyncPrefix + id + StringTerminator
}

// BuildSyncAck builds a sync acknowledgment string for the given ID.
func BuildSyncAck(id string) string {
	// Sanitize the id to avoid control bytes (especially ESC) leaking
	// into the terminal stream. Replace control/unprintable bytes with '?'.
	sanitized := make([]rune, 0, len(id))
	for _, r := range id {
		// Control characters (below 0x20), DEL (0x7f), ESC and backslash
		// should never appear verbatim in terminal output. Convert them to
		// '?' so callers are less at risk of unpredictable terminal control
		// sequences via sync IDs.
		if r < 0x20 || r == 0x7f || r == '\\' {
			sanitized = append(sanitized, '?')
		} else {
			sanitized = append(sanitized, r)
		}
	}
	return SyncAckPrefix + string(sanitized) + StringTerminator
}
