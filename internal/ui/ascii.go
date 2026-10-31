package ui

import "strings"

// asciiGlyphs swaps each non-ASCII glyph for one ASCII cell, so layouts are
// unchanged. Braille chart cells are handled by range in asciiOnly.
var asciiGlyphs = map[rune]rune{
	'╭': '+', '╮': '+', '╰': '+', '╯': '+', '┌': '+', '┐': '+', '└': '+', '┘': '+',
	'─': '-', '│': '|', '━': '=', '▌': '>', '▏': '|',
	'●': '*', '○': 'o', '◆': '#', '✕': 'x', '✓': 'v', '▲': '^', '■': '#', '□': '.',
	'▁': '_', '▂': '_', '▃': '-', '▄': '-', '▅': '=', '▆': '=', '▇': '#', '█': '#', '▀': '#',
	'·': '.', '↓': 'v', '↑': '^', '›': '>', '…': '~', '⏎': 'E', '•': '*', '≥': '>', '×': 'x', '→': '>',
}

// asciiOnly rewrites a rendered screen for terminals without Unicode.
func asciiOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if g, ok := asciiGlyphs[r]; ok {
			return g
		}
		if r >= 0x2800 && r <= 0x28ff {
			if r == 0x2800 {
				return ' '
			}
			return '*'
		}
		if r > 0x7e {
			return '?'
		}
		return r
	}, s)
}
