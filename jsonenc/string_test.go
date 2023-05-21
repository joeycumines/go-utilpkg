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
	"bytes"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"testing"
	"unicode/utf8"
)

var encodeStringTests = []struct {
	in  string
	out string
}{
	{"", `""`},           // empty string
	{"\\", `"\\"`},       // backslash
	{"\x00", `"\u0000"`}, // null character
	{"\x01", `"\u0001"`}, // start of heading
	{"\x02", `"\u0002"`}, // start of text
	{"\x03", `"\u0003"`}, // end of text
	{"\x04", `"\u0004"`}, // end of transmission
	{"\x05", `"\u0005"`}, // enquiry
	{"\x06", `"\u0006"`}, // acknowledge
	{"\x07", `"\u0007"`}, // bell
	{"\x08", `"\u0008"`}, // backspace
	{"\x09", `"\t"`},     // horizontal tab
	{"\x0a", `"\n"`},     // line feed
	{"\x0b", `"\u000b"`}, // vertical tab
	{"\x0c", `"\u000c"`}, // form feed
	{"\x0d", `"\r"`},     // carriage return
	{"\x0e", `"\u000e"`}, // shift out
	{"\x0f", `"\u000f"`}, // shift in
	{"\x10", `"\u0010"`}, // data link escape
	{"\x11", `"\u0011"`}, // device control 1
	{"\x12", `"\u0012"`}, // device control 2
	{"\x13", `"\u0013"`}, // device control 3
	{"\x14", `"\u0014"`}, // device control 4
	{"\x15", `"\u0015"`}, // negative acknowledge
	{"\x16", `"\u0016"`}, // synchronous idle
	{"\x17", `"\u0017"`}, // end of transmission block
	{"\x18", `"\u0018"`}, // cancel
	{"\x19", `"\u0019"`}, // end of medium
	{"\x1a", `"\u001a"`}, // substitute
	{"\x1b", `"\u001b"`}, // escape
	{"\x1c", `"\u001c"`}, // file separator
	{"\x1d", `"\u001d"`}, // group separator
	{"\x1e", `"\u001e"`}, // record separator
	{"\x1f", `"\u001f"`}, // unit separator
	{"✭", `"✭"`},         // star symbol
	{"foo\xc2\x7fbar", `"foo\ufffd` + "\x7f" + `bar"`},         // replaces invalid byte sequence
	{"ascii", `"ascii"`},                                       // ASCII characters
	{"\"a", `"\"a"`},                                           // double quote with letter
	{"\x1fa", `"\u001fa"`},                                     // letter with escape character
	{"foo\"bar\"baz", `"foo\"bar\"baz"`},                       // string with double quotes
	{"\x1ffoo\x1fbar\x1fbaz", `"\u001ffoo\u001fbar\u001fbaz"`}, // string with escape characters
	{"emoji \u2764\ufe0f!", `"emoji ❤️!"`},                     // string with emoji
	{"<", `"\u003c"`},                                          // less than symbol
	{">", `"\u003e"`},                                          // greater than symbol
	{"&", `"\u0026"`},                                          // ampersand symbol
	{"\x7f", "\"\x7f\""},                                       // delete character
	{"\u2028", `"\u2028"`},                                     // line separator
	{"\u2029", `"\u2029"`},                                     // paragraph separator
	{"foo \u2028\u2029 \u2028 \u2029  bar", `"foo \u2028\u2029 \u2028 \u2029  bar"`}, // string with line and paragraph separators
	{"\xc0", `"\ufffd"`},                               // start of a two-byte sequence without a continuation byte
	{"\xed\xa0\x80", `"\ufffd\ufffd\ufffd"`},           // an overlong three-byte sequence
	{"\xf4\x90\x80\x80", `"\ufffd\ufffd\ufffd\ufffd"`}, // a four-byte sequence representing a code point outside the valid range
	{"\u0022", `"\""`},                                 // double quote
	{"\u0027", `"'"`},                                  // single quote
	{"\u005c", `"\\"`},                                 // backslash
	{"\u00a9", `"©"`},                                  // copyright symbol
	{"\u2603", `"☃"`},                                  // snowman
	{"\u20ac", `"€"`},                                  // euro symbol
	{"\u1f600", `"ὠ0"`},                                // grinning face
	{"\u1f9a3", `"ᾚ3"`},                                // zombie
	{"\u1f468\u200d\u2695\ufe0f", `"` + "\u1f468\u200d\u2695\ufe0f" + `"`}, // man health worker
	{"\u1f9d1\u200d\u1f52c", `"` + "\u1f9d1\u200d\u1f52c" + `"`},           // woman mage
	{"\U0001F926\U0001F3FB\u200D\u2642\uFE0F", `"🤦🏻‍♂️"`},                  // man facepalming
	{"\u0654", `"ٔ"`}, // arabic hamza above
	{"\u0301", string([]byte{0x22, 0xcc, 0x81, 0x22})},                                                       // combining acute accent
	{"\u3099", string([]byte{0x22, 0xe3, 0x82, 0x99, 0x22})},                                                 // combining katakana-hiragana voiced sound mark
	{"\u1100\u1161\u11a8", string([]byte{0x22, 0xe1, 0x84, 0x80, 0xe1, 0x85, 0xa1, 0xe1, 0x86, 0xa8, 0x22})}, // hangul syllables
	{"\u3042", `"あ"`}, // Japanese Hiragana character "あ"
	{"\u30a2", `"ア"`}, // Japanese Katakana character "ア"
	{"\u1f00", `"ἀ"`}, // Greek character "ἀ"
	{"\u1f82", `"ᾂ"`}, // Greek character "ᾂ"
	{"\u05d0", `"א"`}, // Hebrew character "א"
	{"\u0627", `"ا"`}, // Arabic character "ا"
	{"\u0628", `"ب"`}, // Arabic character "ب"
	{"\u0e01", `"ก"`}, // Thai character "ก"
	{`ช`, `"ช"`},      // Thai character "ช"
	{"\u0e8a", string([]byte{0x22, 0xe0, 0xba, 0x8a, 0x22})},           // Thai character "จ" encoded as bytes
	{"\U00010c01", string([]byte{0x22, 0xf0, 0x90, 0xb0, 0x81, 0x22})}, // Old Hungarian character "𐰁" encoded as bytes
	{"\U00013000", string([]byte{0x22, 0xf0, 0x93, 0x80, 0x80, 0x22})}, // Egyptian hieroglyph "𓀀" encoded as bytes
	{"\U0001f9db\U0001f3fd", `"🧛🏽"`},                                   // Emoji sequence: "🧛🏽" (vampire woman with skin tone modifier) encoded as bytes
}

func TestAppendString(t *testing.T) {
	for _, tt := range encodeStringTests {
		b := AppendString([]byte{}, tt.in)
		if got, want := string(b), tt.out; got != want {
			t.Errorf("AppendString(%q) = %#q/string(%#v), want %#q", tt.in, got, []byte(got), want)
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

var validatedReplacementRune = func() rune {
	const replacementRune = '�'
	const replacementRuneEncoding = `"\ufffd"`

	// sanity check
	b, err := json.Marshal(string(replacementRune))
	if err != nil {
		panic(err)
	}
	if string(b) != `"`+string(replacementRune)+`"` {
		panic("unexpected replacementRune encoding: " + string(b))
	}

	b, err = json.Marshal(string([]byte{'\xc0'}))
	if err != nil {
		panic(err)
	}
	if string(b) != replacementRuneEncoding {
		panic("unexpected replacementRune encoding: " + string(b))
	}

	var s string
	if err := json.Unmarshal([]byte(replacementRuneEncoding), &s); err != nil {
		panic(err)
	}
	if s != string(replacementRune) {
		panic("unexpected replacementRune decoding: " + s)
	}

	return replacementRune
}()

func normalizeToUTF8(input string) string {
	if utf8.ValidString(input) {
		return input
	}
	var result []rune
	for len(input) > 0 {
		r, size := utf8.DecodeRuneInString(input)
		if r != utf8.RuneError {
			result = append(result, r)
		} else {
			result = append(result, validatedReplacementRune)
		}
		input = input[size:]
	}
	return string(result)
}

func FuzzAppendString(f *testing.F) {
	for _, tc := range encodeStringTests {
		f.Add(``, tc.in)

		f.Add(`{`+tc.out+`:`, tc.in)

		{
			length, err := rand.Int(rand.Reader, big.NewInt(1<<10))
			if err != nil {
				f.Fatal(err)
			}
			randBytes := make([]byte, length.Int64()+1)
			if _, err := rand.Read(randBytes); err != nil {
				f.Fatal(err)
			}

			f.Add(``, string(randBytes))
			f.Add(string(randBytes), tc.in)
		}
	}

	f.Fuzz(func(t *testing.T, dstInput string, val string) {
		var dstOriginal []byte
		if dstInput != `` {
			dstOriginal = make([]byte, len(dstInput))
			copy(dstOriginal, dstInput)
		}

		dst := AppendString(dstOriginal, val)
		if dstInput != string(dstOriginal) {
			t.Errorf("%q: unexpected original: %q", val, dst)
		}
		if dstInput != string(dst[:len(dstInput)]) {
			t.Fatalf("%q: unexpected prefix: %q", val, dst)
		}

		dst = dst[len(dstInput):]

		if len(dst) < 2 || (len(dst) == 2 && (string(dst) != `""` || val != "")) {
			t.Errorf("%q: unexpected output: %q", val, dst)
		}

		// ensure dst is a valid JSON string encoded per normalizeToUTF8
		{
			norm := normalizeToUTF8(val)
			var decoded string
			if err := json.Unmarshal(dst, &decoded); err != nil {
				t.Errorf("%q: error decoding %q: %v", val, dst, err)
			} else if decoded != norm {
				t.Errorf("%q: got %q, want %q", val, decoded, norm)
			}
		}

		// ensure dst is encoded per the behavior of json.Marshal, after going through normalizeToUTF8
		if want, err := json.Marshal(val); err != nil {
			t.Errorf("%q: encoding error: %v", val, err)
		} else if !bytes.Equal(dst, want) {
			t.Errorf("%q: got %s, want %s", val, dst, want)
		}
	})
}

func FuzzInsertString(f *testing.F) {
	for _, tc := range encodeStringTests {
		// case: append
		f.Add(``, 0, tc.in, true)
		f.Add(``, 0, tc.in, false)
		// case: insert at a random index of each of the test cases
		for _, tc2 := range encodeStringTests {
			indexBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(tc2.in))+1))
			if err != nil {
				f.Fatal(err)
			}
			index := int(indexBig.Int64())
			f.Add(tc2.in, index, tc.in, true)
			f.Add(tc2.in, index, tc.in, false)
		}
		// case: insert at a random index of a random string
		for i := 0; i < 3; i++ {
			lRand, err := rand.Int(rand.Reader, big.NewInt(1<<10))
			if err != nil {
				f.Fatal(err)
			}
			randBytes := make([]byte, lRand.Int64()+1)
			if _, err := rand.Read(randBytes); err != nil {
				f.Fatal(err)
			}

			indexBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(randBytes))+1))
			if err != nil {
				f.Fatal(err)
			}
			index := int(indexBig.Int64())

			f.Add(string(randBytes), index, tc.in, true)
			f.Add(string(randBytes), index, tc.in, false)
		}
	}

	f.Fuzz(func(t *testing.T, dstInput string, index int, val string, quotes bool) {
		if index < 0 || index > len(dstInput) {
			t.SkipNow()
		}

		var dstOriginal []byte
		if dstInput != `` {
			dstOriginal = make([]byte, len(dstInput))
			copy(dstOriginal, dstInput)
		}

		dst := insertString(dstOriginal, index, val, quotes)

		if dstInput[:index] != string(dstOriginal[:index]) {
			t.Errorf("%q: unexpected original prefix: %q", val, dstOriginal[:index])
		}

		if string(dst[:index]) != dstInput[:index] {
			t.Errorf("%q: unexpected prefix: %q", val, dst[:index])
		}

		indexCut := len(dst) - (len(dstInput) - index)

		if string(dst[indexCut:]) != dstInput[index:] {
			t.Errorf("%q: unexpected suffix: %q", val, dst[indexCut:])
		}

		dst = dst[index:indexCut]

		if quotes {
			if len(dst) < 2 || (len(dst) == 2 && (string(dst) != `""` || val != "")) {
				t.Errorf("%q: unexpected output: %q", val, dst)
			}
		} else {
			if len(dst) == 0 && val != "" {
				t.Errorf("%q: unexpected output: %q", val, dst)
			}
			dst = append(append([]byte(`"`), dst...), '"')
		}

		// ensure dst is a valid JSON string encoded per normalizeToUTF8
		{
			norm := normalizeToUTF8(val)
			var decoded string
			if err := json.Unmarshal(dst, &decoded); err != nil {
				t.Errorf("%q: error decoding %q: %v", val, dst, err)
			} else if decoded != norm {
				t.Errorf("%q: got %q, want %q", val, decoded, norm)
			}
		}

		// ensure dst is encoded per the behavior of json.Marshal, after going through normalizeToUTF8
		if want, err := json.Marshal(val); err != nil {
			t.Errorf("%q: encoding error: %v", val, err)
		} else if !bytes.Equal(dst, want) {
			t.Errorf("%q: got %s, want %s", val, dst, want)
		}
	})
}

type insertStringTest struct {
	original string
	index    int
	value    string
	quotes   bool
}

func generateInsertStringBenchmarks() (testCases []*insertStringTest) {
	const original = `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`
	const index = len(original) / 2
	for _, tc1 := range encodeStringTests {
		testCases = append(testCases, &insertStringTest{original, index, tc1.in, true})
		testCases = append(testCases, &insertStringTest{original, index, tc1.in, false})
	}
	return
}

func BenchmarkInsertString(b *testing.B) {
	testCases := generateInsertStringBenchmarks()
	buffers := make([][]byte, len(testCases))
	for i, tc := range testCases {
		buffers[i] = append([]byte(nil), tc.original...)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for j, tc := range testCases {
			buffers[j] = insertString(buffers[j], tc.index, tc.value, tc.quotes)
		}
	}
}

func TestInsertString_panic(t *testing.T) {
	const original = `aaaaaaaaaaaa`
	for _, index := range []int{-1, len(original) + 1} {
		for _, quotes := range []bool{true, false} {
			func() {
				defer func() {
					if r := recover(); r != `jsonenc: index out of range` {
						t.Errorf("unexpected recover for index %d / quotes %v: %v", index, quotes, r)
					}
				}()
				insertString([]byte(original), index, ``, false)
			}()
		}
	}
}
