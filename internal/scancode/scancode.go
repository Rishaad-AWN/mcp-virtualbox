// Package scancode maps human-friendly key names to PS/2 Set 1 scancode bytes
// suitable for `VBoxManage controlvm <vm> keyboardputscancode`.
package scancode

import (
	"fmt"
	"sort"
	"strings"
)

// entry holds the make (press) and break (release) byte sequences for a key.
// Multi-byte keys use the 0xE0 prefix.
type entry struct {
	make []byte
	brk  []byte
}

var scancodes = map[string]entry{
	// Letters
	"a": {[]byte{0x1e}, []byte{0x9e}}, "b": {[]byte{0x30}, []byte{0xb0}}, "c": {[]byte{0x2e}, []byte{0xae}}, "d": {[]byte{0x20}, []byte{0xa0}},
	"e": {[]byte{0x12}, []byte{0x92}}, "f": {[]byte{0x21}, []byte{0xa1}}, "g": {[]byte{0x22}, []byte{0xa2}}, "h": {[]byte{0x23}, []byte{0xa3}},
	"i": {[]byte{0x17}, []byte{0x97}}, "j": {[]byte{0x24}, []byte{0xa4}}, "k": {[]byte{0x25}, []byte{0xa5}}, "l": {[]byte{0x26}, []byte{0xa6}},
	"m": {[]byte{0x32}, []byte{0xb2}}, "n": {[]byte{0x31}, []byte{0xb1}}, "o": {[]byte{0x18}, []byte{0x98}}, "p": {[]byte{0x19}, []byte{0x99}},
	"q": {[]byte{0x10}, []byte{0x90}}, "r": {[]byte{0x13}, []byte{0x93}}, "s": {[]byte{0x1f}, []byte{0x9f}}, "t": {[]byte{0x14}, []byte{0x94}},
	"u": {[]byte{0x16}, []byte{0x96}}, "v": {[]byte{0x2f}, []byte{0xaf}}, "w": {[]byte{0x11}, []byte{0x91}}, "x": {[]byte{0x2d}, []byte{0xad}},
	"y": {[]byte{0x15}, []byte{0x95}}, "z": {[]byte{0x2c}, []byte{0xac}},

	// Numbers (main row)
	"1": {[]byte{0x02}, []byte{0x82}}, "2": {[]byte{0x03}, []byte{0x83}}, "3": {[]byte{0x04}, []byte{0x84}}, "4": {[]byte{0x05}, []byte{0x85}},
	"5": {[]byte{0x06}, []byte{0x86}}, "6": {[]byte{0x07}, []byte{0x87}}, "7": {[]byte{0x08}, []byte{0x88}}, "8": {[]byte{0x09}, []byte{0x89}},
	"9": {[]byte{0x0a}, []byte{0x8a}}, "0": {[]byte{0x0b}, []byte{0x8b}},

	// Numeric keypad
	"kp_7": {[]byte{0x47}, []byte{0xc7}}, "kp_8": {[]byte{0x48}, []byte{0xc8}}, "kp_9": {[]byte{0x49}, []byte{0xc9}},
	"kp_4": {[]byte{0x4b}, []byte{0xcb}}, "kp_5": {[]byte{0x4c}, []byte{0xcc}}, "kp_6": {[]byte{0x4d}, []byte{0xcd}},
	"kp_1": {[]byte{0x4f}, []byte{0xcf}}, "kp_2": {[]byte{0x50}, []byte{0xd0}}, "kp_3": {[]byte{0x51}, []byte{0xd1}},
	"kp_0": {[]byte{0x52}, []byte{0xd2}}, "kp_dot": {[]byte{0x53}, []byte{0xd3}},
	"kp_enter": {[]byte{0xe0, 0x1c}, []byte{0xe0, 0x9c}},
	"kp_plus":  {[]byte{0x4e}, []byte{0xce}}, "kp_minus": {[]byte{0x4a}, []byte{0xca}},
	"kp_star": {[]byte{0x37}, []byte{0xb7}}, "kp_multiply": {[]byte{0x37}, []byte{0xb7}},
	"kp_slash": {[]byte{0xe0, 0x35}, []byte{0xe0, 0xb5}}, "kp_divide": {[]byte{0xe0, 0x35}, []byte{0xe0, 0xb5}},

	// Special keys
	"esc": {[]byte{0x01}, []byte{0x81}}, "escape": {[]byte{0x01}, []byte{0x81}},
	"enter": {[]byte{0x1c}, []byte{0x9c}}, "return": {[]byte{0x1c}, []byte{0x9c}},
	"tab":   {[]byte{0x0f}, []byte{0x8f}},
	"space": {[]byte{0x39}, []byte{0xb9}}, "spacebar": {[]byte{0x39}, []byte{0xb9}},
	"backspace": {[]byte{0x0e}, []byte{0x8e}},
	"delete":    {[]byte{0xe0, 0x53}, []byte{0xe0, 0xd3}}, "del": {[]byte{0xe0, 0x53}, []byte{0xe0, 0xd3}},
	"insert": {[]byte{0xe0, 0x52}, []byte{0xe0, 0xd2}},
	"home":   {[]byte{0xe0, 0x47}, []byte{0xe0, 0xc7}},
	"end":    {[]byte{0xe0, 0x4f}, []byte{0xe0, 0xcf}},
	"pageup": {[]byte{0xe0, 0x49}, []byte{0xe0, 0xc9}}, "pgup": {[]byte{0xe0, 0x49}, []byte{0xe0, 0xc9}},
	"pagedown": {[]byte{0xe0, 0x51}, []byte{0xe0, 0xd1}}, "pgdn": {[]byte{0xe0, 0x51}, []byte{0xe0, 0xd1}},

	// Arrow keys
	"up":    {[]byte{0xe0, 0x48}, []byte{0xe0, 0xc8}},
	"down":  {[]byte{0xe0, 0x50}, []byte{0xe0, 0xd0}},
	"left":  {[]byte{0xe0, 0x4b}, []byte{0xe0, 0xcb}},
	"right": {[]byte{0xe0, 0x4d}, []byte{0xe0, 0xcd}},

	// Modifier keys
	"shift": {[]byte{0x2a}, []byte{0xaa}}, "lshift": {[]byte{0x2a}, []byte{0xaa}}, "rshift": {[]byte{0x36}, []byte{0xb6}},
	"ctrl": {[]byte{0x1d}, []byte{0x9d}}, "lctrl": {[]byte{0x1d}, []byte{0x9d}}, "rctrl": {[]byte{0xe0, 0x1d}, []byte{0xe0, 0x9d}},
	"alt": {[]byte{0x38}, []byte{0xb8}}, "lalt": {[]byte{0x38}, []byte{0xb8}}, "ralt": {[]byte{0xe0, 0x38}, []byte{0xe0, 0xb8}},
	"win": {[]byte{0xe0, 0x5b}, []byte{0xe0, 0xdb}}, "lwin": {[]byte{0xe0, 0x5b}, []byte{0xe0, 0xdb}},
	"rwin": {[]byte{0xe0, 0x5c}, []byte{0xe0, 0xdc}},
	"menu": {[]byte{0xe0, 0x5d}, []byte{0xe0, 0xdd}},

	// Function keys
	"f1": {[]byte{0x3b}, []byte{0xbb}}, "f2": {[]byte{0x3c}, []byte{0xbc}}, "f3": {[]byte{0x3d}, []byte{0xbd}}, "f4": {[]byte{0x3e}, []byte{0xbe}},
	"f5": {[]byte{0x3f}, []byte{0xbf}}, "f6": {[]byte{0x40}, []byte{0xc0}}, "f7": {[]byte{0x41}, []byte{0xc1}}, "f8": {[]byte{0x42}, []byte{0xc2}},
	"f9": {[]byte{0x43}, []byte{0xc3}}, "f10": {[]byte{0x44}, []byte{0xc4}}, "f11": {[]byte{0x57}, []byte{0xd7}}, "f12": {[]byte{0x58}, []byte{0xd8}},

	// Symbols
	"minus": {[]byte{0x0c}, []byte{0x8c}}, "-": {[]byte{0x0c}, []byte{0x8c}},
	"equals": {[]byte{0x0d}, []byte{0x8d}}, "=": {[]byte{0x0d}, []byte{0x8d}},
	"lbracket": {[]byte{0x1a}, []byte{0x9a}}, "[": {[]byte{0x1a}, []byte{0x9a}},
	"rbracket": {[]byte{0x1b}, []byte{0x9b}}, "]": {[]byte{0x1b}, []byte{0x9b}},
	"backslash": {[]byte{0x2b}, []byte{0xab}}, "\\": {[]byte{0x2b}, []byte{0xab}},
	"semicolon": {[]byte{0x27}, []byte{0xa7}}, ";": {[]byte{0x27}, []byte{0xa7}},
	"quote": {[]byte{0x28}, []byte{0xa8}}, "'": {[]byte{0x28}, []byte{0xa8}},
	"backtick": {[]byte{0x29}, []byte{0xa9}}, "`": {[]byte{0x29}, []byte{0xa9}},
	"comma": {[]byte{0x33}, []byte{0xb3}}, ",": {[]byte{0x33}, []byte{0xb3}},
	"period": {[]byte{0x34}, []byte{0xb4}}, ".": {[]byte{0x34}, []byte{0xb4}},
	"slash": {[]byte{0x35}, []byte{0xb5}}, "/": {[]byte{0x35}, []byte{0xb5}},

	// Other
	"capslock":    {[]byte{0x3a}, []byte{0xba}},
	"numlock":     {[]byte{0x45}, []byte{0xc5}},
	"scrolllock":  {[]byte{0x46}, []byte{0xc6}},
	"printscreen": {[]byte{0xe0, 0x37}, []byte{0xe0, 0xb7}},
	"pause":       {[]byte{0xe1, 0x1d, 0x45, 0xe1, 0x9d, 0xc5}, nil},
}

// Resolve turns a key combo like "ctrl+alt+delete" into the ordered list of
// hex scancode bytes: all make codes in order, then all break codes in reverse,
// matching how a real keyboard reports a chord.
func Resolve(keyCombo string) ([]string, error) {
	keys := strings.Split(strings.ToLower(keyCombo), "+")
	for i := range keys {
		keys[i] = strings.TrimSpace(keys[i])
	}

	var codes []byte
	for _, key := range keys {
		e, ok := scancodes[key]
		if !ok {
			return nil, fmt.Errorf("unknown key: %q", key)
		}
		codes = append(codes, e.make...)
	}
	for i := len(keys) - 1; i >= 0; i-- {
		codes = append(codes, scancodes[keys[i]].brk...)
	}

	out := make([]string, len(codes))
	for i, b := range codes {
		out[i] = fmt.Sprintf("%02x", b)
	}
	return out, nil
}

// AvailableKeys returns every recognised key name, sorted.
func AvailableKeys() []string {
	keys := make([]string, 0, len(scancodes))
	for k := range scancodes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
