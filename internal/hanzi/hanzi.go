// Package hanzi writes Chinese in the simplified characters the app speaks:
// the words of songs are often kept, or found at LRCLib, in traditional ones.
package hanzi

import (
	_ "embed"
	"strings"
	"unicode/utf8"
)

// ToSimplified returns s with its traditional characters written as their
// simplified forms, as OpenCC's t2s makes them.
func ToSimplified(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if to, ok := t2s[r]; ok {
			r = to
		}
		out = append(out, r)
	}
	return string(out)
}

// t2s.txt is OpenCC's t2s of each character on its own, and 妳 for 你, which
// it keeps: one character and its simplified form on each line.
//
//go:embed t2s.txt
var table string

var t2s = func() map[rune]rune {
	m := make(map[rune]rune, 4300)
	for _, line := range strings.Split(table, "\n") {
		from, size := utf8.DecodeRuneInString(line)
		if size == 0 {
			continue
		}
		to, _ := utf8.DecodeRuneInString(line[size:])
		m[from] = to
	}
	return m
}()
