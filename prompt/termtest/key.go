package termtest

import (
	"fmt"
	"strings"
)

func lookupKey(k string) (string, error) {
	normalized := strings.ToLower(k)

	// Check strict mappings (including sequences and control keys)
	if seq, ok := keyMap[normalized]; ok {
		return seq, nil
	}

	return "", fmt.Errorf("unknown key: %s", k)
}

// keyMap exhaustively maps friendly key names (and legacy aliases) to their
// corresponding ANSI sequences or control bytes.
//
// N.B. I aligned the friendly names with https://github.com/charmbracelet/bubbletea/blob/f9233d51192293dadda7184a4de347738606c328/key.go
var keyMap = map[string]string{
	// Control keys (C0)
	"ctrl+@":  "\x00",
	"ctrl+a":  "\x01",
	"ctrl+b":  "\x02",
	"ctrl+c":  "\x03",
	"ctrl+d":  "\x04",
	"ctrl+e":  "\x05",
	"ctrl+f":  "\x06",
	"ctrl+g":  "\x07",
	"ctrl+h":  "\x08",
	"ctrl+i":  "\x09",
	"ctrl+j":  "\x0a",
	"ctrl+k":  "\x0b",
	"ctrl+l":  "\x0c",
	"ctrl+m":  "\x0d",
	"ctrl+n":  "\x0e",
	"ctrl+o":  "\x0f",
	"ctrl+p":  "\x10",
	"ctrl+q":  "\x11",
	"ctrl+r":  "\x12",
	"ctrl+s":  "\x13",
	"ctrl+t":  "\x14",
	"ctrl+u":  "\x15",
	"ctrl+v":  "\x16",
	"ctrl+w":  "\x17",
	"ctrl+x":  "\x18",
	"ctrl+y":  "\x19",
	"ctrl+z":  "\x1a",
	"ctrl+[":  "\x1b",
	"ctrl+\\": "\x1c",
	"ctrl+]":  "\x1d",
	"ctrl+^":  "\x1e",
	"ctrl+_":  "\x1f",
	"ctrl+?":  "\x7f",

	// Special Control Keys & Aliases
	// Note: "enter" maps to \r (KeyCR), distinct from "ctrl+j" (\n).
	// "backspace" maps to \x7f (KeyDEL).
	"enter":     "\r",
	"tab":       "\t",
	"backspace": "\x7f",
	"esc":       "\x1b",
	"escape":    "\x1b",
	"space":     " ",
	" ":         " ",

	// Arrow Keys
	"up":    "\x1b[A",
	"down":  "\x1b[B",
	"right": "\x1b[C",
	"left":  "\x1b[D",

	"shift+up":    "\x1b[1;2A",
	"shift+down":  "\x1b[1;2B",
	"shift+right": "\x1b[1;2C",
	"shift+left":  "\x1b[1;2D",

	"alt+up":    "\x1b[1;3A",
	"alt+down":  "\x1b[1;3B",
	"alt+right": "\x1b[1;3C",
	"alt+left":  "\x1b[1;3D",

	"alt+shift+up":    "\x1b[1;4A",
	"alt+shift+down":  "\x1b[1;4B",
	"alt+shift+right": "\x1b[1;4C",
	"alt+shift+left":  "\x1b[1;4D",

	"ctrl+up":    "\x1b[1;5A",
	"ctrl+down":  "\x1b[1;5B",
	"ctrl+right": "\x1b[1;5C",
	"ctrl+left":  "\x1b[1;5D",

	"alt+ctrl+up":    "\x1b[Oa", // urxvt
	"alt+ctrl+down":  "\x1b[Ob", // urxvt
	"alt+ctrl+right": "\x1b[Oc", // urxvt
	"alt+ctrl+left":  "\x1b[Od", // urxvt

	"ctrl+shift+up":    "\x1b[1;6A",
	"ctrl+shift+down":  "\x1b[1;6B",
	"ctrl+shift+right": "\x1b[1;6C",
	"ctrl+shift+left":  "\x1b[1;6D",

	"alt+ctrl+shift+up":    "\x1b[1;8A",
	"alt+ctrl+shift+down":  "\x1b[1;8B",
	"alt+ctrl+shift+right": "\x1b[1;8C",
	"alt+ctrl+shift+left":  "\x1b[1;8D",

	// Miscellaneous
	"shift+tab": "\x1b[Z",

	"insert":     "\x1b[2~",
	"alt+insert": "\x1b[3;2~",

	"delete":     "\x1b[3~",
	"alt+delete": "\x1b[3;3~",

	"pgup":          "\x1b[5~",
	"alt+pgup":      "\x1b[5;3~",
	"ctrl+pgup":     "\x1b[5;5~",
	"alt+ctrl+pgup": "\x1b[5;7~",

	"pgdown":          "\x1b[6~",
	"alt+pgdown":      "\x1b[6;3~",
	"ctrl+pgdown":     "\x1b[6;5~",
	"alt+ctrl+pgdown": "\x1b[6;7~",

	"home":                "\x1b[H",
	"alt+home":            "\x1b[1;3H",
	"ctrl+home":           "\x1b[1;5H",
	"alt+ctrl+home":       "\x1b[1;7H",
	"shift+home":          "\x1b[1;2H",
	"alt+shift+home":      "\x1b[1;4H",
	"ctrl+shift+home":     "\x1b[1;6H",
	"alt+ctrl+shift+home": "\x1b[1;8H",

	"end":                "\x1b[F",
	"alt+end":            "\x1b[1;3F",
	"ctrl+end":           "\x1b[1;5F",
	"alt+ctrl+end":       "\x1b[1;7F",
	"shift+end":          "\x1b[1;2F",
	"alt+shift+end":      "\x1b[1;4F",
	"ctrl+shift+end":     "\x1b[1;6F",
	"alt+ctrl+shift+end": "\x1b[1;8F",

	// Function Keys (using standard xterm sequences where multiple exist)
	"f1":  "\x1bOP",
	"f2":  "\x1bOQ",
	"f3":  "\x1bOR",
	"f4":  "\x1bOS",
	"f5":  "\x1b[15~",
	"f6":  "\x1b[17~",
	"f7":  "\x1b[18~",
	"f8":  "\x1b[19~",
	"f9":  "\x1b[20~",
	"f10": "\x1b[21~",
	"f11": "\x1b[23~",
	"f12": "\x1b[24~",
	"f13": "\x1b[1;2P",
	"f14": "\x1b[1;2Q",
	"f15": "\x1b[1;2R",
	"f16": "\x1b[1;2S",
	"f17": "\x1b[15;2~",
	"f18": "\x1b[17;2~",
	"f19": "\x1b[18;2~",
	"f20": "\x1b[19;2~",

	// Modified Function Keys
	"alt+f1":  "\x1b[1;3P",
	"alt+f2":  "\x1b[1;3Q",
	"alt+f3":  "\x1b[1;3R",
	"alt+f4":  "\x1b[1;3S",
	"alt+f5":  "\x1b[15;3~",
	"alt+f6":  "\x1b[17;3~",
	"alt+f7":  "\x1b[18;3~",
	"alt+f8":  "\x1b[19;3~",
	"alt+f9":  "\x1b[20;3~",
	"alt+f10": "\x1b[21;3~",
	"alt+f11": "\x1b[23;3~",
	"alt+f12": "\x1b[24;3~",
	"alt+f13": "\x1b[25;3~",
	"alt+f14": "\x1b[26;3~",
	"alt+f15": "\x1b[28;3~",
	"alt+f16": "\x1b[29;3~",
}
