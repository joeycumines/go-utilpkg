package eventloop

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	fuzzMaxDataLen     = 4096
	fuzzLoopRunTimeout = 2 * time.Second
)

type fuzzReader struct {
	data []byte
	pos  int
}

func newFuzzReader(data []byte) *fuzzReader {
	if len(data) > fuzzMaxDataLen {
		data = data[:fuzzMaxDataLen]
	}
	return &fuzzReader{data: data}
}

func (r *fuzzReader) byte() byte {
	if len(r.data) == 0 {
		return 0
	}
	b := r.data[r.pos%len(r.data)]
	r.pos++
	return b
}

func (r *fuzzReader) bool() bool { return r.byte()&1 == 1 }

func (r *fuzzReader) uint64() uint64 {
	var v uint64
	for i := range 8 {
		v |= uint64(r.byte()) << (8 * i)
	}
	return v
}

func (r *fuzzReader) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.uint64() % uint64(n))
}

func (r *fuzzReader) smallString(maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	n := r.intn(maxLen + 1)
	buf := make([]byte, n)
	for i := range buf {
		b := r.byte()
		// Keep generated names printable enough for useful failure messages while
		// still exercising punctuation, empty strings, and repeated keys.
		buf[i] = byte(32 + int(b)%95)
	}
	return string(buf)
}

type fuzzErrs struct {
	mu   sync.Mutex
	errs []string
}

func (e *fuzzErrs) add(format string, args ...any) {
	e.mu.Lock()
	e.errs = append(e.errs, fmt.Sprintf(format, args...))
	e.mu.Unlock()
}

func (e *fuzzErrs) failNow(t *testing.T) {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.errs) != 0 {
		t.Fatalf("callback errors:\n%s", strings.Join(e.errs, "\n"))
	}
}
