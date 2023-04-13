package stumpy

import (
	"strings"
	"time"
)

// formatTime uses the same behavior as protobuf's timestamp
func formatTime(t time.Time) string {
	t = t.UTC()
	x := t.Format("2006-01-02T15:04:05.000000000") // RFC 3339
	x = strings.TrimSuffix(x, "000")
	x = strings.TrimSuffix(x, "000")
	x = strings.TrimSuffix(x, ".000")
	return x + "Z"
}
