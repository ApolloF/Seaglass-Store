package enrich

import (
	"regexp"
	"strings"
	"unicode"
)

// Review text is user-written and arrives with Steam's BBCode. It is turned
// into readable plain text here, so the interface only ever shows text:
// formatting becomes line breaks and punctuation, links keep their address
// in brackets, and anything that looks like HTML is dropped.

var (
	reImg       = regexp.MustCompile(`(?is)\[img[^\]]*\].*?\[/img\]`)
	reURLText   = regexp.MustCompile(`(?is)\[url=([^\]]*)\](.*?)\[/url\]`)
	reURLBare   = regexp.MustCompile(`(?is)\[url\](.*?)\[/url\]`)
	reSpoiler   = regexp.MustCompile(`(?is)\[spoiler\](.*?)\[/spoiler\]`)
	reQuoteOpen = regexp.MustCompile(`(?i)\[quote(?:=([^\]]*))?\]`)
	reQuoteEnd  = regexp.MustCompile(`(?i)\[/quote\]`)
	reHeadEnd   = regexp.MustCompile(`(?i)\[/h[1-6]\]`)
	reListOpen  = regexp.MustCompile(`(?i)\n?\[o?list\]\n?`)
	reListEnd   = regexp.MustCompile(`(?i)\n?\[/o?list\]\n?`)
	reBullet    = regexp.MustCompile(`(?i)\n?[ \t]*\[\*\]`)
	reRule      = regexp.MustCompile(`(?i)\[hr\]`)
	reRowEnd    = regexp.MustCompile(`(?i)\[/tr\]`)
	reCellEnd   = regexp.MustCompile(`(?i)\[/t[dh]\]`)
	// Only known tags go, so "[Story 9/10]" in a review stays as written.
	reTag        = regexp.MustCompile(`(?i)\[/?(?:b|i|u|s|strike|h[1-6]|spoiler|noparse|code|quote|list|olist|table|tr|td|th|p|center|left|right|url|img)(?:=[^\]]*)?\]`)
	reSteamImage = regexp.MustCompile(`\{STEAM_CLAN_(?:LOC_)?IMAGE\}/\S*`)
	reHTMLCode   = regexp.MustCompile(`(?is)<(?:script|style)[^>]*>.*?</(?:script|style)>`)
	reHTML       = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9]*(?:\s[^<>]*)?/?>`)
	reManyBlank  = regexp.MustCompile(`\n{3,}`)
	reLineSpace  = regexp.MustCompile(`[ \t]+\n`)
)

// plainText converts a Steam review to plain text of at most max characters.
func plainText(s string, max int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = reImg.ReplaceAllString(s, "")
	s = reURLText.ReplaceAllStringFunc(s, func(m string) string {
		p := reURLText.FindStringSubmatch(m)
		return linkText(p[2], p[1])
	})
	s = reURLBare.ReplaceAllString(s, "$1")
	s = reSpoiler.ReplaceAllString(s, "(spoiler: $1)")
	s = reQuoteOpen.ReplaceAllStringFunc(s, func(m string) string {
		if who := strings.TrimSpace(reQuoteOpen.FindStringSubmatch(m)[1]); who != "" {
			return "\n" + who + " wrote: “"
		}
		return "\n“"
	})
	s = reQuoteEnd.ReplaceAllString(s, "”\n")
	s = reHeadEnd.ReplaceAllString(s, "\n")
	s = reListOpen.ReplaceAllString(s, "\n\n")
	s = reListEnd.ReplaceAllString(s, "\n\n")
	s = reBullet.ReplaceAllString(s, "\n• ")
	s = reRule.ReplaceAllString(s, "\n")
	s = reRowEnd.ReplaceAllString(s, "\n")
	s = reCellEnd.ReplaceAllString(s, " | ")
	s = reTag.ReplaceAllString(s, "")
	s = reSteamImage.ReplaceAllString(s, "")
	s = reHTMLCode.ReplaceAllString(s, "")
	s = reHTML.ReplaceAllString(s, "")
	s = cleanChars(s, true)
	s = reLineSpace.ReplaceAllString(s, "\n")
	s = reManyBlank.ReplaceAllString(s, "\n\n")
	return truncate(strings.TrimSpace(s), max)
}

// linkText keeps what a link says and, when that isn't the address itself,
// the address too (web links only).
func linkText(text, href string) string {
	href = strings.TrimSpace(href)
	text = strings.TrimSpace(text)
	lower := strings.ToLower(href)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		return text
	}
	if text == "" || text == href {
		return href
	}
	return text + " (" + href + ")"
}

// cleanText is for short single-line text such as names.
func cleanText(s string, max int) string {
	s = cleanChars(s, false)
	return truncate(strings.TrimSpace(strings.Join(strings.Fields(s), " ")), max)
}

// cleanChars drops control and invisible formatting characters; newlines
// and tabs survive only when asked.
func cleanChars(s string, keepNewlines bool) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' && keepNewlines, r == '\t' && keepNewlines:
			return r
		case r == 0x200d:
			return r // joins emoji
		case r == 0xa0:
			return ' '
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == unicode.ReplacementChar:
			return -1
		}
		return r
	}, s)
}

// truncate cuts to max characters (not bytes), marking the cut.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n == max {
			return strings.TrimRightFunc(s[:i], unicode.IsSpace) + "…"
		}
		n++
	}
	return s
}
