package prompt

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"testing"
)

func TestBuildSyncRequest(t *testing.T) {
	id := "test-123"
	req := BuildSyncRequest(id)
	expected := "\x1b_go-prompt:sync:test-123\x1b\\"
	if req != expected {
		t.Errorf("BuildSyncRequest(%q) = %q, want %q", id, req, expected)
	}
}

func TestBuildSyncAck(t *testing.T) {
	id := "test-456"
	ack := BuildSyncAck(id)
	expected := "\x1b_go-prompt:sync-ack:test-456\x1b\\"
	if ack != expected {
		t.Errorf("BuildSyncAck(%q) = %q, want %q", id, ack, expected)
	}
}

// testWriter is a small in-memory Writer used by tests to capture flushed bytes.
type testWriter struct {
	VT100Writer
	FlushedData []byte
	Flushed     bool
}

func (w *testWriter) Flush() error {
	w.FlushedData = append(w.FlushedData, w.buffer...)
	w.buffer = nil
	w.Flushed = true
	return nil
}

// errWriter simulates a writer that returns an error on Flush.
type errWriter struct {
	VT100Writer
	FlushedData []byte
	Flushed     bool
	Err         error
}

func (w *errWriter) Flush() error {
	w.FlushedData = append(w.FlushedData, w.buffer...)
	w.buffer = nil
	w.Flushed = true
	return w.Err
}

// TestFlushAcks verifies that FlushAcks writes pending acks to the renderer
// writer and calls Flush so buffered bytes are pushed out.
func TestFlushAcks(t *testing.T) {
	// use top-level testWriter

	tw := &testWriter{}
	r := NewRenderer()
	r.out = tw

	s := newSyncState(r)
	s.SetEnabled(true)

	s.QueueAck("id1")
	s.QueueAck("id2")

	// ensure pending queued
	s.mu.Lock()
	if len(s.pending) != 2 {
		s.mu.Unlock()
		t.Fatalf("expected 2 pending acks")
	}
	s.mu.Unlock()

	s.FlushAcks()

	if !tw.Flushed {
		t.Fatalf("expected writer.Flush to be called")
	}

	expected := BuildSyncAck("id1") + BuildSyncAck("id2")
	if string(tw.FlushedData) != expected {
		t.Fatalf("flushed = %q, want %q", string(tw.FlushedData), expected)
	}
}

// TestFlushAcksError ensures FlushAcks still writes pending acks and clears
// pending even when the underlying writer's Flush returns an error.
func TestFlushAcksError(t *testing.T) {
	// use package-scope errWriter

	ew := &errWriter{Err: fmt.Errorf("flush failed")}
	r := NewRenderer()
	r.out = ew

	s := newSyncState(r)
	s.SetEnabled(true)

	s.QueueAck("idE1")

	s.mu.Lock()
	if len(s.pending) != 1 {
		s.mu.Unlock()
		t.Fatalf("expected 1 pending ack")
	}
	s.mu.Unlock()

	// should not panic even if Flush returns an error
	s.FlushAcks()

	if !ew.Flushed {
		t.Fatalf("expected writer.Flush to be called")
	}

	expected := BuildSyncAck("idE1")
	if string(ew.FlushedData) != expected {
		t.Fatalf("flushed = %q, want %q", string(ew.FlushedData), expected)
	}

	// ensure pending was cleared even on Flush error
	s.mu.Lock()
	if len(s.pending) != 0 {
		t.Fatalf("expected pending acks to be cleared, got %d", len(s.pending))
	}
	s.mu.Unlock()
}

func TestSyncState(t *testing.T) {
	s := &syncState{}

	// Initially not enabled
	if s.Enabled() {
		t.Error("expected sync to be disabled initially")
	}

	// Enable it
	s.SetEnabled(true)
	if !s.Enabled() {
		t.Error("expected sync to be enabled after SetEnabled(true)")
	}

	// Queue some acks
	s.QueueAck("id1")
	s.QueueAck("id2")

	s.mu.Lock()
	if len(s.pending) != 2 {
		t.Errorf("expected 2 pending acks, got %d", len(s.pending))
	}
	s.mu.Unlock()

	// Disable it
	s.SetEnabled(false)
	if s.Enabled() {
		t.Error("expected sync to be disabled after SetEnabled(false)")
	}

	// Queue when disabled should not add
	s.QueueAck("id3")
	s.mu.Lock()
	if len(s.pending) != 2 {
		t.Errorf("expected still 2 pending acks (disabled), got %d", len(s.pending))
	}
	s.mu.Unlock()
}

func TestProcessInputBytesFragmentation(t *testing.T) {
	s := newSyncState(nil)
	s.SetEnabled(true)

	// First fragment: incomplete APC sequence
	rem, ids := s.ProcessInputBytes([]byte("\x1b_go-prompt:sy"))
	if string(rem) != "" {
		t.Fatalf("expected no remaining input, got %q", string(rem))
	}
	if len(ids) != 0 {
		t.Fatalf("expected no ids yet, got %v", ids)
	}

	// Second fragment completes the APC
	rem, ids = s.ProcessInputBytes([]byte("nc:123\x1b\\"))
	if string(rem) != "" {
		t.Fatalf("expected no remaining input after complete APC, got %q", string(rem))
	}
	if len(ids) != 1 || ids[0] != "123" {
		t.Fatalf("expected id '123', got %v", ids)
	}

	// Mixed case: text surrounding a fragmented APC
	s = newSyncState(nil)
	s.SetEnabled(true)
	rem, ids = s.ProcessInputBytes([]byte("hello \x1b_go-prompt:sync:partial"))
	if string(rem) != "hello " {
		t.Fatalf("expected 'hello ' remaining, got %q", string(rem))
	}
	if len(ids) != 0 {
		t.Fatalf("expected no ids yet, got %v", ids)
	}
	rem, ids = s.ProcessInputBytes([]byte("id\x1b\\ world"))
	if string(rem) != " world" {
		t.Fatalf("expected ' world' remaining, got %q", string(rem))
	}
	if len(ids) != 1 || ids[0] != "partialid" {
		t.Fatalf("expected id 'partialid', got %v", ids)
	}
}

func TestProcessInputBytesEmptyID(t *testing.T) {
	s := newSyncState(nil)
	s.SetEnabled(true)

	rem, ids := s.ProcessInputBytes([]byte("\x1b_go-prompt:sync:\x1b\\"))
	if string(rem) != "" {
		t.Fatalf("expected no remaining input, got %q", string(rem))
	}
	if len(ids) != 1 || ids[0] != "" {
		t.Fatalf("expected single empty id, got %v", ids)
	}
}

func TestProcessInputBytes_NoDuplicationOnTrailingPartial(t *testing.T) {
	s := newSyncState(nil)
	s.SetEnabled(true)

	// First fragment: complete sync, user input, then a trailing partial prefix
	first := BuildSyncRequest("A1") + "USERCMD" + SyncPrefix[:6]
	rem, ids := s.ProcessInputBytes([]byte(first))
	if len(ids) != 1 || ids[0] != "A1" {
		t.Fatalf("expected ids [A1], got %v", ids)
	}
	if string(rem) != "USERCMD" {
		t.Fatalf("expected remaining user input 'USERCMD', got %q", string(rem))
	}

	// Complete the trailing partial to ensure no duplication occurs.
	// The first fragment left SyncPrefix[:6] in the buffer; complete it by
	// sending the remainder of the prefix + the id + terminator.
	tail := SyncPrefix[6:]
	rem2, ids2 := s.ProcessInputBytes([]byte(tail + "SYNC2" + StringTerminator))
	if len(ids2) != 1 || ids2[0] != "SYNC2" {
		t.Fatalf("expected new id 'SYNC2', got %v", ids2)
	}
	if string(rem2) != "" {
		t.Fatalf("expected no regular remaining input on completion, got %q", string(rem2))
	}
}

// Ensure large inputs (pastes) do not lose the head of user content
// due to premature truncation of the internal buffer.
func TestProcessInputBytes_LargePastePreservesHead(t *testing.T) {
	s := newSyncState(nil)
	s.SetEnabled(true)

	// Build a large payload that exceeds maxSyncBufferSize, but starts with
	// a sync request and some user data that must not be discarded.
	header := BuildSyncRequest("HEAD") + "HELLO-START"
	filler := make([]byte, maxSyncBufferSize+500)
	for i := range filler {
		filler[i] = 'x'
	}
	// Ensure the very end contains a partial prefix so the parser retains
	// a trailing fragment into s.buf.
	tail := SyncPrefix[:10]

	payload := append([]byte(header), filler...)
	payload = append(payload, []byte(tail)...)

	rem, ids := s.ProcessInputBytes(payload)

	// The initial sync ID should be detected and user input between it and
	// the trailing partial should be preserved (HELLO-START)
	if len(ids) != 1 || ids[0] != "HEAD" {
		t.Fatalf("expected HEAD id, got %v", ids)
	}
	if !bytes.Contains(rem, []byte("HELLO-START")) {
		t.Fatalf("expected remaining to include HELLO-START, got %q", string(rem))
	}

	// Ensure internal buffer does not exceed the configured cap
	if len(s.buf) > maxSyncBufferSize {
		t.Fatalf("syncState buf grew beyond cap: %d > %d", len(s.buf), maxSyncBufferSize)
	}
}

func TestBuildSyncAckSanitizesDEL(t *testing.T) {
	id := string([]byte{'h', 'i', 0x7f, 'x'})
	ack := BuildSyncAck(id)
	if bytes.Contains([]byte(ack), []byte{0x7f}) {
		t.Fatalf("expected DEL to be sanitized, but found in ack: %q", ack)
	}
	if !bytes.Contains([]byte(ack), []byte{'?'}) {
		t.Fatalf("expected sanitized character '?' to appear in ack: %q", ack)
	}
}

func TestProcessInputBytesBufferCap(t *testing.T) {
	s := newSyncState(nil)
	s.SetEnabled(true)

	// Construct input exceeding maxSyncBufferSize and feed it.
	big := make([]byte, maxSyncBufferSize+500)
	for i := range big {
		big[i] = 'a'
	}

	// process as fragment; it should not grow s.buf beyond maxSyncBufferSize
	s.ProcessInputBytes(big)
	if len(s.buf) > maxSyncBufferSize {
		t.Fatalf("syncState buf grew beyond cap: %d > %d", len(s.buf), maxSyncBufferSize)
	}
}

// FuzzSyncProtocol_Fragmentation uses the fuzzer input to seed a generator
// that creates interleaved user input and Sync requests, fragments them,
// and verifies reassembly.
func FuzzSyncProtocol_Fragmentation(f *testing.F) {
	// 1. Add an initial seed corpus to kickstart the fuzzer.
	// This ensures we at least cover the "clean" case from the original test.
	f.Add([]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8})

	f.Fuzz(func(t *testing.T, data []byte) {
		// 2. Map the fuzz data to the RNG seed.
		// We use the bytes provided by the fuzzer to determine the
		// sequence of events (Consumer-based Fuzzing).
		var seed [32]byte
		copy(seed[:], data) // Pads with 0s if data is short, truncates if long.

		// Initialize the RNG with the fuzzer-controlled seed.
		rng := rand.New(rand.NewChaCha8(seed))

		const maxChunkSize = 10

		// --- Build "Golden" Stream (Logic Retained) ---
		var fullStream bytes.Buffer
		var expectedIDs []string
		var expectedUserOutput bytes.Buffer

		// Generate a random scenario based on the seeded RNG
		opCount := int(rng.Int32N(50)) + 10 // 10 to 60 operations
		for j := 0; j < opCount; j++ {
			if rng.Float32() < 0.3 {
				// Append Sync Request
				// Note: using 'fuzz' in ID to distinguish runs if debugging
				id := fmt.Sprintf("id-fuzz-%d", j)
				fullStream.WriteString(BuildSyncRequest(id))
				expectedIDs = append(expectedIDs, id)
			} else {
				// Append User Input
				input := fmt.Sprintf("text_%d", j)
				if rng.Float32() < 0.1 {
					input += "\x1b" // Random raw ESC
				}
				fullStream.WriteString(input)
				expectedUserOutput.WriteString(input)
			}
		}

		// Append neutral terminator (Logic Retained)
		fullStream.WriteString(".")
		expectedUserOutput.WriteString(".")

		// --- Fragment the stream (Logic Retained) ---
		originalBytes := fullStream.Bytes()
		var chunks [][]byte
		offset := 0
		for offset < len(originalBytes) {
			remaining := len(originalBytes) - offset
			size := int(rng.Int32N(maxChunkSize)) + 1
			if size > remaining {
				size = remaining
			}
			chunks = append(chunks, originalBytes[offset:offset+size])
			offset += size
		}

		// --- Feed the grinder (Logic Retained) ---
		s := newSyncState(nil)
		s.SetEnabled(true)

		var detectedIDs []string
		var detectedUserOutput bytes.Buffer

		for _, chunk := range chunks {
			rem, ids := s.ProcessInputBytes(chunk)
			detectedUserOutput.Write(rem)
			detectedIDs = append(detectedIDs, ids...)
		}

		// --- Verify (Logic Retained) ---
		if detectedUserOutput.String() != expectedUserOutput.String() {
			t.Fatalf("User output mismatch.\nSeed (Hex): %x\nWant: %q\nGot:  %q",
				seed, expectedUserOutput.String(), detectedUserOutput.String())
		}

		if len(detectedIDs) != len(expectedIDs) {
			t.Fatalf("ID count mismatch.\nSeed (Hex): %x\nWant %d, Got %d",
				seed, len(expectedIDs), len(detectedIDs))
		}

		for k := range expectedIDs {
			if detectedIDs[k] != expectedIDs[k] {
				t.Fatalf("ID mismatch at index %d.\nSeed (Hex): %x\nWant %q, Got %q",
					k, seed, expectedIDs[k], detectedIDs[k])
			}
		}
	})
}

// TestSyncProtocol_DoS_UnboundedGrowth verifies that sending an infinite stream
// of Sync Prefixes without terminators does not crash the system or explode memory.
func TestSyncProtocol_DoS_UnboundedGrowth(t *testing.T) {
	s := newSyncState(nil)
	s.SetEnabled(true)

	// Feed "ESC _ go-prompt:sync:" (The prefix)
	// Followed by 10MB of 'A's without a terminator (ESC \)
	prefix := []byte(SyncPrefix)
	junkSize := 1024 * 1024 * 10 // 10MB
	chunkSize := 4096

	// Feed prefix
	s.ProcessInputBytes(prefix)

	// Feed Junk
	for i := 0; i < junkSize; i += chunkSize {
		chunk := make([]byte, chunkSize)
		for k := range chunk {
			chunk[k] = 'A'
		}
		s.ProcessInputBytes(chunk)

		s.mu.Lock()
		bufLen := len(s.buf)
		s.mu.Unlock()

		// The internal buffer should NEVER exceed maxSyncBufferSize (4096)
		// It might be temporarily larger inside the function before truncation,
		// but the state persisting between calls must be capped.
		if bufLen > maxSyncBufferSize {
			t.Fatalf("DoS Check Failed: Internal buffer grew to %d bytes (limit %d)",
				bufLen, maxSyncBufferSize)
		}
	}

	// Also verify that when a single, large payload that exceeds the cap
	// is processed in one call, any bytes that would be discarded by the
	// cap are instead returned to the caller (no data loss).
	s2 := newSyncState(nil)
	s2.SetEnabled(true)
	// Build payload = prefix + filler > maxSyncBufferSize
	filler := make([]byte, maxSyncBufferSize+500)
	for i := range filler {
		filler[i] = 'A'
	}
	payload := append(prefix, filler...)

	rem, _ := s2.ProcessInputBytes(payload)
	// Expect that overflow bytes (len(payload) - maxSyncBufferSize) were returned
	expectedOverflow := len(payload) - maxSyncBufferSize
	if len(rem) != expectedOverflow {
		t.Fatalf("Expected %d overflow bytes returned, got %d", expectedOverflow, len(rem))
	}
	if !bytes.Equal(rem, payload[:expectedOverflow]) {
		t.Fatalf("Returned overflow bytes do not match expected prefix bytes")
	}
	s2.mu.Lock()
	if len(s2.buf) != maxSyncBufferSize {
		t.Fatalf("Expected internal buffer to retain %d bytes, got %d", maxSyncBufferSize, len(s2.buf))
	}
	s2.mu.Unlock()
}

// TestSyncProtocol_HugeID verifies behavior when a Sync ID is larger than the buffer limit.
// The expected behavior is that the prefix is eventually discarded/truncated,
// effectively treating the massive sequence as raw text to prevent deadlocks.
func TestSyncProtocol_HugeID(t *testing.T) {
	s := newSyncState(nil)
	s.SetEnabled(true)

	// Construct a payload: Prefix + (MaxBuffer + 100 bytes of ID) + Terminator
	payload := bytes.Buffer{}
	payload.WriteString(SyncPrefix)
	for i := 0; i < maxSyncBufferSize+100; i++ {
		payload.WriteByte('X')
	}
	payload.WriteString(StringTerminator)

	// Feed it all at once
	rem, ids := s.ProcessInputBytes(payload.Bytes())

	// Since the ID fits in memory but exceeds the logic cap for a "valid" sync sequence,
	// the parser should have truncated the buffer at some point, losing the prefix.
	// Therefore, NO valid ID should be detected.
	if len(ids) > 0 {
		t.Fatalf("Expected huge ID to be rejected/ignored, but got: %v", ids)
	}

	// The content should be returned as user text because validation failed.
	// Ensure that no bytes were silently lost: the returned remainder must
	// equal the original payload since the parser must 'fail-open' to user input.
	if !bytes.Equal(rem, payload.Bytes()) {
		t.Fatalf("Expected returned remainder to equal original payload, but it did not")
	}
	s.mu.Lock()
	bufLen := len(s.buf)
	s.mu.Unlock()

	// The buffer should be flushed or holding only the tail.
	if bufLen > maxSyncBufferSize {
		t.Errorf("Buffer retained too much data: %d", bufLen)
	}

	// Verify we can recover and process a valid request immediately after
	rem2, ids2 := s.ProcessInputBytes([]byte(BuildSyncRequest("RECOVERY")))
	if len(ids2) != 1 || ids2[0] != "RECOVERY" {
		t.Fatalf("Failed to recover after huge ID. Got rem: %q, ids: %v", rem2, ids2)
	}
}

// TestSyncProtocol_NestedAndInterleaved verifies edge cases with 'fake' prefixes
// appearing inside other sequences.
func TestSyncProtocol_NestedAndInterleaved(t *testing.T) {
	s := newSyncState(nil)
	s.SetEnabled(true)

	// Case 1: A Sync Prefix appearing INSIDE a Sync ID.
	// Protocol: The parser stops at "ESC \". It does not recursively parse.
	// Input:  ESC _ go-prompt:sync:  FAKE_PREFIX_HERE  ESC \
	// ID should be: "FAKE_PREFIX_HERE" (including the raw bytes of the prefix)
	// But wait, our parser looks for the *prefix* specifically.
	// If the ID contains the prefix text, it's just text until the terminator.
	// Let's purposefully construct an ID that looks like a request start.
	nestedID := "some_text_" + SyncPrefix + "_more_text"
	req := BuildSyncRequest(nestedID)

	rem, ids := s.ProcessInputBytes([]byte(req))
	if len(ids) != 1 || ids[0] != nestedID {
		t.Errorf("Nested prefix failed. Want ID %q, got %v", nestedID, ids)
	}
	if len(rem) != 0 {
		t.Errorf("Unexpected remaining text: %q", rem)
	}

	// Case 2: Double Escape leading into a sync.
	// Input: ESC (raw) + SyncRequest
	// The first ESC should be treated as user input (or buffered pending)
	// The Sync Request should be valid.
	input := []byte{0x1b}
	input = append(input, []byte(BuildSyncRequest("valid"))...)

	rem, ids = s.ProcessInputBytes(input)
	// The first 0x1b is a partial match for the prefix. It will be buffered.
	// Then the parser sees the actual prefix following it.
	// The parser logic `bytes.Index` finds the first *complete* prefix.
	// The 0x1b at the start is NOT part of that complete prefix.
	// So `before` should contain the 0x1b.
	if len(ids) != 1 || ids[0] != "valid" {
		t.Errorf("Double ESC handling failed. IDs: %v", ids)
	}
	// Check that the first ESC came out as text
	if !bytes.Equal(rem, []byte{0x1b}) {
		t.Errorf("First ESC was lost or mishandled. Rem: %q", rem)
	}
}

// TestSyncProtocol_SplitTerminator verifies splitting the specific 2-byte terminator sequence.
func TestSyncProtocol_SplitTerminator(t *testing.T) {
	s := newSyncState(nil)
	s.SetEnabled(true)

	// "ESC _ go-prompt:sync:id ESC" ... (pause) ... "\"
	part1 := []byte(SyncPrefix + "split-id" + "\x1b")
	part2 := []byte("\\")

	rem1, ids1 := s.ProcessInputBytes(part1)
	if len(ids1) != 0 {
		t.Error("Prematurely detected ID in part 1")
	}
	if len(rem1) != 0 {
		t.Error("Prematurely leaked input in part 1")
	}

	rem2, ids2 := s.ProcessInputBytes(part2)
	if len(ids2) != 1 || ids2[0] != "split-id" {
		t.Errorf("Failed to reassemble split terminator. IDs: %v", ids2)
	}
	if len(rem2) != 0 {
		t.Error("Unexpected remainder after part 2")
	}
}

// TestSyncProtocol_RapidFire verifies the buffer doesn't deadlock with
// many small, complete requests sent in a single slice.
func TestSyncProtocol_RapidFire(t *testing.T) {
	s := newSyncState(nil)
	s.SetEnabled(true)

	var payload bytes.Buffer
	count := 1000
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("%d", i)
		payload.WriteString(BuildSyncRequest(id))
	}

	// Process 1000 requests in a single massive block
	rem, ids := s.ProcessInputBytes(payload.Bytes())

	if len(rem) != 0 {
		t.Errorf("Expected no remainder, got len %d", len(rem))
	}
	if len(ids) != count {
		t.Fatalf("Count mismatch. Want %d, got %d", count, len(ids))
	}
	if ids[count-1] != "999" {
		t.Errorf("Last ID mismatch. Want '999', got %q", ids[count-1])
	}
}
