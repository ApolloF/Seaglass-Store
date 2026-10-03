package sources

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// SearchURL asks the source's own site search when its provider has one.
func (s Source) SearchURL(query string) (string, error) {
	p, ok := Lookup(s.ID)
	if !ok {
		return "", fmt.Errorf("unknown source %q", s.ID)
	}
	return p.SearchURL(query)
}

// NextPage accepts only a source-authored next link on the same origin.
func NextPage(source Source, page string, data []byte) (string, error) {
	if len(data) > MaxDocumentBytes {
		return "", errors.New("document too large")
	}
	if _, err := source.ValidateURL(page); err != nil {
		return "", err
	}
	if strings.Contains(strings.SplitN(page, "?", 2)[0], "/feed/") {
		// RSS pagination is a WordPress endpoint, not an HTML navigation link.
		u, _ := url.Parse(page)
		q := u.Query()
		n := 1
		if raw := q.Get("paged"); raw != "" {
			var err error
			n, err = parsePageNumber(raw)
			if err != nil {
				return "", err
			}
		}
		q.Set("paged", pageNumber(n+1))
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	root, err := document(data)
	if err != nil {
		return "", err
	}
	for _, a := range find(root, isAnchor) {
		if class(a, "next") || attr(a, "rel") == "next" {
			return resolve(source, page, attr(a, "href"))
		}
	}
	return "", nil
}

func referenceURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.User != nil || !publicHost(u.Hostname()) || u.Fragment != "" {
		return nil, errors.New("unsafe reference URL")
	}
	if u.Scheme == "https" {
		return httpsURL(raw)
	}
	if u.Scheme != "http" || (u.Port() != "" && u.Port() != "80") {
		return nil, errors.New("unsafe reference scheme or port")
	}
	return u, nil
}

func referenceKind(a, root *html.Node) string {
	for parent := a.Parent; parent != nil && parent != root; parent = parent.Parent {
		if parent.Data != "p" && parent.Data != "li" {
			continue
		}
		line := strings.ToLower(text(parent, false))
		if strings.HasPrefix(strings.TrimSpace(line), "torrent") || strings.Contains(line, "(torrents)") {
			return "torrent"
		}
		break
	}
	u, _ := url.Parse(attr(a, "href"))
	if u != nil && (u.Hostname() == "1337x.to" || strings.HasSuffix(strings.ToLower(u.Path), ".torrent")) {
		return "torrent"
	}
	if strings.Contains(strings.ToLower(text(a, false)), "torrent") {
		return "torrent"
	}
	return "download"
}

func parsePageNumber(raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 10000 {
		return 0, errors.New("invalid page number")
	}
	return n, nil
}
func pageNumber(n int) string { return strconv.Itoa(n) }
