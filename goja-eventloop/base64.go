package gojaeventloop

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/joeycumines/goja"
)

// atob/btoa Base64 Functions

// btoa encodes a string to base64.
// This follows the browser's btoa() semantics.
// Each character's code point (0-255) becomes a single byte.
func (a *Adapter) btoa(call goja.FunctionCall) goja.Value {
	input := a.base64Input(call)

	// btoa in browsers only accepts Latin-1 characters (0x00-0xFF)
	// Each character's code point becomes a single byte
	runes := []rune(input)
	bytes := make([]byte, len(runes))
	for i, r := range runes {
		if r > 0xFF {
			panic(a.throwDOMException("InvalidCharacterError", "Invalid character"))
		}
		bytes[i] = byte(r)
	}

	encoded := base64.StdEncoding.EncodeToString(bytes)
	return a.runtime.ToValue(encoded)
}

// atob decodes a base64 string.
// This follows the browser's atob() semantics.
// Each byte in the decoded data becomes a character with that code point.
func (a *Adapter) atob(call goja.FunctionCall) goja.Value {
	input := a.base64Input(call)

	decoded, err := decodeForgivingBase64(input)
	if err != nil {
		panic(a.throwDOMException("InvalidCharacterError", "Invalid character"))
	}

	// Each byte becomes a character with that code point (Latin-1 semantics)
	runes := make([]rune, len(decoded))
	for i, b := range decoded {
		runes[i] = rune(b)
	}

	return a.runtime.ToValue(string(runes))
}

func (a *Adapter) base64Input(call goja.FunctionCall) string {
	if len(call.Arguments) == 0 {
		panic(a.runtime.NewTypeError("The \"input\" argument must be specified"))
	}
	return a.webIDLString(call.Argument(0))
}

func decodeForgivingBase64(input string) ([]byte, error) {
	var cleaned strings.Builder
	cleaned.Grow(len(input))
	for _, r := range input {
		switch r {
		case '\t', '\n', '\f', '\r', ' ':
			continue
		default:
			cleaned.WriteRune(r)
		}
	}

	value := cleaned.String()
	if len(value)%4 == 0 {
		if strings.HasSuffix(value, "==") {
			value = value[:len(value)-2]
		} else if strings.HasSuffix(value, "=") {
			value = value[:len(value)-1]
		}
	}
	if len(value)%4 == 1 {
		return nil, fmt.Errorf("invalid base64 length")
	}
	for i := range len(value) {
		c := value[i]
		if !((c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '+' || c == '/') {
			return nil, fmt.Errorf("invalid base64 character")
		}
	}
	return base64.RawStdEncoding.DecodeString(value)
}
