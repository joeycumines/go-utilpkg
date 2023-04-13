// Go MySQL Driver - A MySQL-Driver for Go's database/sql package
//
// Copyright 2012 The Go-MySQL-Driver Authors. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package mysql

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	digits10 = "0000000000111111111122222222223333333333444444444455555555556666666666777777777788888888889999999999"
	digits01 = "0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789"
)

type (
	// InterpolateConfig provides configuration for it's Interpolate method.
	InterpolateConfig struct {
		Location *time.Location
		// http://dev.mysql.com/doc/internals/en/status-flags.html
		NoBackslashEscapes bool
	}
)

// Interpolate is presumably like mysql_real_escape_string, and has been copied from go-sql-driver/mysql.
// WARNING see https://stackoverflow.com/a/12118602 for potential vul.
// https://github.com/go-sql-driver/mysql/blob/ad9fa14acdcf7d0533e7fbe58728f3d216213ade/connection.go#L198
func (x *InterpolateConfig) Interpolate(query string, args ...driver.Value) (string, error) {
	// Number of ? should be same to len(args)
	if strings.Count(query, "?") != len(args) {
		return "", driver.ErrSkip
	}

	var (
		buf    []byte
		err    error
		argPos int
	)

	for i := 0; i < len(query); i++ {
		q := strings.IndexByte(query[i:], '?')
		if q == -1 {
			buf = append(buf, query[i:]...)
			break
		}
		buf = append(buf, query[i:i+q]...)
		i += q

		arg := args[argPos]
		argPos++

		if arg == nil {
			buf = append(buf, "NULL"...)
			continue
		}

		switch v := arg.(type) {
		case int64:
			buf = strconv.AppendInt(buf, v, 10)
		case uint64:
			buf = strconv.AppendUint(buf, v, 10)
		case float64:
			buf = strconv.AppendFloat(buf, v, 'g', -1, 64)
		case bool:
			if v {
				buf = append(buf, '1')
			} else {
				buf = append(buf, '0')
			}
		case time.Time:
			if v.IsZero() {
				buf = append(buf, "'0000-00-00'"...)
			} else {
				buf = append(buf, '\'')
				buf, err = appendDateTime(buf, v.In(x.location()))
				if err != nil {
					return "", err
				}
				buf = append(buf, '\'')
			}
		case json.RawMessage:
			buf = append(buf, '\'')
			if !x.noBackslashEscapes() {
				buf = escapeBytesBackslash(buf, v)
			} else {
				buf = escapeBytesQuotes(buf, v)
			}
			buf = append(buf, '\'')
		case []byte:
			if v == nil {
				buf = append(buf, "NULL"...)
			} else {
				buf = append(buf, "_binary'"...)
				if !x.noBackslashEscapes() {
					buf = escapeBytesBackslash(buf, v)
				} else {
					buf = escapeBytesQuotes(buf, v)
				}
				buf = append(buf, '\'')
			}
		case string:
			buf = append(buf, '\'')
			if !x.noBackslashEscapes() {
				buf = escapeStringBackslash(buf, v)
			} else {
				buf = escapeStringQuotes(buf, v)
			}
			buf = append(buf, '\'')
		default:
			return "", driver.ErrSkip
		}
	}

	if argPos != len(args) {
		return "", driver.ErrSkip
	}

	return string(buf), nil
}

func (x *InterpolateConfig) location() (loc *time.Location) {
	if x != nil {
		loc = x.Location
	}
	if loc == nil {
		loc = time.UTC
	}
	return
}

func (x *InterpolateConfig) noBackslashEscapes() bool {
	if x == nil {
		return false
	}
	return x.NoBackslashEscapes
}

func appendDateTime(buf []byte, t time.Time) ([]byte, error) {
	year, month, day := t.Date()
	hour, min, sec := t.Clock()
	nsec := t.Nanosecond()

	if year < 1 || year > 9999 {
		return buf, errors.New("year is not in the range [1, 9999]: " + strconv.Itoa(year)) // use errors.New instead of fmt.Errorf to avoid year escape to heap
	}
	year100 := year / 100
	year1 := year % 100

	var localBuf [len("2006-01-02T15:04:05.999999999")]byte // does not escape
	localBuf[0], localBuf[1], localBuf[2], localBuf[3] = digits10[year100], digits01[year100], digits10[year1], digits01[year1]
	localBuf[4] = '-'
	localBuf[5], localBuf[6] = digits10[month], digits01[month]
	localBuf[7] = '-'
	localBuf[8], localBuf[9] = digits10[day], digits01[day]

	if hour == 0 && min == 0 && sec == 0 && nsec == 0 {
		return append(buf, localBuf[:10]...), nil
	}

	localBuf[10] = ' '
	localBuf[11], localBuf[12] = digits10[hour], digits01[hour]
	localBuf[13] = ':'
	localBuf[14], localBuf[15] = digits10[min], digits01[min]
	localBuf[16] = ':'
	localBuf[17], localBuf[18] = digits10[sec], digits01[sec]

	if nsec == 0 {
		return append(buf, localBuf[:19]...), nil
	}
	nsec100000000 := nsec / 100000000
	nsec1000000 := (nsec / 1000000) % 100
	nsec10000 := (nsec / 10000) % 100
	nsec100 := (nsec / 100) % 100
	nsec1 := nsec % 100
	localBuf[19] = '.'

	// milli second
	localBuf[20], localBuf[21], localBuf[22] =
		digits01[nsec100000000], digits10[nsec1000000], digits01[nsec1000000]
	// micro second
	localBuf[23], localBuf[24], localBuf[25] =
		digits10[nsec10000], digits01[nsec10000], digits10[nsec100]
	// nano second
	localBuf[26], localBuf[27], localBuf[28] =
		digits01[nsec100], digits10[nsec1], digits01[nsec1]

	// trim trailing zeros
	n := len(localBuf)
	for n > 0 && localBuf[n-1] == '0' {
		n--
	}

	return append(buf, localBuf[:n]...), nil
}

// escapeBytesBackslash escapes []byte with backslashes (\)
// This escapes the contents of a string (provided as []byte) by adding backslashes before special
// characters, and turning others into specific escape sequences, such as
// turning newlines into \n and null bytes into \0.
// https://github.com/mysql/mysql-server/blob/mysql-5.7.5/mysys/charset.c#L823-L932
func escapeBytesBackslash(buf, v []byte) []byte {
	pos := len(buf)
	buf = reserveBuffer(buf, len(v)*2)

	for _, c := range v {
		switch c {
		case '\x00':
			buf[pos+1] = '0'
			buf[pos] = '\\'
			pos += 2
		case '\n':
			buf[pos+1] = 'n'
			buf[pos] = '\\'
			pos += 2
		case '\r':
			buf[pos+1] = 'r'
			buf[pos] = '\\'
			pos += 2
		case '\x1a':
			buf[pos+1] = 'Z'
			buf[pos] = '\\'
			pos += 2
		case '\'':
			buf[pos+1] = '\''
			buf[pos] = '\\'
			pos += 2
		case '"':
			buf[pos+1] = '"'
			buf[pos] = '\\'
			pos += 2
		case '\\':
			buf[pos+1] = '\\'
			buf[pos] = '\\'
			pos += 2
		default:
			buf[pos] = c
			pos++
		}
	}

	return buf[:pos]
}

// escapeBytesQuotes escapes apostrophes in []byte by doubling them up.
// This escapes the contents of a string by doubling up any apostrophes that
// it contains. This is used when the NO_BACKSLASH_ESCAPES SQL_MODE is in
// effect on the server.
// https://github.com/mysql/mysql-server/blob/mysql-5.7.5/mysys/charset.c#L963-L1038
func escapeBytesQuotes(buf, v []byte) []byte {
	pos := len(buf)
	buf = reserveBuffer(buf, len(v)*2)

	for _, c := range v {
		if c == '\'' {
			buf[pos+1] = '\''
			buf[pos] = '\''
			pos += 2
		} else {
			buf[pos] = c
			pos++
		}
	}

	return buf[:pos]
}

// If cap(buf) is not enough, reallocate new buffer.
func reserveBuffer(buf []byte, appendSize int) []byte {
	newSize := len(buf) + appendSize
	if cap(buf) < newSize {
		// Grow buffer exponentially
		newBuf := make([]byte, len(buf)*2+appendSize)
		copy(newBuf, buf)
		buf = newBuf
	}
	return buf[:newSize]
}

// escapeStringBackslash is similar to escapeBytesBackslash but for string.
func escapeStringBackslash(buf []byte, v string) []byte {
	pos := len(buf)
	buf = reserveBuffer(buf, len(v)*2)

	for i := 0; i < len(v); i++ {
		c := v[i]
		switch c {
		case '\x00':
			buf[pos+1] = '0'
			buf[pos] = '\\'
			pos += 2
		case '\n':
			buf[pos+1] = 'n'
			buf[pos] = '\\'
			pos += 2
		case '\r':
			buf[pos+1] = 'r'
			buf[pos] = '\\'
			pos += 2
		case '\x1a':
			buf[pos+1] = 'Z'
			buf[pos] = '\\'
			pos += 2
		case '\'':
			buf[pos+1] = '\''
			buf[pos] = '\\'
			pos += 2
		case '"':
			buf[pos+1] = '"'
			buf[pos] = '\\'
			pos += 2
		case '\\':
			buf[pos+1] = '\\'
			buf[pos] = '\\'
			pos += 2
		default:
			buf[pos] = c
			pos++
		}
	}

	return buf[:pos]
}

// escapeStringQuotes is similar to escapeBytesQuotes but for string.
func escapeStringQuotes(buf []byte, v string) []byte {
	pos := len(buf)
	buf = reserveBuffer(buf, len(v)*2)

	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\'' {
			buf[pos+1] = '\''
			buf[pos] = '\''
			pos += 2
		} else {
			buf[pos] = c
			pos++
		}
	}

	return buf[:pos]
}
