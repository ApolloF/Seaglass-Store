package sources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	MaxDocumentBytes = 4 << 20
	MaxEntries       = 500
	maxNodes         = 100000
	maxNesting       = 128
)

var (
	versionRE       = regexp.MustCompile(`(?i)(?:^|[\s,(–—-])(v\d+(?:\.\d+)*[a-z]?(?:/v?\d+(?:\.\d+)*[a-z]?)*|build\s+\d+)\b`)
	releaseSuffixRE = regexp.MustCompile(`(?i)\s*\(\+?\s*(?:update\s+\d|all\s+dlcs|multi\d|from\s+\d|fast\s+install|bonus\s+content)`)
	dodiNumberRE    = regexp.MustCompile(`^\d+\s*[-–—]\s*`)
	sizeRE          = regexp.MustCompile(`(?i)^\s*(from\s+)?(\d+(?:\.\d+)?)\s*(KiB|MiB|GiB|TiB|KB|MB|GB|TB)\s*$`)
	metadataRE      = regexp.MustCompile(`(?i)^(?:[-–—]\s*)?(repack size|original size|game size|size|languages?|audio|text|interface(?: language)?|voice language)\s*:\s*(.*)$`)
	sizeOptionsRE   = regexp.MustCompile(`(?i)^((?:\d+(?:\.\d+)?/)+\d+(?:\.\d+)?)\s*(KiB|MiB|GiB|TiB|KB|MB|GB|TB)$`)
)

// Parse never fetches a link, executes page scripts, or interprets source claims as a safety verdict.
func Parse(source Source, page string, data []byte) (out []Entry, err error) {
	defer func() {
		if err == nil {
			sum := sha256.Sum256(data)
			for i := range out {
				if out[i].TitleKey == "" || len(out[i].RawTitle) > 1024 {
					out, err = nil, errors.New("release title is missing or exceeds 1024 bytes")
					return
				}
				out[i].DocumentSHA256 = hex.EncodeToString(sum[:])
			}
		}
	}()
	if _, err := source.ValidateURL(page); err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > MaxDocumentBytes {
		return nil, errors.New("document is empty or exceeds 4 MiB")
	}
	if source.ID == "fitgirl" && (bytes.HasPrefix(bytes.TrimSpace(data), []byte("<?xml")) || bytes.HasPrefix(bytes.TrimSpace(data), []byte("<rss"))) {
		return parseRSS(source, data)
	}
	root, err := document(data)
	if err != nil {
		return nil, err
	}
	if challenge(root) {
		return nil, &ResolutionError{State: "captcha-required", Reason: "source page requires an interactive browser challenge"}
	}
	if source.ID == "1337x" {
		return parse1337x(source, page, root)
	}
	var entries []Entry
	for _, article := range find(root, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "article" }) {
		titles := find(article, func(n *html.Node) bool { return class(n, "entry-title") })
		contents := find(article, func(n *html.Node) bool { return class(n, "entry-content") || class(n, "entry-summary") })
		if len(titles) == 0 || len(contents) == 0 {
			continue
		}
		raw := text(titles[0], false)
		body := text(contents[0], true)
		if !release(raw, body) && !(source.ID == "fitgirl" && class(article, "category-lossless-repack")) {
			continue
		}
		entryPage := page
		for _, a := range find(titles[0], isAnchor) {
			u, err := resolve(source, page, attr(a, "href"))
			if err == nil {
				entryPage = u
				break
			}
		}
		entry := parseEntry(source, entryPage, raw, contents[0])
		if class(contents[0], "entry-summary") {
			entry.SummaryOnly = true
			entry.Warnings = append(entry.Warnings, "Search summary; fetch the release page for complete metadata")
		}
		for _, clock := range find(article, func(n *html.Node) bool { return n.Data == "time" }) {
			date, err := time.Parse(time.RFC3339, attr(clock, "datetime"))
			if err != nil {
				continue
			}
			if class(clock, "updated") {
				entry.UpdatedAt = &date
			} else if entry.PublishedAt == nil {
				entry.PublishedAt = &date
			}
		}
		entries = append(entries, entry)
		if len(entries) > MaxEntries {
			return nil, errors.New("too many entries")
		}
	}
	if len(entries) == 0 {
		if len(find(root, func(n *html.Node) bool { return n.Data == "body" && class(n, "search-no-results") })) > 0 && len(find(root, func(n *html.Node) bool { return class(n, "page-title") && text(n, false) == "Nothing Found" })) > 0 {
			return []Entry{}, nil
		}
		return nil, errors.New("no release articles found; unsupported layout or challenge page")
	}
	return unique(entries), nil
}

func parseRSS(source Source, data []byte) ([]Entry, error) {
	type item struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Date        string `xml:"pubDate"`
		Content     string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
		Description string `xml:"description"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var entries []Entry
	var count int
	var sawRSS bool
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid RSS: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "rss" {
			sawRSS = true
		}
		if start.Name.Local != "item" {
			continue
		}
		count++
		if count > MaxEntries {
			return nil, errors.New("too many RSS items")
		}
		var record item
		if err := decoder.DecodeElement(&record, &start); err != nil {
			return nil, fmt.Errorf("invalid RSS item: %w", err)
		}
		if _, err := source.ValidateURL(record.Link); err != nil {
			return nil, fmt.Errorf("RSS item URL: %w", err)
		}
		content := record.Content
		if content == "" {
			content = record.Description
		}
		root, err := document([]byte(content))
		if err != nil {
			return nil, err
		}
		if !release(record.Title, text(root, true)) {
			continue
		}
		entry := parseEntry(source, record.Link, record.Title, root)
		if date, err := time.Parse(time.RFC1123Z, record.Date); err == nil {
			entry.PublishedAt = &date
		} else if record.Date != "" {
			entry.Warnings = append(entry.Warnings, "Unrecognized publication date")
		}
		entries = append(entries, entry)
	}
	if !sawRSS {
		return nil, errors.New("not an RSS document")
	}
	if len(entries) == 0 {
		return nil, errors.New("no release items found")
	}
	return unique(entries), nil
}

func parse1337x(source Source, page string, root *html.Node) ([]Entry, error) {
	var entries []Entry
	details := find(root, func(n *html.Node) bool { return class(n, "torrent-detail-page") })
	if len(details) > 0 {
		headings := find(details[0], func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "h1" })
		if len(headings) == 0 {
			return nil, errors.New("torrent detail has no title")
		}
		entry := parseEntry(source, page, text(headings[0], false), details[0])
		if len(entry.Transports) == 0 {
			return nil, errors.New("torrent detail has no supported transport")
		}
		return []Entry{entry}, nil
	}
	for _, cell := range find(root, func(n *html.Node) bool { return n.Data == "td" && class(n, "name") }) {
		for _, a := range find(cell, isAnchor) {
			href := attr(a, "href")
			if !strings.HasPrefix(href, "/torrent/") {
				continue
			}
			u, err := resolve(source, page, href)
			if err != nil {
				return nil, err
			}
			entry := parseEntry(source, u, text(a, false), a)
			entry.Warnings = append(entry.Warnings, "Listing only; retrieve this source detail page to inspect transport")
			entries = append(entries, entry)
			if len(entries) > MaxEntries {
				return nil, errors.New("too many listing entries")
			}
		}
	}
	if len(entries) == 0 {
		return nil, errors.New("no torrent entries found; unsupported layout or challenge page")
	}
	return unique(entries), nil
}

func release(title, body string) bool {
	s := strings.ToLower(body)
	return strings.Contains(s, "repack size") || strings.Contains(s, "magnet:") || strings.Contains(strings.ToLower(title), "[dodi repack]")
}

func parseEntry(source Source, page, raw string, root *html.Node) Entry {
	e := Entry{ID: entryID(source.ID, page), SourceID: source.ID, PageURL: page,
		RawTitle: strings.TrimSpace(raw), NeedsReview: true,
		Transports: []Transport{}, References: []Reference{},
		Warnings: []string{"Source metadata is unverified; payload safety has not been assessed"}}
	e.Title, e.Version = cleanTitle(source.ID, e.RawTitle)
	e.TitleKey = titleKey(e.Title)
	e.ReleaseKind = "release"
	if strings.Contains(strings.ToLower(e.RawTitle), "patch from ") || strings.Contains(strings.ToLower(e.RawTitle), "update only") {
		e.ReleaseKind = "update"
		e.Warnings = append(e.Warnings, "Update-only release; not a standalone install")
	}
	for _, line := range strings.Split(text(root, true), "\n") {
		m := metadataRE.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		value := strings.TrimSpace(m[2])
		switch strings.ToLower(m[1]) {
		case "repack size", "size":
			e.SizeClaim = value
			e.SizeBytes, e.SizeIsMinimum = parseSize(value)
			e.SizeOptionsBytes = parseSizeOptions(value)
		case "original size", "game size":
			e.InstalledSizeBytes, _ = parseSize(value)
		case "language", "languages", "audio", "text", "interface", "interface language", "voice language":
			if len(value) <= 512 && value != "" {
				if e.LanguageClaim != "" {
					e.LanguageClaim += "; "
				}
				if strings.EqualFold(m[1], "text") || strings.EqualFold(m[1], "audio") || strings.EqualFold(m[1], "voice language") {
					e.LanguageClaim += m[1] + ": "
				}
				e.LanguageClaim += value
			}
		}
	}
	seen := map[string]bool{}
	for _, a := range find(root, isAnchor) {
		href := strings.TrimSpace(attr(a, "href"))
		if href == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(href), "magnet:") {
			t, err := Magnet(href)
			if err != nil {
				e.Warnings = append(e.Warnings, "Rejected malformed or unsupported magnet")
				continue
			}
			if !seen[t.InfoHash] {
				e.Transports = append(e.Transports, t)
				seen[t.InfoHash] = true
			}
			if len(e.Transports) > 100 {
				e.Transports = e.Transports[:100]
				e.Warnings = append(e.Warnings, "Transport limit reached")
				break
			}
			continue
		}
		u, err := url.Parse(href)
		if err != nil {
			continue
		}
		base, _ := url.Parse(page)
		u = base.ResolveReference(u)
		u.Fragment = ""
		if _, err := referenceURL(u.String()); err != nil {
			continue
		}
		label := text(a, false)
		if len(label) > 200 {
			label = label[:200]
		}
		if u.Scheme == "https" && strings.HasSuffix(strings.ToLower(u.Path), ".torrent") && u.Hostname() == source.Host {
			if !seen[u.String()] {
				e.Transports = append(e.Transports, Transport{Kind: "torrent-url", URI: u.String()})
				seen[u.String()] = true
			}
		} else if strings.Contains(strings.ToLower(label), "torrent") || strings.Contains(strings.ToLower(label), "click here") || strings.Contains(strings.ToLower(label), "download") || u.Hostname() == "1337x.to" {
			if !seen[u.String()] && len(e.References) < 100 {
				e.References = append(e.References, Reference{URL: u.String(), Label: label, Kind: referenceKind(a, root)})
				seen[u.String()] = true
			}
		}
		if len(e.Transports) > 100 {
			e.Transports = e.Transports[:100]
			e.Warnings = append(e.Warnings, "Transport limit reached")
			break
		}
	}
	if len(e.Transports) == 0 {
		e.Warnings = append(e.Warnings, "No direct supported torrent reference; external links remain unresolved")
	}
	if len(e.Transports) > 0 {
		e.Warnings = append(e.Warnings, "Torrent identities are source claims, not verified payload hashes; magnet trackers and extra parameters are removed")
	}
	if e.SizeBytes == 0 {
		if len(e.SizeOptionsBytes) > 0 {
			e.Warnings = append(e.Warnings, "Source lists alternative sizes; select and verify the actual torrent before estimating disk space")
		} else {
			e.Warnings = append(e.Warnings, "Download size unknown")
		}
	}
	if e.SizeIsMinimum {
		e.Warnings = append(e.Warnings, "Reported download size is a minimum, not a disk-space estimate")
	}
	return e
}

func cleanTitle(source, raw string) (string, string) {
	title := raw
	if source == "dodi" {
		title = dodiNumberRE.ReplaceAllString(title, "")
	}
	for _, suffix := range []string{"[DODI Repack]", "[FitGirl Repack]"} {
		title = strings.TrimSuffix(title, suffix)
	}
	version := ""
	if loc := versionRE.FindStringSubmatchIndex(title); loc != nil {
		version = title[loc[2]:loc[3]]
		title = title[:loc[0]]
	}
	// These are release qualifiers, not evidence of a different game or supported languages.
	if loc := releaseSuffixRE.FindStringIndex(title); loc != nil {
		title = title[:loc[0]]
	}
	return strings.Trim(strings.TrimSpace(title), " ,–—-("), version
}

func parseSize(raw string) (int64, bool) {
	raw = stripSizeQualifier(raw)
	m := sizeRE.FindStringSubmatch(raw)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		return 0, false
	}
	mult := map[string]float64{"kb": 1e3, "mb": 1e6, "gb": 1e9, "tb": 1e12, "kib": 1024, "mib": 1 << 20, "gib": 1 << 30, "tib": 1 << 40}[strings.ToLower(m[3])]
	if n <= 0 || n*mult > 1e15 {
		return 0, false
	}
	return int64(n * mult), m[1] != ""
}

func stripSizeQualifier(raw string) string {
	raw = strings.TrimSpace(raw)
	const qualifier = "[Selective Download]"
	if len(raw) >= len(qualifier) && strings.EqualFold(raw[len(raw)-len(qualifier):], qualifier) {
		raw = strings.TrimSpace(raw[:len(raw)-len(qualifier)])
	}
	return raw
}

func parseSizeOptions(raw string) []int64 {
	m := sizeOptionsRE.FindStringSubmatch(stripSizeQualifier(raw))
	if m == nil {
		return nil
	}
	parts := strings.Split(m[1], "/")
	if len(parts) > 10 {
		return nil
	}
	var options []int64
	for _, part := range parts {
		size, _ := parseSize(part + " " + m[2])
		if size == 0 {
			return nil
		}
		options = append(options, size)
	}
	return options
}

func document(data []byte) (*html.Node, error) {
	z := html.NewTokenizer(bytes.NewReader(data))
	var stack []string
	var formatting []string
	for count := 0; ; count++ {
		if count > maxNodes {
			return nil, errors.New("HTML token limit exceeded")
		}
		token := z.Next()
		if token == html.ErrorToken {
			if !errors.Is(z.Err(), io.EOF) {
				return nil, z.Err()
			}
			break
		}
		if token == html.StartTagToken || token == html.EndTagToken {
			nameBytes, _ := z.TagName()
			name := string(nameBytes)
			if formattingTag(name) {
				if token == html.StartTagToken {
					formatting = append(formatting, name)
					if len(formatting) > 64 {
						return nil, errors.New("unclosed HTML formatting limit exceeded")
					}
				} else {
					for i := len(formatting) - 1; i >= 0; i-- {
						if formatting[i] == name {
							formatting = append(formatting[:i], formatting[i+1:]...)
							break
						}
					}
				}
			}
			if token == html.EndTagToken {
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] == name {
						stack = stack[:i]
						break
					}
				}
			} else if !voidTag(name) {
				stack = append(stack, name)
				if len(stack) > maxNesting {
					return nil, errors.New("HTML nesting limit exceeded")
				}
			}
		}
	}
	root, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	// HTML repairs can create a different tree; validate its depth once before repeated text walks.
	for n, depth := root, 0; n != nil; {
		if depth > maxNesting {
			return nil, errors.New("repaired HTML nesting limit exceeded")
		}
		if n.FirstChild != nil {
			n = n.FirstChild
			depth++
			continue
		}
		for n != root && n.NextSibling == nil {
			n = n.Parent
			depth--
		}
		if n == root {
			break
		}
		n = n.NextSibling
	}
	return root, nil
}

func voidTag(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func formattingTag(name string) bool {
	switch name {
	case "a", "b", "big", "code", "em", "font", "i", "nobr", "s", "small", "strike", "strong", "tt", "u":
		return true
	}
	return false
}

func find(root *html.Node, predicate func(*html.Node) bool) []*html.Node {
	var result []*html.Node
	for n := root; n != nil; n = next(root, n) {
		if predicate(n) {
			result = append(result, n)
		}
	}
	return result
}

func next(root, n *html.Node) *html.Node {
	if n.FirstChild != nil {
		return n.FirstChild
	}
	for n != root {
		if n.NextSibling != nil {
			return n.NextSibling
		}
		n = n.Parent
	}
	return nil
}

func text(root *html.Node, lines bool) string {
	var b strings.Builder
	for n := root; n != nil; n = next(root, n) {
		if n.Type == html.TextNode {
			blocked := false
			for p := n.Parent; p != nil; p = p.Parent {
				if p.Data == "script" || p.Data == "style" {
					blocked = true
					break
				}
			}
			if !blocked {
				b.WriteString(n.Data)
			}
		}
		if lines && n.Type == html.ElementNode && (n.Data == "br" || n.Data == "p" || n.Data == "li" || n.Data == "div" || strings.HasPrefix(n.Data, "h")) {
			b.WriteByte('\n')
		}
	}
	if !lines {
		return strings.Join(strings.Fields(b.String()), " ")
	}
	var result []string
	for _, line := range strings.Split(b.String(), "\n") {
		result = append(result, strings.Join(strings.Fields(line), " "))
	}
	return strings.Join(result, "\n")
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func class(n *html.Node, name string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == name {
			return true
		}
	}
	return false
}
func isAnchor(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "a" }
func resolve(source Source, base, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	b, _ := url.Parse(base)
	u = b.ResolveReference(u)
	if _, err := source.ValidateURL(u.String()); err != nil {
		return "", err
	}
	return u.String(), nil
}
func unique(entries []Entry) []Entry {
	seen := map[string]bool{}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if !seen[e.ID] {
			out = append(out, e)
			seen[e.ID] = true
		}
	}
	return out
}
