/*
MIT License

Copyright (c) 2023 Joseph Cumines
Copyright (c) 2017 Olivier Poitrey

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Originally based on zerolog's AppendString implementation, extended with
// to provide additional functionality.
// https://github.com/rs/zerolog/commit/1f50797d7d24e4cf3a6407203bd694f3d35de724

package jsonenc

import (
	"testing"
)

var encodeStringTests = []struct {
	in  string
	out string
}{
	{"", `""`},
	{"\\", `"\\"`},
	{"\x00", `"\u0000"`},
	{"\x01", `"\u0001"`},
	{"\x02", `"\u0002"`},
	{"\x03", `"\u0003"`},
	{"\x04", `"\u0004"`},
	{"\x05", `"\u0005"`},
	{"\x06", `"\u0006"`},
	{"\x07", `"\u0007"`},
	{"\x08", `"\u0008"`},
	{"\x09", `"\t"`},
	{"\x0a", `"\n"`},
	{"\x0b", `"\u000b"`},
	{"\x0c", `"\u000c"`},
	{"\x0d", `"\r"`},
	{"\x0e", `"\u000e"`},
	{"\x0f", `"\u000f"`},
	{"\x10", `"\u0010"`},
	{"\x11", `"\u0011"`},
	{"\x12", `"\u0012"`},
	{"\x13", `"\u0013"`},
	{"\x14", `"\u0014"`},
	{"\x15", `"\u0015"`},
	{"\x16", `"\u0016"`},
	{"\x17", `"\u0017"`},
	{"\x18", `"\u0018"`},
	{"\x19", `"\u0019"`},
	{"\x1a", `"\u001a"`},
	{"\x1b", `"\u001b"`},
	{"\x1c", `"\u001c"`},
	{"\x1d", `"\u001d"`},
	{"\x1e", `"\u001e"`},
	{"\x1f", `"\u001f"`},
	{"✭", `"✭"`},
	{"foo\xc2\x7fbar", `"foo\ufffd` + "\x7f" + `bar"`},
	{"ascii", `"ascii"`},
	{"\"a", `"\"a"`},
	{"\x1fa", `"\u001fa"`},
	{"foo\"bar\"baz", `"foo\"bar\"baz"`},
	{"\x1ffoo\x1fbar\x1fbaz", `"\u001ffoo\u001fbar\u001fbaz"`},
	{"emoji \u2764\ufe0f!", `"emoji ❤️!"`},
	{"<", `"\u003c"`},
	{">", `"\u003e"`},
	{"&", `"\u0026"`},
	{"\x7f", "\"\x7f\""},
	{"\u2028", `"\u2028"`},
	{"\u2029", `"\u2029"`},
	{"foo \u2028\u2029 \u2028 \u2029  bar", `"foo \u2028\u2029 \u2028 \u2029  bar"`},
}

func TestAppendString(t *testing.T) {
	for _, tt := range encodeStringTests {
		b := AppendString([]byte{}, tt.in)
		if got, want := string(b), tt.out; got != want {
			t.Errorf("AppendString(%q) = %#q, want %#q", tt.in, got, want)
		}
	}
}

func BenchmarkAppendString(b *testing.B) {
	tests := map[string]string{
		"NoEncoding":       `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`,
		"EncodingFirst":    `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`,
		"EncodingMiddle":   `aaaaaaaaaaaaaaaaaaaaaaaaa"aaaaaaaaaaaaaaaaaaaaaaaa`,
		"EncodingLast":     `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		"MultiBytesFirst":  `❤️aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`,
		"MultiBytesMiddle": `aaaaaaaaaaaaaaaaaaaaaaaaa❤️aaaaaaaaaaaaaaaaaaaaaaaa`,
		"MultiBytesLast":   `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa❤️`,
	}
	for name, str := range tests {
		b.Run(name, func(b *testing.B) {
			buf := make([]byte, 0, 100)
			for i := 0; i < b.N; i++ {
				_ = AppendString(buf, str)
			}
		})
	}
}

func TestInsertString(t *testing.T) {
	for _, tc := range [...]struct {
		Name    string
		Factory func() (b []byte, offset int)
	}{
		{
			Name: "empty",
			Factory: func() (b []byte, offset int) {
				return nil, 0
			},
		},
		{
			Name: "single character prefix",
			Factory: func() (b []byte, offset int) {
				return []byte(`a`), 1
			},
		},
		{
			Name: "single character suffix",
			Factory: func() (b []byte, offset int) {
				return []byte(`a`), 0
			},
		},
		{
			Name: "prefix and suffix",
			Factory: func() (b []byte, offset int) {
				return []byte(`0123456789`), 4
			},
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			for _, tt := range encodeStringTests {
				b, offset := tc.Factory()
				before := string(b[:offset])
				after := string(b[offset:])
				b = InsertString(b, offset, tt.in)
				if got, want := string(b), before+tt.out+after; got != want {
					t.Errorf("InsertString(%q) = %#q, want %#q", tt.in, got, want)
				} else {
					t.Logf("InsertString(%q) = %#q", tt.in, b)
				}
			}
		})
	}
}

func TestInsertStringContent(t *testing.T) {
	for _, tc := range [...]struct {
		Name    string
		Factory func() (b []byte, offset int)
	}{
		{
			Name: "empty",
			Factory: func() (b []byte, offset int) {
				return nil, 0
			},
		},
		{
			Name: "single character prefix",
			Factory: func() (b []byte, offset int) {
				return []byte(`a`), 1
			},
		},
		{
			Name: "single character suffix",
			Factory: func() (b []byte, offset int) {
				return []byte(`a`), 0
			},
		},
		{
			Name: "prefix and suffix",
			Factory: func() (b []byte, offset int) {
				return []byte(`0123456789`), 4
			},
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			for _, tt := range encodeStringTests {
				b, offset := tc.Factory()
				before := string(b[:offset])
				after := string(b[offset:])
				b = InsertStringContent(b, offset, tt.in)
				if got, want := string(b), before+tt.out[1:len(tt.out)-1]+after; got != want {
					t.Errorf("InsertString(%q) = %#q, want %#q", tt.in, got, want)
				} else {
					t.Logf("InsertString(%q) = %#q", tt.in, b)
				}
			}
		})
	}
}
