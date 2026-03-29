package prompt

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	istrings "github.com/joeycumines/go-prompt/strings"
)

func TestPositionAtEndOfString(t *testing.T) {
	tests := map[string]struct {
		input         string
		columns       istrings.Width
		want          Position
		wantFullWidth bool
	}{
		"empty": {
			input:   "",
			columns: 20,
			want: Position{
				X: 0,
				Y: 0,
			},
		},
		"one letter": {
			input:   "f",
			columns: 20,
			want: Position{
				X: 1,
				Y: 0,
			},
		},
		"one word": {
			input:   "foo",
			columns: 20,
			want: Position{
				X: 3,
				Y: 0,
			},
		},
		"complex emoji": {
			input:   "🙆🏿‍♂️",
			columns: 20,
			want: Position{
				X: 2,
				Y: 0,
			},
		},
		"wide overflow": {
			input:   "🙆🏿‍♂️",
			columns: 1,
			want: Position{
				X: 2,
				Y: 0,
			},
		},
		"one-line fits in columns": {
			input:   "foo bar",
			columns: 20,
			want: Position{
				X: 7,
				Y: 0,
			},
		},
		"multiline": {
			input:   "foo\nbar\n",
			columns: 20,
			want: Position{
				X: 0,
				Y: 2,
			},
		},
		"one-line wrapping": {
			input:   "abcd",
			columns: 3,
			want: Position{
				X: 1,
				Y: 1,
			},
		},
		"exact fill single line": {
			input:   "abc",
			columns: 3,
			want: Position{
				X: 3,
				Y: 0,
			},
			wantFullWidth: true,
		},
		"one char short of fill": {
			input:   "ab",
			columns: 3,
			want: Position{
				X: 2,
				Y: 0,
			},
		},
		"one char over fill": {
			input:   "abcd",
			columns: 3,
			want: Position{
				X: 1,
				Y: 1,
			},
		},
		"exact fill multiple lines": {
			input:   "abcdef",
			columns: 3,
			want: Position{
				X: 3,
				Y: 1,
			},
			wantFullWidth: true,
		},
		"exact fill with newline after": {
			input:   "abc\nd",
			columns: 3,
			want: Position{
				// "abc" fills, '\n' → line 1, 'd' at (1, 1)
				X: 1,
				Y: 1,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotFullWidth := positionAtEndOfString(tc.input, tc.columns)
			if gotFullWidth != tc.wantFullWidth {
				t.Errorf("wantFullWidth %v, got %v", tc.wantFullWidth, gotFullWidth)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestPositionAtEndOfStringLine(t *testing.T) {
	tests := map[string]struct {
		input         string
		cols          istrings.Width
		line          int
		want          Position
		wantFullWidth bool
	}{
		"exact fill target line": {
			input: "foo\nfoobar",
			cols:  3,
			line:  1,
			want: Position{
				X: 3,
				Y: 1,
			},
			wantFullWidth: true,
		},
		"Clipped Soft Wrap Position": {
			input: "abcd",
			cols:  3,
			line:  0,
			want: Position{
				X: 3,
				Y: 0,
			},
			wantFullWidth: true,
		},
		"Clipped Explicit Newline Position": {
			input: "abc\ndef",
			cols:  10,
			line:  0,
			want: Position{
				X: 3,
				Y: 0,
			},
		},
		"exact fill on target line": {
			input: "abc",
			cols:  3,
			line:  0,
			want: Position{
				// "abc" exactly fills 3 cols → stays on line 0 at X=3
				X: 3,
				Y: 0,
			},
			wantFullWidth: true,
		},
		"exact fill not on target line": {
			input: "abcdef",
			cols:  3,
			line:  5,
			want: Position{
				// "abc" fills line 0 (wraps), "def" fills line 1 (no wrap, stays at 3,1)
				X: 3,
				Y: 1,
			},
			wantFullWidth: true,
		},
		"last line is in the middle": {
			input: `hi
foo hey
bar boo ba
baz`,
			cols: 20,
			line: 2,
			want: Position{
				X: 10,
				Y: 2,
			},
		},
		"last line is out of bounds": {
			input: `hi
foo hey
bar boo ba
baz`,
			cols: 20,
			line: 20,
			want: Position{
				X: 3,
				Y: 3,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotFullWidth := positionAtEndOfStringLine(tc.input, tc.cols, tc.line)
			if gotFullWidth != tc.wantFullWidth {
				t.Errorf("wantFullWidth %v, got %v", tc.wantFullWidth, gotFullWidth)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestPositionAdd(t *testing.T) {
	tests := map[string]struct {
		left          Position
		right         Position
		want          Position
		wantFullWidth bool
	}{
		"empty": {
			left:  Position{},
			right: Position{},
			want:  Position{},
		},
		"only X": {
			left:  Position{X: 1},
			right: Position{X: 2},
			want:  Position{X: 3},
		},
		"only Y": {
			left:  Position{Y: 1},
			right: Position{Y: 2},
			want:  Position{Y: 3},
		},
		"different coordinates": {
			left:  Position{X: 1},
			right: Position{Y: 2},
			want:  Position{X: 1, Y: 2},
		},
		"both X and Y": {
			left:  Position{X: 1, Y: 5},
			right: Position{X: 10, Y: 2},
			want:  Position{X: 11, Y: 7},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.left.Add(tc.right)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestPositionSubtract(t *testing.T) {
	tests := map[string]struct {
		left          Position
		right         Position
		want          Position
		wantFullWidth bool
	}{
		"empty": {
			left:  Position{},
			right: Position{},
			want:  Position{},
		},
		"only X": {
			left:  Position{X: 1},
			right: Position{X: 2},
			want:  Position{X: -1},
		},
		"only Y": {
			left:  Position{Y: 5},
			right: Position{Y: 2},
			want:  Position{Y: 3},
		},
		"different coordinates": {
			left:  Position{X: 1},
			right: Position{Y: 2},
			want:  Position{X: 1, Y: -2},
		},
		"both X and Y": {
			left:  Position{X: 1, Y: 5},
			right: Position{X: 10, Y: 2},
			want:  Position{X: -9, Y: 3},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.left.Subtract(tc.right)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestPositionJoin(t *testing.T) {
	tests := map[string]struct {
		left          Position
		right         Position
		want          Position
		wantFullWidth bool
	}{
		"empty": {
			left:  Position{},
			right: Position{},
			want:  Position{},
		},
		"only X": {
			left:  Position{X: 1},
			right: Position{X: 2},
			want:  Position{X: 3},
		},
		"only Y": {
			left:  Position{Y: 1},
			right: Position{Y: 2},
			want:  Position{Y: 3},
		},
		"different coordinates": {
			left:  Position{X: 5},
			right: Position{Y: 2},
			want:  Position{X: 0, Y: 2},
		},
		"both X and Y": {
			left:  Position{X: 1, Y: 5},
			right: Position{X: 10, Y: 2},
			want:  Position{X: 10, Y: 7},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.left.Join(tc.right)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
