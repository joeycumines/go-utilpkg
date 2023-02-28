package logiface

import (
	"bufio"
	"io"
	"sort"
	"strings"
)

// sortedLineWriterSplitOnSpace scans and sorts each line, where the sort is performed by splitting on space.
func sortedLineWriterSplitOnSpace(writer io.Writer) (io.WriteCloser, <-chan error) {
	r, w := io.Pipe()
	out := make(chan error, 1)
	go func() {
		var err error
		defer func() {
			out <- err
			close(out)
			_ = r.CloseWithError(err)
		}()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			v := strings.Split(scanner.Text(), ` `)
			sort.Strings(v)
			_, err = strings.NewReader(strings.Join(v, ` `) + "\n").WriteTo(writer)
			if err != nil {
				return
			}
		}
		err = scanner.Err()
	}()
	return w, out
}
