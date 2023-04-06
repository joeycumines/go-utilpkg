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
	"unicode/utf8"
)

const hex = "0123456789abcdef"

var noEscapeTable = func() (noEscapeTable [256]bool) {
	for i := 0; i <= 0x7e; i++ {
		noEscapeTable[i] = i >= 0x20 && i != '\\' && i != '"'
	}
	return
}()

// AppendString appends a string to a byte slice, encoding it as a JSON string.
// It is optimised to avoid allocations.
func AppendString(dst []byte, s string) []byte {
	// Start with a double quote.
	dst = append(dst, '"')
	// Loop through each character in the string.
	for i := 0; i < len(s); i++ {
		// Check if the character needs encoding. Control characters, slashes,
		// and the double quote need json encoding. Bytes above the ascii
		// boundary needs utf8 encoding.
		if !noEscapeTable[s[i]] {
			// We encountered a character that needs to be encoded. Switch
			// to complex version of the algorithm.
			dst = appendStringComplex(dst, s, i)
			return append(dst, '"')
		}
	}
	// The string has no need for encoding and therefore is directly
	// appended to the byte slice.
	dst = append(dst, s...)
	// End with a double quote
	return append(dst, '"')
}

// InsertString inserts a string into a byte slice, encoding it as a JSON string.
// It is optimised to avoid allocations.
func InsertString(dst []byte, index int, s string) []byte {
	return insertString(dst, index, s, true)
}

// InsertStringContent behaves per [InsertString], but without inserting the
// surrounding double quotes.
func InsertStringContent(dst []byte, index int, s string) []byte {
	return insertString(dst, index, s, false)
}

func insertString(dst []byte, index int, s string, quotes bool) []byte {
	if !quotes && s == `` {
		return dst
	}

	var encodedLen int
	if quotes {
		encodedLen = 2
	}
	for i := 0; i < len(s); i++ {
		if !noEscapeTable[s[i]] {
			encodedLen += 6
		} else {
			encodedLen++
		}
	}

	dst = append(dst, make([]byte, encodedLen)...)

	copy(dst[index+encodedLen:], dst[index:])

	if quotes {
		dst[index] = '"'
		index++
	}

	if (quotes && encodedLen == len(s)+2) || (!quotes && encodedLen == len(s)) {
		index += copy(dst[index:], s)
	} else {
		// the offset of any suffix data we copied
		encodedLen += index
		if quotes {
			encodedLen--
		}

		dst, index = appendStringComplex(dst[:index], s, 0), len(dst)
		dst, index = dst[:index], len(dst)

		// the number of empty bytes that we didn't need
		encodedLen -= index
		if quotes {
			encodedLen--
		}

		if encodedLen > 0 {
			if quotes {
				copy(dst[index+1:], dst[index+1+encodedLen:])
			} else {
				copy(dst[index:], dst[index+encodedLen:])
			}
			dst = dst[:len(dst)-encodedLen]
		}
	}

	if quotes {
		dst[index] = '"'
	}

	return dst
}

// appendStringComplex is used by AppendString to take over an in
// progress JSON string encoding that encountered a character that needs
// to be encoded.
func appendStringComplex(dst []byte, s string, i int) []byte {
	start := 0
	for i < len(s) {
		b := s[i]
		if b >= utf8.RuneSelf {
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				// In case of error, first append previous simple characters to
				// the byte slice if any and append a replacement character code
				// in place of the invalid sequence.
				if start < i {
					dst = append(dst, s[start:i]...)
				}
				dst = append(dst, `\ufffd`...)
				i += size
				start = i
				continue
			}
			i += size
			continue
		}
		if noEscapeTable[b] {
			i++
			continue
		}
		// We encountered a character that needs to be encoded.
		// Let's append the previous simple characters to the byte slice
		// and switch our operation to read and encode the remainder
		// characters byte-by-byte.
		if start < i {
			dst = append(dst, s[start:i]...)
		}
		switch b {
		case '"', '\\':
			dst = append(dst, '\\', b)
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			dst = append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0xF])
		}
		i++
		start = i
	}
	if start < len(s) {
		dst = append(dst, s[start:]...)
	}
	return dst
}
