package commerce

import (
	"strings"
	"unicode"
)

// Text shown to customers is one line of characters they can see. hiddenText is what such a line does not hold, the
// same list as ledger.store_text_plain (migration 123) and the operations web (web/src/storePageForm.ts):
//
//	Cc, Cf, Zl, Zp  control and format characters, line and paragraph separators
//	default ignorable code points  characters a display leaves out: the Hangul fillers, variation selectors, the
//	                               combining grapheme joiner, tags
//	blanks other than the space    the no-break space and the other Zs spaces, the blank Braille pattern
var hiddenText = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x00A0, Hi: 0x00A0, Stride: 1}, {Lo: 0x034F, Hi: 0x034F, Stride: 1}, {Lo: 0x115F, Hi: 0x1160, Stride: 1},
		{Lo: 0x1680, Hi: 0x1680, Stride: 1}, {Lo: 0x17B4, Hi: 0x17B5, Stride: 1}, {Lo: 0x180B, Hi: 0x180F, Stride: 1},
		{Lo: 0x2000, Hi: 0x200A, Stride: 1}, {Lo: 0x202F, Hi: 0x202F, Stride: 1}, {Lo: 0x205F, Hi: 0x205F, Stride: 1},
		{Lo: 0x2800, Hi: 0x2800, Stride: 1}, {Lo: 0x3000, Hi: 0x3000, Stride: 1}, {Lo: 0x3164, Hi: 0x3164, Stride: 1},
		{Lo: 0xFE00, Hi: 0xFE0F, Stride: 1}, {Lo: 0xFFA0, Hi: 0xFFA0, Stride: 1}, {Lo: 0xFFF0, Hi: 0xFFF8, Stride: 1},
	},
	R32: []unicode.Range32{{Lo: 0xE0000, Hi: 0xE0FFF, Stride: 1}},
}

// visibleLine says whether text is one visible line: none of the hidden characters, and when it is not empty at
// least one character that is not a space.
func visibleLine(text string) bool {
	if strings.ContainsFunc(text, func(r rune) bool { return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp, hiddenText) }) {
		return false
	}
	return text == "" || strings.Trim(text, " ") != ""
}
