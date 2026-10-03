package sources

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// The complete catalog is finite and considerably larger than a WordPress page.
const maxElAmigosEntries = 5000

var (
	elAmigosTitleRE   = regexp.MustCompile(`^(.+?)\s+\((?:19|20)\d{2}\)(?:\s*,\s*(.*))?$`)
	elAmigosVersionRE = regexp.MustCompile(`(?i)\bUpdated to (version|build)\s+([a-z0-9][a-z0-9._/-]*)(?:\s+\((\d{2}\.\d{2}\.\d{4})\))?`)
	elAmigosDateRE    = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}$`)
	elAmigosPatchRE   = regexp.MustCompile(`(?i)\b(?:update|patch)\s+[a-z0-9][a-z0-9._]*\s*[-–]\s*[a-z0-9]`)
)

func parseElAmigos(source Source, page string, root *html.Node) ([]Entry, error) {
	u, _ := url.Parse(page)
	if u.Path == "" || u.Path == "/" || u.Path == "/index.html" {
		return parseElAmigosCatalog(source, page, root)
	}
	if !elAmigosDetailURL(u) {
		return nil, errors.New("unsupported ElAmigos document URL")
	}
	return parseElAmigosDetail(source, page, root)
}

func elAmigosDetailURL(u *url.URL) bool {
	return strings.HasPrefix(u.Path, "/data/") && strings.HasSuffix(u.Path, ".html") &&
		strings.Count(u.Path, "/") == 2 && u.RawQuery == ""
}

func parseElAmigosCatalog(source Source, page string, root *html.Node) ([]Entry, error) {
	var entries []Entry
	var date *time.Time
	seen := map[string]bool{}
	for n := root; n != nil; n = next(root, n) {
		// The alphabetic index has no dates; don't inherit the last news batch's date.
		if (n.Type == html.CommentNode && strings.TrimSpace(n.Data) == "Index start") || n.Data == "h4" {
			date = nil
		}
		if n.Type != html.ElementNode {
			continue
		}
		if n.Data == "h1" {
			date = elAmigosDate(text(n, false))
			continue
		}
		if n.Data != "h3" && n.Data != "h5" {
			continue
		}
		raw := text(n, false)
		lower := strings.ToLower(raw)
		pos := strings.Index(lower, " elamigos")
		if pos < 1 {
			continue
		}
		for _, a := range find(n, isAnchor) {
			if !strings.EqualFold(text(a, false), "DOWNLOAD") {
				continue
			}
			resolved, err := resolve(source, page, attr(a, "href"))
			if err != nil {
				return nil, err
			}
			u, _ := url.Parse(resolved)
			if !elAmigosDetailURL(u) {
				return nil, errors.New("unsupported ElAmigos release link")
			}
			if seen[resolved] {
				continue
			}
			seen[resolved] = true
			e := elAmigosEntry(source, resolved, raw, strings.TrimSpace(raw[:pos]))
			e.SummaryOnly = true
			e.UpdatedAt = date
			e.Warnings = append(e.Warnings, "Catalog summary; fetch the detail page to verify availability and the included installer version")
			if date != nil {
				e.Warnings = append(e.Warnings, "Date is a source catalog batch date, not the original game release date")
			}
			if elAmigosPreview(lower) {
				e.ReleaseKind = "preview"
			}
			entries = append(entries, e)
			if len(entries) > maxElAmigosEntries {
				return nil, errors.New("ElAmigos catalog exceeds 5000 entries")
			}
		}
	}
	if len(entries) == 0 {
		return nil, errors.New("no ElAmigos catalog entries found; unsupported layout")
	}
	return entries, nil
}

func parseElAmigosDetail(source Source, page string, root *html.Node) ([]Entry, error) {
	var e Entry
	var found, releaseClaim, preview, patch bool
	var host string
	seen := map[string]bool{}
	for n := root; n != nil; n = next(root, n) {
		if n.Type != html.ElementNode {
			continue
		}
		if n.Data == "h2" {
			line := text(n, false)
			if !found {
				m := elAmigosTitleRE.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				e = elAmigosEntry(source, page, line, m[1])
				e.SizeClaim = m[2]
				e.SizeBytes, _ = parseSize(m[2])
				if elAmigosPatchRE.MatchString(m[1]) || strings.Contains(strings.ToLower(m[1]), "update only") {
					e.ReleaseKind = "update"
					e.Warnings = append(e.Warnings, "Update-only release; not a standalone install")
				}
				found = true
				continue
			}
			if elAmigosPatchRE.MatchString(line) {
				patch, host = true, ""
				if len(line) <= 1024 && len(e.Warnings) < 40 {
					e.Warnings = append(e.Warnings, "Separate patch claim: "+line)
				}
				continue
			}
			host = elAmigosHostSection(line)
		}
		if !found {
			continue
		}
		if (n.Data == "h3" || n.Data == "p") && !patch && host == "" {
			line := text(n, false)
			lower := strings.ToLower(line)
			preview = preview || elAmigosPreview(lower)
			if strings.HasPrefix(lower, "elamigos release") {
				releaseClaim = true
				if m := elAmigosVersionRE.FindStringSubmatch(line); m != nil {
					e.Version = m[2]
					if strings.EqualFold(m[1], "build") {
						e.Version = "build " + e.Version
					}
					e.UpdatedAt = elAmigosDate(m[3])
				}
			}
			if _, value, ok := strings.Cut(line, ":"); ok {
				value = strings.TrimSpace(value)
				switch {
				case strings.HasPrefix(lower, "upload size / to download:"):
					e.SizeClaim = value
					e.SizeBytes, _ = parseSize(value)
				case strings.HasPrefix(lower, "languages:"), strings.HasPrefix(lower, "dubbing/audio:"):
					if value != "" && len(value) <= 512 && len(e.LanguageClaim)+len(line) <= 1024 {
						if e.LanguageClaim != "" {
							e.LanguageClaim += "; "
						}
						if strings.HasPrefix(lower, "dubbing/audio:") {
							e.LanguageClaim += "Audio: "
						}
						e.LanguageClaim += value
					}
				}
			}
		}
		if !isAnchor(n) || patch || host == "" {
			continue
		}
		u, err := url.Parse(strings.TrimSpace(attr(n, "href")))
		if err != nil {
			continue
		}
		base, _ := url.Parse(page)
		u = base.ResolveReference(u)
		if _, err := referenceURL(u.String()); err != nil {
			if len(e.Warnings) < 40 {
				e.Warnings = append(e.Warnings, "Rejected unsafe release reference")
			}
			continue
		}
		if seen[u.String()] || len(e.References) >= 100 {
			continue
		}
		seen[u.String()] = true
		e.References = append(e.References, Reference{URL: u.String(), Label: host,
			Kind: "download", State: "manual-required", Reason: "File-host container requires browser handoff; automated downloads are unsupported"})
	}
	if !found || !releaseClaim && !preview {
		return nil, errors.New("no ElAmigos release metadata found; unsupported layout")
	}
	if preview {
		e.ReleaseKind = "preview"
		e.References = []Reference{}
		e.Warnings = append(e.Warnings, "Preview or announcement; no available standalone release verified")
	} else if len(e.References) == 0 {
		e.Warnings = append(e.Warnings, "No available release links found")
	}
	if e.SizeBytes == 0 {
		e.Warnings = append(e.Warnings, "Download size unknown")
	}
	if e.UpdatedAt != nil {
		e.Warnings = append(e.Warnings, "Date describes the included source version, not the original game release date")
	}
	return []Entry{e}, nil
}

func elAmigosEntry(source Source, page, raw, title string) Entry {
	return Entry{ID: entryID(source.ID, page), SourceID: source.ID, PageURL: page,
		RawTitle: raw, Title: title, TitleKey: titleKey(title), ReleaseKind: "release", NeedsReview: true,
		Transports: []Transport{}, References: []Reference{}, Warnings: []string{
			"Source metadata is unverified; payload safety has not been assessed",
			"Browser handoff only; no supported automated transport has been verified",
		}}
}

func elAmigosDate(raw string) *time.Time {
	if !elAmigosDateRE.MatchString(raw) {
		return nil
	}
	date, err := time.Parse("02.01.2006", raw)
	if err != nil {
		return nil
	}
	return &date
}

func elAmigosPreview(lower string) bool {
	return strings.Contains(lower, "coming soon") || strings.Contains(lower, "not yet available") ||
		strings.Contains(lower, "preview only") || strings.Contains(lower, "announcement only")
}

func elAmigosHostSection(line string) string {
	switch strings.ToUpper(strings.TrimSpace(line)) {
	case "DDOWNLOAD", "RAPIDGATOR", "UPLOADED", "NITROFLARE", "MEGA", "GOFILE", "MEDIAFIRE":
		return strings.ToUpper(strings.TrimSpace(line))
	}
	return ""
}
